package storage

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const masterKeySize = 32

var (
	// ErrMasterKeyMissing means an existing database has no recoverable key.
	ErrMasterKeyMissing = errors.New("subbox master key is missing")
	// ErrMasterKeyInvalid means the key is not a valid SubBox master key file.
	ErrMasterKeyInvalid = errors.New("subbox master key is invalid")
	// ErrMasterKeyInsecure means the key can be read by group or other users.
	ErrMasterKeyInsecure = errors.New("subbox master key permissions are too broad")
)

func masterKeyPath(stateDir string) string {
	return filepath.Join(stateDir, MasterKeyFilename)
}

func provisionMasterKey(stateDir string, databaseExists bool) ([]byte, error) {
	path := masterKeyPath(stateDir)
	info, err := os.Lstat(path)
	if err == nil {
		return readMasterKey(path, info)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect master key: %w", err)
	}
	if databaseExists {
		return nil, ErrMasterKeyMissing
	}

	key := make([]byte, masterKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			info, statErr := os.Lstat(path)
			if statErr != nil {
				return nil, fmt.Errorf("inspect concurrently created master key: %w", statErr)
			}
			return readMasterKey(path, info)
		}
		return nil, fmt.Errorf("create master key: %w", err)
	}

	keepFile := false
	defer func() {
		if !keepFile {
			_ = file.Close()
			_ = os.Remove(path)
		}
	}()

	if err := file.Chmod(0600); err != nil {
		return nil, fmt.Errorf("set master key permissions: %w", err)
	}
	if err := writeAll(file, key); err != nil {
		return nil, fmt.Errorf("write master key: %w", err)
	}
	if err := file.Sync(); err != nil {
		return nil, fmt.Errorf("sync master key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close master key: %w", err)
	}
	keepFile = true

	return key, nil
}

func readMasterKey(path string, info os.FileInfo) ([]byte, error) {
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("master key is not a regular file: %w", ErrMasterKeyInvalid)
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, ErrMasterKeyInsecure
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open master key: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat master key: %w", err)
	}
	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("master key is not a regular file: %w", ErrMasterKeyInvalid)
	}
	if fileInfo.Mode().Perm()&0077 != 0 {
		return nil, ErrMasterKeyInsecure
	}

	contents, err := io.ReadAll(io.LimitReader(file, masterKeySize+1))
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	if len(contents) != masterKeySize {
		return nil, fmt.Errorf("master key must be exactly %d bytes: %w", masterKeySize, ErrMasterKeyInvalid)
	}

	return contents, nil
}

func writeAll(file *os.File, contents []byte) error {
	for len(contents) > 0 {
		written, err := file.Write(contents)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		contents = contents[written:]
	}
	return nil
}
