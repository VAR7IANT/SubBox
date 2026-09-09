package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// migrationFS contains trusted, versioned schema migrations shipped with the
// binary. It is intentionally not configurable from a request or filesystem
// path.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

var migrationFilename = regexp.MustCompile(`^([0-9]+)_[a-z0-9_]+\.sql$`)

type migration struct {
	version int
	name    string
	sql     string
}

func runMigrations(ctx context.Context, db *sql.DB) error {
	return runMigrationsFromFS(ctx, db, migrationFS)
}

func runMigrationsFromFS(ctx context.Context, db *sql.DB, source fs.FS) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY CHECK (version > 0),
			applied_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create schema migrations table: %w", err)
	}

	migrations, err := loadMigrations(source)
	if err != nil {
		return err
	}

	applied, err := appliedMigrationVersions(ctx, db)
	if err != nil {
		return err
	}
	for version := range applied {
		if !containsMigration(migrations, version) {
			return fmt.Errorf("database has unapplied migration version %d", version)
		}
	}

	for _, migration := range migrations {
		if _, ok := applied[migration.version]; ok {
			continue
		}
		if err := applyMigration(ctx, db, migration); err != nil {
			return err
		}
	}

	return nil
}

func loadMigrations(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}

	migrations := make([]migration, 0, len(entries))
	seenVersions := make(map[int]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := migrationFilename.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(matches[1])
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		if _, exists := seenVersions[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d", version)
		}
		contents, err := fs.ReadFile(source, "migrations/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(contents)) == "" {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}

		seenVersions[version] = struct{}{}
		migrations = append(migrations, migration{
			version: version,
			name:    entry.Name(),
			sql:     string(contents),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})
	return migrations, nil
}

func appliedMigrationVersions(ctx context.Context, db *sql.DB) (map[int]struct{}, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]struct{})
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}
	return applied, nil
}

func containsMigration(migrations []migration, version int) bool {
	for _, migration := range migrations {
		if migration.version == version {
			return true
		}
	}
	return false
}

func applyMigration(ctx context.Context, db *sql.DB, migration migration) error {
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection for migration %03d: %w", migration.version, err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin migration %03d: %w", migration.version, err)
	}
	inTransaction := true
	defer func() {
		if inTransaction {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()

	var marker int
	err = conn.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE version = ?`, migration.version).Scan(&marker)
	if err == nil {
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return fmt.Errorf("commit already-applied migration %03d: %w", migration.version, err)
		}
		inTransaction = false
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check migration %03d: %w", migration.version, err)
	}

	if _, err := conn.ExecContext(ctx, migration.sql); err != nil {
		return fmt.Errorf("apply migration %03d (%s): %w", migration.version, migration.name, err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations(version) VALUES (?)`, migration.version); err != nil {
		return fmt.Errorf("record migration %03d: %w", migration.version, err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit migration %03d: %w", migration.version, err)
	}
	inTransaction = false
	return nil
}
