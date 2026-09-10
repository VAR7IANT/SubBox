package configtx

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// FileMetadata is the intended owner and permission metadata for a managed
// live config. Only the two reviewed live-config modes are accepted.
type FileMetadata struct {
	Mode uint32
	UID  int
	GID  int
}

func (m FileMetadata) validateLive() error {
	if m.Mode != 0600 && m.Mode != 0640 {
		return ErrUnsafePermissions
	}
	if m.UID < 0 || m.GID < 0 {
		return ErrUnsafePermissions
	}
	return nil
}

// ConfigSnapshot is a bounded in-memory snapshot of a managed config. Its
// content is deliberately exposed only through a defensive copy and can be
// cleared as soon as the caller no longer needs it.
type ConfigSnapshot struct {
	content  []byte
	hash     string
	metadata FileMetadata
}

// NewConfigSnapshot creates a validated snapshot from trusted bytes. It is
// useful to future transaction code that has already produced candidate data.
func NewConfigSnapshot(content []byte, metadata FileMetadata) (*ConfigSnapshot, error) {
	if err := metadata.validateLive(); err != nil {
		return nil, err
	}
	if len(content) > MaxConfigBytes {
		return nil, ErrConfigTooLarge
	}
	copyOfContent := append([]byte(nil), content...)
	return &ConfigSnapshot{
		content:  copyOfContent,
		hash:     hashConfig(copyOfContent),
		metadata: metadata,
	}, nil
}

// Content returns a defensive copy of the snapshot bytes.
func (s *ConfigSnapshot) Content() []byte {
	if s == nil {
		return nil
	}
	return append([]byte(nil), s.content...)
}

// Hash returns the lowercase SHA-256 hash of the snapshot bytes.
func (s *ConfigSnapshot) Hash() string {
	if s == nil {
		return ""
	}
	return s.hash
}

// Metadata returns the captured intended file metadata.
func (s *ConfigSnapshot) Metadata() FileMetadata {
	if s == nil {
		return FileMetadata{}
	}
	return s.metadata
}

// Destroy clears the snapshot's retained content. It is idempotent and is a
// best-effort memory hygiene measure; Go compiler copies cannot be guaranteed
// to be erased.
func (s *ConfigSnapshot) Destroy() {
	if s == nil {
		return
	}
	clear(s.content)
	s.content = nil
	s.hash = ""
	s.metadata = FileMetadata{}
}

// Clear is a descriptive alias for Destroy.
func (s *ConfigSnapshot) Clear() {
	s.Destroy()
}

func hashConfig(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func readLiveSnapshot(path string) (*ConfigSnapshot, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		if isSymlinkOpenError(err) {
			return nil, ErrUnsafeFile
		}
		return nil, fmt.Errorf("open live config: %w", err)
	}

	data, metadata, readErr := readBoundedFile(file, MaxConfigBytes, ErrConfigTooLarge)
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close live config: %w", closeErr)
	}
	snapshot, snapshotErr := NewConfigSnapshot(data, metadata)
	clear(data)
	return snapshot, snapshotErr
}

func readBoundedFile(file *os.File, maxBytes int, tooLarge error) ([]byte, FileMetadata, error) {
	metadata, err := statRegularFile(file)
	if err != nil {
		return nil, FileMetadata{}, err
	}
	if int64(maxBytes) < 0 || metadata.size > int64(maxBytes) {
		return nil, FileMetadata{}, tooLarge
	}

	data, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return nil, FileMetadata{}, fmt.Errorf("read bounded config file: %w", err)
	}
	if len(data) > maxBytes {
		return nil, FileMetadata{}, tooLarge
	}
	return data, metadata.FileMetadata, nil
}

type fileStat struct {
	FileMetadata
	size int64
}

func statRegularFile(file *os.File) (fileStat, error) {
	if file == nil {
		return fileStat{}, ErrUnsafeFile
	}
	info, err := file.Stat()
	if err != nil {
		return fileStat{}, fmt.Errorf("stat transaction file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fileStat{}, ErrUnsafeFile
	}
	metadata := FileMetadata{
		Mode: uint32(info.Mode().Perm()),
	}
	if err := metadata.validateLive(); err != nil {
		return fileStat{}, err
	}

	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return fileStat{}, fmt.Errorf("fstat transaction file: %w", err)
	}
	metadata.UID = int(stat.Uid)
	metadata.GID = int(stat.Gid)
	if metadata.UID < 0 || metadata.GID < 0 {
		return fileStat{}, ErrUnsafePermissions
	}
	return fileStat{FileMetadata: metadata, size: info.Size()}, nil
}

func isSymlinkOpenError(err error) bool {
	return err != nil && errors.Is(err, unix.ELOOP)
}

func (g *LockGuard) SnapshotLiveConfig() (*ConfigSnapshot, error) {
	done, err := g.beginOperation()
	if err != nil {
		return nil, err
	}
	defer done()
	if err := ensureLiveDirectory(g.layout); err != nil {
		return nil, err
	}
	return readLiveSnapshot(g.layout.LiveConfigPath)
}

// Snapshot is a descriptive alias used by future Config Manager code.
func (g *LockGuard) Snapshot() (*ConfigSnapshot, error) {
	return g.SnapshotLiveConfig()
}
