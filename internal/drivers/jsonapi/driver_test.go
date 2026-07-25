package jsonapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/source"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

func response(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestDriverMapsFieldsAndFollowsPagePagination(t *testing.T) {
	var pages []string
	driver := New(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		page := request.URL.Query().Get("page")
		pages = append(pages, page)
		switch page {
		case "1":
			return response(`{"data":{"items":[{"id":"1","headline":"First","link":"https://example.com/1"}]}}`), nil
		case "2":
			return response(`{"data":{"items":[{"id":"2","headline":"Second","link":"https://example.com/2"}]}}`), nil
		default:
			return response(`{"data":{"items":[]}}`), nil
		}
	}))
	batch, err := driver.Acquire(context.Background(), requestWithConfig(t, map[string]any{
		"items_path": "data.items",
		"fields":     map[string]string{"id": "id", "title": "headline", "url": "link"},
		"pagination": map[string]any{"mode": "page", "page_param": "page", "start": 1},
	}))
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if strings.Join(pages, ",") != "1,2,3" {
		t.Errorf("pages = %v", pages)
	}
	if len(batch.Candidates) != 2 || batch.Candidates[1].ExternalID != "2" ||
		batch.Candidates[1].Title != "Second" {
		t.Errorf("candidates = %+v", batch.Candidates)
	}
}

func TestDriverFollowsNextURLAndCursorPagination(t *testing.T) {
	tests := []struct {
		name       string
		pagination map[string]any
		first      string
		wantSecond string
	}{
		{
			name: "next URL",
			pagination: map[string]any{
				"mode": "next", "next_path": "paging.next",
			},
			first:      `{"items":[{"id":"1"}],"paging":{"next":"/api?after=one"}}`,
			wantSecond: "after=one",
		},
		{
			name: "cursor",
			pagination: map[string]any{
				"mode": "cursor", "cursor_path": "paging.cursor", "cursor_param": "after",
			},
			first:      `{"items":[{"id":"1"}],"paging":{"cursor":"one"}}`,
			wantSecond: "after=one",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			call := 0
			driver := New(roundTripFunc(func(request *http.Request) (*http.Response, error) {
				call++
				if call == 1 {
					return response(test.first), nil
				}
				if !strings.Contains(request.URL.RawQuery, test.wantSecond) {
					t.Errorf("second URL = %s", request.URL)
				}
				return response(`{"items":[],"paging":{}}`), nil
			}))
			batch, err := driver.Acquire(context.Background(), requestWithConfig(t, map[string]any{
				"items_path": "items",
				"fields":     map[string]string{"id": "id"},
				"pagination": test.pagination,
			}))
			if err != nil {
				t.Fatalf("Acquire() error = %v", err)
			}
			if len(batch.Candidates) != 1 || call != 2 {
				t.Errorf("candidates = %d, calls = %d", len(batch.Candidates), call)
			}
		})
	}
}

func TestDriverEnforcesEntryAndResponseLimits(t *testing.T) {
	driver := New(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(`{"items":[{"id":"1"},{"id":"2"},{"id":"3"}]}`), nil
	}))
	request := requestWithConfig(t, map[string]any{
		"items_path": "items",
		"fields":     map[string]string{"id": "id"},
	})
	request.Limits.MaxEntries = 2
	batch, err := driver.Acquire(context.Background(), request)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if len(batch.Candidates) != 2 || batch.Diagnostics.Details["truncated"] != "entries" {
		t.Errorf("batch = %+v", batch)
	}

	request.Limits.MaxBytes = 4
	if _, err := driver.Acquire(context.Background(), request); err == nil {
		t.Fatal("response limit error = nil")
	}
}

func TestDriverValidateRejectsInvalidMapping(t *testing.T) {
	driver := New(roundTripFunc(nil))
	_, err := driver.Validate(context.Background(), source.Spec{
		Name: "API", Kind: source.KindJSONAPI, Locator: "https://example.com/api",
		Config: json.RawMessage(`{"items_path":"","fields":{}}`),
	})
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
}

func requestWithConfig(t *testing.T, config map[string]any) ingestion.AcquireRequest {
	t.Helper()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return ingestion.AcquireRequest{
		Source: source.Source{
			ID: "source", Name: "API", Kind: source.KindJSONAPI,
			Locator: "https://example.com/api", Config: raw, Enabled: true,
		},
		Limits: ingestion.Limits{MaxBytes: 1 << 20, MaxEntries: 100, MaxPages: 10},
	}
}
