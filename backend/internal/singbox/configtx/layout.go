package configtx

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultStateDir        = "/etc/subbox"
	DefaultLiveConfigPath  = "/etc/sing-box/config.json"
	LockFile               = "config.lock"
	JournalFile            = "config-transaction.json"
	BackupDirectory        = "backups"
	MaxConfigBytes         = 4 * 1024 * 1024
	MaxJournalBytes        = 16 * 1024
	DefaultBackupRetention = 5
)

// Layout contains only trusted, application-derived transaction paths. The
// state and live-config roots are supplied by trusted setup code; callers do
// not supply individual artifact paths.
type Layout struct {
	StateDir       string
	LockPath       string
	JournalPath    string
	BackupDir      string
	LiveConfigPath string
}

// DefaultLayout returns the fixed Phase 1 production layout.
func DefaultLayout() Layout {
	return NewLayout(DefaultStateDir, DefaultLiveConfigPath)
}

// NewLayout derives all transaction paths from the two trusted roots.
func NewLayout(stateDir, liveConfigPath string) Layout {
	stateDir = filepath.Clean(stateDir)
	liveConfigPath = filepath.Clean(liveConfigPath)
	return Layout{
		StateDir:       stateDir,
		LockPath:       filepath.Join(stateDir, LockFile),
		JournalPath:    filepath.Join(stateDir, JournalFile),
		BackupDir:      filepath.Join(stateDir, BackupDirectory),
		LiveConfigPath: liveConfigPath,
	}
}

// CandidatePath derives the current transaction's candidate path. Invalid
// identifiers return an empty path so an invalid identifier can never be
// turned into a filesystem operation.
func (l Layout) CandidatePath(transactionID string) string {
	candidateBase, err := derivedCandidateName(transactionID)
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(l.LiveConfigPath), candidateBase)
}

// BackupPath derives the current transaction's backup path.
func (l Layout) BackupPath(transactionID string) string {
	backupBase, err := derivedBackupName(transactionID)
	if err != nil {
		return ""
	}
	return filepath.Join(l.BackupDir, backupBase)
}

// ApplyTempPrefix derives the only permitted prefix for an apply temporary.
func (l Layout) ApplyTempPrefix(transactionID string) string {
	if _, err := canonicalTransactionID(transactionID); err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(l.LiveConfigPath), ".subbox-apply-"+transactionID+"-")
}

func (l Layout) validate() error {
	if l.StateDir == "" || l.LockPath == "" || l.JournalPath == "" || l.BackupDir == "" || l.LiveConfigPath == "" {
		return ErrUnsafePath
	}
	if !filepath.IsAbs(l.StateDir) || !filepath.IsAbs(l.LiveConfigPath) {
		return ErrUnsafePath
	}
	if filepath.Clean(l.LockPath) != filepath.Join(filepath.Clean(l.StateDir), LockFile) ||
		filepath.Clean(l.JournalPath) != filepath.Join(filepath.Clean(l.StateDir), JournalFile) ||
		filepath.Clean(l.BackupDir) != filepath.Join(filepath.Clean(l.StateDir), BackupDirectory) {
		return ErrUnsafePath
	}
	if filepath.Base(l.LiveConfigPath) != "config.json" {
		return ErrUnsafePath
	}
	return nil
}

func ensureSecureDirectory(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return ErrUnsafePath
	}

	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return fmt.Errorf("create trusted directory: %w", err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("inspect trusted directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafePath
	}
	if !info.IsDir() {
		return ErrUnsafePath
	}
	if info.Mode().Perm() != 0700 {
		return ErrUnsafePermissions
	}
	return nil
}

func ensureTransactionDirectories(l Layout) error {
	if err := l.validate(); err != nil {
		return err
	}
	if err := ensureSecureDirectory(l.StateDir); err != nil {
		return err
	}
	return ensureSecureDirectory(l.BackupDir)
}

func ensureLiveDirectory(l Layout) error {
	if err := l.validate(); err != nil {
		return err
	}
	dir := filepath.Dir(l.LiveConfigPath)
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("inspect live config directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return ErrUnsafePath
	}
	return nil
}
