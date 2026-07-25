package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const encryptedKey = "$pulse_encrypted"

var (
	diagnosticQuerySecret  = regexp.MustCompile(`(?i)(authorization|cookie|api[-_]?key|secret|password|token)=([^&\s"'<>]+)`)
	diagnosticHeaderSecret = regexp.MustCompile(`(?i)(authorization|cookie|x-api-key):[^\r\n;]+`)
)

type CredentialCipher struct {
	aead cipher.AEAD
}

func NewCredentialCipher(encodedKey string) (*CredentialCipher, error) {
	if strings.TrimSpace(encodedKey) == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil {
		return nil, fmt.Errorf("decode PULSE_MASTER_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("PULSE_MASTER_KEY must encode exactly 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher mode: %w", err)
	}
	return &CredentialCipher{aead: aead}, nil
}

func (credentialCipher *CredentialCipher) Protect(config json.RawMessage) (json.RawMessage, error) {
	value, err := decodeObject(config)
	if err != nil {
		return nil, err
	}
	protected, sensitive, err := credentialCipher.protectValue(value, "")
	if err != nil {
		return nil, err
	}
	if sensitive && credentialCipher == nil {
		return nil, fmt.Errorf("PULSE_MASTER_KEY is required when source config contains credentials")
	}
	return encodeObject(protected)
}

func (credentialCipher *CredentialCipher) protectValue(value any, key string) (any, bool, error) {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		sensitive := false
		for childKey, child := range typed {
			if isSensitiveKey(childKey) {
				sensitive = true
				if credentialCipher == nil {
					result[childKey] = child
					continue
				}
				raw, err := json.Marshal(child)
				if err != nil {
					return nil, false, fmt.Errorf("encode credential %s: %w", childKey, err)
				}
				nonce := make([]byte, credentialCipher.aead.NonceSize())
				if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
					return nil, false, fmt.Errorf("create credential nonce: %w", err)
				}
				sealed := credentialCipher.aead.Seal(nil, nonce, raw, []byte(childKey))
				result[childKey] = map[string]any{
					encryptedKey: base64.StdEncoding.EncodeToString(append(nonce, sealed...)),
				}
				continue
			}
			next, childSensitive, err := credentialCipher.protectValue(child, childKey)
			if err != nil {
				return nil, false, err
			}
			sensitive = sensitive || childSensitive
			result[childKey] = next
		}
		return result, sensitive, nil
	case []any:
		result := make([]any, len(typed))
		sensitive := false
		for index, child := range typed {
			next, childSensitive, err := credentialCipher.protectValue(child, key)
			if err != nil {
				return nil, false, err
			}
			sensitive = sensitive || childSensitive
			result[index] = next
		}
		return result, sensitive, nil
	default:
		return value, false, nil
	}
}

func (credentialCipher *CredentialCipher) Reveal(config json.RawMessage) (json.RawMessage, error) {
	value, err := decodeObject(config)
	if err != nil {
		return nil, err
	}
	revealed, err := credentialCipher.revealValue(value, "")
	if err != nil {
		return nil, err
	}
	return encodeObject(revealed)
}

func (credentialCipher *CredentialCipher) revealValue(value any, key string) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		if encoded, ok := typed[encryptedKey].(string); ok && len(typed) == 1 {
			if credentialCipher == nil {
				return nil, fmt.Errorf("PULSE_MASTER_KEY is required to decrypt source credentials")
			}
			data, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil || len(data) < credentialCipher.aead.NonceSize() {
				return nil, fmt.Errorf("decode encrypted source credential")
			}
			nonceSize := credentialCipher.aead.NonceSize()
			plain, err := credentialCipher.aead.Open(
				nil, data[:nonceSize], data[nonceSize:], []byte(key),
			)
			if err != nil {
				return nil, fmt.Errorf("decrypt source credential: %w", err)
			}
			var result any
			if err := json.Unmarshal(plain, &result); err != nil {
				return nil, fmt.Errorf("decode source credential: %w", err)
			}
			return result, nil
		}
		result := make(map[string]any, len(typed))
		for childKey, child := range typed {
			next, err := credentialCipher.revealValue(child, childKey)
			if err != nil {
				return nil, err
			}
			result[childKey] = next
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			next, err := credentialCipher.revealValue(child, key)
			if err != nil {
				return nil, err
			}
			result[index] = next
		}
		return result, nil
	default:
		return value, nil
	}
}

func RedactConfig(config json.RawMessage) json.RawMessage {
	value, err := decodeObject(config)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return mustEncode(redactValue(value))
}

func RedactDiagnosticText(value string) string {
	value = diagnosticQuerySecret.ReplaceAllString(value, "$1=[REDACTED]")
	return diagnosticHeaderSecret.ReplaceAllString(value, "$1: [REDACTED]")
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if isSensitiveKey(key) {
				result[key] = "[REDACTED]"
			} else {
				result[key] = redactValue(child)
			}
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = redactValue(child)
		}
		return result
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(key))
	for _, marker := range []string{"authorization", "cookie", "apikey", "secret", "password", "token"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func decodeObject(config json.RawMessage) (any, error) {
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	var value any
	if err := json.Unmarshal(config, &value); err != nil {
		return nil, fmt.Errorf("decode source config: %w", err)
	}
	return value, nil
}

func encodeObject(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode source config: %w", err)
	}
	return data, nil
}

func mustEncode(value any) json.RawMessage {
	data, _ := json.Marshal(value)
	return data
}
