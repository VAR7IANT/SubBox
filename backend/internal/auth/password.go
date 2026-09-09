package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	PasswordMinCharacters = 12
	PasswordMaxBytes      = 256

	argonVersion     = 19
	argonMemoryKiB   = 32 * 1024
	argonIterations  = 3
	argonParallelism = 1
	argonSaltBytes   = 16
	argonOutputBytes = 32
)

var (
	ErrPasswordPolicy  = errors.New("password does not meet policy")
	ErrInvalidVerifier = errors.New("invalid password verifier")
)

// ValidatePassword applies the provisioning policy without normalizing or
// trimming the password. Authentication deliberately uses the supplied bytes
// as-is and treats policy-invalid input as a failed credential.
func ValidatePassword(password string) error {
	if !utf8.ValidString(password) {
		return ErrPasswordPolicy
	}
	if utf8.RuneCountInString(password) < PasswordMinCharacters {
		return ErrPasswordPolicy
	}
	if len([]byte(password)) > PasswordMaxBytes {
		return ErrPasswordPolicy
	}
	return nil
}

func GeneratePasswordVerifier(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}

	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemoryKiB, argonParallelism, argonOutputBytes)
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argonVersion,
		argonMemoryKiB,
		argonIterations,
		argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(password, verifier string) bool {
	parsed, err := ParsePasswordVerifier(verifier)
	if err != nil || !utf8.ValidString(password) || len([]byte(password)) > PasswordMaxBytes {
		return false
	}
	computed := argon2.IDKey([]byte(password), parsed.Salt, parsed.Iterations, parsed.MemoryKiB, parsed.Parallelism, uint32(len(parsed.Hash)))
	return subtle.ConstantTimeCompare(computed, parsed.Hash) == 1
}

type ParsedPasswordVerifier struct {
	Algorithm   string
	Version     int
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	Salt        []byte
	Hash        []byte
}

func ParsePasswordVerifier(verifier string) (*ParsedPasswordVerifier, error) {
	parts := strings.Split(verifier, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return nil, ErrInvalidVerifier
	}
	if parts[2] != "v=19" {
		return nil, ErrInvalidVerifier
	}
	if parts[3] != "m=32768,t=3,p=1" {
		return nil, ErrInvalidVerifier
	}

	salt, err := decodeVerifierBytes(parts[4], argonSaltBytes)
	if err != nil {
		return nil, ErrInvalidVerifier
	}
	hash, err := decodeVerifierBytes(parts[5], argonOutputBytes)
	if err != nil {
		return nil, ErrInvalidVerifier
	}
	return &ParsedPasswordVerifier{
		Algorithm:   parts[1],
		Version:     argonVersion,
		MemoryKiB:   argonMemoryKiB,
		Iterations:  argonIterations,
		Parallelism: argonParallelism,
		Salt:        salt,
		Hash:        hash,
	}, nil
}

func decodeVerifierBytes(encoded string, wantLength int) ([]byte, error) {
	if encoded == "" || strings.ContainsAny(encoded, "= \t\r\n") {
		return nil, ErrInvalidVerifier
	}
	decoded, err := base64.RawStdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) != wantLength {
		return nil, ErrInvalidVerifier
	}
	return decoded, nil
}
