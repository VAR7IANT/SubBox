package configtx

import (
	"context"
	"errors"
	"testing"
)

func TestJournalPhaseStateMachineIsForwardOnly(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard := acquireTest(t, workspace.layout)
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	metadata := snapshot.Metadata()
	snapshot.Destroy()
	j := mustJournal(t, mustTransactionID(t), metadata)
	if err := guard.CreatePreparedJournal(j); err != nil {
		t.Fatal(err)
	}

	if err := guard.MarkLiveApplied(); err != nil {
		t.Fatal(err)
	}
	readBack, err := guard.ReadJournal()
	if err != nil {
		t.Fatal(err)
	}
	if readBack.Phase != PhaseLiveApplied {
		t.Fatalf("phase after live apply = %q", readBack.Phase)
	}
	if err := guard.MarkStateCommitted(); err != nil {
		t.Fatal(err)
	}
	readBack, err = guard.ReadJournal()
	if err != nil {
		t.Fatal(err)
	}
	if readBack.Phase != PhaseStateCommitted {
		t.Fatalf("phase after state commit = %q", readBack.Phase)
	}
	if err := guard.MarkStateCommitted(); err != nil {
		t.Fatal(err)
	}
	if err := guard.MarkRollbackFailed(); !errors.Is(err, ErrInvalidJournalTransition) {
		t.Fatalf("state_committed -> rollback_failed error = %v", err)
	}
	if err := guard.MarkLiveApplied(); !errors.Is(err, ErrInvalidJournalTransition) {
		t.Fatalf("state_committed -> live_applied error = %v", err)
	}
}

func TestJournalRecoveryRequiredPhaseBlocksForwardTransitions(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard, err := workspace.layout.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		_ = guard.Release()
		t.Fatal(err)
	}
	j := mustJournal(t, mustTransactionID(t), snapshot.Metadata())
	snapshot.Destroy()
	if err := guard.CreatePreparedJournal(j); err != nil {
		_ = guard.Release()
		t.Fatal(err)
	}
	if err := guard.UpdateJournalPhase(PhaseRecoveryRequired); err != nil {
		_ = guard.Release()
		t.Fatal(err)
	}
	if err := guard.MarkLiveApplied(); !errors.Is(err, ErrInvalidJournalTransition) {
		t.Fatalf("recovery_required -> live_applied error = %v", err)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
}
