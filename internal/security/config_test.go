package security

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestCredentialCipherProtectsAndRevealsSensitiveConfig(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := NewCredentialCipher(key)
	if err != nil {
		t.Fatalf("NewCredentialCipher() error = %v", err)
	}
	plain := json.RawMessage(`{
		"headers":{"Authorization":"Bearer secret","Accept":"application/json"},
		"pagination":{"cursor_param":"cursor"}
	}`)
	protected, err := cipher.Protect(plain)
	if err != nil {
		t.Fatalf("Protect() error = %v", err)
	}
	if strings.Contains(string(protected), "Bearer secret") {
		t.Fatalf("protected config leaked credential: %s", protected)
	}
	revealed, err := cipher.Reveal(protected)
	if err != nil {
		t.Fatalf("Reveal() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(revealed, &got); err != nil {
		t.Fatalf("decode revealed: %v", err)
	}
	headers := got["headers"].(map[string]any)
	if headers["Authorization"] != "Bearer secret" || headers["Accept"] != "application/json" {
		t.Errorf("revealed headers = %#v", headers)
	}
}

func TestCredentialCipherRequiresExternalKeyForSensitiveConfig(t *testing.T) {
	var cipher *CredentialCipher
	_, err := cipher.Protect(json.RawMessage(`{"headers":{"Authorization":"secret"}}`))
	if err == nil {
		t.Fatal("Protect() error = nil")
	}
}

func TestRedactConfigHidesCredentials(t *testing.T) {
	redacted := RedactConfig(json.RawMessage(`{
		"headers":{"Authorization":"Bearer secret","X-Api-Key":"key","Accept":"application/json"}
	}`))
	if strings.Contains(string(redacted), "secret") || strings.Contains(string(redacted), `"key"`) {
		t.Fatalf("redacted config leaked credential: %s", redacted)
	}
	if !strings.Contains(string(redacted), `"Accept":"application/json"`) {
		t.Errorf("redacted config removed safe header: %s", redacted)
	}
}

func TestRedactDiagnosticTextHidesCredentialValues(t *testing.T) {
	message := `GET https://user:pass@example.com/feed?token=abc123&safe=yes failed; Authorization: Bearer-secret`
	redacted := RedactDiagnosticText(message)
	if strings.Contains(redacted, "user:pass") || strings.Contains(redacted, "abc123") || strings.Contains(redacted, "Bearer-secret") {
		t.Fatalf("diagnostic leaked credential: %s", redacted)
	}
	if !strings.Contains(redacted, "safe=yes") {
		t.Errorf("diagnostic removed safe context: %s", redacted)
	}
}
