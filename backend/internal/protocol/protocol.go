// Package protocol contains the typed, versioned Phase 1 protocol schemas.
package protocol

import (
	"errors"
	"reflect"
)

const (
	// ProtocolConfigVersion is the authoritative version for protocol_config.
	ProtocolConfigVersion = 1
	// CredentialsVersion is the authoritative version for encrypted credentials.
	CredentialsVersion = 1
)

// Protocol is the complete Phase 1 protocol set.
type Protocol string

const (
	ProtocolShadowsocks  Protocol = "shadowsocks"
	ProtocolHysteria2    Protocol = "hysteria2"
	ProtocolTUIC         Protocol = "tuic"
	ProtocolVLESSReality Protocol = "vless_reality"
	ProtocolAnyTLS       Protocol = "anytls"
)

// These aliases make the enum names convenient at call sites while retaining
// one authoritative set of values.
const (
	ShadowsocksProtocol  = ProtocolShadowsocks
	Hysteria2Protocol    = ProtocolHysteria2
	TUICProtocol         = ProtocolTUIC
	VLESSRealityProtocol = ProtocolVLESSReality
	AnyTLSProtocol       = ProtocolAnyTLS
)

var (
	ErrInvalidProtocol    = errors.New("invalid protocol")
	ErrUnsupportedVersion = errors.New("unsupported schema version")
	ErrInvalidConfig      = errors.New("invalid protocol config")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrProtocolMismatch   = errors.New("protocol and credentials do not match")
	ErrMalformedJSON      = errors.New("malformed JSON")
	ErrUnknownJSONField   = errors.New("unknown JSON field")
	ErrDuplicateJSONField = errors.New("duplicate JSON field")
	ErrTrailingJSON       = errors.New("trailing JSON value")
	ErrJSONTooLarge       = errors.New("JSON input is too large")
)

// ProtocolConfig and Credentials are sealed interfaces. Implementations must
// be declared in this package so callers cannot inject arbitrary schemas.
type ProtocolConfig interface {
	protocolConfigMarker()
	protocolConfigProtocol() Protocol
}

type Credentials interface {
	credentialsMarker()
	credentialsProtocol() Protocol
}

// ShadowsocksConfigV1 is the non-secret Shadowsocks server configuration.
type ShadowsocksConfigV1 struct {
	Method string `json:"method"`
}

// ShadowsocksCredentialsV1 contains the Shadowsocks authentication secret.
type ShadowsocksCredentialsV1 struct {
	Password string `json:"password"`
}

// Hysteria2ConfigV1 is the non-secret Hysteria2 server configuration.
type Hysteria2ConfigV1 struct {
	TLSServerName string `json:"tls_server_name"`
	TLSMaterialID string `json:"tls_material_id"`
	Obfs          string `json:"obfs"`
}

// Hysteria2CredentialsV1 contains authentication and optional obfuscation
// secrets. The two passwords intentionally remain separate for future Rotate.
type Hysteria2CredentialsV1 struct {
	Password     string `json:"password"`
	ObfsPassword string `json:"obfs_password,omitempty"`
}

// TUICConfigV1 is the non-secret TUIC server configuration.
type TUICConfigV1 struct {
	TLSServerName     string `json:"tls_server_name"`
	TLSMaterialID     string `json:"tls_material_id"`
	CongestionControl string `json:"congestion_control"`
}

// TUICCredentialsV1 contains the TUIC client identity and authentication
// secret.
type TUICCredentialsV1 struct {
	UUID     string `json:"uuid"`
	Password string `json:"password"`
}

// VLESSRealityConfigV1 is the fixed TCP-preset VLESS Reality configuration.
type VLESSRealityConfigV1 struct {
	ServerName      string `json:"server_name"`
	HandshakeServer string `json:"handshake_server"`
	HandshakePort   int    `json:"handshake_port"`
	Flow            string `json:"flow"`
}

// VLESSRealityCredentialsV1 contains the Reality key pair and short IDs.
type VLESSRealityCredentialsV1 struct {
	UUID       string   `json:"uuid"`
	PrivateKey string   `json:"private_key"`
	PublicKey  string   `json:"public_key"`
	ShortIDs   []string `json:"short_ids"`
}

// AnyTLSConfigV1 is the non-secret AnyTLS server configuration. Padding and
// certificate paths are intentionally not part of this Phase 1 schema.
type AnyTLSConfigV1 struct {
	UserDisplayName string `json:"user_display_name"`
	TLSServerName   string `json:"tls_server_name"`
	TLSMaterialID   string `json:"tls_material_id"`
}

// AnyTLSCredentialsV1 contains the AnyTLS authentication secret.
type AnyTLSCredentialsV1 struct {
	Password string `json:"password"`
}

func (ShadowsocksConfigV1) protocolConfigMarker()             {}
func (ShadowsocksConfigV1) protocolConfigProtocol() Protocol  { return ProtocolShadowsocks }
func (Hysteria2ConfigV1) protocolConfigMarker()               {}
func (Hysteria2ConfigV1) protocolConfigProtocol() Protocol    { return ProtocolHysteria2 }
func (TUICConfigV1) protocolConfigMarker()                    {}
func (TUICConfigV1) protocolConfigProtocol() Protocol         { return ProtocolTUIC }
func (VLESSRealityConfigV1) protocolConfigMarker()            {}
func (VLESSRealityConfigV1) protocolConfigProtocol() Protocol { return ProtocolVLESSReality }
func (AnyTLSConfigV1) protocolConfigMarker()                  {}
func (AnyTLSConfigV1) protocolConfigProtocol() Protocol       { return ProtocolAnyTLS }

func (ShadowsocksCredentialsV1) credentialsMarker()             {}
func (ShadowsocksCredentialsV1) credentialsProtocol() Protocol  { return ProtocolShadowsocks }
func (Hysteria2CredentialsV1) credentialsMarker()               {}
func (Hysteria2CredentialsV1) credentialsProtocol() Protocol    { return ProtocolHysteria2 }
func (TUICCredentialsV1) credentialsMarker()                    {}
func (TUICCredentialsV1) credentialsProtocol() Protocol         { return ProtocolTUIC }
func (VLESSRealityCredentialsV1) credentialsMarker()            {}
func (VLESSRealityCredentialsV1) credentialsProtocol() Protocol { return ProtocolVLESSReality }
func (AnyTLSCredentialsV1) credentialsMarker()                  {}
func (AnyTLSCredentialsV1) credentialsProtocol() Protocol       { return ProtocolAnyTLS }

// IsSupported reports whether p is one of the five Phase 1 protocols.
func IsSupported(p Protocol) bool {
	switch p {
	case ProtocolShadowsocks, ProtocolHysteria2, ProtocolTUIC, ProtocolVLESSReality, ProtocolAnyTLS:
		return true
	default:
		return false
	}
}

// ProtocolOfConfig returns the protocol encoded by a typed config.
func ProtocolOfConfig(config ProtocolConfig) (Protocol, error) {
	if isNilInterface(config) {
		return "", ErrInvalidConfig
	}
	return config.protocolConfigProtocol(), nil
}

// ProtocolOfCredentials returns the protocol encoded by typed credentials.
func ProtocolOfCredentials(credentials Credentials) (Protocol, error) {
	if isNilInterface(credentials) {
		return "", ErrInvalidCredentials
	}
	return credentials.credentialsProtocol(), nil
}

func isNilInterface(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
