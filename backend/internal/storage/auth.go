package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const (
	HashSize            = 32
	MaxSessionCleanup   = 100
	adminSingletonValue = 1
)

var (
	ErrAdminNotProvisioned = errors.New("administrator is not provisioned")
	ErrStaleAdministrator  = errors.New("administrator authentication state is stale")
	ErrSessionNotFound     = errors.New("session not found")
	ErrSessionInactive     = errors.New("session is inactive")
	ErrInvalidHash         = errors.New("hash must be exactly 32 bytes")
)

type AdminUser struct {
	ID                string
	PasswordVerifier  string
	PasswordChangedAt time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type Session struct {
	IDHash         []byte
	CSRFSecretHash []byte
	CreatedAt      time.Time
	LastSeenAt     time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
}

// SetAdminPassword creates the singleton administrator on first use and
// updates that same administrator thereafter. Password changes and revocation
// of every existing session are committed as one SQLite transaction.
func (s *Store) SetAdminPassword(ctx context.Context, verifier string, now time.Time) (*AdminUser, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("storage is not open")
	}
	if verifier == "" {
		return nil, errors.New("password verifier must not be empty")
	}
	now = now.UTC()
	nowText := formatTimestamp(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin password update: %w", err)
	}
	inTransaction := true
	defer func() {
		if inTransaction {
			_ = tx.Rollback()
		}
	}()

	var admin AdminUser
	var createdText, changedText, updatedText string
	err = tx.QueryRowContext(ctx, `
		SELECT id, password_verifier, password_changed_at, created_at, updated_at
		FROM admin_user
		WHERE singleton = ?
	`, adminSingletonValue).Scan(
		&admin.ID,
		&admin.PasswordVerifier,
		&changedText,
		&createdText,
		&updatedText,
	)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		admin.ID, err = newUUID()
		if err != nil {
			return nil, fmt.Errorf("generate administrator id: %w", err)
		}
		admin.PasswordVerifier = verifier
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO admin_user (
				singleton, id, password_verifier, password_changed_at,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?)
		`, adminSingletonValue, admin.ID, verifier, nowText, nowText, nowText); err != nil {
			return nil, fmt.Errorf("create administrator: %w", err)
		}
		createdText, changedText, updatedText = nowText, nowText, nowText
	case err != nil:
		return nil, fmt.Errorf("read administrator: %w", err)
	default:
		if _, err := tx.ExecContext(ctx, `
			UPDATE admin_user
			SET password_verifier = ?, password_changed_at = ?, updated_at = ?
			WHERE singleton = ?
		`, verifier, nowText, nowText, adminSingletonValue); err != nil {
			return nil, fmt.Errorf("update administrator password: %w", err)
		}
		admin.PasswordVerifier = verifier
		changedText, updatedText = nowText, nowText
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions
		SET revoked_at = ?
		WHERE revoked_at IS NULL
	`, nowText); err != nil {
		return nil, fmt.Errorf("revoke sessions after password change: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit password update: %w", err)
	}
	inTransaction = false

	admin.CreatedAt, err = parseTimestamp(createdText)
	if err != nil {
		return nil, fmt.Errorf("parse administrator created_at: %w", err)
	}
	admin.PasswordChangedAt, err = parseTimestamp(changedText)
	if err != nil {
		return nil, fmt.Errorf("parse administrator password_changed_at: %w", err)
	}
	admin.UpdatedAt, err = parseTimestamp(updatedText)
	if err != nil {
		return nil, fmt.Errorf("parse administrator updated_at: %w", err)
	}
	return &admin, nil
}

func (s *Store) GetAdmin(ctx context.Context) (*AdminUser, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("storage is not open")
	}

	var admin AdminUser
	var changedText, createdText, updatedText string
	err := s.db.QueryRowContext(ctx, `
		SELECT id, password_verifier, password_changed_at, created_at, updated_at
		FROM admin_user
		WHERE singleton = ?
	`, adminSingletonValue).Scan(
		&admin.ID,
		&admin.PasswordVerifier,
		&changedText,
		&createdText,
		&updatedText,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAdminNotProvisioned
	}
	if err != nil {
		return nil, fmt.Errorf("read administrator: %w", err)
	}
	var parseErr error
	if admin.PasswordChangedAt, parseErr = parseTimestamp(changedText); parseErr != nil {
		return nil, fmt.Errorf("parse administrator password_changed_at: %w", parseErr)
	}
	if admin.CreatedAt, parseErr = parseTimestamp(createdText); parseErr != nil {
		return nil, fmt.Errorf("parse administrator created_at: %w", parseErr)
	}
	if admin.UpdatedAt, parseErr = parseTimestamp(updatedText); parseErr != nil {
		return nil, fmt.Errorf("parse administrator updated_at: %w", parseErr)
	}
	return &admin, nil
}

func (s *Store) CreateSession(ctx context.Context, idHash, csrfSecretHash []byte, createdAt, expiresAt time.Time) error {
	if err := validateHash(idHash); err != nil {
		return fmt.Errorf("session id hash: %w", err)
	}
	if err := validateHash(csrfSecretHash); err != nil {
		return fmt.Errorf("csrf secret hash: %w", err)
	}
	if !expiresAt.After(createdAt) {
		return errors.New("session expiration must be after creation")
	}

	tx, err := s.beginTransaction(ctx, "create session")
	if err != nil {
		return err
	}
	inTransaction := true
	defer func() {
		if inTransaction {
			_ = tx.Rollback()
		}
	}()
	if _, err := cleanupSessionsTx(ctx, tx, createdAt.UTC(), MaxSessionCleanup); err != nil {
		return fmt.Errorf("cleanup sessions: %w", err)
	}
	if err := insertSession(ctx, tx, idHash, csrfSecretHash, createdAt, expiresAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session creation: %w", err)
	}
	inTransaction = false
	return nil
}

// RotateSession checks that the administrator verifier observed during
// credential verification is still current, then revokes the supplied
// previous session and creates a fresh session in one transaction.
func (s *Store) RotateSession(ctx context.Context, expectedAdminVerifier string, previousIDHash, idHash, csrfSecretHash []byte, createdAt, expiresAt time.Time) error {
	if expectedAdminVerifier == "" {
		return errors.New("expected administrator verifier must not be empty")
	}
	if err := validateHash(idHash); err != nil {
		return fmt.Errorf("session id hash: %w", err)
	}
	if err := validateHash(csrfSecretHash); err != nil {
		return fmt.Errorf("csrf secret hash: %w", err)
	}
	if len(previousIDHash) != 0 {
		if err := validateHash(previousIDHash); err != nil {
			return fmt.Errorf("previous session id hash: %w", err)
		}
	}
	if !expiresAt.After(createdAt) {
		return errors.New("session expiration must be after creation")
	}

	tx, err := s.beginTransaction(ctx, "rotate session")
	if err != nil {
		return err
	}
	inTransaction := true
	defer func() {
		if inTransaction {
			_ = tx.Rollback()
		}
	}()

	var currentAdminVerifier string
	err = tx.QueryRowContext(ctx, `
		SELECT password_verifier
		FROM admin_user
		WHERE singleton = ?
	`, adminSingletonValue).Scan(&currentAdminVerifier)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrStaleAdministrator
	case err != nil:
		return fmt.Errorf("check current administrator: %w", err)
	}
	if currentAdminVerifier != expectedAdminVerifier {
		return ErrStaleAdministrator
	}

	if _, err := cleanupSessionsTx(ctx, tx, createdAt.UTC(), MaxSessionCleanup); err != nil {
		return fmt.Errorf("cleanup sessions: %w", err)
	}
	nowText := formatTimestamp(createdAt)
	if len(previousIDHash) != 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE sessions SET revoked_at = ?
			WHERE id_hash = ? AND revoked_at IS NULL
		`, nowText, previousIDHash); err != nil {
			return fmt.Errorf("revoke previous session: %w", err)
		}
	}
	if err := insertSession(ctx, tx, idHash, csrfSecretHash, createdAt, expiresAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session rotation: %w", err)
	}
	inTransaction = false
	return nil
}

func (s *Store) GetSessionByHash(ctx context.Context, idHash []byte) (*Session, error) {
	if err := validateHash(idHash); err != nil {
		return nil, err
	}
	if s == nil || s.db == nil {
		return nil, errors.New("storage is not open")
	}

	var session Session
	var createdText, lastSeenText, expiresText string
	var revokedText sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id_hash, csrf_secret_hash, created_at, last_seen_at, expires_at, revoked_at
		FROM sessions
		WHERE id_hash = ?
	`, idHash).Scan(
		&session.IDHash,
		&session.CSRFSecretHash,
		&createdText,
		&lastSeenText,
		&expiresText,
		&revokedText,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}
	var parseErr error
	if session.CreatedAt, parseErr = parseTimestamp(createdText); parseErr != nil {
		return nil, fmt.Errorf("parse session created_at: %w", parseErr)
	}
	if session.LastSeenAt, parseErr = parseTimestamp(lastSeenText); parseErr != nil {
		return nil, fmt.Errorf("parse session last_seen_at: %w", parseErr)
	}
	if session.ExpiresAt, parseErr = parseTimestamp(expiresText); parseErr != nil {
		return nil, fmt.Errorf("parse session expires_at: %w", parseErr)
	}
	if revokedText.Valid {
		revokedAt, parseErr := parseTimestamp(revokedText.String)
		if parseErr != nil {
			return nil, fmt.Errorf("parse session revoked_at: %w", parseErr)
		}
		session.RevokedAt = &revokedAt
	}
	session.IDHash = cloneBytes(session.IDHash)
	session.CSRFSecretHash = cloneBytes(session.CSRFSecretHash)
	return &session, nil
}

func (s *Store) TouchSession(ctx context.Context, idHash []byte, now, expiresAt time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("storage is not open")
	}
	if err := validateHash(idHash); err != nil {
		return err
	}
	if !expiresAt.After(now) {
		return errors.New("session expiration must be after touch time")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET last_seen_at = ?, expires_at = ?
		WHERE id_hash = ?
		  AND revoked_at IS NULL
		  AND julianday(expires_at) > julianday(?)
	`, formatTimestamp(now), formatTimestamp(expiresAt), idHash, formatTimestamp(now))
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check touched session: %w", err)
	}
	if rows == 0 {
		return ErrSessionInactive
	}
	return nil
}

func (s *Store) RevokeSession(ctx context.Context, idHash []byte, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("storage is not open")
	}
	if err := validateHash(idHash); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ?
		WHERE id_hash = ? AND revoked_at IS NULL
	`, formatTimestamp(now), idHash); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *Store) RevokeAllSessions(ctx context.Context, now time.Time) error {
	if s == nil || s.db == nil {
		return errors.New("storage is not open")
	}
	tx, err := s.beginTransaction(ctx, "revoke sessions")
	if err != nil {
		return err
	}
	inTransaction := true
	defer func() {
		if inTransaction {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ?
		WHERE revoked_at IS NULL
	`, formatTimestamp(now)); err != nil {
		return fmt.Errorf("revoke sessions: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session revocation: %w", err)
	}
	inTransaction = false
	return nil
}

func (s *Store) CleanupSessions(ctx context.Context, now time.Time, limit int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("storage is not open")
	}
	if limit <= 0 {
		return 0, nil
	}
	if limit > MaxSessionCleanup {
		limit = MaxSessionCleanup
	}
	tx, err := s.beginTransaction(ctx, "cleanup sessions")
	if err != nil {
		return 0, err
	}
	inTransaction := true
	defer func() {
		if inTransaction {
			_ = tx.Rollback()
		}
	}()
	count, err := cleanupSessionsTx(ctx, tx, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit session cleanup: %w", err)
	}
	inTransaction = false
	return count, nil
}

func (s *Store) beginTransaction(ctx context.Context, operation string) (*sql.Tx, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("storage is not open")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin %s: %w", operation, err)
	}
	return tx, nil
}

func insertSession(ctx context.Context, tx *sql.Tx, idHash, csrfSecretHash []byte, createdAt, expiresAt time.Time) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (
			id_hash, csrf_secret_hash, created_at, last_seen_at, expires_at
		) VALUES (?, ?, ?, ?, ?)
	`, idHash, csrfSecretHash, formatTimestamp(createdAt), formatTimestamp(createdAt), formatTimestamp(expiresAt)); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func cleanupSessionsTx(ctx context.Context, tx *sql.Tx, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		return 0, nil
	}
	if limit > MaxSessionCleanup {
		limit = MaxSessionCleanup
	}
	result, err := tx.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE id_hash IN (
			SELECT id_hash
			FROM sessions
			WHERE revoked_at IS NOT NULL
			   OR julianday(expires_at) <= julianday(?)
			ORDER BY COALESCE(revoked_at, expires_at), id_hash
			LIMIT ?
		)
	`, formatTimestamp(now), limit)
	if err != nil {
		return 0, fmt.Errorf("delete stale sessions: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count deleted sessions: %w", err)
	}
	return count, nil
}

func validateHash(value []byte) error {
	if len(value) != HashSize {
		return ErrInvalidHash
	}
	return nil
}

func cloneBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTimestamp(value string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, errors.New("invalid timestamp")
}

func newUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	// RFC 9562 UUID version 4 and variant bits.
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return hex.EncodeToString(value[0:4]) + "-" +
		hex.EncodeToString(value[4:6]) + "-" +
		hex.EncodeToString(value[6:8]) + "-" +
		hex.EncodeToString(value[8:10]) + "-" +
		hex.EncodeToString(value[10:16]), nil
}
