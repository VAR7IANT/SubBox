package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAdminSingletonConstraint(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if _, err := store.SetAdminPassword(context.Background(), "verifier-one", now); err != nil {
		t.Fatalf("first SetAdminPassword() error = %v", err)
	}
	if _, err := store.SetAdminPassword(context.Background(), "verifier-two", now.Add(time.Minute)); err != nil {
		t.Fatalf("second SetAdminPassword() error = %v", err)
	}
	var count int
	if err := store.DB().QueryRow(`SELECT count(*) FROM admin_user`).Scan(&count); err != nil {
		t.Fatalf("count admin users: %v", err)
	}
	if count != 1 {
		t.Fatalf("admin count = %d, want 1", count)
	}
	if _, err := store.DB().Exec(`
		INSERT INTO admin_user (singleton, id, password_verifier, password_changed_at, created_at, updated_at)
		VALUES (1, 'another-admin', 'verifier', '2026-09-09T12:00:00Z', '2026-09-09T12:00:00Z', '2026-09-09T12:00:00Z')
	`); err == nil {
		t.Fatal("database accepted a second singleton administrator")
	}
}

func TestPasswordUpdateRevokesSessionsInSameStateChange(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	idHash := make([]byte, HashSize)
	idHash[0] = 1
	csrfHash := make([]byte, HashSize)
	csrfHash[0] = 2
	if err := store.CreateSession(context.Background(), idHash, csrfHash, now, now.Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := store.SetAdminPassword(context.Background(), "verifier", now.Add(time.Minute)); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}
	session, err := store.GetSessionByHash(context.Background(), idHash)
	if err != nil {
		t.Fatalf("GetSessionByHash() error = %v", err)
	}
	if session.RevokedAt == nil {
		t.Fatal("password update left an existing session active")
	}
	if _, err := store.GetAdmin(context.Background()); err != nil {
		t.Fatalf("GetAdmin() error = %v", err)
	}
}

func TestRotateSessionRejectsStaleAdministratorVerifier(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if _, err := store.SetAdminPassword(context.Background(), "old-verifier", now); err != nil {
		t.Fatalf("initial SetAdminPassword() error = %v", err)
	}
	if _, err := store.SetAdminPassword(context.Background(), "new-verifier", now.Add(time.Minute)); err != nil {
		t.Fatalf("updated SetAdminPassword() error = %v", err)
	}

	staleID := make([]byte, HashSize)
	staleID[0] = 3
	staleCSRF := make([]byte, HashSize)
	staleCSRF[0] = 4
	if err := store.RotateSession(
		context.Background(),
		"old-verifier",
		nil,
		staleID,
		staleCSRF,
		now.Add(2*time.Minute),
		now.Add(3*time.Hour),
	); !errors.Is(err, ErrStaleAdministrator) {
		t.Fatalf("stale RotateSession() error = %v, want ErrStaleAdministrator", err)
	}
	var sessionCount int
	if err := store.DB().QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions after stale rotation: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("stale rotation created or changed sessions: count = %d", sessionCount)
	}

	currentID := make([]byte, HashSize)
	currentID[0] = 5
	currentCSRF := make([]byte, HashSize)
	currentCSRF[0] = 6
	if err := store.RotateSession(
		context.Background(),
		"new-verifier",
		nil,
		currentID,
		currentCSRF,
		now.Add(3*time.Minute),
		now.Add(4*time.Hour),
	); err != nil {
		t.Fatalf("current RotateSession() error = %v", err)
	}
}

func TestCleanupSessionsIsBoundedAndPreservesActiveSessions(t *testing.T) {
	store := openTestStore(t)
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	for index := 0; index < MaxSessionCleanup+3; index++ {
		idHash := make([]byte, HashSize)
		idHash[0] = byte(index + 1)
		idHash[1] = byte(index >> 8)
		csrfHash := make([]byte, HashSize)
		csrfHash[0] = byte(index + 7)
		if err := store.CreateSession(context.Background(), idHash, csrfHash, now.Add(-time.Hour), now.Add(-time.Minute)); err != nil {
			t.Fatalf("CreateSession(%d) error = %v", index, err)
		}
	}
	activeID := make([]byte, HashSize)
	activeID[0] = 250
	activeCSRF := make([]byte, HashSize)
	activeCSRF[0] = 251
	if err := store.CreateSession(context.Background(), activeID, activeCSRF, now, now.Add(time.Hour)); err != nil {
		t.Fatalf("active CreateSession() error = %v", err)
	}
	if _, err := store.CleanupSessions(context.Background(), now, MaxSessionCleanup); err != nil {
		t.Fatalf("CleanupSessions() error = %v", err)
	}
	if _, err := store.GetSessionByHash(context.Background(), activeID); err != nil {
		t.Fatalf("active session was deleted: %v", err)
	}
	var staleCount int
	if err := store.DB().QueryRow(`SELECT count(*) FROM sessions WHERE expires_at <= ?`, formatTimestamp(now)).Scan(&staleCount); err != nil {
		t.Fatalf("count stale sessions: %v", err)
	}
	if staleCount > 3 {
		t.Fatalf("cleanup removed fewer than the bounded batch: %d stale rows remain", staleCount)
	}
}

func TestSessionHashInputIsStrictlySized(t *testing.T) {
	store := openTestStore(t)
	now := time.Now().UTC()
	err := store.CreateSession(context.Background(), []byte("short"), make([]byte, HashSize), now, now.Add(time.Hour))
	if !errors.Is(err, ErrInvalidHash) {
		t.Fatalf("CreateSession() error = %v, want ErrInvalidHash", err)
	}
}
