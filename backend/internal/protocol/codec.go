package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// MaxProtocolJSONBytes is the limit for both non-secret config JSON and the
// short-lived plaintext JSON used while sealing credentials.
const MaxProtocolJSONBytes = 16 * 1024

// EncodeProtocolConfig validates and returns compact deterministic JSON. The
// JSON contains no protocol or version field; those values are authoritative
// in the surrounding node columns.
func EncodeProtocolConfig(config ProtocolConfig) ([]byte, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode protocol config: %w", err)
	}
	if len(encoded) > MaxProtocolJSONBytes {
		return nil, ErrJSONTooLarge
	}
	return encoded, nil
}

// EncodeConfig is a concise alias for EncodeProtocolConfig.
func EncodeConfig(config ProtocolConfig) ([]byte, error) {
	return EncodeProtocolConfig(config)
}

// DecodeProtocolConfig strictly decodes the schema selected by the database
// protocol and version columns.
func DecodeProtocolConfig(protocol Protocol, version int, encoded []byte) (ProtocolConfig, error) {
	if !IsSupported(protocol) {
		return nil, ErrInvalidProtocol
	}
	if version != ProtocolConfigVersion {
		return nil, fmt.Errorf("%w: protocol config", ErrUnsupportedVersion)
	}

	var config ProtocolConfig
	switch protocol {
	case ProtocolShadowsocks:
		config = &ShadowsocksConfigV1{}
	case ProtocolHysteria2:
		config = &Hysteria2ConfigV1{}
	case ProtocolTUIC:
		config = &TUICConfigV1{}
	case ProtocolVLESSReality:
		config = &VLESSRealityConfigV1{}
	case ProtocolAnyTLS:
		config = &AnyTLSConfigV1{}
	}
	if err := decodeStrictJSON(encoded, config); err != nil {
		return nil, err
	}
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	return configValue(config), nil
}

// DecodeConfig is a concise alias for DecodeProtocolConfig.
func DecodeConfig(protocol Protocol, version int, encoded []byte) (ProtocolConfig, error) {
	return DecodeProtocolConfig(protocol, version, encoded)
}

// EncodeCredentials validates and returns compact deterministic plaintext
// JSON for immediate use as AEAD input. Callers should not persist this value.
func EncodeCredentials(credentials Credentials) ([]byte, error) {
	if err := ValidateCredentials(credentials); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(credentials)
	if err != nil {
		return nil, fmt.Errorf("encode credentials: %w", err)
	}
	if len(encoded) > MaxProtocolJSONBytes {
		return nil, ErrJSONTooLarge
	}
	return encoded, nil
}

// DecodeCredentials strictly decodes the credentials schema selected by the
// database protocol and version columns.
func DecodeCredentials(protocol Protocol, version int, encoded []byte) (Credentials, error) {
	if !IsSupported(protocol) {
		return nil, ErrInvalidProtocol
	}
	if version != CredentialsVersion {
		return nil, fmt.Errorf("%w: credentials", ErrUnsupportedVersion)
	}

	var credentials Credentials
	switch protocol {
	case ProtocolShadowsocks:
		credentials = &ShadowsocksCredentialsV1{}
	case ProtocolHysteria2:
		credentials = &Hysteria2CredentialsV1{}
	case ProtocolTUIC:
		credentials = &TUICCredentialsV1{}
	case ProtocolVLESSReality:
		credentials = &VLESSRealityCredentialsV1{}
	case ProtocolAnyTLS:
		credentials = &AnyTLSCredentialsV1{}
	}
	if err := decodeStrictJSON(encoded, credentials); err != nil {
		return nil, err
	}
	if err := ValidateCredentials(credentials); err != nil {
		return nil, err
	}
	return credentialsValue(credentials), nil
}

func configValue(config ProtocolConfig) ProtocolConfig {
	switch c := config.(type) {
	case *ShadowsocksConfigV1:
		return *c
	case *Hysteria2ConfigV1:
		return *c
	case *TUICConfigV1:
		return *c
	case *VLESSRealityConfigV1:
		return *c
	case *AnyTLSConfigV1:
		return *c
	default:
		return config
	}
}

func credentialsValue(credentials Credentials) Credentials {
	switch c := credentials.(type) {
	case *ShadowsocksCredentialsV1:
		return *c
	case *Hysteria2CredentialsV1:
		return *c
	case *TUICCredentialsV1:
		return *c
	case *VLESSRealityCredentialsV1:
		return *c
	case *AnyTLSCredentialsV1:
		return *c
	default:
		return credentials
	}
}

func decodeStrictJSON(encoded []byte, destination any) error {
	if len(encoded) == 0 || len(encoded) > MaxProtocolJSONBytes {
		if len(encoded) > MaxProtocolJSONBytes {
			return ErrJSONTooLarge
		}
		return ErrMalformedJSON
	}
	if !utf8.Valid(encoded) {
		return ErrMalformedJSON
	}
	if err := rejectDuplicateTopLevelFields(encoded); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if isUnknownFieldError(err) {
			return fmt.Errorf("%w: %v", ErrUnknownJSONField, err)
		}
		return fmt.Errorf("%w: %v", ErrMalformedJSON, err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return ErrTrailingJSON
		}
		return fmt.Errorf("%w: %v", ErrTrailingJSON, err)
	}
	return nil
}

func rejectDuplicateTopLevelFields(encoded []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	first, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedJSON, err)
	}
	delim, ok := first.(json.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("%w: expected object", ErrMalformedJSON)
	}

	seen := make(map[string]struct{})
	for decoder.More() {
		fieldToken, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrMalformedJSON, err)
		}
		field, ok := fieldToken.(string)
		if !ok {
			return fmt.Errorf("%w: expected object field", ErrMalformedJSON)
		}
		if _, exists := seen[field]; exists {
			return fmt.Errorf("%w: %s", ErrDuplicateJSONField, field)
		}
		seen[field] = struct{}{}
		if err := skipJSONValue(decoder); err != nil {
			return fmt.Errorf("%w: %v", ErrMalformedJSON, err)
		}
	}
	if closing, err := decoder.Token(); err != nil || closing != json.Delim('}') {
		if err != nil {
			return fmt.Errorf("%w: %v", ErrMalformedJSON, err)
		}
		return fmt.Errorf("%w: expected object close", ErrMalformedJSON)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return ErrTrailingJSON
		}
		return fmt.Errorf("%w: %v", ErrTrailingJSON, err)
	}
	return nil
}

func skipJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return nil
	}

	switch delim {
	case '{':
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			if _, ok := key.(string); !ok {
				return fmt.Errorf("object key is not a string")
			}
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := skipJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("array is not closed")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	return nil
}

func isUnknownFieldError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unknown field")
}
