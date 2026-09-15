package configtx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreJournalBackupPreservesBackupAndMetadata(t *testing.T) {
	workspace := newTestWorkspace(t, 0640, "OLD")
	guard := acquireTest(t, workspace.layout)
	oldSnapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer oldSnapshot.Destroy()
	transactionID := mustTransactionID(t)
	candidate, err := guard.WriteCandidate(transactionID, oldSnapshot, []byte("NEW"))
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Destroy()
	if err := guard.CreateBackup(transactionID, oldSnapshot); err != nil {
		t.Fatal(err)
	}
	journal, err := NewJournal(transactionID, oldSnapshot.Hash(), candidate.Hash(), 7, true, oldSnapshot.Metadata())
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.InstallCandidate(transactionID, candidate); err != nil {
		t.Fatal(err)
	}
	if err := guard.RestoreJournalBackup(journal); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(workspace.livePath); err != nil || string(got) != "OLD" {
		t.Fatalf("restored live config %q, err=%v", got, err)
	}
	if err := verifyLiveMetadata(workspace.livePath, oldSnapshot.Metadata()); err != nil {
		t.Fatalf("restored metadata: %v", err)
	}
	if got, err := os.ReadFile(workspace.layout.BackupPath(transactionID)); err != nil || string(got) != "OLD" {
		t.Fatalf("backup was not preserved: %q, err=%v", got, err)
	}
	if _, err := os.Stat(workspace.layout.ApplyTempPrefix(transactionID) + "deadbeef"); !os.IsNotExist(err) {
		t.Fatalf("unexpected apply temporary: %v", err)
	}
}

func TestRestoreJournalBackupFailsClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, backupPath, livePath string)
		want   error
	}{
		{
			name: "wrong hash",
			mutate: func(t *testing.T, backupPath, _ string) {
				if err := os.WriteFile(backupPath, []byte("WRONG"), 0600); err != nil {
					t.Fatal(err)
				}
			},
			want: ErrUnsafeFile,
		},
		{
			name: "symlink",
			mutate: func(t *testing.T, backupPath, livePath string) {
				if err := os.Remove(backupPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(livePath, backupPath); err != nil {
					t.Fatal(err)
				}
			},
			want: ErrUnsafeFile,
		},
		{
			name: "missing",
			mutate: func(t *testing.T, backupPath, _ string) {
				if err := os.Remove(backupPath); err != nil {
					t.Fatal(err)
				}
			},
			want: ErrArtifactNotFound,
		},
		{
			name: "wrong permissions",
			mutate: func(t *testing.T, backupPath, _ string) {
				if err := os.Chmod(backupPath, 0640); err != nil {
					t.Fatal(err)
				}
			},
			want: ErrUnsafePermissions,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			workspace := newTestWorkspace(t, 0600, "OLD")
			guard := acquireTest(t, workspace.layout)
			oldSnapshot, err := guard.SnapshotLiveConfig()
			if err != nil {
				t.Fatal(err)
			}
			defer oldSnapshot.Destroy()
			transactionID := mustTransactionID(t)
			candidate, err := guard.WriteCandidate(transactionID, oldSnapshot, []byte("NEW"))
			if err != nil {
				t.Fatal(err)
			}
			defer candidate.Destroy()
			if err := guard.CreateBackup(transactionID, oldSnapshot); err != nil {
				t.Fatal(err)
			}
			journal, err := NewJournal(transactionID, oldSnapshot.Hash(), candidate.Hash(), 7, true, oldSnapshot.Metadata())
			if err != nil {
				t.Fatal(err)
			}
			if err := guard.InstallCandidate(transactionID, candidate); err != nil {
				t.Fatal(err)
			}
			backupPath := workspace.layout.BackupPath(transactionID)
			testCase.mutate(t, backupPath, workspace.livePath)
			if err := guard.RestoreJournalBackup(journal); !errors.Is(err, testCase.want) {
				t.Fatalf("restore error %v, want %v", err, testCase.want)
			}
			if got, readErr := os.ReadFile(workspace.livePath); readErr != nil || string(got) != "NEW" {
				t.Fatalf("failed restore changed live config: %q, err=%v", got, readErr)
			}
		})
	}
}

func TestRestoreJournalBackupRejectsUnsafeJournalNames(t *testing.T) {
	workspace := newTestWorkspace(t, 0600, "OLD")
	guard := acquireTest(t, workspace.layout)
	snapshot, err := guard.SnapshotLiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Destroy()
	journal, err := NewJournal(mustTransactionID(t), snapshot.Hash(), snapshot.Hash(), 0, false, snapshot.Metadata())
	if err != nil {
		t.Fatal(err)
	}
	journal.BackupName = filepath.Base(workspace.layout.BackupPath(journal.TransactionID)) + "/escape"
	if err := guard.RestoreJournalBackup(journal); !errors.Is(err, ErrInvalidJournal) {
		t.Fatalf("unsafe journal name error %v", err)
	}
}
