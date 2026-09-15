package configmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/VAR7IANT/SubBox/backend/internal/singbox/configtx"
)

type managerWorkspace struct {
	layout   configtx.Layout
	stateDir string
	livePath string
}

func newManagerWorkspace(t *testing.T, content string) managerWorkspace {
	t.Helper()
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	liveDir := filepath.Join(root, "sing-box")
	if err := os.Mkdir(liveDir, 0700); err != nil {
		t.Fatal(err)
	}
	livePath := filepath.Join(liveDir, "config.json")
	if err := os.WriteFile(livePath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return managerWorkspace{layout: configtx.NewLayout(stateDir, livePath), stateDir: stateDir, livePath: livePath}
}

func configHash(content string) string {
	digest := sha256.Sum256([]byte(content))
	return hex.EncodeToString(digest[:])
}

type fakeChecker struct {
	mu       sync.Mutex
	calls    []string
	failNext error
	hook     func(string)
}

func (f *fakeChecker) Check(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	f.calls = append(f.calls, path)
	hook := f.hook
	err := f.failNext
	f.failNext = nil
	f.mu.Unlock()
	if hook != nil {
		hook(path)
	}
	return err
}

func (f *fakeChecker) Paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

type fakeService struct {
	mu          sync.Mutex
	active      bool
	trace       []string
	failNext    map[string]error
	failAlways  map[string]error
	statusFails map[int]error
	statusCalls int
	statusHook  func(int)
}

func newFakeService(active bool) *fakeService {
	return &fakeService{
		active:      active,
		failNext:    make(map[string]error),
		failAlways:  make(map[string]error),
		statusFails: make(map[int]error),
	}
}

func (f *fakeService) Status(ctx context.Context) (ServiceState, error) {
	if err := ctx.Err(); err != nil {
		return ServiceState{}, err
	}
	f.mu.Lock()
	f.trace = append(f.trace, "status")
	f.statusCalls++
	callNumber := f.statusCalls
	hook := f.statusHook
	statusFailure := f.statusFails[callNumber]
	delete(f.statusFails, callNumber)
	if err := f.failNext["status"]; err != nil {
		delete(f.failNext, "status")
		f.mu.Unlock()
		if hook != nil {
			hook(callNumber)
		}
		return ServiceState{}, err
	}
	if statusFailure != nil {
		f.mu.Unlock()
		if hook != nil {
			hook(callNumber)
		}
		return ServiceState{}, statusFailure
	}
	state := ServiceState{Active: f.active}
	f.mu.Unlock()
	if hook != nil {
		hook(callNumber)
	}
	return state, nil
}

func (f *fakeService) Start(ctx context.Context) error {
	return f.mutate(ctx, "start", true)
}

func (f *fakeService) Stop(ctx context.Context) error {
	return f.mutate(ctx, "stop", false)
}

func (f *fakeService) Restart(ctx context.Context) error {
	return f.mutate(ctx, "restart", true)
}

func (f *fakeService) mutate(ctx context.Context, operation string, active bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trace = append(f.trace, operation)
	if err := f.failAlways[operation]; err != nil {
		return err
	}
	if err := f.failNext[operation]; err != nil {
		delete(f.failNext, operation)
		return err
	}
	f.active = active
	return nil
}

func (f *fakeService) Trace() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.trace...)
}

func (f *fakeService) IsActive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active
}

type commitRequest struct {
	expected  RuntimeState
	candidate RuntimeState
}

type fakeCommitter struct {
	mu                sync.Mutex
	state             RuntimeState
	subscriptionState RuntimeState
	requests          []commitRequest
	failNext          error
	hook              func(RuntimeState)
}

func (f *fakeCommitter) CurrentRuntimeState(ctx context.Context) (RuntimeState, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeState{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, nil
}

func (f *fakeCommitter) CommitCandidate(ctx context.Context, expectedOld RuntimeState, candidate RuntimeState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	f.requests = append(f.requests, commitRequest{expected: expectedOld, candidate: candidate})
	hook := f.hook
	err := f.failNext
	f.failNext = nil
	if err == nil && f.state != expectedOld {
		f.mu.Unlock()
		return errors.New("optimistic state mismatch")
	}
	if err == nil {
		f.state = candidate
		f.subscriptionState = candidate
	}
	f.mu.Unlock()
	if hook != nil {
		hook(candidate)
	}
	return err
}

func (f *fakeCommitter) State() RuntimeState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state
}

func (f *fakeCommitter) SubscriptionState() RuntimeState {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subscriptionState
}

func (f *fakeCommitter) Requests() []commitRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]commitRequest(nil), f.requests...)
}

func newManagerFor(t *testing.T, workspace managerWorkspace, active bool) (*Manager, *fakeChecker, *fakeService, *fakeCommitter) {
	t.Helper()
	checker := &fakeChecker{}
	service := newFakeService(active)
	committer := &fakeCommitter{
		state:             RuntimeState{ConfigGeneration: 7, LiveConfigHash: configHash("OLD")},
		subscriptionState: RuntimeState{ConfigGeneration: 7, LiveConfigHash: configHash("OLD")},
	}
	manager, err := NewManager(workspace.layout, checker, service, committer)
	if err != nil {
		t.Fatal(err)
	}
	return manager, checker, service, committer
}

func assertManagerIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("errors.Is(%v, %v) = false", err, target)
	}
}

func TestNewManagerRejectsNilDependencies(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	checker := &fakeChecker{}
	service := newFakeService(true)
	committer := &fakeCommitter{}
	cases := []struct {
		name  string
		value any
	}{
		{name: "layout", value: nil},
		{name: "checker", value: checker},
		{name: "service", value: service},
		{name: "committer", value: committer},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var got *Manager
			var err error
			switch testCase.name {
			case "layout":
				got, err = NewManager(testCase.value, checker, service, committer)
			case "checker":
				got, err = NewManager(workspace.layout, nil, service, committer)
			case "service":
				got, err = NewManager(workspace.layout, checker, nil, committer)
			case "committer":
				got, err = NewManager(workspace.layout, checker, service, nil)
			}
			if got != nil {
				t.Fatal("manager was returned for invalid dependency")
			}
			assertManagerIs(t, err, ErrInvalidDependency)
		})
	}
}

func TestApplyHappyActive(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, checker, service, committer := newManagerFor(t, workspace, true)
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed || result.CleanupPending || result.OldGeneration != 7 || result.CandidateGeneration != 8 || result.CandidateHash != configHash("NEW") {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "NEW" {
		t.Fatalf("live config %q", got)
	}
	if state := committer.State(); state.ConfigGeneration != 8 || state.LiveConfigHash != configHash("NEW") {
		t.Fatalf("unexpected runtime state: %#v", state)
	}
	if state := committer.SubscriptionState(); state.ConfigGeneration != 8 || state.LiveConfigHash != configHash("NEW") {
		t.Fatalf("unexpected subscription state: %#v", state)
	}
	if got := service.Trace(); !containsInOrder(got, "status", "restart", "status") {
		t.Fatalf("service trace %v", got)
	}
	if len(checker.Paths()) != 1 || filepath.Base(checker.Paths()[0]) != ".subbox-candidate-"+result.TransactionID+".json" {
		t.Fatalf("checker paths %v", checker.Paths())
	}
	if _, err := os.Stat(workspace.layout.JournalPath); !os.IsNotExist(err) {
		t.Fatalf("journal still exists: %v", err)
	}
	if _, err := os.Stat(workspace.layout.CandidatePath(result.TransactionID)); !os.IsNotExist(err) {
		t.Fatalf("candidate still exists: %v", err)
	}
	if _, err := os.Stat(workspace.layout.BackupPath(result.TransactionID)); err != nil {
		t.Fatalf("backup was not retained: %v", err)
	}
}

func TestApplyHappyInactiveDoesNotStartOrRestart(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, committer := newManagerFor(t, workspace, false)
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	if err != nil || !result.Committed {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	for _, operation := range service.Trace() {
		if operation == "start" || operation == "restart" {
			t.Fatalf("inactive service unexpectedly used %s: %v", operation, service.Trace())
		}
	}
	if service.IsActive() || committer.State().ConfigGeneration != 8 {
		t.Fatalf("inactive apply changed state: active=%v runtime=%#v", service.IsActive(), committer.State())
	}
}

func TestApplyCheckerFailureLeavesLiveServiceAndStateUntouched(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, checker, service, committer := newManagerFor(t, workspace, true)
	checker.failNext = errors.New("candidate contains secret: NEW")
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrCandidateCheck)
	if result.Committed || result.CleanupPending {
		t.Fatalf("unexpected result: %#v", result)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("live config changed: %q", got)
	}
	if committer.State().LiveConfigHash != configHash("OLD") || hasServiceMutation(service.Trace()) {
		t.Fatalf("state changed: runtime=%#v service=%v", committer.State(), service.Trace())
	}
	if strings.Contains(err.Error(), "NEW") {
		t.Fatalf("checker error leaked candidate content: %v", err)
	}
	if _, statErr := os.Stat(workspace.layout.JournalPath); !os.IsNotExist(statErr) {
		t.Fatalf("checker failure left journal: %v", statErr)
	}
	if len(checker.Paths()) != 0 {
		if _, statErr := os.Stat(checker.Paths()[0]); !os.IsNotExist(statErr) {
			t.Fatalf("checker failure left candidate: %v", statErr)
		}
	}
}

func TestApplyBackupPreparationFailureLeavesStateUntouched(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, checker, service, committer := newManagerFor(t, workspace, true)
	checker.hook = func(string) {
		if err := os.Chmod(workspace.layout.BackupDir, 0750); err != nil {
			t.Fatalf("make backup directory unsafe: %v", err)
		}
	}
	t.Cleanup(func() { _ = os.Chmod(workspace.layout.BackupDir, 0700) })

	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, configtx.ErrUnsafePermissions)
	if result.Committed || result.CleanupPending {
		t.Fatalf("backup failure committed: %#v", result)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("backup failure changed live config: %q", got)
	}
	if committer.State() != (RuntimeState{ConfigGeneration: 7, LiveConfigHash: configHash("OLD")}) ||
		hasServiceMutation(service.Trace()) {
		t.Fatalf("backup failure changed state: runtime=%#v service=%v", committer.State(), service.Trace())
	}
	if len(committer.Requests()) != 0 {
		t.Fatalf("backup failure reached application commit: %v", committer.Requests())
	}
	if _, statErr := os.Stat(workspace.layout.JournalPath); !os.IsNotExist(statErr) {
		t.Fatalf("backup failure left journal: %v", statErr)
	}
}

func TestApplyJournalPreparationFailureLeavesStateUntouched(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, checker, service, committer := newManagerFor(t, workspace, true)
	checker.hook = func(string) {
		if err := os.WriteFile(workspace.layout.JournalPath, []byte("journal preparation blocker"), 0640); err != nil {
			t.Fatalf("create journal blocker: %v", err)
		}
	}

	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, configtx.ErrUnsafePermissions)
	if result.Committed || result.CleanupPending {
		t.Fatalf("journal preparation failure committed: %#v", result)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("journal preparation failure changed live config: %q", got)
	}
	if committer.State() != (RuntimeState{ConfigGeneration: 7, LiveConfigHash: configHash("OLD")}) ||
		hasServiceMutation(service.Trace()) {
		t.Fatalf("journal preparation failure changed state: runtime=%#v service=%v", committer.State(), service.Trace())
	}
	if len(committer.Requests()) != 0 {
		t.Fatalf("journal preparation failure reached application commit: %v", committer.Requests())
	}
	if _, statErr := os.Stat(workspace.layout.CandidatePath(result.TransactionID)); statErr != nil {
		t.Fatalf("journal preparation failure lost candidate evidence: %v", statErr)
	}
	if journalBytes, readErr := os.ReadFile(workspace.layout.JournalPath); readErr != nil || strings.Contains(string(journalBytes), `"phase":"rollback_failed"`) {
		t.Fatalf("journal preparation failure left rollback marker: err=%v bytes=%s", readErr, journalBytes)
	}
}

func TestApplyRestartFailureRollsBack(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, committer := newManagerFor(t, workspace, true)
	service.failNext["restart"] = errors.New("restart failed")
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrServiceState)
	if result.Committed {
		t.Fatalf("restart failure committed: %#v", result)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("rollback live config %q", got)
	}
	if committer.State().LiveConfigHash != configHash("OLD") || !service.IsActive() {
		t.Fatalf("rollback state: runtime=%#v service=%v", committer.State(), service.IsActive())
	}
	if _, statErr := os.Stat(workspace.layout.JournalPath); !os.IsNotExist(statErr) {
		t.Fatalf("successful rollback left journal: %v", statErr)
	}
}

func TestApplyCandidateRuntimeVerificationFailureRollsBack(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, committer := newManagerFor(t, workspace, true)
	service.statusFails[3] = errors.New("candidate runtime verification failed")

	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrServiceState)
	if result.Committed {
		t.Fatalf("candidate verification failure committed: %#v", result)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("candidate verification rollback live config %q", got)
	}
	if committer.State() != (RuntimeState{ConfigGeneration: 7, LiveConfigHash: configHash("OLD")}) || !service.IsActive() {
		t.Fatalf("candidate verification rollback state: runtime=%#v active=%v", committer.State(), service.IsActive())
	}
	if countTrace(service.Trace(), "restart") != 2 {
		t.Fatalf("candidate verification did not restore service after rollback: %v", service.Trace())
	}
	if _, statErr := os.Stat(workspace.layout.JournalPath); !os.IsNotExist(statErr) {
		t.Fatalf("candidate verification rollback left journal: %v", statErr)
	}
}

func TestApplyCancellationAfterInstallStillRollsBack(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, committer := newManagerFor(t, workspace, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.statusHook = func(callNumber int) {
		if callNumber == 2 {
			cancel()
		}
	}
	result, err := manager.Apply(ctx, []byte("NEW"))
	assertManagerIs(t, err, ErrServiceState)
	if result.Committed {
		t.Fatalf("canceled apply committed: %#v", result)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("canceled apply left candidate live: %q", got)
	}
	if committer.State().LiveConfigHash != configHash("OLD") || !service.IsActive() {
		t.Fatalf("canceled rollback state: runtime=%#v service=%v", committer.State(), service.IsActive())
	}
}

func TestApplyAtomicInstallFailureLeavesLiveUsable(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, checker, service, committer := newManagerFor(t, workspace, true)
	checker.hook = func(string) {
		if err := os.Chmod(filepath.Dir(workspace.livePath), 0500); err != nil {
			t.Fatalf("make live directory read-only: %v", err)
		}
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(workspace.livePath), 0700) })

	result, err := manager.Apply(context.Background(), []byte("NEW"))
	if err == nil {
		t.Fatal("atomic install unexpectedly succeeded")
	}
	if errors.Is(err, ErrRollbackFailed) {
		t.Fatalf("pre-rename install failure entered rollback_failed: %v", err)
	}
	if result.Committed {
		t.Fatalf("atomic install failure committed: %#v", result)
	}
	if got, readErr := os.ReadFile(workspace.livePath); readErr != nil || string(got) != "OLD" {
		t.Fatalf("atomic install failure left unusable live config: %q, err=%v", got, readErr)
	}
	if committer.State() != (RuntimeState{ConfigGeneration: 7, LiveConfigHash: configHash("OLD")}) ||
		len(committer.Requests()) != 0 || hasServiceMutation(service.Trace()) {
		t.Fatalf("atomic install failure changed state: runtime=%#v requests=%v service=%v", committer.State(), committer.Requests(), service.Trace())
	}

	if err := os.Chmod(filepath.Dir(workspace.livePath), 0700); err != nil {
		t.Fatal(err)
	}
	if journalBytes, readErr := os.ReadFile(workspace.layout.JournalPath); readErr == nil {
		journal := readTestJournal(t, journalBytes)
		if journal.Phase == configtx.PhaseRollbackFailed {
			t.Fatalf("atomic install failure left rollback_failed journal: %#v", journal)
		}
	} else if !os.IsNotExist(readErr) {
		t.Fatalf("read atomic install failure journal: %v", readErr)
	}
}

func TestApplyCommitFailureRollsBack(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, committer := newManagerFor(t, workspace, true)
	committer.failNext = errors.New("commit failed")
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrCommitFailed)
	if result.Committed {
		t.Fatalf("commit failure committed: %#v", result)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("commit rollback live config %q", got)
	}
	if state := committer.State(); state.ConfigGeneration != 7 || state.LiveConfigHash != configHash("OLD") {
		t.Fatalf("commit failure changed runtime: %#v", state)
	}
	if state := committer.SubscriptionState(); state.ConfigGeneration != 7 || state.LiveConfigHash != configHash("OLD") {
		t.Fatalf("commit failure changed subscription state: %#v", state)
	}
	if !service.IsActive() {
		t.Fatal("commit rollback did not restore active service")
	}
}

type blockingChecker struct{}

func (blockingChecker) Check(ctx context.Context, _ string) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestApplyCheckerReceivesCancellationWithoutLiveMutation(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	service := newFakeService(true)
	committer := &fakeCommitter{state: RuntimeState{ConfigGeneration: 7, LiveConfigHash: configHash("OLD")}}
	manager, err := NewManager(workspace.layout, blockingChecker{}, service, committer)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err = manager.Apply(ctx, []byte("NEW"))
	assertManagerIs(t, err, ErrCandidateCheck)
	assertManagerIs(t, err, context.DeadlineExceeded)
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("canceled checker changed live config: %q", got)
	}
	if committer.State() != (RuntimeState{ConfigGeneration: 7, LiveConfigHash: configHash("OLD")}) {
		t.Fatalf("canceled checker changed runtime state: %#v", committer.State())
	}
	if hasServiceMutation(service.Trace()) {
		t.Fatalf("canceled checker changed service state: %v", service.Trace())
	}
}

func TestApplyHoldsConfigLockThroughApplicationCommit(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, _, committer := newManagerFor(t, workspace, true)
	commitEntered := make(chan struct{})
	allowCommitReturn := make(chan struct{})
	commitReleased := false
	defer func() {
		if !commitReleased {
			close(allowCommitReturn)
		}
	}()
	committer.hook = func(RuntimeState) {
		close(commitEntered)
		<-allowCommitReturn
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := manager.Apply(context.Background(), []byte("NEW"))
		firstDone <- err
	}()
	select {
	case <-commitEntered:
	case <-time.After(time.Second):
		t.Fatal("first transaction did not reach application commit")
	}

	secondManager, err := NewManager(workspace.layout, &fakeChecker{}, newFakeService(true), committer)
	if err != nil {
		t.Fatal(err)
	}
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, err := secondManager.Apply(secondCtx, []byte("SECOND"))
		secondDone <- err
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("second transaction crossed lock while first commit was active: %v", err)
	case <-time.After(60 * time.Millisecond):
	}
	cancelSecond()
	select {
	case err := <-secondDone:
		assertManagerIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("second transaction did not stop waiting for the held lock")
	}

	close(allowCommitReturn)
	commitReleased = true
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("first transaction did not finish after commit was released")
	}
}

func TestApplyPersistsLiveAppliedBeforeApplicationCommit(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, _, committer := newManagerFor(t, workspace, true)
	committer.hook = func(RuntimeState) {
		diskJournal, err := os.ReadFile(workspace.layout.JournalPath)
		if err != nil {
			t.Fatalf("read journal in commit hook: %v", err)
		}
		var journal configtx.Journal
		if err := json.Unmarshal(diskJournal, &journal); err != nil {
			t.Fatalf("decode journal in commit hook: %v", err)
		}
		if journal.Phase != configtx.PhaseLiveApplied {
			t.Fatalf("application commit ran in journal phase %q", journal.Phase)
		}
	}
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	if err != nil || !result.Committed {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestApplyCommitAndRollbackFailureQuarantinesEvidence(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, _, committer := newManagerFor(t, workspace, true)
	committer.failNext = errors.New("commit failed")
	committer.hook = func(RuntimeState) {
		paths, err := os.ReadDir(workspace.layout.BackupDir)
		if err != nil || len(paths) != 1 {
			t.Fatalf("backup hook read: %v entries=%v", err, paths)
		}
		if err := os.Remove(filepath.Join(workspace.layout.BackupDir, paths[0].Name())); err != nil {
			t.Fatalf("destroy backup: %v", err)
		}
	}
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrRollbackFailed)
	if result.Committed {
		t.Fatalf("rollback failure committed: %#v", result)
	}
	journal, journalErr := os.ReadFile(workspace.layout.JournalPath)
	if journalErr != nil || !strings.Contains(string(journal), `"phase":"rollback_failed"`) {
		t.Fatalf("rollback_failed journal missing: err=%v content=%s", journalErr, journal)
	}
	if _, err := os.Stat(workspace.layout.CandidatePath(result.TransactionID)); err != nil {
		t.Fatalf("candidate evidence missing: %v", err)
	}
}

func TestApplyRollbackFailurePreservesEvidenceAndBlocksMutation(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, _ := newManagerFor(t, workspace, true)
	service.failAlways["restart"] = errors.New("service cannot restart")
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrRollbackFailed)
	if result.Committed {
		t.Fatalf("rollback failure reported committed: %#v", result)
	}
	for _, path := range []string{
		workspace.layout.JournalPath,
		workspace.layout.CandidatePath(result.TransactionID),
		workspace.layout.BackupPath(result.TransactionID),
	} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("rollback evidence %s missing: %v", path, statErr)
		}
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("rollback failure did not restore old config: %q", got)
	}
	if _, err := manager.Apply(context.Background(), []byte("THIRD")); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("mutation after rollback failure returned %v", err)
	}
}

func TestApplyRollbackConfigRestoreFailureQuarantines(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, _, committer := newManagerFor(t, workspace, true)
	committer.failNext = errors.New("database commit failed")
	committer.hook = func(RuntimeState) {
		if err := os.Chmod(filepath.Dir(workspace.livePath), 0500); err != nil {
			t.Fatalf("make live directory read-only for rollback: %v", err)
		}
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(workspace.livePath), 0700) })

	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrRollbackFailed)
	if result.Committed {
		t.Fatalf("config restore failure committed: %#v", result)
	}
	if err := os.Chmod(filepath.Dir(workspace.livePath), 0700); err != nil {
		t.Fatal(err)
	}
	journal := readTestJournal(t, mustReadFile(t, workspace.layout.JournalPath))
	if journal.Phase != configtx.PhaseRollbackFailed {
		t.Fatalf("config restore failure journal phase %q", journal.Phase)
	}
	assertRollbackEvidence(t, workspace, result.TransactionID)
	if _, err := manager.Apply(context.Background(), []byte("MUST-NOT-APPLY")); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("mutation after config restore failure returned %v", err)
	}
}

func TestApplyRollbackServiceRestoreFailureQuarantines(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, committer := newManagerFor(t, workspace, true)
	committer.failNext = errors.New("database commit failed")
	committer.hook = func(RuntimeState) {
		service.failAlways["restart"] = errors.New("old service restore failed")
	}

	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrRollbackFailed)
	if result.Committed {
		t.Fatalf("service restore failure committed: %#v", result)
	}
	journal := readTestJournal(t, mustReadFile(t, workspace.layout.JournalPath))
	if journal.Phase != configtx.PhaseRollbackFailed {
		t.Fatalf("service restore failure journal phase %q", journal.Phase)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("service restore failure did not restore config: %q", got)
	}
	assertRollbackEvidence(t, workspace, result.TransactionID)
	if _, err := manager.Apply(context.Background(), []byte("MUST-NOT-APPLY")); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("mutation after service restore failure returned %v", err)
	}
}

func TestApplyRollbackVerificationFailureQuarantines(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, committer := newManagerFor(t, workspace, true)
	committer.failNext = errors.New("database commit failed")
	// Status calls are: pre-transaction snapshot, candidate verify (before and
	// after restart), rollback verify (before and after restart). Fail only the
	// final old-state verification call.
	service.statusFails[5] = errors.New("old service verification failed")

	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrRollbackFailed)
	if result.Committed {
		t.Fatalf("rollback verification failure committed: %#v", result)
	}
	journal := readTestJournal(t, mustReadFile(t, workspace.layout.JournalPath))
	if journal.Phase != configtx.PhaseRollbackFailed {
		t.Fatalf("rollback verification failure journal phase %q", journal.Phase)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("rollback verification failure did not restore config: %q", got)
	}
	assertRollbackEvidence(t, workspace, result.TransactionID)
	if _, err := manager.Apply(context.Background(), []byte("MUST-NOT-APPLY")); !errors.Is(err, ErrQuarantined) {
		t.Fatalf("mutation after rollback verification failure returned %v", err)
	}
}

func TestApplyDoesNotLeakSecretSentinelInJournalOrError(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, committer := newManagerFor(t, workspace, true)
	sentinel := "SUPER_SECRET_DO_NOT_LEAK_009B"
	candidate := []byte(`{"password":"SUPER_SECRET_DO_NOT_LEAK_009B","private_key":"SUPER_SECRET_DO_NOT_LEAK_009B","subscription_token":"SUPER_SECRET_DO_NOT_LEAK_009B"}`)
	committer.failNext = errors.New("database commit failed: " + sentinel)
	committer.hook = func(RuntimeState) {
		service.failAlways["restart"] = errors.New("service restore failed")
	}

	result, err := manager.Apply(context.Background(), candidate)
	assertManagerIs(t, err, ErrCommitFailed)
	assertManagerIs(t, err, ErrRollbackFailed)
	for _, forbidden := range []string{sentinel, "password", "private_key", "subscription_token", "credential", string(candidate)} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("returned error leaked %q: %v", forbidden, err)
		}
	}
	journalBytes := mustReadFile(t, workspace.layout.JournalPath)
	for _, forbidden := range []string{sentinel, "password", "private_key", "subscription_token", "credential"} {
		if strings.Contains(string(journalBytes), forbidden) {
			t.Fatalf("journal leaked %q: %s", forbidden, journalBytes)
		}
	}
	if readTestJournal(t, journalBytes).Phase != configtx.PhaseRollbackFailed {
		t.Fatalf("secret test did not preserve rollback evidence")
	}
	assertRollbackEvidence(t, workspace, result.TransactionID)
}

func TestRecoverNoJournal(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, _, _ := newManagerFor(t, workspace, true)
	result, err := manager.Recover(context.Background())
	if err != nil || result.Recovered || result.Committed {
		t.Fatalf("unexpected no-journal recovery: %#v err=%v", result, err)
	}
}

type seededTransaction struct {
	journal          configtx.Journal
	oldRuntime       RuntimeState
	candidateRuntime RuntimeState
}

func seedPreparedTransaction(t *testing.T, workspace managerWorkspace, candidate string, serviceActive, liveCandidate bool) seededTransaction {
	t.Helper()
	guard, err := workspace.layout.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	oldSnapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		_ = guard.Release()
		t.Fatal(err)
	}
	id := configtx.NewTransactionID()
	candidateSnapshot, err := guard.WriteCandidate(id, oldSnapshot, []byte(candidate))
	if err != nil {
		oldSnapshot.Destroy()
		_ = guard.Release()
		t.Fatal(err)
	}
	if err := guard.CreateBackup(id, oldSnapshot); err != nil {
		t.Fatal(err)
	}
	journal, err := configtx.NewJournal(id, oldSnapshot.Hash(), candidateSnapshot.Hash(), 7, serviceActive, oldSnapshot.Metadata())
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.CreatePreparedJournal(journal); err != nil {
		t.Fatal(err)
	}
	if liveCandidate {
		if err := guard.InstallJournalCandidate(journal); err != nil {
			t.Fatal(err)
		}
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	oldSnapshot.Destroy()
	candidateSnapshot.Destroy()
	return seededTransaction{
		journal:          journal,
		oldRuntime:       RuntimeState{ConfigGeneration: 7, LiveConfigHash: oldSnapshotHash()},
		candidateRuntime: RuntimeState{ConfigGeneration: 8, LiveConfigHash: configHash(candidate)},
	}
}

func oldSnapshotHash() string { return configHash("OLD") }

func TestRecoverOldDBCandidateLiveRestoresOld(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	seeded := seedPreparedTransaction(t, workspace, "NEW", true, true)
	committer := &fakeCommitter{state: seeded.oldRuntime}
	service := newFakeService(true)
	checker := &fakeChecker{}
	manager, err := NewManager(workspace.layout, checker, service, committer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Recover(context.Background())
	if err != nil || !result.Recovered || result.Committed {
		t.Fatalf("unexpected recovery: %#v err=%v", result, err)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("old-side recovery live %q", got)
	}
	if _, err := os.Stat(workspace.layout.JournalPath); !os.IsNotExist(err) {
		t.Fatalf("old-side journal exists: %v", err)
	}
	if len(checker.Paths()) != 0 {
		t.Fatalf("old-side recovery checked candidate: %v", checker.Paths())
	}
}

func TestRecoverOldDBOldLiveOnlyRestoresService(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	seeded := seedPreparedTransaction(t, workspace, "NEW", true, false)
	committer := &fakeCommitter{state: seeded.oldRuntime}
	service := newFakeService(false)
	manager, err := NewManager(workspace.layout, &fakeChecker{}, service, committer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Recover(context.Background())
	if err != nil || !result.Recovered || result.Committed || !service.IsActive() {
		t.Fatalf("old/live-old recovery result=%#v err=%v active=%v", result, err, service.IsActive())
	}
}

func TestRecoverCandidateDBCandidateLive(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	seeded := seedPreparedTransaction(t, workspace, "NEW", true, true)
	committer := &fakeCommitter{state: seeded.candidateRuntime}
	checker := &fakeChecker{}
	service := newFakeService(false)
	manager, err := NewManager(workspace.layout, checker, service, committer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Recover(context.Background())
	if err != nil || !result.Recovered || !result.Committed || len(checker.Paths()) != 0 || !service.IsActive() {
		t.Fatalf("candidate-side recovery result=%#v err=%v checker=%v active=%v", result, err, checker.Paths(), service.IsActive())
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "NEW" {
		t.Fatalf("candidate-side recovery live %q", got)
	}
}

func TestRecoverCandidateDBOldLiveReinstallsCandidate(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	seeded := seedPreparedTransaction(t, workspace, "NEW", false, false)
	committer := &fakeCommitter{state: seeded.candidateRuntime}
	checker := &fakeChecker{}
	service := newFakeService(true)
	manager, err := NewManager(workspace.layout, checker, service, committer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Recover(context.Background())
	if err != nil || !result.Recovered || !result.Committed || len(checker.Paths()) != 1 || service.IsActive() {
		t.Fatalf("candidate reinstall result=%#v err=%v checker=%v active=%v", result, err, checker.Paths(), service.IsActive())
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "NEW" {
		t.Fatalf("candidate reinstall live %q", got)
	}
}

func TestRecoverUnknownDBOrLiveQuarantines(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		db           RuntimeState
		mutateLive   bool
		wantCallFree bool
	}{
		{name: "unknown-db", db: RuntimeState{ConfigGeneration: 9, LiveConfigHash: configHash("OTHER")}, wantCallFree: true},
		{name: "unknown-live", db: RuntimeState{ConfigGeneration: 7, LiveConfigHash: oldSnapshotHash()}, mutateLive: true, wantCallFree: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			workspace := newManagerWorkspace(t, "OLD")
			seeded := seedPreparedTransaction(t, workspace, "NEW", true, false)
			if testCase.mutateLive {
				if err := os.WriteFile(workspace.livePath, []byte("OTHER"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			checker := &fakeChecker{}
			service := newFakeService(true)
			committer := &fakeCommitter{state: testCase.db}
			manager, err := NewManager(workspace.layout, checker, service, committer)
			if err != nil {
				t.Fatal(err)
			}
			_, err = manager.Recover(context.Background())
			assertManagerIs(t, err, ErrQuarantined)
			if testCase.wantCallFree && (len(checker.Paths()) != 0 || len(service.Trace()) != 0) {
				t.Fatalf("quarantine performed calls: checker=%v service=%v", checker.Paths(), service.Trace())
			}
			if _, err := os.Stat(workspace.layout.JournalPath); err != nil {
				t.Fatalf("quarantine removed journal: %v", err)
			}
			if _, err := manager.Apply(context.Background(), []byte("MUST-NOT-APPLY")); !errors.Is(err, ErrQuarantined) {
				t.Fatalf("mutation after quarantine returned %v", err)
			}
			_ = seeded
		})
	}
}

func TestRecoverTamperedCandidateQuarantinesWithoutDeletingEvidence(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	seeded := seedPreparedTransaction(t, workspace, "NEW", true, false)
	if err := os.WriteFile(workspace.layout.CandidatePath(seeded.journal.TransactionID), []byte("TAMPERED"), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(
		workspace.layout,
		&fakeChecker{},
		newFakeService(true),
		&fakeCommitter{state: seeded.oldRuntime},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Recover(context.Background())
	assertManagerIs(t, err, ErrQuarantined)
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "OLD" {
		t.Fatalf("tampered recovery changed live config: %q", got)
	}
	if _, err := os.Stat(workspace.layout.CandidatePath(seeded.journal.TransactionID)); err != nil {
		t.Fatalf("tampered candidate evidence was removed: %v", err)
	}
	if _, err := os.Stat(workspace.layout.JournalPath); err != nil {
		t.Fatalf("quarantine journal missing: %v", err)
	}
}

func TestRecoverRollbackFailedQuarantinesWithoutMutation(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	seeded := seedPreparedTransaction(t, workspace, "NEW", true, true)
	guard, err := workspace.layout.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.UpdateJournalPhase(configtx.PhaseRollbackFailed); err != nil {
		_ = guard.Release()
		t.Fatal(err)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	checker := &fakeChecker{}
	service := newFakeService(true)
	manager, err := NewManager(workspace.layout, checker, service, &fakeCommitter{state: seeded.oldRuntime})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Recover(context.Background())
	assertManagerIs(t, err, ErrQuarantined)
	if len(checker.Paths()) != 0 || len(service.Trace()) != 0 {
		t.Fatalf("rollback_failed recovery mutated: checker=%v service=%v", checker.Paths(), service.Trace())
	}
}

func TestApplyRecoversExistingJournalBeforeNewChecker(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	seeded := seedPreparedTransaction(t, workspace, "RECOVERY", true, true)
	checker := &fakeChecker{}
	checker.hook = func(path string) {
		live, readErr := os.ReadFile(workspace.livePath)
		if readErr != nil || string(live) != "OLD" {
			t.Fatalf("new checker ran before old recovery: live=%q err=%v path=%s", live, readErr, path)
		}
	}
	service := newFakeService(true)
	committer := &fakeCommitter{state: seeded.oldRuntime}
	manager, err := NewManager(workspace.layout, checker, service, committer)
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	if err != nil || !result.Committed {
		t.Fatalf("apply after recovery result=%#v err=%v", result, err)
	}
	paths := checker.Paths()
	if len(paths) != 1 {
		t.Fatalf("expected only new apply checker call, got %v", paths)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "NEW" {
		t.Fatalf("apply after recovery live %q", got)
	}
}

func TestPostCommitCleanupFailureIsCommittedAndPending(t *testing.T) {
	workspace := newManagerWorkspace(t, "OLD")
	manager, _, service, committer := newManagerFor(t, workspace, true)
	committer.hook = func(RuntimeState) {
		if err := os.Chmod(workspace.layout.BackupDir, 0750); err != nil {
			t.Fatalf("make retention directory unsafe: %v", err)
		}
	}
	t.Cleanup(func() { _ = os.Chmod(workspace.layout.BackupDir, 0700) })
	result, err := manager.Apply(context.Background(), []byte("NEW"))
	assertManagerIs(t, err, ErrPostCommitCleanup)
	if !result.Committed || !result.CleanupPending {
		t.Fatalf("post-commit result=%#v", result)
	}
	if err := os.Chmod(workspace.layout.BackupDir, 0700); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "NEW" {
		t.Fatalf("post-commit live changed: %q", got)
	}
	if _, err := os.Stat(workspace.layout.JournalPath); err != nil {
		t.Fatalf("cleanup evidence journal missing: %v", err)
	}
	if committer.State().ConfigGeneration != 8 {
		t.Fatalf("post-commit runtime rolled back: %#v", committer.State())
	}
	recovery, err := manager.Recover(context.Background())
	if err != nil || !recovery.Recovered || !recovery.Committed {
		t.Fatalf("post-commit candidate recovery result=%#v err=%v", recovery, err)
	}
	if got, _ := os.ReadFile(workspace.livePath); string(got) != "NEW" || !service.IsActive() {
		t.Fatalf("post-commit recovery changed candidate state: live=%q active=%v", got, service.IsActive())
	}
	if committer.State().ConfigGeneration != 8 || committer.State().LiveConfigHash != configHash("NEW") {
		t.Fatalf("post-commit recovery rolled back runtime: %#v", committer.State())
	}
	if _, err := os.Stat(workspace.layout.JournalPath); !os.IsNotExist(err) {
		t.Fatalf("post-commit recovery left journal: %v", err)
	}
}

func assertRollbackEvidence(t *testing.T, workspace managerWorkspace, transactionID string) {
	t.Helper()
	for _, path := range []string{
		workspace.layout.JournalPath,
		workspace.layout.CandidatePath(transactionID),
		workspace.layout.BackupPath(transactionID),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("rollback evidence %s missing: %v", path, err)
		}
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func readTestJournal(t *testing.T, data []byte) configtx.Journal {
	t.Helper()
	var journal configtx.Journal
	if err := json.Unmarshal(data, &journal); err != nil {
		t.Fatalf("decode journal: %v", err)
	}
	return journal
}

func hasServiceMutation(trace []string) bool {
	for _, operation := range trace {
		switch operation {
		case "start", "stop", "restart":
			return true
		}
	}
	return false
}

func countTrace(trace []string, expected string) int {
	count := 0
	for _, operation := range trace {
		if operation == expected {
			count++
		}
	}
	return count
}

func containsInOrder(values []string, expected ...string) bool {
	position := 0
	for _, value := range values {
		if position < len(expected) && value == expected[position] {
			position++
		}
	}
	return position == len(expected)
}
