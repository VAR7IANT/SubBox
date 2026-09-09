package storage

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestOpenInitializesSecurePersistentState(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	store, err := Open(stateDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	assertPermissions(t, stateDir, 0700)
	assertPermissions(t, filepath.Join(stateDir, DatabaseFilename), 0600)
	assertPermissions(t, filepath.Join(stateDir, MasterKeyFilename), 0600)

	if got := len(store.MasterKey()); got != masterKeySize {
		t.Fatalf("master key length = %d, want %d", got, masterKeySize)
	}

	for _, table := range []string{
		"schema_migrations",
		"nodes",
		"subscriptions",
		"settings",
		"runtime_state",
		"admin_user",
		"sessions",
	} {
		var found string
		err := store.DB().QueryRow(
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`,
			table,
		).Scan(&found)
		if err != nil {
			t.Fatalf("table %q missing: %v", table, err)
		}
	}

	for _, table := range []string{"auth", "session"} {
		var count int
		if err := store.DB().QueryRow(
			`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND lower(name) = ?`,
			table,
		).Scan(&count); err != nil {
			t.Fatalf("check deferred table %q: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("deferred table %q exists", table)
		}
	}

	var migrationCount int
	if err := store.DB().QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 2 {
		t.Fatalf("migration count = %d, want 2", migrationCount)
	}

	var minimumVersion, maximumVersion, versionCount int
	if err := store.DB().QueryRow(`SELECT min(version), max(version), count(*) FROM schema_migrations`).Scan(&minimumVersion, &maximumVersion, &versionCount); err != nil {
		t.Fatalf("read migration versions: %v", err)
	}
	if minimumVersion != 1 || maximumVersion != 2 || versionCount != 2 {
		t.Fatalf("migration versions = min %d, max %d, count %d; want 1,2", minimumVersion, maximumVersion, versionCount)
	}

	var foreignKeys int
	if err := store.DB().QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
}

func TestOpenIsIdempotentAndPreservesMasterKey(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	first, err := Open(stateDir)
	if err != nil {
		t.Fatalf("first Open() error = %v", err)
	}
	firstKey := first.MasterKey()
	if err := first.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	second, err := Open(stateDir)
	if err != nil {
		t.Fatalf("second Open() error = %v", err)
	}
	defer second.Close()
	if got := second.MasterKey(); string(got) != string(firstKey) {
		t.Fatal("second initialization regenerated the master key")
	}

	var migrationCount int
	if err := second.DB().QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 2 {
		t.Fatalf("migration count after repeat = %d, want 2", migrationCount)
	}
}

func TestNodePortAndProtocolConstraints(t *testing.T) {
	store := openTestStore(t)

	protocols := []string{"shadowsocks", "hysteria2", "tuic", "vless_reality", "anytls"}
	for index, protocol := range protocols {
		insertNode(t, store.DB(), nodeValues{
			ID:         "node-" + protocol,
			Protocol:   protocol,
			ListenPort: 1 + index,
			PublicPort: 1 + index,
		})
	}

	for _, test := range []struct {
		name        string
		listenPort  int
		publicPort  int
		protocol    string
		wantFailure bool
	}{
		{name: "listen port zero", listenPort: 0, publicPort: 10, protocol: "shadowsocks", wantFailure: true},
		{name: "listen port too high", listenPort: 65536, publicPort: 10, protocol: "shadowsocks", wantFailure: true},
		{name: "public port zero", listenPort: 10, publicPort: 0, protocol: "shadowsocks", wantFailure: true},
		{name: "public port too high", listenPort: 10, publicPort: 65536, protocol: "shadowsocks", wantFailure: true},
		{name: "unknown protocol", listenPort: 10, publicPort: 10, protocol: "unknown", wantFailure: true},
		{name: "minimum valid ports", listenPort: 1, publicPort: 1, protocol: "shadowsocks"},
		{name: "maximum valid ports", listenPort: 65535, publicPort: 65535, protocol: "hysteria2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := insertNode(t, store.DB(), nodeValues{
				ID:         "constraint-" + test.name,
				Protocol:   test.protocol,
				ListenPort: test.listenPort,
				PublicPort: test.publicPort,
			})
			if test.wantFailure && err == nil {
				t.Fatal("invalid node was accepted")
			}
			if !test.wantFailure && err != nil {
				t.Fatalf("valid node was rejected: %v", err)
			}
		})
	}
}

func TestMasterKeyMissingWithExistingDatabaseFailsClosed(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	store, err := Open(stateDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.Remove(filepath.Join(stateDir, MasterKeyFilename)); err != nil {
		t.Fatalf("remove master key: %v", err)
	}

	_, err = Open(stateDir)
	if !errors.Is(err, ErrMasterKeyMissing) {
		t.Fatalf("Open() error = %v, want ErrMasterKeyMissing", err)
	}
}

func TestCorruptMasterKeyFailsWithoutReplacement(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	store, err := Open(stateDir)
	if err != nil {
		t.Fatalf("initial Open() error = %v", err)
	}
	originalKey := store.MasterKey()
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	keyPath := filepath.Join(stateDir, MasterKeyFilename)
	if err := os.WriteFile(keyPath, []byte("too short"), 0600); err != nil {
		t.Fatalf("write corrupt key: %v", err)
	}
	_, err = Open(stateDir)
	if !errors.Is(err, ErrMasterKeyInvalid) {
		t.Fatalf("wrong-length key error = %v, want ErrMasterKeyInvalid", err)
	}
	contents, readErr := os.ReadFile(keyPath)
	if readErr != nil {
		t.Fatalf("read corrupt key: %v", readErr)
	}
	if string(contents) == string(originalKey) {
		t.Fatal("corrupt key was silently replaced")
	}
}

func TestSymlinkAndBroadMasterKeyFailClosed(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(t *testing.T, path string)
		want  error
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, path string) {
				target := filepath.Join(t.TempDir(), "target-key")
				if err := os.WriteFile(target, make([]byte, masterKeySize), 0600); err != nil {
					t.Fatalf("write target key: %v", err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatalf("remove key: %v", err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatalf("create key symlink: %v", err)
				}
			},
			want: ErrMasterKeyInvalid,
		},
		{
			name: "broad permissions",
			setup: func(t *testing.T, path string) {
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatalf("chmod key: %v", err)
				}
			},
			want: ErrMasterKeyInsecure,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateDir := filepath.Join(t.TempDir(), "state")
			store, err := Open(stateDir)
			if err != nil {
				t.Fatalf("initial Open() error = %v", err)
			}
			if err := store.Close(); err != nil {
				t.Fatalf("initial Close() error = %v", err)
			}
			test.setup(t, filepath.Join(stateDir, MasterKeyFilename))

			_, err = Open(stateDir)
			if !errors.Is(err, test.want) {
				t.Fatalf("Open() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestSecretColumnsAreEncryptedStorageBoundaries(t *testing.T) {
	store := openTestStore(t)
	for _, table := range []string{"nodes", "subscriptions"} {
		rows, err := store.DB().Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatalf("table_info(%s): %v", table, err)
		}
		for rows.Next() {
			var cid int
			var name, columnType string
			var notNull, primaryKey int
			var defaultValue sql.NullString
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				t.Fatalf("scan table_info(%s): %v", table, err)
			}
			switch name {
			case "password", "private_key", "reality_private_key", "token", "plaintext_token", "subscription_url_with_token":
				t.Fatalf("plaintext secret column %q exists in %s", name, table)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("iterate table_info(%s): %v", table, err)
		}
		rows.Close()
	}
}

func TestFailedMigrationIsNotRecorded(t *testing.T) {
	db := openSQLiteTestDB(t)
	broken := fstest.MapFS{
		"migrations/001_broken.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE should_rollback (id INTEGER); THIS IS NOT SQL;"),
		},
	}

	if err := runMigrationsFromFS(context.Background(), db, broken); err == nil {
		t.Fatal("broken migration unexpectedly succeeded")
	}
	var migrationCount int
	if err := db.QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&migrationCount); err != nil {
		t.Fatalf("count failed migrations: %v", err)
	}
	if migrationCount != 0 {
		t.Fatalf("failed migration count = %d, want 0", migrationCount)
	}
	var tableCount int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name = 'should_rollback'`).Scan(&tableCount); err != nil {
		t.Fatalf("check rolled back table: %v", err)
	}
	if tableCount != 0 {
		t.Fatal("failed migration left a table behind")
	}
}

type nodeValues struct {
	ID         string
	Protocol   string
	ListenPort int
	PublicPort int
}

func insertNode(t *testing.T, db *sql.DB, values nodeValues) error {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO nodes (
			id, name, protocol, host, listen_port, public_port, enabled,
			protocol_config
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, values.ID, values.ID, values.Protocol, "example.com", values.ListenPort, values.PublicPort, 1, "{}")
	return err
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	stateDir := filepath.Join(t.TempDir(), "state")
	store, err := Open(stateDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return store
}

func openSQLiteTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return db
}

func assertPermissions(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("permissions for %s = %o, want %o", path, got, want)
	}
}
