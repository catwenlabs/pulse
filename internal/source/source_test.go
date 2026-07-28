package source

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSourceJSONRedactsCredentialFields(t *testing.T) {
	item := Source{
		ID: "source", Config: json.RawMessage(`{
			"headers":{"Authorization":"Bearer secret","Accept":"application/json"}
		}`),
		SecretRef: "must-never-be-serialized",
	}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	text := string(data)
	if strings.Contains(text, "Bearer secret") || strings.Contains(text, "must-never") {
		t.Fatalf("source JSON leaked credential: %s", text)
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Errorf("source JSON did not mark redaction: %s", text)
	}
}

func TestValidateNormalizesHTTPSource(t *testing.T) {
	spec := Spec{
		Name:    "  Example Feed  ",
		Kind:    KindRSS,
		Locator: "HTTPS://Example.COM:443/feed?b=2&a=1#section",
	}

	validated, err := spec.Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if validated.Name != "Example Feed" {
		t.Errorf("Name = %q", validated.Name)
	}
	if validated.NormalizedLocator != "https://example.com/feed?a=1&b=2" {
		t.Errorf("NormalizedLocator = %q", validated.NormalizedLocator)
	}
}

func TestValidateRejectsUnsupportedURLScheme(t *testing.T) {
	_, err := (Spec{
		Name:    "Feed",
		Kind:    KindRSS,
		Locator: "file:///etc/passwd",
	}).Validate()

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if validationErr.Field != "locator" {
		t.Errorf("field = %q, want locator", validationErr.Field)
	}
}

func TestValidateRejectsUnknownKind(t *testing.T) {
	_, err := (Spec{
		Name:    "Unknown",
		Kind:    Kind("unknown"),
		Locator: "value",
	}).Validate()

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
	if validationErr.Field != "kind" {
		t.Errorf("field = %q, want kind", validationErr.Field)
	}
}

func TestValidateRequiresName(t *testing.T) {
	_, err := (Spec{Kind: KindManual, Locator: "reading-list"}).Validate()

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field != "name" {
		t.Fatalf("error = %v, want name ValidationError", err)
	}
}

func TestValidateAcceptsAnnotationSource(t *testing.T) {
	got, err := (Spec{
		Name: "Apple Books", Kind: KindAnnotations, Locator: "apple-books",
	}).Validate()
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got.Kind != KindAnnotations || got.NormalizedLocator != "apple-books" {
		t.Errorf("validated = %#v", got)
	}
}
