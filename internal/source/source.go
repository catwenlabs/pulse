package source

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/wenpengfei/pulse/internal/security"
)

var (
	ErrDuplicate = errors.New("source already exists")
	ErrNotFound  = errors.New("source not found")
)

type ID string

type Kind string

const (
	KindRSS         Kind = "rss"
	KindJSONAPI     Kind = "json-api"
	KindHTML        Kind = "html"
	KindWebhook     Kind = "webhook"
	KindManual      Kind = "manual"
	KindFile        Kind = "file"
	KindAnnotations Kind = "annotations"
)

type Spec struct {
	Name      string
	Kind      Kind
	Locator   string
	Config    json.RawMessage
	SecretRef string
}

type ValidatedSpec struct {
	Name              string
	Kind              Kind
	Locator           string
	NormalizedLocator string
	Config            json.RawMessage
	SecretRef         string
}

type Source struct {
	ID                ID              `json:"id"`
	Name              string          `json:"name"`
	Kind              Kind            `json:"kind"`
	Locator           string          `json:"locator"`
	NormalizedLocator string          `json:"normalized_locator"`
	Config            json.RawMessage `json:"config"`
	SecretRef         string          `json:"-"`
	Enabled           bool            `json:"enabled"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	ArchivedAt        *time.Time      `json:"archived_at,omitempty"`
}

func (item Source) MarshalJSON() ([]byte, error) {
	type sourceJSON Source
	safe := sourceJSON(item)
	safe.Config = security.RedactConfig(item.Config)
	return json.Marshal(safe)
}

type Health struct {
	SourceID             ID         `json:"source_id"`
	Status               string     `json:"status"`
	LastRequestedAt      *time.Time `json:"last_requested_at,omitempty"`
	LastFinishedAt       *time.Time `json:"last_finished_at,omitempty"`
	NextScheduledAt      *time.Time `json:"next_scheduled_at,omitempty"`
	DurationMilliseconds int64      `json:"duration_milliseconds"`
	CandidateCount       int        `json:"candidate_count"`
	NewCount             int        `json:"new_count"`
	UpdatedCount         int        `json:"updated_count"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	LastError            string     `json:"last_error,omitempty"`
}

type ValidationError struct {
	Field   string
	Message string
}

func (err *ValidationError) Error() string {
	return fmt.Sprintf("validate %s: %s", err.Field, err.Message)
}

func (spec Spec) Validate() (ValidatedSpec, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return ValidatedSpec{}, &ValidationError{Field: "name", Message: "must not be empty"}
	}
	if !spec.Kind.Valid() {
		return ValidatedSpec{}, &ValidationError{Field: "kind", Message: fmt.Sprintf("%q is not supported", spec.Kind)}
	}

	locator := strings.TrimSpace(spec.Locator)
	if locator == "" {
		return ValidatedSpec{}, &ValidationError{Field: "locator", Message: "must not be empty"}
	}

	normalized := locator
	if spec.Kind.IsHTTP() {
		var err error
		normalized, err = NormalizeHTTPURL(locator)
		if err != nil {
			return ValidatedSpec{}, &ValidationError{Field: "locator", Message: err.Error()}
		}
	}

	return ValidatedSpec{
		Name:              name,
		Kind:              spec.Kind,
		Locator:           locator,
		NormalizedLocator: normalized,
		Config:            append(json.RawMessage(nil), spec.Config...),
		SecretRef:         strings.TrimSpace(spec.SecretRef),
	}, nil
}

func (kind Kind) Valid() bool {
	switch kind {
	case KindRSS, KindJSONAPI, KindHTML, KindWebhook, KindManual, KindFile, KindAnnotations:
		return true
	default:
		return false
	}
}

func (kind Kind) IsHTTP() bool {
	return kind == KindRSS || kind == KindJSONAPI || kind == KindHTML
}

func NormalizeHTTPURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("must be a valid URL: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("scheme must be http or https")
	}
	if parsed.Hostname() == "" {
		return "", fmt.Errorf("host must not be empty")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("credentials must not be embedded in URL")
	}

	host := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}

	parsed.Host = host
	parsed.Fragment = ""
	parsed.RawQuery = parsed.Query().Encode()
	return parsed.String(), nil
}
