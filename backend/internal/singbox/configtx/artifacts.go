package configtx

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

func (g *LockGuard) WriteCandidate(transactionID string, oldSnapshot *ConfigSnapshot, content []byte) (*ConfigSnapshot, error) {
	done, err := g.beginOperation()
	if err != nil {
		return nil, err
	}
	defer done()
	if oldSnapshot == nil {
		return nil, ErrUnsafeFile
	}
	if err := oldSnapshot.metadata.validateLive(); err != nil {
		return nil, err
	}
	if _, err := canonicalTransactionID(transactionID); err != nil {
		return nil, err
	}
	if len(content) > MaxConfigBytes {
		return nil, ErrConfigTooLarge
	}
	if err := ensureLiveDirectory(g.layout); err != nil {
		return nil, err
	}

	candidatePath := g.layout.CandidatePath(transactionID)
	if candidatePath == "" {
		return nil, ErrUnsafePath
	}
	candidate, err := NewConfigSnapshot(content, oldSnapshot.metadata)
	if err != nil {
		return nil, err
	}
	if err := createExclusiveArtifact(candidatePath, candidate.content, candidate.metadata, false); err != nil {
		candidate.Destroy()
		return nil, err
	}
	return candidate, nil
}

// CreateCandidate is a descriptive alias for WriteCandidate.
func (g *LockGuard) CreateCandidate(transactionID string, oldSnapshot *ConfigSnapshot, content []byte) (*ConfigSnapshot, error) {
	return g.WriteCandidate(transactionID, oldSnapshot, content)
}

func (g *LockGuard) CreateBackup(transactionID string, oldSnapshot *ConfigSnapshot) error {
	done, err := g.beginOperation()
	if err != nil {
		return err
	}
	defer done()
	if oldSnapshot == nil {
		return ErrUnsafeFile
	}
	if err := oldSnapshot.metadata.validateLive(); err != nil {
		return err
	}
	if _, err := canonicalTransactionID(transactionID); err != nil {
		return err
	}
	if err := ensureTransactionDirectories(g.layout); err != nil {
		return err
	}

	backupPath := g.layout.BackupPath(transactionID)
	if backupPath == "" {
		return ErrUnsafePath
	}
	return createExclusiveArtifact(backupPath, oldSnapshot.content, FileMetadata{
		Mode: 0600,
		UID:  oldSnapshot.metadata.UID,
		GID:  oldSnapshot.metadata.GID,
	}, true)
}

// InstallCandidate copies the trusted candidate into a same-directory apply
// temporary, durably writes it, and atomically replaces the live config. The
// candidate artifact is intentionally retained for journal-driven recovery.
func (g *LockGuard) InstallCandidate(transactionID string, candidate *ConfigSnapshot) error {
	done, err := g.beginOperation()
	if err != nil {
		return err
	}
	defer done()
	if candidate == nil {
		return ErrUnsafeFile
	}
	if err := candidate.metadata.validateLive(); err != nil {
		return err
	}
	if candidate.hash == "" {
		return ErrUnsafeFile
	}
	return g.installCandidate(transactionID, candidate.hash, candidate.metadata)
}

// InstallJournalCandidate installs the candidate identified by a validated
// prepared journal, without retaining candidate plaintext in another object.
func (g *LockGuard) InstallJournalCandidate(journal Journal) error {
	done, err := g.beginOperation()
	if err != nil {
		return err
	}
	defer done()
	if err := journal.Validate(); err != nil {
		return err
	}
	if journal.Phase != PhasePrepared {
		return ErrInvalidJournal
	}
	return g.installCandidate(journal.TransactionID, journal.CandidateConfigHash, FileMetadata{
		Mode: journal.LiveConfigMode,
		UID:  journal.LiveConfigUID,
		GID:  journal.LiveConfigGID,
	})
}

func (g *LockGuard) installCandidate(transactionID, expectedHash string, intended FileMetadata) error {
	if _, err := canonicalTransactionID(transactionID); err != nil {
		return err
	}
	if err := intended.validateLive(); err != nil {
		return err
	}
	if !isLowerHexHash(expectedHash) {
		return ErrInvalidJournal
	}
	if err := ensureLiveDirectory(g.layout); err != nil {
		return err
	}

	candidatePath := g.layout.CandidatePath(transactionID)
	candidateContent, candidateMetadata, err := readExistingArtifact(candidatePath, MaxConfigBytes)
	if err != nil {
		return err
	}
	if hashConfig(candidateContent) != expectedHash || candidateMetadata != intended {
		clear(candidateContent)
		return ErrUnsafeFile
	}

	applyPath, applyFile, err := g.createApplyTemp(transactionID, candidateContent, intended)
	clear(candidateContent)
	if err != nil {
		return err
	}
	removeApply := true
	defer func() {
		if removeApply {
			_ = os.Remove(applyPath)
		}
	}()

	if err := fsyncAndClose(applyFile); err != nil {
		return fmt.Errorf("durably write apply temporary: %w", err)
	}
	if err := fsyncDirectory(filepath.Dir(g.layout.LiveConfigPath)); err != nil {
		return fmt.Errorf("sync live config directory: %w", err)
	}

	// Recheck the target immediately before replacement. A symlink target is
	// never accepted, even though rename itself does not follow it.
	if err := verifyLiveTarget(g.layout.LiveConfigPath); err != nil {
		return err
	}
	if err := os.Rename(applyPath, g.layout.LiveConfigPath); err != nil {
		return fmt.Errorf("%w: %v", ErrAtomicReplace, err)
	}
	removeApply = false
	if err := fsyncDirectory(filepath.Dir(g.layout.LiveConfigPath)); err != nil {
		return fmt.Errorf("%w: sync replaced live config directory: %v", ErrAtomicReplace, err)
	}
	if err := verifyLiveMetadata(g.layout.LiveConfigPath, intended); err != nil {
		return fmt.Errorf("%w: verify replaced live config: %v", ErrAtomicReplace, err)
	}
	return nil
}

func (g *LockGuard) createApplyTemp(transactionID string, content []byte, metadata FileMetadata) (string, *os.File, error) {
	prefix := g.layout.ApplyTempPrefix(transactionID)
	if prefix == "" {
		return "", nil, ErrUnsafePath
	}
	for attempt := 0; attempt < 16; attempt++ {
		randomPart, err := randomSuffix()
		if err != nil {
			return "", nil, fmt.Errorf("generate apply temporary name: %w", err)
		}
		path := prefix + randomPart
		file, err := openNewArtifact(path, metadata.Mode)
		if err == nil {
			if err := applyMetadata(file, metadata); err != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return "", nil, err
			}
			if err := writeAll(file, content); err != nil {
				_ = file.Close()
				_ = os.Remove(path)
				return "", nil, fmt.Errorf("write apply temporary: %w", err)
			}
			return path, file, nil
		}
		if errors.Is(err, ErrArtifactExists) {
			continue
		}
		return "", nil, err
	}
	return "", nil, ErrArtifactExists
}

func createExclusiveArtifact(path string, content []byte, metadata FileMetadata, backup bool) error {
	if len(content) > MaxConfigBytes {
		return ErrConfigTooLarge
	}
	if backup {
		if metadata.Mode != 0600 {
			return ErrUnsafePermissions
		}
	} else if err := metadata.validateLive(); err != nil {
		return err
	}
	if err := checkNewDestination(path); err != nil {
		return err
	}
	file, err := openNewArtifact(path, metadata.Mode)
	if err != nil {
		return err
	}
	created := true
	defer func() {
		if created {
			_ = os.Remove(path)
		}
	}()
	if err := applyMetadata(file, metadata); err != nil {
		_ = file.Close()
		return err
	}
	if err := writeAll(file, content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write transaction artifact: %w", err)
	}
	if err := fsyncAndClose(file); err != nil {
		return fmt.Errorf("sync transaction artifact: %w", err)
	}
	if err := fsyncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync transaction artifact directory: %w", err)
	}
	created = false
	return nil
}

func openNewArtifact(path string, mode uint32) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, os.FileMode(mode))
	if err == nil {
		return file, nil
	}
	if errors.Is(err, os.ErrExist) {
		info, statErr := os.Lstat(path)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrUnsafeFile
		}
		return nil, ErrArtifactExists
	}
	if isSymlinkOpenError(err) {
		return nil, ErrUnsafeFile
	}
	return nil, fmt.Errorf("create transaction artifact: %w", err)
}

func checkNewDestination(path string) error {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafeFile
		}
		return ErrArtifactExists
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("inspect transaction artifact: %w", err)
	}
	return nil
}

func applyMetadata(file *os.File, metadata FileMetadata) error {
	if metadata.UID < 0 || metadata.GID < 0 {
		return ErrUnsafePermissions
	}
	if err := unix.Fchown(int(file.Fd()), metadata.UID, metadata.GID); err != nil {
		return fmt.Errorf("apply transaction artifact owner: %w", err)
	}
	if err := unix.Fchmod(int(file.Fd()), metadata.Mode); err != nil {
		return fmt.Errorf("apply transaction artifact mode: %w", err)
	}
	actual, err := statRegularFile(file)
	if err != nil {
		return err
	}
	if actual.FileMetadata != metadata {
		return ErrUnsafePermissions
	}
	return nil
}

func writeAll(file *os.File, content []byte) error {
	for len(content) > 0 {
		written, err := file.Write(content)
		if err != nil {
			return err
		}
		if written <= 0 {
			return ioErrShortWrite
		}
		content = content[written:]
	}
	return nil
}

var ioErrShortWrite = errors.New("short write")

func fsyncAndClose(file *os.File) error {
	if file == nil {
		return ErrUnsafeFile
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(syncErr, closeErr)
}

func fsyncDirectory(path string) error {
	file, err := openTrustedDirectory(path)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(syncErr, closeErr)
}

func openTrustedDirectory(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect transaction directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, ErrUnsafePath
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open transaction directory: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, ErrUnsafePath
	}
	return file, nil
}

func readExistingArtifact(path string, maxBytes int) ([]byte, FileMetadata, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if isSymlinkOpenError(err) {
			return nil, FileMetadata{}, ErrUnsafeFile
		}
		if os.IsNotExist(err) {
			return nil, FileMetadata{}, ErrArtifactNotFound
		}
		return nil, FileMetadata{}, fmt.Errorf("open transaction artifact: %w", err)
	}
	data, metadata, readErr := readBoundedFile(file, maxBytes, ErrConfigTooLarge)
	closeErr := file.Close()
	if readErr != nil {
		return nil, FileMetadata{}, readErr
	}
	if closeErr != nil {
		return nil, FileMetadata{}, fmt.Errorf("close transaction artifact: %w", closeErr)
	}
	return data, metadata, nil
}

func verifyLiveTarget(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrUnsafeFile
		}
		return fmt.Errorf("inspect live config target: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrUnsafeFile
	}
	return verifyLiveMetadata(path, FileMetadata{Mode: uint32(info.Mode().Perm()), UID: -1, GID: -1})
}

func verifyLiveMetadata(path string, expected FileMetadata) error {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if isSymlinkOpenError(err) {
			return ErrUnsafeFile
		}
		return fmt.Errorf("open live config for verification: %w", err)
	}
	actual, statErr := statRegularFile(file)
	closeErr := file.Close()
	if statErr != nil {
		return statErr
	}
	if closeErr != nil {
		return fmt.Errorf("close live config verification: %w", closeErr)
	}
	if expected.Mode != 0 && actual.Mode != expected.Mode {
		return ErrUnsafePermissions
	}
	if expected.UID >= 0 && actual.UID != expected.UID {
		return ErrUnsafePermissions
	}
	if expected.GID >= 0 && actual.GID != expected.GID {
		return ErrUnsafePermissions
	}
	return nil
}

func randomSuffix() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func removeDerived(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return ErrArtifactNotFound
	}
	if err != nil {
		return fmt.Errorf("inspect transaction artifact for removal: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafeFile
	}
	if !info.Mode().IsRegular() {
		return ErrUnsafeFile
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove transaction artifact: %w", err)
	}
	return fsyncDirectory(filepath.Dir(path))
}

func matchesApplyPrefix(name, transactionID string) bool {
	prefix := ".subbox-apply-" + transactionID + "-"
	return strings.HasPrefix(name, prefix) && len(name) > len(prefix) && isLowerHex(name[len(prefix):])
}

func isLowerHex(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func isLowerHexHash(value string) bool {
	return len(value) == sha256HexLength && isLowerHex(value)
}

const sha256HexLength = 64

func canonicalTransactionID(value string) (string, error) {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != value || parsed.Version() != 4 {
		return "", ErrUnsafePath
	}
	return value, nil
}

func derivedCandidateName(transactionID string) (string, error) {
	canonical, err := canonicalTransactionID(transactionID)
	if err != nil {
		return "", err
	}
	return ".subbox-candidate-" + canonical + ".json", nil
}

func derivedBackupName(transactionID string) (string, error) {
	canonical, err := canonicalTransactionID(transactionID)
	if err != nil {
		return "", err
	}
	return "config-" + canonical + ".json", nil
}

func derivedApplyPrefix(transactionID string) (string, error) {
	canonical, err := canonicalTransactionID(transactionID)
	if err != nil {
		return "", err
	}
	return ".subbox-apply-" + canonical + "-", nil
}
