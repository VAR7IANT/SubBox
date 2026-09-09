package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/VAR7IANT/SubBox/backend/internal/storage"
)

const (
	SessionTokenBytes  = 32
	CSRFTokenBytes     = 32
	DefaultIdleTimeout = 30 * time.Minute
)

var (
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrAuthenticationRequired = errors.New("authentication required")
	ErrCSRFValidation         = errors.New("csrf validation failed")
)

type ServiceConfig struct {
	IdleTimeout time.Duration
	Now         func() time.Time
}

type Service struct {
	store       *storage.Store
	idleTimeout time.Duration
	now         func() time.Time
}

type SessionCredentials struct {
	SessionToken string
	CSRFToken    string
}

type SessionState struct {
	IDHash         []byte
	CSRFSecretHash []byte
	CreatedAt      time.Time
	LastSeenAt     time.Time
	ExpiresAt      time.Time
}

func NewService(store *storage.Store, config ServiceConfig) *Service {
	idleTimeout := config.IdleTimeout
	if idleTimeout <= 0 {
		idleTimeout = DefaultIdleTimeout
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, idleTimeout: idleTimeout, now: now}
}

func (s *Service) SetAdminPassword(ctx context.Context, password string) error {
	if s == nil || s.store == nil {
		return errors.New("authentication service is not configured")
	}
	verifier, err := GeneratePasswordVerifier(password)
	if err != nil {
		return err
	}
	_, err = s.store.SetAdminPassword(ctx, verifier, s.currentTime())
	return err
}

func (s *Service) Login(ctx context.Context, password, previousSessionToken string) (*SessionCredentials, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("authentication service is not configured")
	}
	admin, err := s.store.GetAdmin(ctx)
	if errors.Is(err, storage.ErrAdminNotProvisioned) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("load administrator: %w", err)
	}
	if !VerifyPassword(password, admin.PasswordVerifier) {
		return nil, ErrInvalidCredentials
	}

	sessionRaw, err := randomBytes(SessionTokenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate session: %w", err)
	}
	csrfRaw, err := randomBytes(CSRFTokenBytes)
	if err != nil {
		return nil, fmt.Errorf("generate csrf secret: %w", err)
	}
	now := s.currentTime()
	previousHash, _ := hashEncodedToken(previousSessionToken, SessionTokenBytes)
	err = s.store.RotateSession(
		ctx,
		admin.PasswordVerifier,
		previousHash,
		hashRawToken(sessionRaw),
		hashRawToken(csrfRaw),
		now,
		now.Add(s.idleTimeout),
	)
	if errors.Is(err, storage.ErrStaleAdministrator) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("create authenticated session: %w", err)
	}
	return &SessionCredentials{
		SessionToken: base64.RawURLEncoding.EncodeToString(sessionRaw),
		CSRFToken:    base64.RawURLEncoding.EncodeToString(csrfRaw),
	}, nil
}

func (s *Service) ValidateSession(ctx context.Context, encodedSessionToken string) (*SessionState, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("authentication service is not configured")
	}
	state, now, err := s.loadSession(ctx, encodedSessionToken)
	if err != nil {
		return nil, err
	}
	if err := s.touchLoadedSession(ctx, state.IDHash, now); err != nil {
		return nil, err
	}
	state.LastSeenAt = now
	state.ExpiresAt = now.Add(s.idleTimeout)
	return state, nil
}

// ValidateSessionWithCSRF validates both credentials before extending the
// idle timeout. A rejected CSRF request therefore does not refresh a session.
func (s *Service) ValidateSessionWithCSRF(ctx context.Context, encodedSessionToken, headerToken, cookieToken string) (*SessionState, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("authentication service is not configured")
	}
	state, now, err := s.loadSession(ctx, encodedSessionToken)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateCSRF(state, headerToken, cookieToken); err != nil {
		return nil, err
	}
	if err := s.touchLoadedSession(ctx, state.IDHash, now); err != nil {
		return nil, err
	}
	state.LastSeenAt = now
	state.ExpiresAt = now.Add(s.idleTimeout)
	return state, nil
}

func (s *Service) loadSession(ctx context.Context, encodedSessionToken string) (*SessionState, time.Time, error) {
	raw, err := decodeToken(encodedSessionToken, SessionTokenBytes)
	if err != nil {
		return nil, time.Time{}, ErrAuthenticationRequired
	}
	idHash := hashRawToken(raw)
	session, err := s.store.GetSessionByHash(ctx, idHash)
	if errors.Is(err, storage.ErrSessionNotFound) || errors.Is(err, storage.ErrInvalidHash) {
		return nil, time.Time{}, ErrAuthenticationRequired
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("load session: %w", err)
	}
	now := s.currentTime()
	if session.RevokedAt != nil || !session.ExpiresAt.After(now) {
		return nil, time.Time{}, ErrAuthenticationRequired
	}
	return &SessionState{
		IDHash:         cloneBytes(idHash),
		CSRFSecretHash: cloneBytes(session.CSRFSecretHash),
		CreatedAt:      session.CreatedAt,
		LastSeenAt:     session.LastSeenAt,
		ExpiresAt:      session.ExpiresAt,
	}, now, nil
}

func (s *Service) touchLoadedSession(ctx context.Context, idHash []byte, now time.Time) error {
	if err := s.store.TouchSession(ctx, idHash, now, now.Add(s.idleTimeout)); err != nil {
		if errors.Is(err, storage.ErrSessionInactive) {
			return ErrAuthenticationRequired
		}
		return fmt.Errorf("touch authenticated session: %w", err)
	}
	return nil
}

func (s *Service) ValidateCSRF(state *SessionState, headerToken, cookieToken string) error {
	if state == nil || len(state.CSRFSecretHash) != storage.HashSize {
		return ErrCSRFValidation
	}
	headerRaw, err := decodeToken(headerToken, CSRFTokenBytes)
	if err != nil {
		return ErrCSRFValidation
	}
	cookieRaw, err := decodeToken(cookieToken, CSRFTokenBytes)
	if err != nil {
		return ErrCSRFValidation
	}
	if subtle.ConstantTimeCompare(headerRaw, cookieRaw) != 1 {
		return ErrCSRFValidation
	}
	if subtle.ConstantTimeCompare(hashRawToken(headerRaw), state.CSRFSecretHash) != 1 {
		return ErrCSRFValidation
	}
	return nil
}

func (s *Service) Logout(ctx context.Context, state *SessionState) error {
	if state == nil || len(state.IDHash) != storage.HashSize {
		return ErrAuthenticationRequired
	}
	if err := s.store.RevokeSession(ctx, state.IDHash, s.currentTime()); err != nil {
		return err
	}
	return nil
}

func HashSessionToken(encodedToken string) ([]byte, error) {
	raw, err := decodeToken(encodedToken, SessionTokenBytes)
	if err != nil {
		return nil, err
	}
	return hashRawToken(raw), nil
}

func HashCSRFToken(encodedToken string) ([]byte, error) {
	raw, err := decodeToken(encodedToken, CSRFTokenBytes)
	if err != nil {
		return nil, err
	}
	return hashRawToken(raw), nil
}

func (s *Service) currentTime() time.Time {
	return s.now().UTC()
}

func randomBytes(size int) ([]byte, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return nil, err
	}
	return value, nil
}

func hashRawToken(raw []byte) []byte {
	digest := sha256.Sum256(raw)
	return digest[:]
}

func hashEncodedToken(encoded string, wantBytes int) ([]byte, error) {
	raw, err := decodeToken(encoded, wantBytes)
	if err != nil {
		return nil, err
	}
	return hashRawToken(raw), nil
}

func decodeToken(encoded string, wantBytes int) ([]byte, error) {
	if encoded == "" {
		return nil, errors.New("token is empty")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) != wantBytes {
		return nil, errors.New("token is invalid")
	}
	return decoded, nil
}

func cloneBytes(value []byte) []byte {
	return append([]byte(nil), value...)
}
