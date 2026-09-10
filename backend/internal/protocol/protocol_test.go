package protocol

import (
	"bytes"
	"crypto/ecdh"
	"encoding/base64"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const testUUID = "123e4567-e89b-12d3-a456-426614174000"

type protocolFixture struct {
	name        string
	protocol    Protocol
	config      ProtocolConfig
	credentials Credentials
}

func testFixtures(t *testing.T) []protocolFixture {
	t.Helper()
	privateBytes := bytes.Repeat([]byte{0x07}, 32)
	private, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		t.Fatal(err)
	}
	encodeKey := func(value []byte) string {
		return base64.RawURLEncoding.EncodeToString(value)
	}

	return []protocolFixture{
		{
			name:        "shadowsocks",
			protocol:    ProtocolShadowsocks,
			config:      ShadowsocksConfigV1{Method: "chacha20-ietf-poly1305"},
			credentials: ShadowsocksCredentialsV1{Password: "task008-known-secret"},
		},
		{
			name:     "hysteria2",
			protocol: ProtocolHysteria2,
			config: Hysteria2ConfigV1{
				TLSServerName: "hy.example.com",
				TLSMaterialID: testUUID,
				Obfs:          "salamander",
			},
			credentials: Hysteria2CredentialsV1{
				Password:     "hysteria-auth",
				ObfsPassword: "salamander-secret",
			},
		},
		{
			name:     "tuic",
			protocol: ProtocolTUIC,
			config: TUICConfigV1{
				TLSServerName:     "tuic.example.com",
				TLSMaterialID:     testUUID,
				CongestionControl: "bbr",
			},
			credentials: TUICCredentialsV1{
				UUID:     testUUID,
				Password: "tuic-secret",
			},
		},
		{
			name:     "vless-reality",
			protocol: ProtocolVLESSReality,
			config: VLESSRealityConfigV1{
				ServerName:      "vless.example.com",
				HandshakeServer: "cdn.example.com",
				HandshakePort:   443,
				Flow:            "xtls-rprx-vision",
			},
			credentials: VLESSRealityCredentialsV1{
				UUID:       testUUID,
				PrivateKey: encodeKey(private.Bytes()),
				PublicKey:  encodeKey(private.PublicKey().Bytes()),
				ShortIDs:   []string{"01", "a1b2c3"},
			},
		},
		{
			name:     "anytls",
			protocol: ProtocolAnyTLS,
			config: AnyTLSConfigV1{
				UserDisplayName: "Primary user",
				TLSServerName:   "anytls.example.com",
				TLSMaterialID:   testUUID,
			},
			credentials: AnyTLSCredentialsV1{Password: "anytls-secret"},
		},
	}
}

func TestProtocolEnumIsExactlyPhaseOneSet(t *testing.T) {
	protocols := []Protocol{
		ProtocolShadowsocks,
		ProtocolHysteria2,
		ProtocolTUIC,
		ProtocolVLESSReality,
		ProtocolAnyTLS,
	}
	if len(protocols) != 5 {
		t.Fatalf("got %d protocols, want 5", len(protocols))
	}
	for _, p := range protocols {
		if !IsSupported(p) {
			t.Errorf("%q is not supported", p)
		}
	}
	if IsSupported(Protocol("wireguard")) {
		t.Fatal("unsupported protocol accepted")
	}
}

func TestAllProtocolSchemasRoundTripAndCanonicalEncoding(t *testing.T) {
	for _, fixture := range testFixtures(t) {
		t.Run(fixture.name, func(t *testing.T) {
			configJSON, err := EncodeProtocolConfig(fixture.config)
			if err != nil {
				t.Fatal(err)
			}
			configJSONAgain, err := EncodeProtocolConfig(fixture.config)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(configJSON, configJSONAgain) {
				t.Fatal("config encoding is not deterministic")
			}
			decodedConfig, err := DecodeProtocolConfig(fixture.protocol, ProtocolConfigVersion, configJSON)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decodedConfig, fixture.config) {
				t.Fatalf("decoded config %#v, want %#v", decodedConfig, fixture.config)
			}

			credentialsJSON, err := EncodeCredentials(fixture.credentials)
			if err != nil {
				t.Fatal(err)
			}
			credentialsJSONAgain, err := EncodeCredentials(fixture.credentials)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(credentialsJSON, credentialsJSONAgain) {
				t.Fatal("credentials encoding is not deterministic")
			}
			decodedCredentials, err := DecodeCredentials(fixture.protocol, CredentialsVersion, credentialsJSON)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decodedCredentials, fixture.credentials) {
				t.Fatalf("decoded credentials %#v, want %#v", decodedCredentials, fixture.credentials)
			}
			if err := ValidatePair(fixture.config, fixture.credentials); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConfigEncodingHasNoSecretFields(t *testing.T) {
	for _, fixture := range testFixtures(t) {
		encoded, err := EncodeProtocolConfig(fixture.config)
		if err != nil {
			t.Fatal(err)
		}
		for _, secretField := range []string{"password", "obfs_password", "private_key"} {
			if bytes.Contains(encoded, []byte(secretField)) {
				t.Errorf("%s config contains secret field %q: %s", fixture.name, secretField, encoded)
			}
		}
	}
}

func TestStrictJSONRejectsUnknownTrailingDuplicateVersionProtocolAndOversized(t *testing.T) {
	base := []byte(`{"method":"aes-128-gcm"}`)
	tests := []struct {
		name     string
		protocol Protocol
		version  int
		input    []byte
		want     error
	}{
		{name: "unknown field", protocol: ProtocolShadowsocks, version: 1, input: []byte(`{"method":"aes-128-gcm","padding":true}`), want: ErrUnknownJSONField},
		{name: "trailing value", protocol: ProtocolShadowsocks, version: 1, input: append(append([]byte{}, base...), []byte(` {"method":"aes-128-gcm"}`)...), want: ErrTrailingJSON},
		{name: "duplicate field", protocol: ProtocolShadowsocks, version: 1, input: []byte(`{"method":"aes-128-gcm","method":"aes-256-gcm"}`), want: ErrDuplicateJSONField},
		{name: "wrong version", protocol: ProtocolShadowsocks, version: 2, input: base, want: ErrUnsupportedVersion},
		{name: "unknown protocol", protocol: Protocol("unknown"), version: 1, input: base, want: ErrInvalidProtocol},
		{name: "malformed", protocol: ProtocolShadowsocks, version: 1, input: []byte(`{"method":}`), want: ErrMalformedJSON},
		{name: "oversized", protocol: ProtocolShadowsocks, version: 1, input: bytes.Repeat([]byte(" "), MaxProtocolJSONBytes+1), want: ErrJSONTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeProtocolConfig(test.protocol, test.version, test.input)
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want errors.Is(..., %v)", err, test.want)
			}
		})
	}

	if _, err := DecodeCredentials(ProtocolShadowsocks, CredentialsVersion, []byte(`{"password":"x","password":"y"}`)); !errors.Is(err, ErrDuplicateJSONField) {
		t.Fatalf("duplicate credential field error = %v", err)
	}
	if _, err := DecodeCredentials(ProtocolShadowsocks, CredentialsVersion, []byte(`{"password":"x"} {"password":"y"}`)); !errors.Is(err, ErrTrailingJSON) {
		t.Fatalf("trailing credential value error = %v", err)
	}
	if _, err := DecodeCredentials(ProtocolShadowsocks, CredentialsVersion, []byte(`{"password":"x","unknown":true}`)); !errors.Is(err, ErrUnknownJSONField) {
		t.Fatalf("unknown credential field error = %v", err)
	}
	if _, err := DecodeCredentials(ProtocolShadowsocks, CredentialsVersion, bytes.Repeat([]byte("x"), MaxProtocolJSONBytes+1)); !errors.Is(err, ErrJSONTooLarge) {
		t.Fatalf("oversized credential error = %v", err)
	}
}

func TestStrictProtocolSpecificFields(t *testing.T) {
	validTUIC := []byte(`{"tls_server_name":"tuic.example.com","tls_material_id":"` + testUUID + `","congestion_control":"bbr"}`)
	if _, err := DecodeProtocolConfig(ProtocolTUIC, 1, append(validTUIC[:len(validTUIC)-1], []byte(`,"zero_rtt_handshake":false}`)...)); !errors.Is(err, ErrUnknownJSONField) {
		t.Fatalf("zero_rtt_handshake error = %v", err)
	}
	validAnyTLS := []byte(`{"user_display_name":"user","tls_server_name":"tls.example.com","tls_material_id":"` + testUUID + `"}`)
	for _, field := range []string{"padding", "padding_scheme", "cert_path", "key_path"} {
		input := append(append([]byte{}, validAnyTLS[:len(validAnyTLS)-1]...), []byte(`,"`+field+`":"blocked"}`)...)
		if _, err := DecodeProtocolConfig(ProtocolAnyTLS, 1, input); !errors.Is(err, ErrUnknownJSONField) {
			t.Errorf("%s error = %v", field, err)
		}
	}
}

func TestValidationRules(t *testing.T) {
	if err := (ShadowsocksConfigV1{Method: "2022-blake3-aes-128-gcm"}).Validate(); err == nil {
		t.Fatal("unsupported Shadowsocks method accepted")
	}
	if err := (Hysteria2ConfigV1{TLSServerName: "tls.example.com", TLSMaterialID: testUUID, Obfs: "none"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePair(
		Hysteria2ConfigV1{TLSServerName: "tls.example.com", TLSMaterialID: testUUID, Obfs: "none"},
		Hysteria2CredentialsV1{Password: "auth", ObfsPassword: "must-not-be-here"},
	); err == nil {
		t.Fatal("none obfs accepted an obfs password")
	}
	if err := ValidatePair(
		Hysteria2ConfigV1{TLSServerName: "tls.example.com", TLSMaterialID: testUUID, Obfs: "salamander"},
		Hysteria2CredentialsV1{Password: "auth"},
	); err == nil {
		t.Fatal("salamander obfs accepted an empty obfs password")
	}
	if err := (TUICCredentialsV1{UUID: strings.ToUpper(testUUID), Password: "secret"}).Validate(); err == nil {
		t.Fatal("noncanonical UUID accepted")
	}
	if err := ValidatePair(ShadowsocksConfigV1{Method: "aes-128-gcm"}, TUICCredentialsV1{UUID: testUUID, Password: "secret"}); !errors.Is(err, ErrProtocolMismatch) {
		t.Fatalf("mismatched pair error = %v", err)
	}
	if err := (AnyTLSConfigV1{TLSServerName: "tls.example.com", TLSMaterialID: testUUID}).Validate(); err == nil {
		t.Fatal("empty AnyTLS display name accepted")
	}
	if err := (Hysteria2ConfigV1{TLSServerName: "tls / example.com", TLSMaterialID: testUUID, Obfs: "none"}).Validate(); err == nil {
		t.Fatal("host-like value with whitespace or slash accepted")
	}
	if err := (Hysteria2ConfigV1{TLSServerName: "tls.example.com", TLSMaterialID: testUUID, Obfs: "unsupported"}).Validate(); err == nil {
		t.Fatal("unsupported Hysteria2 obfs accepted")
	}
	if err := (TUICConfigV1{TLSServerName: "tls.example.com", TLSMaterialID: testUUID, CongestionControl: "reno"}).Validate(); err == nil {
		t.Fatal("unsupported TUIC congestion control accepted")
	}
	if err := (AnyTLSConfigV1{UserDisplayName: "user", TLSServerName: "tls.example.com", TLSMaterialID: "not-a-uuid"}).Validate(); err == nil {
		t.Fatal("invalid TLS material UUID accepted")
	}
	if err := (AnyTLSCredentialsV1{}).Validate(); err == nil {
		t.Fatal("empty AnyTLS password accepted")
	}
	for name, password := range map[string]string{
		"empty":        "",
		"nul":          "secret\x00suffix",
		"too long":     strings.Repeat("x", 257),
		"invalid utf8": string([]byte{0xff}),
	} {
		t.Run("password "+name, func(t *testing.T) {
			if err := (ShadowsocksCredentialsV1{Password: password}).Validate(); err == nil {
				t.Fatal("invalid password accepted")
			}
		})
	}
}

func TestRealityValidation(t *testing.T) {
	privateBytes := bytes.Repeat([]byte{0x09}, 32)
	private, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		t.Fatal(err)
	}
	encode := func(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
	valid := VLESSRealityCredentialsV1{
		UUID:       testUUID,
		PrivateKey: encode(private.Bytes()),
		PublicKey:  encode(private.PublicKey().Bytes()),
		ShortIDs:   []string{"01", "aabb"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*VLESSRealityCredentialsV1){
		"bad uuid":            func(value *VLESSRealityCredentialsV1) { value.UUID = "not-a-uuid" },
		"bad private key":     func(value *VLESSRealityCredentialsV1) { value.PrivateKey = "bad" },
		"bad public key":      func(value *VLESSRealityCredentialsV1) { value.PublicKey = "bad" },
		"mismatched key pair": func(value *VLESSRealityCredentialsV1) { value.PublicKey = encode(bytes.Repeat([]byte{0x01}, 32)) },
		"bad short ID":        func(value *VLESSRealityCredentialsV1) { value.ShortIDs = []string{"abc"} },
		"duplicate short ID":  func(value *VLESSRealityCredentialsV1) { value.ShortIDs = []string{"01", "01"} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.ShortIDs = append([]string(nil), valid.ShortIDs...)
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid Reality credentials accepted")
			}
		})
	}
	for _, port := range []int{0, 65536} {
		config := VLESSRealityConfigV1{ServerName: "server.example.com", HandshakeServer: "cdn.example.com", HandshakePort: port, Flow: "xtls-rprx-vision"}
		if err := config.Validate(); err == nil {
			t.Errorf("invalid port %d accepted", port)
		}
	}
	wrongFlow := VLESSRealityConfigV1{ServerName: "server.example.com", HandshakeServer: "cdn.example.com", HandshakePort: 443, Flow: "tcp"}
	if err := wrongFlow.Validate(); err == nil {
		t.Fatal("wrong Reality flow accepted")
	}
}
