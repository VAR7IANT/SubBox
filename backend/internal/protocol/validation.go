package protocol

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxHostLikeBytes = 253
	maxDisplayBytes  = 256
	maxPasswordBytes = 256
)

var (
	errInvalidMethod        = fmt.Errorf("%w: method", ErrInvalidConfig)
	errInvalidObfs          = fmt.Errorf("%w: obfs", ErrInvalidConfig)
	errInvalidCongestion    = fmt.Errorf("%w: congestion_control", ErrInvalidConfig)
	errInvalidFlow          = fmt.Errorf("%w: flow", ErrInvalidConfig)
	errInvalidPassword      = fmt.Errorf("%w: password", ErrInvalidCredentials)
	errInvalidObfsPassword  = fmt.Errorf("%w: obfs_password", ErrInvalidCredentials)
	errInvalidUUID          = fmt.Errorf("%w: uuid", ErrInvalidCredentials)
	errInvalidKey           = fmt.Errorf("%w: key", ErrInvalidCredentials)
	errInvalidShortIDs      = fmt.Errorf("%w: short_ids", ErrInvalidCredentials)
	errInvalidTLSMaterialID = fmt.Errorf("%w: tls_material_id", ErrInvalidConfig)
	errInvalidDisplayName   = fmt.Errorf("%w: user_display_name", ErrInvalidConfig)
)

// Validate checks the Shadowsocks non-secret settings.
func (c ShadowsocksConfigV1) Validate() error {
	switch c.Method {
	case "aes-128-gcm", "aes-256-gcm", "chacha20-ietf-poly1305":
		return nil
	default:
		return errInvalidMethod
	}
}

// Validate checks the Shadowsocks secret fields.
func (c ShadowsocksCredentialsV1) Validate() error {
	return validatePassword(c.Password, errInvalidPassword)
}

// Validate checks the Hysteria2 non-secret settings.
func (c Hysteria2ConfigV1) Validate() error {
	if err := validateHostLike(c.TLSServerName); err != nil {
		return fmt.Errorf("%w: tls_server_name", ErrInvalidConfig)
	}
	if err := validateCanonicalUUID(c.TLSMaterialID); err != nil {
		return errInvalidTLSMaterialID
	}
	switch c.Obfs {
	case "none", "salamander":
		return nil
	default:
		return errInvalidObfs
	}
}

// Validate checks the Hysteria2 secret fields. The obfuscation password is
// deliberately validated independently from the authentication password.
func (c Hysteria2CredentialsV1) Validate() error {
	if err := validatePassword(c.Password, errInvalidPassword); err != nil {
		return err
	}
	if c.ObfsPassword == "" {
		return nil
	}
	return validatePassword(c.ObfsPassword, errInvalidObfsPassword)
}

// Validate checks the TUIC non-secret settings.
func (c TUICConfigV1) Validate() error {
	if err := validateHostLike(c.TLSServerName); err != nil {
		return fmt.Errorf("%w: tls_server_name", ErrInvalidConfig)
	}
	if err := validateCanonicalUUID(c.TLSMaterialID); err != nil {
		return errInvalidTLSMaterialID
	}
	switch c.CongestionControl {
	case "bbr", "cubic", "new_reno":
		return nil
	default:
		return errInvalidCongestion
	}
}

// Validate checks the TUIC secret fields.
func (c TUICCredentialsV1) Validate() error {
	if err := validateCanonicalUUID(c.UUID); err != nil {
		return errInvalidUUID
	}
	return validatePassword(c.Password, errInvalidPassword)
}

// Validate checks the fixed TCP-preset VLESS Reality settings.
func (c VLESSRealityConfigV1) Validate() error {
	if err := validateHostLike(c.ServerName); err != nil {
		return fmt.Errorf("%w: server_name", ErrInvalidConfig)
	}
	if err := validateHostLike(c.HandshakeServer); err != nil {
		return fmt.Errorf("%w: handshake_server", ErrInvalidConfig)
	}
	if c.HandshakePort < 1 || c.HandshakePort > 65535 {
		return fmt.Errorf("%w: handshake_port", ErrInvalidConfig)
	}
	if c.Flow != "xtls-rprx-vision" {
		return errInvalidFlow
	}
	return nil
}

// Validate checks the VLESS Reality identity and X25519 key material.
func (c VLESSRealityCredentialsV1) Validate() error {
	if err := validateCanonicalUUID(c.UUID); err != nil {
		return errInvalidUUID
	}
	privateKey, err := decodeRawURL32(c.PrivateKey)
	if err != nil {
		return fmt.Errorf("%w: private_key", errInvalidKey)
	}
	publicKey, err := decodeRawURL32(c.PublicKey)
	if err != nil {
		return fmt.Errorf("%w: public_key", errInvalidKey)
	}
	private, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("%w: private_key", errInvalidKey)
	}
	if !bytes.Equal(private.PublicKey().Bytes(), publicKey) {
		return fmt.Errorf("%w: key pair", errInvalidKey)
	}
	if len(c.ShortIDs) < 1 || len(c.ShortIDs) > 8 {
		return errInvalidShortIDs
	}
	seen := make(map[string]struct{}, len(c.ShortIDs))
	for _, shortID := range c.ShortIDs {
		if err := validateShortID(shortID); err != nil {
			return errInvalidShortIDs
		}
		if _, exists := seen[shortID]; exists {
			return errInvalidShortIDs
		}
		seen[shortID] = struct{}{}
	}
	return nil
}

// Validate checks the AnyTLS non-secret settings.
func (c AnyTLSConfigV1) Validate() error {
	if err := validateDisplayName(c.UserDisplayName); err != nil {
		return errInvalidDisplayName
	}
	if err := validateHostLike(c.TLSServerName); err != nil {
		return fmt.Errorf("%w: tls_server_name", ErrInvalidConfig)
	}
	if err := validateCanonicalUUID(c.TLSMaterialID); err != nil {
		return errInvalidTLSMaterialID
	}
	return nil
}

// Validate checks the AnyTLS secret fields.
func (c AnyTLSCredentialsV1) Validate() error {
	return validatePassword(c.Password, errInvalidPassword)
}

// ValidateConfig validates any sealed Phase 1 config implementation.
func ValidateConfig(config ProtocolConfig) error {
	if isNilInterface(config) {
		return ErrInvalidConfig
	}
	switch c := config.(type) {
	case ShadowsocksConfigV1:
		return c.Validate()
	case *ShadowsocksConfigV1:
		return c.Validate()
	case Hysteria2ConfigV1:
		return c.Validate()
	case *Hysteria2ConfigV1:
		return c.Validate()
	case TUICConfigV1:
		return c.Validate()
	case *TUICConfigV1:
		return c.Validate()
	case VLESSRealityConfigV1:
		return c.Validate()
	case *VLESSRealityConfigV1:
		return c.Validate()
	case AnyTLSConfigV1:
		return c.Validate()
	case *AnyTLSConfigV1:
		return c.Validate()
	default:
		return ErrInvalidConfig
	}
}

// ValidateCredentials validates any sealed Phase 1 credentials
// implementation.
func ValidateCredentials(credentials Credentials) error {
	if isNilInterface(credentials) {
		return ErrInvalidCredentials
	}
	switch c := credentials.(type) {
	case ShadowsocksCredentialsV1:
		return c.Validate()
	case *ShadowsocksCredentialsV1:
		return c.Validate()
	case Hysteria2CredentialsV1:
		return c.Validate()
	case *Hysteria2CredentialsV1:
		return c.Validate()
	case TUICCredentialsV1:
		return c.Validate()
	case *TUICCredentialsV1:
		return c.Validate()
	case VLESSRealityCredentialsV1:
		return c.Validate()
	case *VLESSRealityCredentialsV1:
		return c.Validate()
	case AnyTLSCredentialsV1:
		return c.Validate()
	case *AnyTLSCredentialsV1:
		return c.Validate()
	default:
		return ErrInvalidCredentials
	}
}

// ValidatePair verifies that a config and credentials use the same protocol
// and the protocol-specific relationship rules.
func ValidatePair(config ProtocolConfig, credentials Credentials) error {
	if err := ValidateConfig(config); err != nil {
		return err
	}
	if err := ValidateCredentials(credentials); err != nil {
		return err
	}
	configProtocol, _ := ProtocolOfConfig(config)
	credentialsProtocol, _ := ProtocolOfCredentials(credentials)
	if configProtocol != credentialsProtocol {
		return ErrProtocolMismatch
	}

	switch configProtocol {
	case ProtocolShadowsocks:
		return nil
	case ProtocolHysteria2:
		cfg := asHysteria2Config(config)
		creds := asHysteria2Credentials(credentials)
		if cfg.Obfs == "none" && creds.ObfsPassword != "" {
			return fmt.Errorf("%w: obfs_password must be empty for none", ErrInvalidCredentials)
		}
		if cfg.Obfs == "salamander" && creds.ObfsPassword == "" {
			return fmt.Errorf("%w: obfs_password is required for salamander", ErrInvalidCredentials)
		}
		return nil
	case ProtocolTUIC, ProtocolVLESSReality, ProtocolAnyTLS:
		return nil
	default:
		return ErrInvalidProtocol
	}
}

// ValidateConfigCredentials is the descriptive alias used by callers that
// prefer the full domain terminology.
func ValidateConfigCredentials(config ProtocolConfig, credentials Credentials) error {
	return ValidatePair(config, credentials)
}

func asHysteria2Config(config ProtocolConfig) Hysteria2ConfigV1 {
	switch c := config.(type) {
	case Hysteria2ConfigV1:
		return c
	case *Hysteria2ConfigV1:
		return *c
	default:
		return Hysteria2ConfigV1{}
	}
}

func asHysteria2Credentials(credentials Credentials) Hysteria2CredentialsV1 {
	switch c := credentials.(type) {
	case Hysteria2CredentialsV1:
		return c
	case *Hysteria2CredentialsV1:
		return *c
	default:
		return Hysteria2CredentialsV1{}
	}
}

func validatePassword(value string, invalid error) error {
	if !utf8.ValidString(value) || len(value) < 1 || len(value) > maxPasswordBytes || strings.IndexByte(value, 0) >= 0 {
		return invalid
	}
	return nil
}

func validateHostLike(value string) error {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > maxHostLikeBytes {
		return ErrInvalidConfig
	}
	for _, r := range value {
		if r == '/' || r == '\\' || unicode.IsControl(r) || unicode.IsSpace(r) {
			return ErrInvalidConfig
		}
	}
	return nil
}

func validateDisplayName(value string) error {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > maxDisplayBytes {
		return ErrInvalidConfig
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '\x00' {
			return ErrInvalidConfig
		}
	}
	return nil
}

func validateCanonicalUUID(value string) error {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.String() != value {
		return ErrInvalidCredentials
	}
	return nil
}

func decodeRawURL32(value string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, ErrInvalidCredentials
	}
	return decoded, nil
}

func validateShortID(value string) error {
	if len(value) < 2 || len(value) > 16 || len(value)%2 != 0 || strings.ToLower(value) != value {
		return ErrInvalidCredentials
	}
	_, err := hex.DecodeString(value)
	return err
}
