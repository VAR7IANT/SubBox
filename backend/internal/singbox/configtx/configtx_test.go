package configtx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type testWorkspace struct {
	layout   Layout
	stateDir string
	liveDir  string
	livePath string
}

func newTestWorkspace(t *testing.T, mode os.FileMode, content string) testWorkspace {
	t.Helper()
	root := t.TempDir()
	stateDir := filepath.Join(root, "state")
	liveDir := filepath.Join(root, "sing-box")
	if err := os.Mkdir(liveDir, 0700); err != nil {
		t.Fatal(err)
	}
	livePath := filepath.Join(liveDir, "config.json")
	writeTestFile(t, livePath, []byte(content), mode)
	return testWorkspace{
		layout:   NewLayout(stateDir, livePath),
		stateDir: stateDir,
		liveDir:  liveDir,
		livePath: livePath,
	}
}

func acquireTest(t *testing.T, layout Layout) *LockGuard {
	t.Helper()
	guard, err := layout.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = guard.Release() })
	return guard
}

func writeTestFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mustTransactionID(t *testing.T) string {
	t.Helper()
	return NewTransactionID()
}

func mustJournal(t *testing.T, transactionID string, metadata FileMetadata) Journal {
	t.Helper()
	journal, err := NewJournal(
		transactionID,
		strings.Repeat("a", 64),
		strings.Repeat("b", 64),
		7,
		true,
		metadata,
	)
	if err != nil {
		t.Fatal(err)
	}
	return journal
}

func assertIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("errors.Is(%v, %v) = false", err, target)
	}
}

func TestDefaultLayoutAndDerivedNames(t *testing.T) {
	layout := DefaultLayout()
	if layout.StateDir != "/etc/subbox" ||
		layout.LockPath != "/etc/subbox/config.lock" ||
		layout.JournalPath != "/etc/subbox/config-transaction.json" ||
		layout.BackupDir != "/etc/subbox/backups" ||
		layout.LiveConfigPath != "/etc/sing-box/config.json" {
		t.Fatalf("unexpected default layout: %#v", layout)
	}
	id := "01234567-89ab-4cde-8fab-0123456789ab"
	if got, want := filepath.Base(layout.CandidatePath(id)), ".subbox-candidate-"+id+".json"; got != want {
		t.Fatalf("candidate name %q, want %q", got, want)
	}
	if got, want := filepath.Base(layout.BackupPath(id)), "config-"+id+".json"; got != want {
		t.Fatalf("backup name %q, want %q", got, want)
	}
	if got, want := filepath.Base(layout.ApplyTempPrefix(id)), ".subbox-apply-"+id+"-"; got != want {
		t.Fatalf("apply prefix %q, want %q", got, want)
	}
	if layout.CandidatePath("../escape") != "" || layout.BackupPath("/absolute") != "" || layout.ApplyTempPrefix(`..\escape`) != "" {
		t.Fatal("untrusted transaction identifiers produced paths")
	}
}

func TestDirectoriesAndLockSecurity(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard := acquireTest(t, workspace.layout)
	for _, path := range []string{workspace.stateDir, workspace.layout.BackupDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0700 {
			t.Fatalf("directory %s mode %o, want 700", path, info.Mode().Perm())
		}
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(workspace.layout.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("lock mode %o, want 600", info.Mode().Perm())
	}

	unsafeState := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(unsafeState, 0777); err != nil {
		t.Fatal(err)
	}
	unsafeLayout := NewLayout(unsafeState, workspace.livePath)
	_, err = unsafeLayout.Acquire(context.Background())
	assertIs(t, err, ErrUnsafePermissions)

	symlinkState := filepath.Join(t.TempDir(), "state")
	stateTarget := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(stateTarget, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stateTarget, symlinkState); err != nil {
		t.Fatal(err)
	}
	_, err = NewLayout(symlinkState, workspace.livePath).Acquire(context.Background())
	assertIs(t, err, ErrUnsafePath)

	unsafeBackupRoot := t.TempDir()
	unsafeBackupState := filepath.Join(unsafeBackupRoot, "state")
	if err := os.Mkdir(unsafeBackupState, 0700); err != nil {
		t.Fatal(err)
	}
	backupTarget := filepath.Join(unsafeBackupRoot, "backup-target")
	if err := os.Mkdir(backupTarget, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(backupTarget, filepath.Join(unsafeBackupState, BackupDirectory)); err != nil {
		t.Fatal(err)
	}
	_, err = NewLayout(unsafeBackupState, workspace.livePath).Acquire(context.Background())
	assertIs(t, err, ErrUnsafePath)
}

func TestLockReleaseIdempotentAndContention(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	first := acquireTest(t, workspace.layout)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	_, err := workspace.layout.Acquire(ctx)
	assertIs(t, err, context.DeadlineExceeded)
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second := acquireTest(t, workspace.layout)
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	_, err = workspace.layout.Acquire(ctx)
	assertIs(t, err, context.Canceled)

	for i := 0; i < 32; i++ {
		guard, err := workspace.layout.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := guard.Release(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLockRejectsSymlink(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	if err := os.MkdirAll(workspace.stateDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(workspace.livePath, workspace.layout.LockPath); err != nil {
		t.Fatal(err)
	}
	_, err := workspace.layout.Acquire(context.Background())
	assertIs(t, err, ErrUnsafeFile)
}

func TestLockHelperProcess(t *testing.T) {
	if os.Getenv("SUBBOX_LOCK_PROBE") == "1" {
		lockProbeHelper()
		return
	}
	if os.Getenv("SUBBOX_LOCK_HELPER") != "1" {
		return
	}
	layout := NewLayout(os.Getenv("SUBBOX_LOCK_STATE"), os.Getenv("SUBBOX_LOCK_LIVE"))
	guard, err := layout.Acquire(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper lock acquire failed")
		os.Exit(2)
	}
	if _, err := fmt.Fprintln(os.Stdout, "READY"); err != nil {
		_ = guard.Release()
		os.Exit(3)
	}
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	_ = guard.Release()
}

func lockProbeHelper() {
	layout := NewLayout(os.Getenv("SUBBOX_LOCK_STATE"), os.Getenv("SUBBOX_LOCK_LIVE"))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_, err := layout.Acquire(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		fmt.Fprintln(os.Stderr, "probe unexpectedly acquired config lock")
		os.Exit(4)
	}
	if _, err := fmt.Fprintln(os.Stdout, "BLOCKED"); err != nil {
		os.Exit(5)
	}
	guard, err := layout.Acquire(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe failed to acquire after release")
		os.Exit(6)
	}
	if _, err := fmt.Fprintln(os.Stdout, "READY"); err != nil {
		_ = guard.Release()
		os.Exit(7)
	}
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	_ = guard.Release()
}

func TestLockCrossProcess(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	command := exec.Command(os.Args[0], "-test.run=TestLockHelperProcess", "--")
	command.Env = append(os.Environ(),
		"SUBBOX_LOCK_HELPER=1",
		"SUBBOX_LOCK_STATE="+workspace.stateDir,
		"SUBBOX_LOCK_LIVE="+workspace.livePath,
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal(err)
	}
	if strings.TrimSpace(line) != "READY" {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("helper readiness %q", line)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_, err = workspace.layout.Acquire(ctx)
	cancel()
	assertIs(t, err, context.DeadlineExceeded)
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	_, err = workspace.layout.Acquire(ctx)
	assertIs(t, err, context.Canceled)

	if _, err := io.WriteString(stdin, "RELEASE\n"); err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	guard, err := workspace.layout.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestLockGuardReleasePreservesCrossProcessLock(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard, err := workspace.layout.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	finish, err := guard.beginOperation()
	if err != nil {
		_ = guard.Release()
		t.Fatal(err)
	}
	finished := false
	defer func() {
		if !finished {
			finish()
		}
		_ = guard.Release()
	}()

	command := exec.Command(os.Args[0], "-test.run=TestLockHelperProcess", "--")
	command.Env = append(os.Environ(),
		"SUBBOX_LOCK_PROBE=1",
		"SUBBOX_LOCK_STATE="+workspace.stateDir,
		"SUBBOX_LOCK_LIVE="+workspace.livePath,
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	cleanupProcess := true
	defer func() {
		if cleanupProcess {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(line) != "BLOCKED" {
		t.Fatalf("probe readiness %q", line)
	}

	releaseDone := make(chan error, 1)
	go func() { releaseDone <- guard.Release() }()
	waitForReleaseBoundary(t, guard)
	probeReady := make(chan string, 1)
	go func() {
		line, err := reader.ReadString('\n')
		if err != nil {
			probeReady <- "read-error"
			return
		}
		probeReady <- strings.TrimSpace(line)
	}()
	select {
	case line := <-probeReady:
		t.Fatalf("probe acquired before active operation finished: %q", line)
	case <-time.After(60 * time.Millisecond):
	}

	finish()
	finished = true
	if err := <-releaseDone; err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-probeReady:
		if line != "READY" {
			t.Fatalf("probe post-release readiness %q", line)
		}
	case <-time.After(time.Second):
		t.Fatal("probe did not acquire after Release")
	}
	if _, err := io.WriteString(stdin, "RELEASE\n"); err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	cleanupProcess = false
}

func TestLockGuardReleaseWaitsForActiveOperation(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard := acquireTest(t, workspace.layout)
	finish, err := guard.beginOperation()
	if err != nil {
		t.Fatal(err)
	}

	releaseDone := make(chan error, 1)
	go func() {
		releaseDone <- guard.Release()
	}()
	waitForReleaseBoundary(t, guard)
	select {
	case err := <-releaseDone:
		t.Fatalf("Release completed while operation was active: %v", err)
	case <-time.After(40 * time.Millisecond):
	}
	if _, err := guard.beginOperation(); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("operation crossed release boundary with error %v", err)
	}

	finish()
	if err := <-releaseDone; err != nil {
		t.Fatal(err)
	}
	if _, err := guard.beginOperation(); !errors.Is(err, ErrLockNotHeld) {
		t.Fatalf("operation after Release returned error %v", err)
	}
}

func waitForReleaseBoundary(t *testing.T, guard *LockGuard) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		guard.stateMu.Lock()
		releasing := guard.releasing
		guard.stateMu.Unlock()
		if releasing {
			return
		}
		select {
		case <-deadline:
			t.Fatal("Release did not enter its release boundary")
		default:
			runtime.Gosched()
		}
	}
}

func TestLockGuardSerializesSameGuardOperations(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard := acquireTest(t, workspace.layout)
	finishFirst, err := guard.beginOperation()
	if err != nil {
		t.Fatal(err)
	}

	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		finishSecond, err := guard.beginOperation()
		if err == nil {
			finishSecond()
		}
		secondDone <- err
	}()
	<-secondStarted
	select {
	case err := <-secondDone:
		t.Fatalf("second operation crossed active operation boundary: %v", err)
	case <-time.After(40 * time.Millisecond):
	}

	finishFirst()
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotSecurityHashAndMetadata(t *testing.T) {
	for _, mode := range []os.FileMode{0600, 0640} {
		t.Run(fmt.Sprintf("mode-%o", mode), func(t *testing.T) {
			workspace := newTestWorkspace(t, mode, "snapshot-secret")
			guard := acquireTest(t, workspace.layout)
			snapshot, err := guard.SnapshotLiveConfig()
			if err != nil {
				t.Fatal(err)
			}
			defer snapshot.Destroy()
			if got := string(snapshot.Content()); got != "snapshot-secret" {
				t.Fatalf("snapshot content %q", got)
			}
			if got, want := snapshot.Hash(), hashConfig([]byte("snapshot-secret")); got != want {
				t.Fatalf("snapshot hash %q, want %q", got, want)
			}
			if snapshot.Metadata().Mode != uint32(mode.Perm()) || snapshot.Metadata().UID < 0 || snapshot.Metadata().GID < 0 {
				t.Fatalf("unexpected snapshot metadata: %#v", snapshot.Metadata())
			}
			copyOfContent := snapshot.Content()
			copyOfContent[0] = 'X'
			if string(snapshot.Content()) != "snapshot-secret" {
				t.Fatal("snapshot content was not defensively copied")
			}
			snapshot.Destroy()
			if snapshot.Content() != nil || snapshot.Hash() != "" {
				t.Fatal("snapshot was not cleared")
			}
		})
	}

	unsafeMode := newTestWorkspace(t, 0644, "mode-secret")
	_, err := acquireTestSnapshot(unsafeMode)
	assertIs(t, err, ErrUnsafePermissions)

	symlinkWorkspace := newTestWorkspace(t, 0600, "target-secret")
	symlinkTarget := filepath.Join(symlinkWorkspace.liveDir, "real.json")
	if err := os.Rename(symlinkWorkspace.livePath, symlinkTarget); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(symlinkTarget, symlinkWorkspace.livePath); err != nil {
		t.Fatal(err)
	}
	guard := acquireTest(t, symlinkWorkspace.layout)
	_, err = guard.SnapshotLiveConfig()
	assertIs(t, err, ErrUnsafeFile)

	directoryWorkspace := newTestWorkspace(t, 0600, "directory-placeholder")
	if err := os.Remove(directoryWorkspace.livePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directoryWorkspace.livePath, 0700); err != nil {
		t.Fatal(err)
	}
	guard = acquireTest(t, directoryWorkspace.layout)
	_, err = guard.SnapshotLiveConfig()
	assertIs(t, err, ErrUnsafeFile)

	largeWorkspace := newTestWorkspace(t, 0600, "small")
	if err := os.Remove(largeWorkspace.livePath); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, largeWorkspace.livePath, bytes.Repeat([]byte{'x'}, MaxConfigBytes+1), 0600)
	guard = acquireTest(t, largeWorkspace.layout)
	_, err = guard.SnapshotLiveConfig()
	assertIs(t, err, ErrConfigTooLarge)
	if strings.Contains(err.Error(), "snapshot-secret") {
		t.Fatal("snapshot error contained config plaintext")
	}
}

func acquireTestSnapshot(workspace testWorkspace) (*ConfigSnapshot, error) {
	guard, err := workspace.layout.Acquire(context.Background())
	if err != nil {
		return nil, err
	}
	defer guard.Release()
	return guard.SnapshotLiveConfig()
}

func TestCandidateAndBackupArtifacts(t *testing.T) {
	workspace := newTestWorkspace(t, 0640, "OLD")
	guard := acquireTest(t, workspace.layout)
	oldSnapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer oldSnapshot.Destroy()
	id := mustTransactionID(t)
	candidateSnapshot, err := guard.WriteCandidate(id, oldSnapshot, []byte("NEW"))
	if err != nil {
		t.Fatal(err)
	}
	defer candidateSnapshot.Destroy()
	candidatePath := workspace.layout.CandidatePath(id)
	if got, err := os.ReadFile(candidatePath); err != nil || string(got) != "NEW" {
		t.Fatalf("candidate content %q, err %v", got, err)
	}
	if info, err := os.Stat(candidatePath); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0640 || !info.Mode().IsRegular() {
		t.Fatalf("candidate info: %#v", info)
	}
	if candidateSnapshot.Hash() != hashConfig([]byte("NEW")) {
		t.Fatal("candidate hash mismatch")
	}

	if _, err := guard.WriteCandidate(id, oldSnapshot, []byte("OVERWRITE")); !errors.Is(err, ErrArtifactExists) {
		t.Fatalf("second candidate write error %v", err)
	}
	if got, err := os.ReadFile(candidatePath); err != nil || string(got) != "NEW" {
		t.Fatalf("existing candidate changed: %q, err %v", got, err)
	}

	backupID := mustTransactionID(t)
	if err := guard.CreateBackup(backupID, oldSnapshot); err != nil {
		t.Fatal(err)
	}
	backupPath := workspace.layout.BackupPath(backupID)
	if got, err := os.ReadFile(backupPath); err != nil || string(got) != "OLD" {
		t.Fatalf("backup content %q, err %v", got, err)
	}
	if info, err := os.Stat(backupPath); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0600 || !info.Mode().IsRegular() {
		t.Fatalf("backup info: %#v", info)
	}
	if err := guard.CreateBackup(backupID, oldSnapshot); !errors.Is(err, ErrArtifactExists) {
		t.Fatalf("second backup write error %v", err)
	}

	symlinkID := mustTransactionID(t)
	symlinkPath := workspace.layout.CandidatePath(symlinkID)
	if err := os.Symlink(workspace.livePath, symlinkPath); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.WriteCandidate(symlinkID, oldSnapshot, []byte("SYMLINK")); !errors.Is(err, ErrUnsafeFile) {
		t.Fatalf("candidate symlink error %v", err)
	}
	if err := os.Remove(symlinkPath); err != nil {
		t.Fatal(err)
	}
	if err := guard.CreateBackup(symlinkID, oldSnapshot); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(workspace.layout.BackupPath(symlinkID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(workspace.livePath, workspace.layout.BackupPath(symlinkID)); err != nil {
		t.Fatal(err)
	}
	if err := guard.CreateBackup(symlinkID, oldSnapshot); !errors.Is(err, ErrUnsafeFile) {
		t.Fatalf("backup symlink error %v", err)
	}

	if _, err := guard.WriteCandidate(mustTransactionID(t), oldSnapshot, bytes.Repeat([]byte{'z'}, MaxConfigBytes+1)); !errors.Is(err, ErrConfigTooLarge) {
		t.Fatalf("oversized candidate error %v", err)
	}
	tooLargeSnapshot, err := NewConfigSnapshot(bytes.Repeat([]byte{'z'}, MaxConfigBytes+1), oldSnapshot.Metadata())
	if !errors.Is(err, ErrConfigTooLarge) || tooLargeSnapshot != nil {
		t.Fatalf("oversized snapshot result %#v, err %v", tooLargeSnapshot, err)
	}

	cleanupID := mustTransactionID(t)
	if _, err := guard.WriteCandidate(cleanupID, oldSnapshot, []byte("CLEANUP")); err != nil {
		t.Fatal(err)
	}
	if err := guard.RemoveCandidate(cleanupID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workspace.layout.CandidatePath(cleanupID)); !os.IsNotExist(err) {
		t.Fatalf("candidate cleanup result %v", err)
	}
	applyPath := workspace.layout.ApplyTempPrefix(cleanupID) + "deadbeef"
	writeTestFile(t, applyPath, []byte("APPLY"), 0600)
	if err := guard.RemoveApplyTemps(cleanupID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(applyPath); !os.IsNotExist(err) {
		t.Fatalf("apply cleanup result %v", err)
	}
}

func TestAtomicInstallRetainsCandidateAndRejectsLiveSymlink(t *testing.T) {
	workspace := newTestWorkspace(t, 0640, "OLD")
	guard := acquireTest(t, workspace.layout)
	oldSnapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer oldSnapshot.Destroy()
	id := mustTransactionID(t)
	candidate, err := guard.WriteCandidate(id, oldSnapshot, []byte("NEW"))
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Destroy()
	if err := guard.InstallCandidate(id, candidate); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(workspace.livePath); err != nil || string(got) != "NEW" {
		t.Fatalf("live content %q, err %v", got, err)
	}
	if got, err := os.ReadFile(workspace.layout.CandidatePath(id)); err != nil || string(got) != "NEW" {
		t.Fatalf("retained candidate %q, err %v", got, err)
	}
	if err := verifyLiveMetadata(workspace.livePath, oldSnapshot.Metadata()); err != nil {
		t.Fatalf("installed metadata: %v", err)
	}

	if err := os.Remove(workspace.livePath); err != nil {
		t.Fatal(err)
	}
	symlinkTarget := filepath.Join(workspace.liveDir, "target.json")
	writeTestFile(t, symlinkTarget, []byte("TARGET"), 0600)
	if err := os.Symlink(symlinkTarget, workspace.livePath); err != nil {
		t.Fatal(err)
	}
	id = mustTransactionID(t)
	if _, err := guard.WriteCandidate(id, oldSnapshot, []byte("SHOULD-NOT-APPLY")); err != nil {
		t.Fatal(err)
	}
	installSnapshot, err := NewConfigSnapshot([]byte("SHOULD-NOT-APPLY"), oldSnapshot.Metadata())
	if err != nil {
		t.Fatal(err)
	}
	defer installSnapshot.Destroy()
	if err := guard.InstallCandidate(id, installSnapshot); !errors.Is(err, ErrUnsafeFile) {
		t.Fatalf("live symlink install error %v", err)
	}
	if got, err := os.ReadFile(symlinkTarget); err != nil || string(got) != "TARGET" {
		t.Fatalf("symlink target changed: %q, err %v", got, err)
	}

	if err := os.Remove(workspace.livePath); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, workspace.livePath, []byte("OLD-AGAIN"), 0640)
	id = mustTransactionID(t)
	if err := guard.InstallCandidate(id, installSnapshot); !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("missing candidate install error %v", err)
	}
	if got, err := os.ReadFile(workspace.livePath); err != nil || string(got) != "OLD-AGAIN" {
		t.Fatalf("live changed after pre-rename failure: %q, err %v", got, err)
	}
}

func TestInstallJournalCandidateUsesValidatedHashAndMetadata(t *testing.T) {
	workspace := newTestWorkspace(t, 0640, "OLD")
	guard := acquireTest(t, workspace.layout)
	oldSnapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer oldSnapshot.Destroy()
	id := mustTransactionID(t)
	candidateBytes := []byte("NEW-FROM-JOURNAL")
	candidateSnapshot, err := guard.WriteCandidate(id, oldSnapshot, candidateBytes)
	if err != nil {
		t.Fatal(err)
	}
	defer candidateSnapshot.Destroy()
	journal, err := NewJournal(id, oldSnapshot.Hash(), candidateSnapshot.Hash(), 0, false, oldSnapshot.Metadata())
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.CreatePreparedJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := guard.InstallJournalCandidate(journal); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(workspace.livePath); err != nil || string(got) != string(candidateBytes) {
		t.Fatalf("journal install content %q, err %v", got, err)
	}
}

func TestJournalRoundTripAndPhaseUpdates(t *testing.T) {
	workspace := newTestWorkspace(t, 0640, "OLD")
	guard := acquireTest(t, workspace.layout)
	metadata := FileMetadata{Mode: 0640, UID: 0, GID: 0}
	// Use the actual development user's ownership so fchown remains portable.
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	metadata = snapshot.Metadata()
	snapshot.Destroy()
	id := mustTransactionID(t)
	journal := mustJournal(t, id, metadata)
	if err := guard.CreatePreparedJournal(journal); err != nil {
		t.Fatal(err)
	}
	readBack, err := guard.ReadJournal()
	if err != nil {
		t.Fatal(err)
	}
	if readBack != journal {
		t.Fatalf("journal round trip mismatch:\nwant %#v\ngot  %#v", journal, readBack)
	}
	if info, err := os.Stat(workspace.layout.JournalPath); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0600 || !info.Mode().IsRegular() {
		t.Fatalf("journal info: %#v", info)
	}
	if err := guard.UpdateJournalPhase(PhaseRollbackFailed); err != nil {
		t.Fatal(err)
	}
	readBack, err = guard.ReadJournal()
	if err != nil {
		t.Fatal(err)
	}
	if readBack.Phase != PhaseRollbackFailed {
		t.Fatalf("phase %q, want %q", readBack.Phase, PhaseRollbackFailed)
	}
	if err := guard.UpdateJournalPhase(PhaseRollbackFailed); err != nil {
		t.Fatal(err)
	}
	if err := guard.UpdateJournalPhase(PhasePrepared); !errors.Is(err, ErrInvalidJournal) {
		t.Fatalf("rollback_failed -> prepared error %v", err)
	}
	if err := guard.RemoveJournal(); err != nil {
		t.Fatal(err)
	}
	if _, err := guard.ReadJournal(); !errors.Is(err, ErrJournalNotFound) {
		t.Fatalf("missing journal read error %v", err)
	}
	if err := guard.RemoveJournal(); !errors.Is(err, ErrJournalNotFound) {
		t.Fatalf("missing journal remove error %v", err)
	}
}

func TestJournalExistingDoesNotOverwrite(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard := acquireTest(t, workspace.layout)
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	metadata := snapshot.Metadata()
	snapshot.Destroy()
	first := mustJournal(t, mustTransactionID(t), metadata)
	if err := guard.CreatePreparedJournal(first); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(workspace.layout.JournalPath)
	if err != nil {
		t.Fatal(err)
	}
	second := mustJournal(t, mustTransactionID(t), metadata)
	if err := guard.CreatePreparedJournal(second); !errors.Is(err, ErrJournalExists) {
		t.Fatalf("existing journal error %v", err)
	}
	unchanged, err := os.ReadFile(workspace.layout.JournalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, unchanged) {
		t.Fatal("existing journal was overwritten")
	}
}

func TestJournalSecretBoundary(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "task009a-known-config-secret")
	guard := acquireTest(t, workspace.layout)
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	journal := mustJournal(t, mustTransactionID(t), snapshot.Metadata())
	snapshot.Destroy()
	if err := guard.CreatePreparedJournal(journal); err != nil {
		t.Fatal(err)
	}
	diskBytes, err := os.ReadFile(workspace.layout.JournalPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(diskBytes, []byte("task009a-known-config-secret")) {
		t.Fatal("journal contained raw config content")
	}

	base, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"config", "credential", "password", "private_key", "subscription_token"} {
		data := appendJSON(base, `,"`+field+`":"task009a-known-config-secret"`)
		if err := os.WriteFile(workspace.layout.JournalPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := guard.ReadJournal(); !errors.Is(err, ErrMalformedJournal) {
			t.Fatalf("raw field %q accepted with error %v", field, err)
		}
	}
}

func TestJournalStrictValidation(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard := acquireTest(t, workspace.layout)
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	metadata := snapshot.Metadata()
	snapshot.Destroy()
	baseJournal := mustJournal(t, mustTransactionID(t), metadata)
	base, err := json.Marshal(baseJournal)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		data       []byte
		expectSent error
	}{
		{name: "duplicate field", data: appendJSON(base, `,"phase":"prepared"`), expectSent: ErrMalformedJournal},
		{name: "unknown field", data: appendJSON(base, `,"unknown":1`), expectSent: ErrMalformedJournal},
		{name: "trailing value", data: append(base, []byte(`{}`)...), expectSent: ErrMalformedJournal},
		{name: "malformed", data: []byte(`{"format_version":`), expectSent: ErrMalformedJournal},
		{name: "oversized", data: bytes.Repeat([]byte{'x'}, MaxJournalBytes+1), expectSent: ErrMalformedJournal},
		{name: "wrong version", data: replaceJSON(base, `"format_version":1`, `"format_version":2`), expectSent: ErrUnsupportedJournalVersion},
		{name: "bad uuid", data: replaceJSON(base, `"transaction_id":"`+baseJournal.TransactionID+`"`, `"transaction_id":"not-a-uuid"`), expectSent: ErrInvalidJournal},
		{name: "bad phase", data: replaceJSON(base, `"phase":"prepared"`, `"phase":"committed"`), expectSent: ErrInvalidJournal},
		{name: "bad old hash", data: replaceJSON(base, `"old_config_hash":"`+strings.Repeat("a", 64)+`"`, `"old_config_hash":""`), expectSent: ErrInvalidJournal},
		{name: "bad candidate hash", data: replaceJSON(base, `"candidate_config_hash":"`+strings.Repeat("b", 64)+`"`, `"candidate_config_hash":"short"`), expectSent: ErrInvalidJournal},
		{name: "uppercase hash", data: replaceJSON(base, `"old_config_hash":"`+strings.Repeat("a", 64)+`"`, `"old_config_hash":"`+strings.Repeat("A", 64)+`"`), expectSent: ErrInvalidJournal},
		{name: "generation relation", data: replaceJSON(base, `"candidate_generation":8`, `"candidate_generation":9`), expectSent: ErrInvalidJournal},
		{name: "negative generation", data: replaceJSON(base, `"old_generation":7`, `"old_generation":-1`), expectSent: ErrInvalidJournal},
		{name: "generation overflow", data: replaceJSON(base, `"old_generation":7`, `"old_generation":9223372036854775807`), expectSent: ErrInvalidJournal},
		{name: "bad mode", data: replaceJSON(base, `"live_config_mode":`+fmt.Sprint(metadata.Mode), `"live_config_mode":420`), expectSent: ErrInvalidJournal},
		{name: "negative uid", data: replaceJSON(base, `"live_config_uid":`+fmt.Sprint(metadata.UID), `"live_config_uid":-1`), expectSent: ErrInvalidJournal},
		{name: "negative gid", data: replaceJSON(base, `"live_config_gid":`+fmt.Sprint(metadata.GID), `"live_config_gid":-1`), expectSent: ErrInvalidJournal},
		{name: "bad candidate name", data: replaceJSON(base, `"candidate_name":"`+baseJournal.CandidateName+`"`, `"candidate_name":"../candidate.json"`), expectSent: ErrInvalidJournal},
		{name: "bad backup name", data: replaceJSON(base, `"backup_name":"`+baseJournal.BackupName+`"`, `"backup_name":"/tmp/backup.json"`), expectSent: ErrInvalidJournal},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := os.WriteFile(workspace.layout.JournalPath, testCase.data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(workspace.layout.JournalPath, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := guard.ReadJournal()
			assertIs(t, err, testCase.expectSent)
		})
	}
}

func appendJSON(data []byte, suffix string) []byte {
	trimmed := bytes.TrimSuffix(append([]byte(nil), data...), []byte{'}'})
	return append(append(trimmed, []byte(suffix)...), '}')
}

func replaceJSON(data []byte, oldValue, newValue string) []byte {
	return bytes.Replace(append([]byte(nil), data...), []byte(oldValue), []byte(newValue), 1)
}

func TestBackupRetentionAndPreservation(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard := acquireTest(t, workspace.layout)
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Destroy()

	ids := make([]string, 0, DefaultBackupRetention+2)
	for i := 0; i < DefaultBackupRetention+2; i++ {
		id := mustTransactionID(t)
		if err := guard.CreateBackup(id, snapshot); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	unknownFile := filepath.Join(workspace.layout.BackupDir, "notes.txt")
	writeTestFile(t, unknownFile, []byte("keep"), 0600)
	unknownDirectory := filepath.Join(workspace.layout.BackupDir, "config-not-a-uuid.json")
	if err := os.Mkdir(unknownDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := guard.RetainBackups(filepath.Base(workspace.layout.BackupPath(ids[0]))); err != nil {
		t.Fatal(err)
	}
	recognized := recognizedBackups(t, workspace.layout.BackupDir)
	if len(recognized) != DefaultBackupRetention+1 {
		t.Fatalf("recognized backups after preservation = %d, want %d", len(recognized), DefaultBackupRetention+1)
	}
	if _, err := os.Stat(workspace.layout.BackupPath(ids[0])); err != nil {
		t.Fatalf("preserved backup missing: %v", err)
	}
	if _, err := os.Stat(unknownFile); err != nil {
		t.Fatalf("unknown file removed: %v", err)
	}
	if _, err := os.Stat(unknownDirectory); err != nil {
		t.Fatalf("unknown directory removed: %v", err)
	}
	if err := guard.RetainBackups(""); err != nil {
		t.Fatal(err)
	}
	if got := len(recognizedBackups(t, workspace.layout.BackupDir)); got != DefaultBackupRetention {
		t.Fatalf("recognized backups after normal retention = %d, want %d", got, DefaultBackupRetention)
	}

	symlinkID := mustTransactionID(t)
	symlinkPath := workspace.layout.BackupPath(symlinkID)
	target := filepath.Join(workspace.layout.BackupDir, "outside")
	writeTestFile(t, target, []byte("outside"), 0600)
	if err := os.Symlink(target, symlinkPath); err != nil {
		t.Fatal(err)
	}
	if err := guard.RetainBackups(""); !errors.Is(err, ErrUnsafeFile) {
		t.Fatalf("recognized backup symlink error %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "outside" {
		t.Fatalf("symlink target changed: %q, err %v", got, err)
	}
}

func recognizedBackups(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	backups := make([]string, 0)
	for _, entry := range entries {
		if _, ok := transactionIDFromBackupName(entry.Name()); ok {
			backups = append(backups, entry.Name())
		}
	}
	return backups
}

func TestReleasedGuardRejectsEveryMutation(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard := acquireTest(t, workspace.layout)
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Destroy()
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	id := mustTransactionID(t)
	journal := mustJournal(t, id, snapshot.Metadata())
	candidate, _ := NewConfigSnapshot([]byte("NEW"), snapshot.Metadata())
	defer candidate.Destroy()

	mutations := []struct {
		name string
		call func() error
	}{
		{name: "write candidate", call: func() error { _, err := guard.WriteCandidate(id, snapshot, []byte("NEW")); return err }},
		{name: "create backup", call: func() error { return guard.CreateBackup(id, snapshot) }},
		{name: "create journal", call: func() error { return guard.CreatePreparedJournal(journal) }},
		{name: "install candidate", call: func() error { return guard.InstallCandidate(id, candidate) }},
		{name: "update journal", call: func() error { return guard.UpdateJournalPhase(PhaseRollbackFailed) }},
		{name: "remove journal", call: func() error { return guard.RemoveJournal() }},
		{name: "remove candidate", call: func() error { return guard.RemoveCandidate(id) }},
		{name: "remove backup", call: func() error { return guard.RemoveBackup(id) }},
		{name: "remove apply", call: func() error { return guard.RemoveApplyTemps(id) }},
		{name: "retain backups", call: func() error { return guard.RetainBackups("") }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			assertIs(t, mutation.call(), ErrLockNotHeld)
		})
	}
}
