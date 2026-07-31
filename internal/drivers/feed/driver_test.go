package feed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

type failingClient struct{}

func (failingClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("network down")
}

func TestDriverParsesFeedFormats(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		wantID      string
		wantTitle   string
	}{
		{
			name:        "RSS",
			contentType: "application/rss+xml",
			body: `<?xml version="1.0"?><rss version="2.0"><channel><item>
				<guid>rss-1</guid><title>RSS entry</title><link>https://example.com/rss-1</link>
				<description>RSS summary</description><pubDate>Sat, 25 Jul 2026 10:30:00 GMT</pubDate>
			</item></channel></rss>`,
			wantID: "rss-1", wantTitle: "RSS entry",
		},
		{
			name:        "Atom",
			contentType: "application/atom+xml",
			body: `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry>
				<id>atom-1</id><title>Atom entry</title><link href="https://example.com/atom-1"/>
				<summary>Atom summary</summary><updated>2026-07-25T10:30:00Z</updated>
			</entry></feed>`,
			wantID: "atom-1", wantTitle: "Atom entry",
		},
		{
			name:        "JSON Feed",
			contentType: "application/feed+json",
			body: `{"version":"https://jsonfeed.org/version/1.1","items":[{
				"id":"json-1","url":"https://example.com/json-1","title":"JSON entry",
				"content_html":"<p>JSON content</p>","date_published":"2026-07-25T10:30:00Z"
			}]}`,
			wantID: "json-1", wantTitle: "JSON entry",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", test.contentType)
				_, _ = w.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)

			batch, err := New(server.Client()).Acquire(context.Background(), requestFor(server.URL, nil))
			if err != nil {
				t.Fatalf("Acquire() error = %v", err)
			}
			if len(batch.Candidates) != 1 {
				t.Fatalf("candidate count = %d", len(batch.Candidates))
			}
			if batch.Candidates[0].ExternalID != test.wantID ||
				batch.Candidates[0].Title != test.wantTitle {
				t.Errorf("candidate = %+v", batch.Candidates[0])
			}
		})
	}
}

func TestDriverPreservesRichRSSContentAndMediaImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
			<rss version="2.0"
				xmlns:content="http://purl.org/rss/1.0/modules/content/"
				xmlns:media="http://search.yahoo.com/mrss/">
				<channel><item>
					<guid>rich-1</guid>
					<title>Rich entry</title>
					<link>https://example.com/rich-1</link>
					<description>Short summary</description>
					<content:encoded><![CDATA[<h2>Section</h2><p>Full body</p>]]></content:encoded>
					<media:thumbnail url="https://images.example/cover.jpg"/>
				</item></channel>
			</rss>`))
	}))
	t.Cleanup(server.Close)

	batch, err := New(server.Client()).Acquire(context.Background(), requestFor(server.URL, nil))
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if len(batch.Candidates) != 1 {
		t.Fatalf("candidate count = %d", len(batch.Candidates))
	}
	content := batch.Candidates[0].ContentHTML
	if !strings.Contains(content, `<img src="https://images.example/cover.jpg"`) ||
		!strings.Contains(content, `<h2>Section</h2><p>Full body</p>`) {
		t.Errorf("ContentHTML = %q", content)
	}
	if batch.Candidates[0].Summary != "Short summary" {
		t.Errorf("Summary = %q", batch.Candidates[0].Summary)
	}
}

func TestDriverUsesConditionalRequestCheckpoint(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(`<rss><channel></channel></rss>`))
	}))
	t.Cleanup(server.Close)
	driver := New(server.Client())

	first, err := driver.Acquire(context.Background(), requestFor(server.URL, nil))
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	second, err := driver.Acquire(context.Background(), requestFor(server.URL, first.NextCheckpoint))
	if err != nil {
		t.Fatalf("second Acquire() error = %v", err)
	}
	if requests != 2 || second.Diagnostics.Status != "not_modified" {
		t.Errorf("requests = %d, diagnostics = %+v", requests, second.Diagnostics)
	}
}

func TestDriverRefetchesAfterParserUpgrade(t *testing.T) {
	var conditionalHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		conditionalHeader = request.Header.Get("If-None-Match")
		w.Header().Set("ETag", `"v2"`)
		_, _ = w.Write([]byte(`<rss><channel><item><guid>upgraded-1</guid><description>Body</description></item></channel></rss>`))
	}))
	t.Cleanup(server.Close)

	legacyCheckpoint := ingestion.Checkpoint(`{"etag":"\"v1\"","last_modified":"Sat, 25 Jul 2026 10:30:00 GMT"}`)
	batch, err := New(server.Client()).Acquire(context.Background(), requestFor(server.URL, legacyCheckpoint))
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if conditionalHeader != "" {
		t.Errorf("If-None-Match = %q, want empty after parser upgrade", conditionalHeader)
	}
	if len(batch.Candidates) != 1 {
		t.Errorf("candidate count = %d", len(batch.Candidates))
	}
}

func TestDriverRejectsMalformedFeed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<not-a-feed>`))
	}))
	t.Cleanup(server.Close)

	_, err := New(server.Client()).Acquire(context.Background(), requestFor(server.URL, nil))
	if err == nil || !errors.Is(err, ingestion.ErrParse) {
		t.Fatalf("Acquire() error = %v, want errors.Is ErrParse", err)
	}
}

func TestDriverValidation(t *testing.T) {
	driver := New(http.DefaultClient)
	if _, err := driver.Validate(context.Background(), source.Spec{
		Name: "Feed", Kind: source.KindRSS, Locator: "https://example.com/feed",
	}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if _, err := driver.Validate(context.Background(), source.Spec{
		Name: "API", Kind: source.KindJSONAPI, Locator: "https://example.com/api",
	}); err == nil {
		t.Fatal("wrong kind validation error = nil")
	}
}

func TestDriverRejectsBadCheckpointAndNetworkFailure(t *testing.T) {
	driver := New(failingClient{})
	request := requestFor("https://example.com/feed", ingestion.Checkpoint(`not-json`))
	if _, err := driver.Acquire(context.Background(), request); err == nil {
		t.Fatal("bad checkpoint error = nil")
	}
	request.Checkpoint = nil
	if _, err := driver.Acquire(context.Background(), request); err == nil {
		t.Fatal("network error = nil")
	}
}

func TestDriverRejectsHTTPErrorAndOversizedResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		limit  int64
	}{
		{name: "HTTP error", status: http.StatusBadGateway},
		{name: "oversized", status: http.StatusOK, body: strings.Repeat("x", 20), limit: 10},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)
			request := requestFor(server.URL, nil)
			request.Limits.MaxBytes = test.limit
			if _, err := New(server.Client()).Acquire(context.Background(), request); err == nil {
				t.Fatal("Acquire() error = nil")
			}
		})
	}
}

func requestFor(url string, checkpoint ingestion.Checkpoint) ingestion.AcquireRequest {
	return ingestion.AcquireRequest{
		Source:     source.Source{Kind: source.KindRSS, Locator: url},
		Trigger:    ingestion.TriggerSchedule,
		Checkpoint: checkpoint,
		Limits:     ingestion.Limits{MaxBytes: 1 << 20},
	}
}

func TestAcquireWrapsFetchError(t *testing.T) {
	_, err := New(failingClient{}).Acquire(context.Background(), requestFor("https://example.invalid/feed", nil))
	if !errors.Is(err, ingestion.ErrFetch) {
		t.Fatalf("Acquire() err = %v, want errors.Is ErrFetch", err)
	}
}

func TestAcquireWrapsParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<<< not a feed"))
	}))
	t.Cleanup(server.Close)

	_, err := New(server.Client()).Acquire(context.Background(), requestFor(server.URL, nil))
	if !errors.Is(err, ingestion.ErrParse) {
		t.Fatalf("Acquire() err = %v, want errors.Is ErrParse", err)
	}
}
