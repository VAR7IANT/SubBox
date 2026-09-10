// Package secretbox implements SubBox's authenticated credential envelope.
package secretbox

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/VAR7IANT/SubBox/backend/internal/protocol"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	// EnvelopeMagic identifies the SubBox credential envelope format.
	EnvelopeMagic = "SBX1"
	// AlgorithmXChaCha20Poly1305 is the only supported envelope algorithm.
	AlgorithmXChaCha20Poly1305 byte = 0x01

	// CredentialAADDomain is the versioned purpose/domain prefix for node
	// credentials. Other encrypted data must use a different purpose.
	CredentialAADDomain = "subbox/node-credentials/v1"

	// MaxCredentialPlaintextBytes bounds the short-lived plaintext JSON.
	MaxCredentialPlaintextBytes = protocol.MaxProtocolJSONBytes
	// EnvelopeHeaderSize is magic + algorithm ID + XChaCha20 nonce.
	EnvelopeHeaderSize = len(EnvelopeMagic) + 1 + chacha20poly1305.NonceSizeX
	// MaxCredentialEnvelopeBytes bounds corrupted database blobs before any
	// allocation or AEAD operation.
	MaxCredentialEnvelopeBytes = EnvelopeHeaderSize + MaxCredentialPlaintextBytes + chacha20poly1305.Overhead
)

var (
	ErrInvalidMasterKey     = errors.New("invalid credential master key")
	ErrMalformedEnvelope    = errors.New("malformed credential envelope")
	ErrUnknownAlgorithm     = errors.New("unknown credential envelope algorithm")
	ErrAuthenticationFailed = errors.New("credential authentication failed")
	ErrInvalidAADContext    = errors.New("invalid credential authentication context")
	ErrPlaintextTooLarge    = errors.New("credential plaintext is too large")
	ErrEnvelopeTooLarge     = errors.New("credential envelope is too large")
	ErrVaultUnavailable     = errors.New("credential vault is unavailable")
)

// CredentialVault derives and holds only the node-credential purpose key.
// The master key is never used directly as an AEAD key and is not retained.
type CredentialVault struct {
	mu         sync.RWMutex
	purposeKey [chacha20poly1305.KeySize]byte
	destroyed  bool
}

// NewCredentialVault derives the node-credential key from a Task 006 master
// key. The caller remains responsible for obtaining the master key from Store.
func NewCredentialVault(masterKey []byte) (*CredentialVault, error) {
	if len(masterKey) != chacha20poly1305.KeySize {
		return nil, ErrInvalidMasterKey
	}

	derived := make([]byte, chacha20poly1305.KeySize)
	reader := hkdf.New(sha256.New, masterKey, nil, []byte("SubBox v1 node credentials"))
	if _, err := io.ReadFull(reader, derived); err != nil {
		clear(derived)
		return nil, fmt.Errorf("derive credential key: %w", err)
	}

	vault := &CredentialVault{}
	copy(vault.purposeKey[:], derived)
	clear(derived)
	return vault, nil
}

// Destroy clears the derived purpose key. It is safe to call more than once.
func (v *CredentialVault) Destroy() {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	clear(v.purposeKey[:])
	v.destroyed = true
}

// Seal validates and encrypts typed credentials. The returned version is the
// value that belongs in nodes.credentials_version.
func (v *CredentialVault) Seal(nodeID string, credentials protocol.Credentials) ([]byte, int, error) {
	if v == nil {
		return nil, 0, ErrVaultUnavailable
	}
	if err := v.ensureAvailable(); err != nil {
		return nil, 0, err
	}
	if err := validateContext(nodeID, protocol.CredentialsVersion, credentialsProtocol(credentials)); err != nil {
		return nil, 0, err
	}
	if err := protocol.ValidateCredentials(credentials); err != nil {
		return nil, 0, err
	}
	credentialProtocol, _ := protocol.ProtocolOfCredentials(credentials)
	plaintext, err := protocol.EncodeCredentials(credentials)
	if err != nil {
		return nil, 0, err
	}
	defer clear(plaintext)
	if len(plaintext) > MaxCredentialPlaintextBytes {
		return nil, 0, ErrPlaintextTooLarge
	}

	aad, err := associatedData(nodeID, credentialProtocol, protocol.CredentialsVersion)
	if err != nil {
		return nil, 0, err
	}
	aead, err := v.aead()
	if err != nil {
		return nil, 0, err
	}
	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		clear(nonce)
		return nil, 0, fmt.Errorf("generate credential nonce: %w", err)
	}
	defer clear(nonce)

	sealed := aead.Seal(nil, nonce, plaintext, aad)
	defer clear(sealed)
	if len(sealed) > MaxCredentialEnvelopeBytes-EnvelopeHeaderSize {
		return nil, 0, ErrEnvelopeTooLarge
	}

	envelope := make([]byte, 0, EnvelopeHeaderSize+len(sealed))
	envelope = append(envelope, EnvelopeMagic...)
	envelope = append(envelope, AlgorithmXChaCha20Poly1305)
	envelope = append(envelope, nonce...)
	envelope = append(envelope, sealed...)
	return envelope, protocol.CredentialsVersion, nil
}

// SealCredentials is a descriptive alias for Seal.
func (v *CredentialVault) SealCredentials(nodeID string, credentials protocol.Credentials) ([]byte, int, error) {
	return v.Seal(nodeID, credentials)
}

// Open authenticates, decrypts, strictly decodes, and validates typed
// credentials. It never returns an arbitrary map or raw JSON value.
func (v *CredentialVault) Open(nodeID string, credentialProtocol protocol.Protocol, version int, envelope []byte) (protocol.Credentials, error) {
	if v == nil {
		return nil, ErrVaultUnavailable
	}
	if err := v.ensureAvailable(); err != nil {
		return nil, err
	}
	if err := validateContext(nodeID, version, credentialProtocol); err != nil {
		return nil, err
	}
	if len(envelope) > MaxCredentialEnvelopeBytes {
		return nil, ErrEnvelopeTooLarge
	}
	if len(envelope) < EnvelopeHeaderSize+chacha20poly1305.Overhead {
		return nil, ErrMalformedEnvelope
	}
	if string(envelope[:len(EnvelopeMagic)]) != EnvelopeMagic {
		return nil, ErrMalformedEnvelope
	}
	if envelope[len(EnvelopeMagic)] != AlgorithmXChaCha20Poly1305 {
		return nil, ErrUnknownAlgorithm
	}

	nonceStart := len(EnvelopeMagic) + 1
	nonceEnd := nonceStart + chacha20poly1305.NonceSizeX
	nonce := envelope[nonceStart:nonceEnd]
	ciphertext := envelope[nonceEnd:]
	aad, err := associatedData(nodeID, credentialProtocol, version)
	if err != nil {
		return nil, err
	}
	aead, err := v.aead()
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, ErrAuthenticationFailed
	}
	defer clear(plaintext)
	if len(plaintext) > MaxCredentialPlaintextBytes {
		return nil, ErrPlaintextTooLarge
	}

	credentials, err := protocol.DecodeCredentials(credentialProtocol, version, plaintext)
	if err != nil {
		return nil, err
	}
	return credentials, nil
}

// OpenCredentials is a descriptive alias for Open.
func (v *CredentialVault) OpenCredentials(nodeID string, credentialProtocol protocol.Protocol, version int, envelope []byte) (protocol.Credentials, error) {
	return v.Open(nodeID, credentialProtocol, version, envelope)
}

func (v *CredentialVault) aead() (interface {
	NonceSize() int
	Overhead() int
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}, error) {
	v.mu.RLock()
	if v.destroyed {
		v.mu.RUnlock()
		return nil, ErrVaultUnavailable
	}
	key := make([]byte, chacha20poly1305.KeySize)
	copy(key, v.purposeKey[:])
	v.mu.RUnlock()
	defer clear(key)
	return chacha20poly1305.NewX(key)
}

func (v *CredentialVault) ensureAvailable() error {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.destroyed {
		return ErrVaultUnavailable
	}
	return nil
}

func credentialsProtocol(credentials protocol.Credentials) protocol.Protocol {
	credentialProtocol, err := protocol.ProtocolOfCredentials(credentials)
	if err != nil {
		return ""
	}
	return credentialProtocol
}

func validateContext(nodeID string, version int, credentialProtocol protocol.Protocol) error {
	if nodeID == "" || strings.IndexByte(nodeID, 0) >= 0 {
		return ErrInvalidAADContext
	}
	if !protocol.IsSupported(credentialProtocol) {
		return protocol.ErrInvalidProtocol
	}
	if version != protocol.CredentialsVersion {
		return protocol.ErrUnsupportedVersion
	}
	return nil
}

func associatedData(nodeID string, credentialProtocol protocol.Protocol, version int) ([]byte, error) {
	if err := validateContext(nodeID, version, credentialProtocol); err != nil {
		return nil, err
	}
	return []byte(strings.Join([]string{
		CredentialAADDomain,
		nodeID,
		string(credentialProtocol),
		strconv.Itoa(version),
	}, "\x00")), nil
}
