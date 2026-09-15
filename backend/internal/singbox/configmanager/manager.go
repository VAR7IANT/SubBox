// Package configmanager coordinates the crash-recoverable transaction that
// changes the managed Sing-box config. It deliberately knows nothing about
// protocol generation, HTTP requests, or SQLite schema details.
package configmanager

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/VAR7IANT/SubBox/backend/internal/singbox/configtx"
)

// RollbackTimeout is the maximum time allowed for service/config recovery
// after a caller context is cancelled or an operation fails after apply.
const RollbackTimeout = 30 * time.Second

const maxConfigGeneration = int64(^uint64(0) >> 1)

var (
	ErrInvalidDependency = errors.New("config manager dependency is invalid")
	ErrInvalidContext    = errors.New("config manager context is invalid")
	ErrStateMismatch     = errors.New("config manager runtime state mismatch")
	ErrCandidateCheck    = errors.New("candidate config check failed")
	ErrServiceState      = errors.New("config manager service state operation failed")
	ErrCommitFailed      = errors.New("config manager state commit failed")
	ErrRollbackFailed    = errors.New("config manager rollback failed")
	ErrRecoveryFailed    = errors.New("config manager recovery failed")
	ErrQuarantined       = errors.New("config manager transaction is quarantined")
	ErrRecoveryRequired  = ErrQuarantined
	ErrPostCommitCleanup = errors.New("config manager post-commit cleanup is pending")
)

// Checker validates a durable candidate. Production implementations are
// responsible for invoking the fixed sing-box binary; the manager supplies
// only a path derived from its trusted layout and transaction ID.
type Checker interface {
	Check(ctx context.Context, candidatePath string) error
}

// ServiceMode gives callers a named running/stopped interpretation without
// allowing an arbitrary service name into the transaction layer.
type ServiceMode string

const (
	ServiceRunning ServiceMode = "running"
	ServiceStopped ServiceMode = "stopped"
)

// ServiceState is the deliberately small Phase 1 service state model. Active
// is retained as the wire-neutral representation used by the 009A boundary.
type ServiceState struct {
	Active bool
}

func (s ServiceState) Mode() ServiceMode {
	if s.Active {
		return ServiceRunning
	}
	return ServiceStopped
}

// ServiceController controls the one injected Sing-box service. It does not
// accept an arbitrary service name.
type ServiceController interface {
	Status(ctx context.Context) (ServiceState, error)
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Restart(ctx context.Context) error
}

// RuntimeState is the application commit-decision state persisted by SQLite.
// The generation and hash are compared as a pair during recovery.
type RuntimeState struct {
	ConfigGeneration int64
	LiveConfigHash   string
}

// ApplicationStateCommitter is the atomic application-state boundary. A
// production CommitCandidate implementation MUST atomically include the node
// or other application mutation, the matching subscription snapshot, and the
// runtime_state generation/hash write in one SQLite transaction. expectedOld
// must be checked optimistically before changing any of those values. The
// coordinator never treats journal cleanup as a commit decision.
type ApplicationStateCommitter interface {
	CurrentRuntimeState(ctx context.Context) (RuntimeState, error)
	CommitCandidate(ctx context.Context, expectedOld RuntimeState, candidate RuntimeState) error
}

// StateCommitter is the shorter compatibility name for
// ApplicationStateCommitter.
type StateCommitter = ApplicationStateCommitter

// ApplyResult reports only non-secret transaction metadata.
type ApplyResult struct {
	TransactionID       string
	OldGeneration       int64
	CandidateGeneration int64
	CandidateHash       string
	Committed           bool
	CleanupPending      bool
}

// RecoveryResult reports the side selected by the runtime-state commit
// decision. A missing journal returns Recovered=false with no error.
type RecoveryResult struct {
	TransactionID  string
	Recovered      bool
	Committed      bool
	CleanupPending bool
}

// Manager owns one trusted config workspace and its injected dependencies.
// Callers cannot provide filesystem paths, service names, or transaction IDs
// to an individual operation.
type Manager struct {
	workspace *configtx.Workspace
	layout    configtx.Layout
	checker   Checker
	service   ServiceController
	committer ApplicationStateCommitter
}

// NewManager constructs a manager. trusted may be a configtx.Layout or the
// *configtx.Workspace returned by configtx.NewWorkspace. Accepting both keeps
// the trusted layout/workspace boundary explicit while retaining the 009A
// workspace abstraction.
func NewManager(trusted any, checker Checker, service ServiceController, committer ApplicationStateCommitter) (*Manager, error) {
	if isNilDependency(trusted) || isNilDependency(checker) || isNilDependency(service) || isNilDependency(committer) {
		return nil, ErrInvalidDependency
	}
	layout, err := trustedLayout(trusted)
	if err != nil {
		return nil, err
	}
	return &Manager{
		workspace: configtx.NewWorkspace(layout),
		layout:    layout,
		checker:   checker,
		service:   service,
		committer: committer,
	}, nil
}

// New is a concise constructor alias for NewManager.
func New(trusted any, checker Checker, service ServiceController, committer ApplicationStateCommitter) (*Manager, error) {
	return NewManager(trusted, checker, service, committer)
}

// NewWithWorkspace constructs a manager from a trusted 009A workspace.
func NewWithWorkspace(workspace *configtx.Workspace, checker Checker, service ServiceController, committer ApplicationStateCommitter) (*Manager, error) {
	return NewManager(workspace, checker, service, committer)
}

// ConfigManager is a descriptive public alias for Manager.
type ConfigManager = Manager

// NewConfigManager is a descriptive constructor alias for NewManager.
func NewConfigManager(trusted any, checker Checker, service ServiceController, committer ApplicationStateCommitter) (*Manager, error) {
	return NewManager(trusted, checker, service, committer)
}

// Apply executes one complete live-config transaction. Candidate bytes are
// trusted output from a future config generator; they are never echoed in a
// result or an error.
func (m *Manager) Apply(ctx context.Context, candidate []byte) (ApplyResult, error) {
	if m == nil || m.workspace == nil {
		return ApplyResult{}, ErrInvalidDependency
	}
	if ctx == nil {
		return ApplyResult{}, ErrInvalidContext
	}
	if len(candidate) > configtx.MaxConfigBytes {
		return ApplyResult{}, configtx.ErrConfigTooLarge
	}
	if err := contextErr(ctx); err != nil {
		return ApplyResult{}, err
	}

	guard, err := m.workspace.Acquire(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	result, applyErr := m.applyLocked(ctx, guard, candidate)
	releaseErr := guard.Release()
	if releaseErr != nil {
		if result.Committed {
			result.CleanupPending = true
		}
		if applyErr == nil {
			applyErr = releaseErr
		} else {
			applyErr = errors.Join(applyErr, releaseErr)
		}
	}
	return result, applyErr
}

// ApplyCandidate is a descriptive alias for Apply.
func (m *Manager) ApplyCandidate(ctx context.Context, candidate []byte) (ApplyResult, error) {
	return m.Apply(ctx, candidate)
}

func (m *Manager) applyLocked(ctx context.Context, guard *configtx.LockGuard, candidate []byte) (ApplyResult, error) {
	// Re-entry recovery is performed before taking a new application snapshot.
	// The guard remains held for recovery and the entire new transaction.
	if _, err := m.recoverLocked(ctx, guard); err != nil {
		return ApplyResult{}, err
	}

	oldSnapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		return ApplyResult{}, err
	}
	defer oldSnapshot.Destroy()
	oldRuntime, err := m.currentRuntimeState(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	if oldSnapshot.Hash() != oldRuntime.LiveConfigHash {
		return ApplyResult{}, ErrStateMismatch
	}

	// Snapshot the authoritative pre-transaction service state before any
	// candidate or backup work. A status read is not a service mutation.
	serviceWasActive, err := m.snapshotServiceState(ctx)
	if err != nil {
		return ApplyResult{}, err
	}

	transactionID := configtx.NewTransactionID()
	candidateSnapshot, err := guard.WriteCandidate(transactionID, oldSnapshot, candidate)
	if err != nil {
		return ApplyResult{}, err
	}
	defer candidateSnapshot.Destroy()
	result := ApplyResult{
		TransactionID:       transactionID,
		OldGeneration:       oldRuntime.ConfigGeneration,
		CandidateGeneration: oldRuntime.ConfigGeneration + 1,
		CandidateHash:       candidateSnapshot.Hash(),
	}

	candidatePath := m.layout.CandidatePath(transactionID)
	if err := m.checker.Check(ctx, candidatePath); err != nil {
		cleanupErr := ignoreMissing(guard.RemoveCandidate(transactionID))
		return result, errors.Join(checkerError(err), cleanupErr)
	}

	if err := guard.CreateBackup(transactionID, oldSnapshot); err != nil {
		cleanupErr := ignoreMissing(guard.RemoveCandidate(transactionID))
		return result, errors.Join(err, cleanupErr)
	}
	j, err := configtx.NewJournal(
		transactionID,
		oldSnapshot.Hash(),
		candidateSnapshot.Hash(),
		oldRuntime.ConfigGeneration,
		serviceWasActive,
		oldSnapshot.Metadata(),
	)
	if err != nil {
		return result, m.cleanupWithoutJournal(guard, transactionID, err)
	}
	if err := guard.CreatePreparedJournal(j); err != nil {
		// An fsync/directory-sync error may be reported after the journal rename.
		// If a readable marker exists, preserve it for re-entry recovery.
		if _, readErr := guard.ReadJournal(); !errors.Is(readErr, configtx.ErrJournalNotFound) {
			return result, err
		}
		return result, m.cleanupWithoutJournal(guard, transactionID, err)
	}

	// Do not cross the apply boundary after the caller has cancelled. Once the
	// atomic replacement begins, all failure paths use the controlled rollback
	// context below rather than the caller context.
	if err := contextErr(ctx); err != nil {
		return result, m.finishBeforeLiveApply(guard, j, err)
	}

	liveChanged, err := guard.InstallCandidateWithOutcome(transactionID, candidateSnapshot)
	if err != nil {
		if liveChanged {
			return m.failAfterLiveApply(guard, j, result, err)
		}
		return result, m.finishBeforeLiveApply(guard, j, err)
	}
	// Durable phase must precede service control and all later irreversible
	// work. If this update fails, the live config may already be changed, so
	// rollback is mandatory and the prepared marker is retained if necessary.
	if err := guard.MarkLiveApplied(); err != nil {
		return m.failAfterLiveApply(guard, j, result, err)
	}

	if err := m.ensureServiceState(ctx, serviceWasActive); err != nil {
		return m.failAfterLiveApply(guard, j, result, err)
	}

	candidateRuntime := RuntimeState{
		ConfigGeneration: j.CandidateGeneration,
		LiveConfigHash:   j.CandidateConfigHash,
	}
	commitErr := m.committer.CommitCandidate(ctx, oldRuntime, candidateRuntime)
	if commitErr != nil {
		// A committer must return an error without committing, but an interrupted
		// database call can leave the outcome unknown. Compare the persisted
		// generation/hash before deciding whether rollback is safe.
		outcome, resolveErr := m.resolveCommitOutcome(oldRuntime, candidateRuntime)
		if resolveErr != nil {
			_ = m.markRecoveryRequired(guard, j)
			return result, errors.Join(ErrQuarantined, ErrCommitFailed)
		}
		if outcome == commitCandidate {
			result, finalErr := m.finishCommitted(guard, j, result)
			return result, errors.Join(ErrCommitFailed, finalErr)
		}
		return m.failAfterLiveApply(guard, j, result, errors.Join(ErrCommitFailed, contextCause(commitErr)))
	}

	return m.finishCommitted(guard, j, result)
}

// Recover resolves a durable journal while holding the same cross-process
// lock used by Apply. It is safe to call at startup and before every mutation.
func (m *Manager) Recover(ctx context.Context) (RecoveryResult, error) {
	if m == nil || m.workspace == nil {
		return RecoveryResult{}, ErrInvalidDependency
	}
	if ctx == nil {
		return RecoveryResult{}, ErrInvalidContext
	}
	if err := contextErr(ctx); err != nil {
		return RecoveryResult{}, err
	}
	guard, err := m.workspace.Acquire(ctx)
	if err != nil {
		return RecoveryResult{}, err
	}
	result, recoveryErr := m.recoverLocked(ctx, guard)
	if releaseErr := guard.Release(); releaseErr != nil {
		if recoveryErr == nil {
			recoveryErr = releaseErr
		} else {
			recoveryErr = errors.Join(recoveryErr, releaseErr)
		}
	}
	return result, recoveryErr
}

func (m *Manager) recoverLocked(ctx context.Context, guard *configtx.LockGuard) (RecoveryResult, error) {
	if err := contextErr(ctx); err != nil {
		return RecoveryResult{}, err
	}
	j, err := guard.ReadJournal()
	if errors.Is(err, configtx.ErrJournalNotFound) {
		return RecoveryResult{}, nil
	}
	if err != nil {
		// Malformed/unsupported journal data is itself the durable quarantine
		// record. Do not call the checker or service controller.
		return RecoveryResult{}, ErrQuarantined
	}
	result := RecoveryResult{TransactionID: j.TransactionID}
	if j.Phase == configtx.PhaseRollbackFailed || j.Phase == configtx.PhaseRecoveryRequired {
		return result, ErrQuarantined
	}

	runtimeState, err := m.currentRuntimeState(ctx)
	if err != nil {
		return result, m.quarantineLocked(guard, j)
	}
	liveSnapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		return result, m.quarantineLocked(guard, j)
	}
	defer liveSnapshot.Destroy()
	if liveSnapshot.Metadata() != journalMetadata(j) {
		return result, m.quarantineLocked(guard, j)
	}

	dbOld := runtimeState.ConfigGeneration == j.OldGeneration && runtimeState.LiveConfigHash == j.OldConfigHash
	dbCandidate := runtimeState.ConfigGeneration == j.CandidateGeneration && runtimeState.LiveConfigHash == j.CandidateConfigHash
	liveOld := liveSnapshot.Hash() == j.OldConfigHash
	liveCandidate := liveSnapshot.Hash() == j.CandidateConfigHash
	if (!dbOld && !dbCandidate) || (!liveOld && !liveCandidate) {
		return result, m.quarantineLocked(guard, j)
	}
	// state_committed can only be paired with the candidate application state.
	// The database, not this phase, remains the final commit decision.
	if j.Phase == configtx.PhaseStateCommitted && !dbCandidate {
		return result, m.quarantineLocked(guard, j)
	}
	if err := contextErr(ctx); err != nil {
		return result, err
	}

	critical, cancel := context.WithTimeout(context.Background(), RollbackTimeout)
	defer cancel()
	if dbOld {
		return m.recoverOldLocked(critical, guard, j, liveOld, liveCandidate, result)
	}
	return m.recoverCandidateLocked(critical, guard, j, liveOld, liveCandidate, result)
}

func (m *Manager) recoverOldLocked(
	ctx context.Context,
	guard *configtx.LockGuard,
	j configtx.Journal,
	liveOld bool,
	liveCandidate bool,
	result RecoveryResult,
) (RecoveryResult, error) {
	if err := guard.ValidateJournalBackup(j); err != nil {
		return result, m.quarantineLocked(guard, j)
	}
	// A missing candidate is tolerated only because a prior cleanup may have
	// removed it before a crash. A present candidate must still match the
	// journal; silently deleting a tampered artifact would destroy evidence.
	if candidateErr := guard.ValidateJournalCandidate(j); candidateErr != nil && !errors.Is(candidateErr, configtx.ErrArtifactNotFound) {
		return result, m.quarantineLocked(guard, j)
	}
	if liveCandidate && !liveOld {
		if err := guard.RestoreJournalBackup(j); err != nil {
			return result, m.recoveryRollbackFailure(guard, j, err)
		}
	}
	if err := verifyLiveHash(guard, j.OldConfigHash, journalMetadata(j)); err != nil {
		return result, m.recoveryRollbackFailure(guard, j, err)
	}
	if err := m.ensureServiceState(ctx, j.ServiceWasActive); err != nil {
		return result, m.recoveryRollbackFailure(guard, j, err)
	}
	if err := m.cleanupUncommitted(guard, j); err != nil {
		result.CleanupPending = true
		return result, errors.Join(ErrRecoveryFailed, err)
	}
	result.Recovered = true
	result.Committed = false
	return result, nil
}

func (m *Manager) recoverCandidateLocked(
	ctx context.Context,
	guard *configtx.LockGuard,
	j configtx.Journal,
	liveOld bool,
	liveCandidate bool,
	result RecoveryResult,
) (RecoveryResult, error) {
	if err := guard.ValidateJournalBackup(j); err != nil {
		return result, m.quarantineLocked(guard, j)
	}
	candidateErr := guard.ValidateJournalCandidate(j)
	if candidateErr != nil && !errors.Is(candidateErr, configtx.ErrArtifactNotFound) {
		return result, m.quarantineLocked(guard, j)
	}

	if liveOld && !liveCandidate {
		if candidateErr != nil {
			return result, m.quarantineLocked(guard, j)
		}
		if err := m.checker.Check(ctx, m.layout.CandidatePath(j.TransactionID)); err != nil {
			return result, m.recoveryRequiredFailure(guard, j, err)
		}
		liveChanged, err := guard.InstallJournalCandidateWithOutcome(j)
		if err != nil {
			_ = liveChanged
			return result, m.recoveryRequiredFailure(guard, j, err)
		}
		if err := verifyLiveHash(guard, j.CandidateConfigHash, journalMetadata(j)); err != nil {
			return result, m.recoveryRequiredFailure(guard, j, err)
		}
		liveCandidate = true
	}

	// A recovered candidate that was installed before a crash must be marked
	// durably before service restoration. A state_committed marker is already
	// sufficient for the candidate side and is never moved backwards.
	if j.Phase == configtx.PhasePrepared {
		if err := guard.MarkLiveApplied(); err != nil {
			return result, m.recoveryRequiredFailure(guard, j, err)
		}
		j.Phase = configtx.PhaseLiveApplied
	}
	if !liveCandidate {
		return result, m.recoveryRequiredFailure(guard, j, errors.New("candidate config was not made live"))
	}
	if err := m.ensureServiceState(ctx, j.ServiceWasActive); err != nil {
		return result, m.recoveryRequiredFailure(guard, j, err)
	}
	if j.Phase == configtx.PhaseLiveApplied {
		if err := guard.MarkStateCommitted(); err != nil {
			return result, m.recoveryRequiredFailure(guard, j, err)
		}
		j.Phase = configtx.PhaseStateCommitted
	}
	if err := m.cleanupCommitted(guard, j); err != nil {
		result.Recovered = true
		result.Committed = true
		result.CleanupPending = true
		return result, errors.Join(ErrPostCommitCleanup, err)
	}
	result.Recovered = true
	result.Committed = true
	return result, nil
}

func (m *Manager) finishBeforeLiveApply(guard *configtx.LockGuard, j configtx.Journal, primary error) error {
	cleanupErr := m.cleanupUncommitted(guard, j)
	return errors.Join(primary, cleanupErr)
}

func (m *Manager) finishCommitted(guard *configtx.LockGuard, j configtx.Journal, result ApplyResult) (ApplyResult, error) {
	result.Committed = true
	if j.Phase != configtx.PhaseStateCommitted {
		if err := guard.MarkStateCommitted(); err != nil {
			// SQLite already returned success. Never roll the live config back
			// because cleanup metadata could not be advanced.
			result.CleanupPending = true
			return result, errors.Join(ErrPostCommitCleanup, err)
		}
		j.Phase = configtx.PhaseStateCommitted
	}
	if err := m.cleanupCommitted(guard, j); err != nil {
		result.CleanupPending = true
		return result, errors.Join(ErrPostCommitCleanup, err)
	}
	return result, nil
}

func (m *Manager) failAfterLiveApply(guard *configtx.LockGuard, j configtx.Journal, result ApplyResult, primary error) (ApplyResult, error) {
	rollbackErr := m.rollback(guard, j)
	if rollbackErr != nil {
		// Do not run normal cleanup after a failed rollback. The journal,
		// candidate, and backup are the operator's recovery evidence.
		return result, errors.Join(primary, ErrRollbackFailed)
	}
	cleanupErr := m.cleanupUncommitted(guard, j)
	return result, errors.Join(primary, cleanupErr)
}

func (m *Manager) rollback(guard *configtx.LockGuard, j configtx.Journal) error {
	critical, cancel := context.WithTimeout(context.Background(), RollbackTimeout)
	defer cancel()
	configErr := guard.RestoreJournalBackup(j)
	serviceErr := m.ensureServiceState(critical, j.ServiceWasActive)
	if configErr == nil && serviceErr == nil {
		return nil
	}
	// The old phase remains recoverable if this update itself fails. The
	// normal path preserves every artifact either way; a durable
	// rollback_failed phase makes subsequent mutations fail closed.
	_ = guard.MarkRollbackFailed()
	return errors.Join(ErrRollbackFailed, configErr, serviceErr)
}

func (m *Manager) recoveryRollbackFailure(guard *configtx.LockGuard, j configtx.Journal, primary error) error {
	_ = guard.MarkRollbackFailed()
	return errors.Join(ErrRecoveryFailed, ErrRollbackFailed, primary)
}

func (m *Manager) recoveryRequiredFailure(guard *configtx.LockGuard, j configtx.Journal, primary error) error {
	_ = m.markRecoveryRequired(guard, j)
	return errors.Join(ErrRecoveryFailed, ErrQuarantined, safeRecoveryCause(primary))
}

func safeRecoveryCause(err error) error {
	switch {
	case errors.Is(err, ErrCandidateCheck):
		return ErrCandidateCheck
	case errors.Is(err, ErrServiceState):
		return ErrServiceState
	case errors.Is(err, ErrStateMismatch):
		return ErrStateMismatch
	case errors.Is(err, configtx.ErrAtomicReplace):
		return configtx.ErrAtomicReplace
	case errors.Is(err, configtx.ErrInvalidJournal):
		return configtx.ErrInvalidJournal
	default:
		return nil
	}
}

func (m *Manager) quarantineLocked(guard *configtx.LockGuard, j configtx.Journal) error {
	_ = m.markRecoveryRequired(guard, j)
	return ErrQuarantined
}

func (m *Manager) markRecoveryRequired(guard *configtx.LockGuard, j configtx.Journal) error {
	if j.Phase == configtx.PhaseRecoveryRequired || j.Phase == configtx.PhaseRollbackFailed {
		return nil
	}
	return guard.UpdateJournalPhase(configtx.PhaseRecoveryRequired)
}

func (m *Manager) currentRuntimeState(ctx context.Context) (RuntimeState, error) {
	state, err := m.committer.CurrentRuntimeState(ctx)
	if err != nil {
		return RuntimeState{}, errors.Join(ErrStateMismatch, contextCause(err))
	}
	if !validRuntimeState(state) {
		return RuntimeState{}, ErrStateMismatch
	}
	return state, nil
}

func (m *Manager) resolveCommitOutcome(oldState, candidateState RuntimeState) (commitOutcome, error) {
	critical, cancel := context.WithTimeout(context.Background(), RollbackTimeout)
	defer cancel()
	state, err := m.currentRuntimeState(critical)
	if err != nil {
		return commitUnknown, err
	}
	switch state {
	case oldState:
		return commitOld, nil
	case candidateState:
		return commitCandidate, nil
	default:
		return commitUnknown, ErrStateMismatch
	}
}

type commitOutcome uint8

const (
	commitUnknown commitOutcome = iota
	commitOld
	commitCandidate
)

func (m *Manager) snapshotServiceState(ctx context.Context) (bool, error) {
	state, err := m.service.Status(ctx)
	if err != nil {
		return false, serviceError(err)
	}
	return state.Mode() == ServiceRunning, nil
}

func (m *Manager) ensureServiceState(ctx context.Context, wasActive bool) error {
	state, err := m.service.Status(ctx)
	if err != nil {
		return serviceError(err)
	}
	if wasActive {
		if state.Mode() == ServiceRunning {
			err = m.service.Restart(ctx)
		} else {
			err = m.service.Start(ctx)
		}
		if err != nil {
			return serviceError(err)
		}
	} else if state.Mode() == ServiceRunning {
		err = m.service.Stop(ctx)
		if err != nil {
			return serviceError(err)
		}
	}
	finalState, err := m.service.Status(ctx)
	if err != nil {
		return serviceError(err)
	}
	if finalState.Mode() != modeFromActive(wasActive) {
		return ErrServiceState
	}
	return nil
}

func modeFromActive(active bool) ServiceMode {
	if active {
		return ServiceRunning
	}
	return ServiceStopped
}

func verifyLiveHash(guard *configtx.LockGuard, expectedHash string, metadata configtx.FileMetadata) error {
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		return err
	}
	defer snapshot.Destroy()
	if snapshot.Hash() != expectedHash || snapshot.Metadata() != metadata {
		return ErrStateMismatch
	}
	return nil
}

func (m *Manager) cleanupUncommitted(guard *configtx.LockGuard, j configtx.Journal) error {
	return m.cleanupArtifacts(guard, j)
}

func (m *Manager) cleanupCommitted(guard *configtx.LockGuard, j configtx.Journal) error {
	return m.cleanupArtifacts(guard, j)
}

func (m *Manager) cleanupArtifacts(guard *configtx.LockGuard, j configtx.Journal) error {
	var cleanupErrs []error
	if err := ignoreMissing(guard.RemoveApplyTemps(j.TransactionID)); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	if err := ignoreMissing(guard.RemoveCandidate(j.TransactionID)); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	if err := guard.RetainBackups(j.BackupName); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	if len(cleanupErrs) > 0 {
		return errors.Join(cleanupErrs...)
	}
	if err := ignoreMissing(guard.RemoveJournal()); err != nil {
		return err
	}
	return nil
}

func (m *Manager) cleanupWithoutJournal(guard *configtx.LockGuard, transactionID string, primary error) error {
	cleanupErrs := []error{
		ignoreMissing(guard.RemoveCandidate(transactionID)),
		ignoreMissing(guard.RemoveBackup(transactionID)),
	}
	return errors.Join(append([]error{primary}, cleanupErrs...)...)
}

func trustedLayout(value any) (configtx.Layout, error) {
	switch trusted := value.(type) {
	case configtx.Layout:
		return trusted, nil
	case *configtx.Layout:
		if trusted == nil {
			return configtx.Layout{}, ErrInvalidDependency
		}
		return *trusted, nil
	case *configtx.Workspace:
		if trusted == nil {
			return configtx.Layout{}, ErrInvalidDependency
		}
		return trusted.Layout(), nil
	default:
		return configtx.Layout{}, fmt.Errorf("%w: trusted layout", ErrInvalidDependency)
	}
}

func validRuntimeState(state RuntimeState) bool {
	if state.ConfigGeneration < 0 || state.ConfigGeneration == maxConfigGeneration || len(state.LiveConfigHash) != 64 {
		return false
	}
	for _, char := range state.LiveConfigHash {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func journalMetadata(j configtx.Journal) configtx.FileMetadata {
	return configtx.FileMetadata{Mode: j.LiveConfigMode, UID: j.LiveConfigUID, GID: j.LiveConfigGID}
}

func ignoreMissing(err error) error {
	if errors.Is(err, configtx.ErrArtifactNotFound) || errors.Is(err, configtx.ErrJournalNotFound) {
		return nil
	}
	return err
}

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func contextCause(err error) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return nil
}

func checkerError(err error) error {
	if cause := contextCause(err); cause != nil {
		return errors.Join(ErrCandidateCheck, cause)
	}
	return ErrCandidateCheck
}

func serviceError(err error) error {
	if cause := contextCause(err); cause != nil {
		return errors.Join(ErrServiceState, cause)
	}
	return ErrServiceState
}

func isNilDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
