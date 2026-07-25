package entry

import (
	"testing"
	"time"

	"github.com/wenpengfei/pulse/internal/ingestion"
)

func TestIdentityPrefersExternalID(t *testing.T) {
	candidate := ingestion.Candidate{
		ExternalID: "  post-42 ",
		URL:        "https://example.com/fallback",
		Title:      "Fallback",
	}

	got, err := Identity(candidate)
	if err != nil {
		t.Fatalf("Identity() error = %v", err)
	}
	if got != "external:post-42" {
		t.Errorf("identity = %q", got)
	}
}

func TestIdentityNormalizesURL(t *testing.T) {
	candidate := ingestion.Candidate{
		URL: "HTTPS://Example.COM:443/post?b=2&a=1#comments",
	}

	got, err := Identity(candidate)
	if err != nil {
		t.Fatalf("Identity() error = %v", err)
	}
	if got != "url:https://example.com/post?a=1&b=2" {
		t.Errorf("identity = %q", got)
	}
}

func TestIdentityUsesTitleAndPublishedDate(t *testing.T) {
	published := time.Date(2026, 7, 25, 10, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	candidate := ingestion.Candidate{
		Title:       " Daily Report ",
		PublishedAt: &published,
	}

	got, err := Identity(candidate)
	if err != nil {
		t.Fatalf("Identity() error = %v", err)
	}
	if got != "title-time:Daily Report|2026-07-25T02:30:00Z" {
		t.Errorf("identity = %q", got)
	}
}

func TestIdentityRejectsEmptyCandidate(t *testing.T) {
	if _, err := Identity(ingestion.Candidate{}); err == nil {
		t.Fatal("Identity() error = nil, want error")
	}
}
