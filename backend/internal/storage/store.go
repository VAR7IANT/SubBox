package storage

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
)

type Store struct {
	db        *sql.DB
	stateDir  string
	masterKey []byte
}

func Open(stateDir string) (*Store, error) {
	return OpenContext(context.Background(), stateDir)
}

func OpenDefault() (*Store, error) {
	return Open(DefaultStateDir)
}

func OpenContext(ctx context.Context, stateDir string) (*Store, error) {
	if ctx == nil {
		return nil, fmt.Errorf("open storage: nil context")
	}
	if err := ensureStateDir(stateDir); err != nil {
		return nil, err
	}
	stateDir = filepath.Clean(stateDir)

	databasePath := filepath.Join(stateDir, DatabaseFilename)
	databasePresent, err := databaseExists(databasePath)
	if err != nil {
		return nil, err
	}
	masterKey, err := provisionMasterKey(stateDir, databasePresent)
	if err != nil {
		return nil, err
	}

	db, err := openDatabase(ctx, stateDir)
	if err != nil {
		return nil, err
	}
	if err := runMigrations(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{
		db:        db,
		stateDir:  stateDir,
		masterKey: masterKey,
	}, nil
}

func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) StateDir() string {
	if s == nil {
		return ""
	}
	return s.stateDir
}

func (s *Store) MasterKey() []byte {
	if s == nil {
		return nil
	}
	key := make([]byte, len(s.masterKey))
	copy(key, s.masterKey)
	return key
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
