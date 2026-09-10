package secretbox

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/VAR7IANT/SubBox/backend/internal/protocol"
	"golang.org/x/crypto/chacha20poly1305"
)

const vaultTestUUID = "123e4567-e89b-12d3-a456-426614174000"

type credentialFixture struct {
	name        string
	protocol    protocol.Protocol
	credentials protocol.Credentials
}

func vaultFixtures(t *testing.T) []credentialFixture {
	t.Helper()
	private, err := ecdh.X25519().NewPrivateKey(bytes.Repeat([]byte{0x03}, 32))
	if err != nil {
		t.Fatal(err)
	}
	encode := func(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
	return []credentialFixture{
		{name: "shadowsocks", protocol: protocol.ProtocolShadowsocks, credentials: protocol.ShadowsocksCredentialsV1{Password: "task008-known-secret"}},
		{name: "hysteria2", protocol: protocol.ProtocolHysteria2, credentials: protocol.Hysteria2CredentialsV1{Password: "auth-secret", ObfsPassword: "obfs-secret"}},
		{name: "tuic", protocol: protocol.ProtocolTUIC, credentials: protocol.TUICCredentialsV1{UUID: vaultTestUUID, Password: "tuic-secret"}},
		{name: "vless-reality", protocol: protocol.ProtocolVLESSReality, credentials: protocol.VLESSRealityCredentialsV1{UUID: vaultTestUUID, PrivateKey: encode(private.Bytes()), PublicKey: encode(private.PublicKey().Bytes()), ShortIDs: []string{"01"}}},
		{name: "anytls", protocol: protocol.ProtocolAnyTLS, credentials: protocol.AnyTLSCredentialsV1{Password: "anytls-secret"}},
	}
}

func newTestVault(t *testing.T) *CredentialVault {
	t.Helper()
	masterKey := bytes.Repeat([]byte{0x42}, chacha20poly1305.KeySize)
	vault, err := NewCredentialVault(masterKey)
	if err != nil {
		t.Fatal(err)
	}
	return vault
}

func TestCredentialVaultRoundTripForAllProtocols(t *testing.T) {
	vault := newTestVault(t)
	defer vault.Destroy()
	for _, fixture := range vaultFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			envelope, version, err := vault.Seal("node-001", fixture.credentials)
			if err != nil {
				t.Fatal(err)
			}
			if version != protocol.CredentialsVersion {
				t.Fatalf("version = %d, want %d", version, protocol.CredentialsVersion)
			}
			if len(envelope) < EnvelopeHeaderSize+chacha20poly1305.Overhead {
				t.Fatalf("envelope is too short: %d", len(envelope))
			}
			opened, err := vault.Open("node-001", fixture.protocol, version, envelope)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(opened, fixture.credentials) {
				t.Fatalf("opened %#v, want %#v", opened, fixture.credentials)
			}
		})
	}
}

func TestEnvelopeLayoutAndFreshNonce(t *testing.T) {
	vault := newTestVault(t)
	defer vault.Destroy()
	credentials := protocol.ShadowsocksCredentialsV1{Password: "same-plaintext"}
	first, _, err := vault.Seal("node-001", credentials)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := vault.Seal("node-001", credentials)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("same plaintext produced the same ciphertext")
	}
	if string(first[:len(EnvelopeMagic)]) != EnvelopeMagic {
		t.Fatalf("magic = %q", first[:len(EnvelopeMagic)])
	}
	if first[len(EnvelopeMagic)] != AlgorithmXChaCha20Poly1305 {
		t.Fatalf("algorithm byte = %#x", first[len(EnvelopeMagic)])
	}
	expectedPlaintext, err := protocol.EncodeCredentials(credentials)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(expectedPlaintext)
	expectedEnvelopeLength := EnvelopeHeaderSize + len(expectedPlaintext) + chacha20poly1305.Overhead
	if len(first) != expectedEnvelopeLength {
		t.Fatalf("envelope length = %d, want %d", len(first), expectedEnvelopeLength)
	}
	firstNonce := first[len(EnvelopeMagic)+1 : EnvelopeHeaderSize]
	secondNonce := second[len(EnvelopeMagic)+1 : EnvelopeHeaderSize]
	if bytes.Equal(firstNonce, secondNonce) {
		t.Fatal("repeated seals reused the nonce")
	}
}

func TestCredentialVaultDestroyMakesVaultUnavailable(t *testing.T) {
	vault := newTestVault(t)
	credentials := protocol.ShadowsocksCredentialsV1{Password: "destroy-secret"}
	envelope, version, err := vault.Seal("node-001", credentials)
	if err != nil {
		t.Fatal(err)
	}

	vault.Destroy()
	if _, _, err := vault.Seal("node-001", credentials); !errors.Is(err, ErrVaultUnavailable) {
		t.Fatalf("Seal after Destroy error = %v", err)
	}
	if _, err := vault.Open("node-001", protocol.ProtocolShadowsocks, version, envelope); !errors.Is(err, ErrVaultUnavailable) {
		t.Fatalf("Open after Destroy error = %v", err)
	}
	if _, _, err := vault.SealCredentials("node-001", credentials); !errors.Is(err, ErrVaultUnavailable) {
		t.Fatalf("SealCredentials after Destroy error = %v", err)
	}
	if _, err := vault.OpenCredentials("node-001", protocol.ProtocolShadowsocks, version, envelope); !errors.Is(err, ErrVaultUnavailable) {
		t.Fatalf("OpenCredentials after Destroy error = %v", err)
	}

	vault.Destroy()
}

func TestCredentialVaultDestroyIsRaceSafe(t *testing.T) {
	vault := newTestVault(t)
	credentials := protocol.ShadowsocksCredentialsV1{Password: "concurrent-secret"}
	envelope, version, err := vault.Seal("node-001", credentials)
	if err != nil {
		t.Fatal(err)
	}

	const workers = 8
	const operationsPerWorker = 32
	start := make(chan struct{})
	destroyReady := make(chan struct{})
	destroyStart := make(chan struct{})
	destroyDone := make(chan struct{})
	var workersDone sync.WaitGroup
	workersDone.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer workersDone.Done()
			<-start
			for j := 0; j < operationsPerWorker; j++ {
				_, _, _ = vault.Seal("node-001", credentials)
				_, _ = vault.Open("node-001", protocol.ProtocolShadowsocks, version, envelope)
			}
		}()
	}
	go func() {
		close(destroyReady)
		<-destroyStart
		vault.Destroy()
		close(destroyDone)
	}()
	<-destroyReady
	close(start)
	close(destroyStart)
	workersDone.Wait()
	<-destroyDone

	if _, _, err := vault.Seal("node-001", credentials); !errors.Is(err, ErrVaultUnavailable) {
		t.Fatalf("post-concurrency Seal error = %v", err)
	}
	vault.Destroy()
}

func TestCredentialVaultRejectsWrongKeysAndContext(t *testing.T) {
	vault := newTestVault(t)
	defer vault.Destroy()
	credentials := protocol.ShadowsocksCredentialsV1{Password: "context-secret"}
	envelope, version, err := vault.Seal("node-001", credentials)
	if err != nil {
		t.Fatal(err)
	}

	wrongKey := bytes.Repeat([]byte{0x43}, chacha20poly1305.KeySize)
	otherVault, err := NewCredentialVault(wrongKey)
	if err != nil {
		t.Fatal(err)
	}
	defer otherVault.Destroy()
	if _, err := otherVault.Open("node-001", protocol.ProtocolShadowsocks, version, envelope); !errors.Is(err, ErrAuthenticationFailed) {
		t.Fatalf("wrong master key error = %v", err)
	}
	for name, call := range map[string]func() error{
		"wrong node": func() error {
			_, err := vault.Open("node-002", protocol.ProtocolShadowsocks, version, envelope)
			return err
		},
		"wrong protocol": func() error {
			_, err := vault.Open("node-001", protocol.ProtocolAnyTLS, version, envelope)
			return err
		},
		"wrong version": func() error {
			_, err := vault.Open("node-001", protocol.ProtocolShadowsocks, version+1, envelope)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("context substitution was accepted")
			}
		})
	}
}

func TestCredentialVaultRejectsEnvelopeCorruption(t *testing.T) {
	vault := newTestVault(t)
	defer vault.Destroy()
	envelope, version, err := vault.Seal("node-001", protocol.ShadowsocksCredentialsV1{Password: "tamper-secret"})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func([]byte)
		want   error
	}{
		{name: "magic", mutate: func(value []byte) { value[0] ^= 0xff }, want: ErrMalformedEnvelope},
		{name: "algorithm", mutate: func(value []byte) { value[len(EnvelopeMagic)] = 0x7f }, want: ErrUnknownAlgorithm},
		{name: "nonce", mutate: func(value []byte) { value[EnvelopeHeaderSize-1] ^= 0x01 }, want: ErrAuthenticationFailed},
		{name: "ciphertext", mutate: func(value []byte) { value[EnvelopeHeaderSize] ^= 0x01 }, want: ErrAuthenticationFailed},
		{name: "tag", mutate: func(value []byte) { value[len(value)-1] ^= 0x01 }, want: ErrAuthenticationFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := append([]byte(nil), envelope...)
			test.mutate(candidate)
			if _, err := vault.Open("node-001", protocol.ProtocolShadowsocks, version, candidate); !errors.Is(err, test.want) {
				t.Fatalf("got %v, want errors.Is(..., %v)", err, test.want)
			}
		})
	}
	if _, err := vault.Open("node-001", protocol.ProtocolShadowsocks, version, envelope[:EnvelopeHeaderSize]); !errors.Is(err, ErrMalformedEnvelope) {
		t.Fatalf("truncated envelope error = %v", err)
	}
	if _, err := vault.Open("node-001", protocol.ProtocolShadowsocks, version, bytes.Repeat([]byte{0x01}, MaxCredentialEnvelopeBytes+1)); !errors.Is(err, ErrEnvelopeTooLarge) {
		t.Fatalf("oversized envelope error = %v", err)
	}
	if _, _, err := vault.Seal("", protocol.ShadowsocksCredentialsV1{Password: "secret"}); !errors.Is(err, ErrInvalidAADContext) {
		t.Fatalf("empty node ID error = %v", err)
	}
	if _, _, err := vault.Seal("node\x00bad", protocol.ShadowsocksCredentialsV1{Password: "secret"}); !errors.Is(err, ErrInvalidAADContext) {
		t.Fatalf("NUL node ID error = %v", err)
	}
}

func TestCredentialVaultDoesNotContainKnownSecret(t *testing.T) {
	vault := newTestVault(t)
	defer vault.Destroy()
	const secret = "task008-known-secret"
	envelope, _, err := vault.Seal("node-001", protocol.ShadowsocksCredentialsV1{Password: secret})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(envelope, []byte(secret)) {
		t.Fatal("ciphertext contains the known plaintext secret")
	}
}
