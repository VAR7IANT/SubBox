package configtx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func (g *LockGuard) RemoveCandidate(transactionID string) error {
	done, err := g.beginOperation()
	if err != nil {
		return err
	}
	defer done()
	if _, err := canonicalTransactionID(transactionID); err != nil {
		return err
	}
	if err := ensureLiveDirectory(g.layout); err != nil {
		return err
	}
	return removeDerived(g.layout.CandidatePath(transactionID))
}

func (g *LockGuard) RemoveBackup(transactionID string) error {
	done, err := g.beginOperation()
	if err != nil {
		return err
	}
	defer done()
	if _, err := canonicalTransactionID(transactionID); err != nil {
		return err
	}
	if err := ensureTransactionDirectories(g.layout); err != nil {
		return err
	}
	return removeDerived(g.layout.BackupPath(transactionID))
}

// RemoveApplyTemps removes only apply temporaries for the supplied trusted
// transaction ID. It never accepts or follows an arbitrary filesystem path.
func (g *LockGuard) RemoveApplyTemps(transactionID string) error {
	done, err := g.beginOperation()
	if err != nil {
		return err
	}
	defer done()
	if _, err := canonicalTransactionID(transactionID); err != nil {
		return err
	}
	if err := ensureLiveDirectory(g.layout); err != nil {
		return err
	}

	directory, err := openTrustedDirectory(filepath.Dir(g.layout.LiveConfigPath))
	if err != nil {
		return err
	}
	entries, readErr := directory.Readdir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return fmt.Errorf("list apply temporaries: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close live config directory: %w", closeErr)
	}

	removed := false
	prefix := ".subbox-apply-" + transactionID + "-"
	for _, entry := range entries {
		entryBase := entry.Name()
		if !matchesApplyPrefix(entryBase, transactionID) {
			continue
		}
		path := filepath.Join(filepath.Dir(g.layout.LiveConfigPath), entryBase)
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect apply temporary: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return ErrUnsafeFile
		}
		if !strings.HasPrefix(entryBase, prefix) {
			continue
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove apply temporary: %w", err)
		}
		removed = true
	}
	if removed {
		return fsyncDirectory(filepath.Dir(g.layout.LiveConfigPath))
	}
	return nil
}

// RetainBackups keeps at most DefaultBackupRetention recognized regular
// backups, while always preserving preserveBackupName when it is valid. Files
// outside the exact config-<uuid>.json pattern are untouched.
func (g *LockGuard) RetainBackups(preserveBackupName string) error {
	done, err := g.beginOperation()
	if err != nil {
		return err
	}
	defer done()
	if err := ensureTransactionDirectories(g.layout); err != nil {
		return err
	}
	if preserveBackupName != "" {
		if _, ok := transactionIDFromBackupName(preserveBackupName); !ok {
			return ErrUnsafePath
		}
	}

	directory, err := openTrustedDirectory(g.layout.BackupDir)
	if err != nil {
		return err
	}
	entries, readErr := directory.Readdir(-1)
	closeErr := directory.Close()
	if readErr != nil {
		return fmt.Errorf("list config backups: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close backup directory: %w", closeErr)
	}

	backups := make([]backupEntry, 0, len(entries))
	for _, entry := range entries {
		_, ok := transactionIDFromBackupName(entry.Name())
		if !ok {
			continue
		}
		backupBase := entry.Name()
		path := filepath.Join(g.layout.BackupDir, backupBase)
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect config backup: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return ErrUnsafeFile
		}
		if info.Mode().Perm() != 0600 {
			return ErrUnsafePermissions
		}
		backups = append(backups, backupEntry{
			base:       backupBase,
			modifiedAt: info.ModTime(),
		})
	}

	protected := preserveBackupName != ""
	deletableCount := len(backups)
	if protected {
		for _, backup := range backups {
			if backup.base == preserveBackupName {
				deletableCount--
				break
			}
		}
	}
	if deletableCount <= DefaultBackupRetention {
		return nil
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].modifiedAt.Equal(backups[j].modifiedAt) {
			return backups[i].base < backups[j].base
		}
		return backups[i].modifiedAt.Before(backups[j].modifiedAt)
	})

	removed := false
	for _, backup := range backups {
		if deletableCount <= DefaultBackupRetention {
			break
		}
		if backup.base == preserveBackupName {
			continue
		}
		path := filepath.Join(g.layout.BackupDir, backup.base)
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove retained config backup: %w", err)
		}
		deletableCount--
		removed = true
	}
	if removed {
		return fsyncDirectory(g.layout.BackupDir)
	}
	return nil
}

// EnforceBackupRetention is a descriptive alias for RetainBackups.
func (g *LockGuard) EnforceBackupRetention(preserveBackupName string) error {
	return g.RetainBackups(preserveBackupName)
}

func (g *LockGuard) CleanupCandidate(transactionID string) error {
	return g.RemoveCandidate(transactionID)
}

func (g *LockGuard) CleanupBackup(transactionID string) error {
	return g.RemoveBackup(transactionID)
}

func (g *LockGuard) CleanupApplyTemps(transactionID string) error {
	return g.RemoveApplyTemps(transactionID)
}

type backupEntry struct {
	base       string
	modifiedAt time.Time
}

func transactionIDFromBackupName(name string) (string, bool) {
	const prefix = "config-"
	const suffix = ".json"
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
	if _, err := canonicalTransactionID(id); err != nil {
		return "", false
	}
	derived, err := derivedBackupName(id)
	if err != nil || derived != name {
		return "", false
	}
	return id, true
}
