package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/VAR7IANT/SubBox/backend/internal/storage"
)

func TestSetAdminPasswordCreatesSingletonAndInvalidatesSessions(t *testing.T) {
	store := openAuthTestStore(t)
	service := NewService(store, ServiceConfig{})
	if err := service.SetAdminPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatalf("first SetAdminPassword() error = %v", err)
	}
	admin, err := store.GetAdmin(context.Background())
	if err != nil {
		t.Fatalf("GetAdmin() error = %v", err)
	}
	firstID := admin.ID
	if admin.PasswordVerifier == "correct horse battery" {
		t.Fatal("plaintext password was stored as verifier")
	}

	login, err := service.Login(context.Background(), "correct horse battery", "")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if err := service.SetAdminPassword(context.Background(), "another correct password"); err != nil {
		t.Fatalf("second SetAdminPassword() error = %v", err)
	}
	admin, err = store.GetAdmin(context.Background())
	if err != nil {
		t.Fatalf("GetAdmin() after update error = %v", err)
	}
	if admin.ID != firstID {
		t.Fatal("password update created a second administrator")
	}
	var adminCount int
	if err := store.DB().QueryRow(`SELECT count(*) FROM admin_user`).Scan(&adminCount); err != nil {
		t.Fatalf("count administrators: %v", err)
	}
	if adminCount != 1 {
		t.Fatalf("administrator count = %d, want 1", adminCount)
	}
	if _, err := service.ValidateSession(context.Background(), login.SessionToken); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("old session validation error = %v, want authentication required", err)
	}
	if _, err := service.Login(context.Background(), "correct horse battery", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password login error = %v, want invalid credentials", err)
	}
	if _, err := service.Login(context.Background(), "another correct password", ""); err != nil {
		t.Fatalf("new password login error = %v", err)
	}
}

func TestLoginSessionHashesAndRotation(t *testing.T) {
	store := openAuthTestStore(t)
	clock := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	service := NewService(store, ServiceConfig{Now: func() time.Time { return clock }})
	if err := service.SetAdminPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}
	first, err := service.Login(context.Background(), "correct horse battery", "")
	if err != nil {
		t.Fatalf("first Login() error = %v", err)
	}
	second, err := service.Login(context.Background(), "correct horse battery", first.SessionToken)
	if err != nil {
		t.Fatalf("second Login() error = %v", err)
	}
	if first.SessionToken == second.SessionToken || first.CSRFToken == second.CSRFToken {
		t.Fatal("login did not generate independent random tokens")
	}
	firstHash, err := HashSessionToken(first.SessionToken)
	if err != nil {
		t.Fatalf("hash first session: %v", err)
	}
	secondHash, err := HashSessionToken(second.SessionToken)
	if err != nil {
		t.Fatalf("hash second session: %v", err)
	}
	var storedID, storedCSRF []byte
	if err := store.DB().QueryRow(`SELECT id_hash, csrf_secret_hash FROM sessions WHERE id_hash = ?`, secondHash).Scan(&storedID, &storedCSRF); err != nil {
		t.Fatalf("read stored session hashes: %v", err)
	}
	if string(storedID) == second.SessionToken || string(storedCSRF) == second.CSRFToken {
		t.Fatal("raw token was stored in the database")
	}
	if len(storedID) != storage.HashSize || len(storedCSRF) != storage.HashSize {
		t.Fatal("stored token hash has an unexpected size")
	}
	if _, err := store.GetSessionByHash(context.Background(), firstHash); err != nil {
		t.Fatalf("read rotated session: %v", err)
	}
	firstSession, err := store.GetSessionByHash(context.Background(), firstHash)
	if err != nil || firstSession.RevokedAt == nil {
		t.Fatalf("rotated session was not revoked: %v", err)
	}
	state, err := service.ValidateSession(context.Background(), second.SessionToken)
	if err != nil {
		t.Fatalf("ValidateSession() error = %v", err)
	}
	if err := service.ValidateCSRF(state, second.CSRFToken, second.CSRFToken); err != nil {
		t.Fatalf("valid CSRF rejected: %v", err)
	}
	if err := service.ValidateCSRF(state, "", second.CSRFToken); !errors.Is(err, ErrCSRFValidation) {
		t.Fatalf("missing CSRF header error = %v", err)
	}
	if err := service.ValidateCSRF(state, second.CSRFToken, first.CSRFToken); !errors.Is(err, ErrCSRFValidation) {
		t.Fatalf("mismatched CSRF error = %v", err)
	}
}

func TestSessionExpirationAndIdleExtension(t *testing.T) {
	store := openAuthTestStore(t)
	clock := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	service := NewService(store, ServiceConfig{Now: func() time.Time { return clock }, IdleTimeout: 30 * time.Minute})
	if err := service.SetAdminPassword(context.Background(), "correct horse battery"); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}
	credentials, err := service.Login(context.Background(), "correct horse battery", "")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	clock = clock.Add(10 * time.Minute)
	if _, err := service.ValidateSession(context.Background(), credentials.SessionToken); err != nil {
		t.Fatalf("session failed before idle timeout: %v", err)
	}
	clock = clock.Add(29 * time.Minute)
	if _, err := service.ValidateSession(context.Background(), credentials.SessionToken); err != nil {
		t.Fatalf("session failed after idle extension: %v", err)
	}
	clock = clock.Add(31 * time.Minute)
	if _, err := service.ValidateSession(context.Background(), credentials.SessionToken); !errors.Is(err, ErrAuthenticationRequired) {
		t.Fatalf("expired session error = %v, want authentication required", err)
	}
}

func openAuthTestStore(t *testing.T) *storage.Store {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
