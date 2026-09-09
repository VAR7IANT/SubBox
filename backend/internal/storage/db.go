package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const (
	DefaultStateDir   = "/etc/subbox"
	DatabaseFilename  = "subbox.db"
	MasterKeyFilename = "master.key"
)

var ErrDatabaseInvalid = errors.New("subbox database is invalid")

func openDatabase(ctx context.Context, stateDir string) (*sql.DB, error) {
	databasePath := filepath.Join(stateDir, DatabaseFilename)
	if info, err := os.Lstat(databasePath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("database is not a regular file: %w", ErrDatabaseInvalid)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect database: %w", err)
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	closeWithError := func(openErr error) (*sql.DB, error) {
		_ = db.Close()
		return nil, openErr
	}

	if err := db.PingContext(ctx); err != nil {
		return closeWithError(fmt.Errorf("ping database: %w", err))
	}
	if err := secureDatabaseFile(databasePath); err != nil {
		return closeWithError(err)
	}

	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = FULL",
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return closeWithError(fmt.Errorf("configure database: %w", err))
		}
	}

	return db, nil
}

func ensureStateDir(stateDir string) error {
	if filepath.Clean(stateDir) == "." && stateDir == "" {
		return errors.New("state directory must not be empty")
	}
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	info, err := os.Lstat(stateDir)
	if err != nil {
		return fmt.Errorf("inspect state directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("state directory is not a regular directory")
	}
	if err := os.Chmod(stateDir, 0700); err != nil {
		return fmt.Errorf("set state directory permissions: %w", err)
	}
	return nil
}

func databaseExists(databasePath string) (bool, error) {
	info, err := os.Lstat(databasePath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect database: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return true, fmt.Errorf("database is not a regular file: %w", ErrDatabaseInvalid)
	}
	return true, nil
}

func secureDatabaseFile(databasePath string) error {
	info, err := os.Lstat(databasePath)
	if err != nil {
		return fmt.Errorf("inspect database after open: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("database is not a regular file: %w", ErrDatabaseInvalid)
	}
	if err := os.Chmod(databasePath, 0600); err != nil {
		return fmt.Errorf("set database permissions: %w", err)
	}
	return nil
}
