package html

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/source"
)

type clientFunc func(*http.Request) (*http.Response, error)

func (function clientFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func htmlResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDriverExtractsCollectionWithCSSSelectors(t *testing.T) {
	driver := New(clientFunc(func(*http.Request) (*http.Response, error) {
		return htmlResponse(`
			<html><body><section id="news">
				<article class="card featured"><h2 class="title">First &amp; best</h2><a class="permalink" href="/first">Read</a><span class="author">Ada</span></article>
				<article class="card"><h2 class="title">Second</h2><a class="permalink" href="https://example.com/second">Read</a></article>
			</section></body></html>`), nil
	}))
	batch, err := driver.Acquire(context.Background(), htmlRequest(t, map[string]any{
		"mode":          "collection",
		"item_selector": "#news article.card",
		"fields": map[string]any{
			"title":  map[string]string{"selector": ".title"},
			"url":    map[string]string{"selector": "a.permalink", "attribute": "href"},
			"author": map[string]string{"selector": ".author"},
		},
	}))
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if len(batch.Candidates) != 2 {
		t.Fatalf("candidate count = %d", len(batch.Candidates))
	}
	if batch.Candidates[0].Title != "First & best" ||
		batch.Candidates[0].URL != "https://example.com/first" ||
		batch.Candidates[0].Author != "Ada" {
		t.Errorf("first = %+v", batch.Candidates[0])
	}
}

func TestDriverExtractsSingleDocumentWithStableIdentity(t *testing.T) {
	driver := New(clientFunc(func(*http.Request) (*http.Response, error) {
		return htmlResponse(`<main><h1>Living document</h1><div class="content"><p>Hello</p></div></main>`), nil
	}))
	batch, err := driver.Acquire(context.Background(), htmlRequest(t, map[string]any{
		"mode": "single",
		"fields": map[string]any{
			"title":        map[string]string{"selector": "h1"},
			"content_html": map[string]any{"selector": ".content", "html": true},
		},
	}))
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if len(batch.Candidates) != 1 || batch.Candidates[0].ExternalID != "source-html" ||
		!strings.Contains(batch.Candidates[0].ContentHTML, "<p>Hello</p>") {
		t.Errorf("candidate = %+v", batch.Candidates)
	}
}

func TestDriverRejectsZeroCollectionMatches(t *testing.T) {
	driver := New(clientFunc(func(*http.Request) (*http.Response, error) {
		return htmlResponse(`<main><p>No cards</p></main>`), nil
	}))
	_, err := driver.Acquire(context.Background(), htmlRequest(t, map[string]any{
		"mode": "collection", "item_selector": ".card",
		"fields": map[string]any{"title": map[string]string{"selector": "h2"}},
	}))
	if err == nil || !strings.Contains(err.Error(), "zero") {
		t.Fatalf("Acquire() error = %v", err)
	}
}

func htmlRequest(t *testing.T, config map[string]any) ingestion.AcquireRequest {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return ingestion.AcquireRequest{
		Source: source.Source{
			ID: "source-html", Name: "Page", Kind: source.KindHTML,
			Locator: "https://example.com/news", Config: raw, Enabled: true,
		},
		Limits: ingestion.Limits{MaxBytes: 1 << 20, MaxEntries: 100},
	}
}
