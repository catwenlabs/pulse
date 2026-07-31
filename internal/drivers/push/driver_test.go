package push

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/source"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) Do(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestWebhookAndManualDriversDecodeCandidatePayload(t *testing.T) {
	for _, kind := range []source.Kind{source.KindWebhook, source.KindManual} {
		t.Run(string(kind), func(t *testing.T) {
			driver := New(kind)
			batch, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
				Source:  source.Source{ID: "source-1", Kind: kind},
				Payload: bytes.NewBufferString(`{"id":"item-1","title":"Pushed","url":"https://example.com/item"}`),
				Limits:  ingestion.Limits{MaxBytes: 1024},
			})
			if err != nil {
				t.Fatalf("Acquire() error = %v", err)
			}
			if len(batch.Candidates) != 1 || batch.Candidates[0].ExternalID != "item-1" ||
				batch.Candidates[0].Title != "Pushed" {
				t.Errorf("candidates = %+v", batch.Candidates)
			}
		})
	}
}

func TestManualDriverFetchesReadableArticleSnapshot(t *testing.T) {
	var requested *http.Request
	driver := NewManual(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = request
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
			Body: io.NopCloser(strings.NewReader(`<!doctype html>
				<html><head><title>Extracted title</title>
				<meta name="author" content="Ada">
				<meta name="description" content="A useful article">
				</head><body>
				<nav>Navigation that should not be saved</nav>
				<article><h1>Extracted title</h1>
				<p>This is the first substantial paragraph of the saved article, with enough text for reader mode extraction.</p>
				<p>This is the second substantial paragraph. <a href="/next">Continue reading</a>.</p>
				<script>alert("unsafe")</script></article>
				</body></html>`)),
			Request: request,
		}, nil
	}))

	batch, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source:  source.Source{ID: "manual-1", Kind: source.KindManual},
		Payload: bytes.NewBufferString(`{"title":"Bookmark title","url":"https://example.com/posts/one"}`),
		Limits:  ingestion.Limits{MaxBytes: 1 << 20},
	})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if requested == nil || requested.Header.Get("Accept") == "" {
		t.Fatalf("request = %+v", requested)
	}
	candidate := batch.Candidates[0]
	if candidate.Title != "Bookmark title" {
		t.Errorf("Title = %q", candidate.Title)
	}
	if candidate.Author != "Ada" || candidate.Summary != "A useful article" {
		t.Errorf("metadata = author %q summary %q", candidate.Author, candidate.Summary)
	}
	if !strings.Contains(candidate.ContentHTML, "first substantial paragraph") {
		t.Errorf("ContentHTML = %q", candidate.ContentHTML)
	}
	if strings.Contains(candidate.ContentHTML, "<script") ||
		!strings.Contains(candidate.ContentHTML, `href="https://example.com/next"`) {
		t.Errorf("ContentHTML was not safely normalized = %q", candidate.ContentHTML)
	}
	if got := batch.Diagnostics.Details["snapshot"]; got != "saved" {
		t.Errorf("snapshot diagnostic = %q", got)
	}
}

func TestManualDriverKeepsBookmarkWhenSnapshotFails(t *testing.T) {
	driver := NewManual(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network unavailable")
	}))
	batch, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source:  source.Source{Kind: source.KindManual},
		Payload: bytes.NewBufferString(`{"title":"Still saved","url":"https://example.com/item"}`),
	})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if len(batch.Candidates) != 1 || batch.Candidates[0].URL != "https://example.com/item" {
		t.Fatalf("candidates = %+v", batch.Candidates)
	}
	if batch.Candidates[0].ContentHTML != "" {
		t.Errorf("ContentHTML = %q", batch.Candidates[0].ContentHTML)
	}
	if got := batch.Diagnostics.Details["snapshot"]; got != "failed" {
		t.Errorf("snapshot diagnostic = %q", got)
	}
	if batch.Diagnostics.Details["snapshot_error"] == "" {
		t.Fatal("snapshot error diagnostic is empty")
	}
}

func TestManualDriverDoesNotFetchWhenContentWasSupplied(t *testing.T) {
	driver := NewManual(roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("HTTP client must not be called")
		return nil, nil
	}))
	batch, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source: source.Source{Kind: source.KindManual},
		Payload: bytes.NewBufferString(
			`{"title":"Pushed","url":"https://example.com/item","content_html":"<p>Already captured</p>"}`,
		),
	})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if batch.Candidates[0].ContentHTML != "<p>Already captured</p>" {
		t.Errorf("ContentHTML = %q", batch.Candidates[0].ContentHTML)
	}
}

func TestPushDriverRejectsOversizedAndUnknownJSON(t *testing.T) {
	driver := New(source.KindWebhook)
	if _, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source:  source.Source{Kind: source.KindWebhook},
		Payload: bytes.NewBufferString(`{"title":"too long"}`),
		Limits:  ingestion.Limits{MaxBytes: 4},
	}); err == nil {
		t.Fatal("size error = nil")
	}
	if _, err := driver.Acquire(context.Background(), ingestion.AcquireRequest{
		Source:  source.Source{Kind: source.KindWebhook},
		Payload: bytes.NewBufferString(`{"title":"ok","unexpected":true}`),
		Limits:  ingestion.Limits{MaxBytes: 1024},
	}); err == nil {
		t.Fatal("unknown field error = nil")
	}
}
