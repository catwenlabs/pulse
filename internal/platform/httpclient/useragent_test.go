package httpclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestUserAgentTransportSetsDefault(t *testing.T) {
	var recorded string
	transport := userAgentTransport{base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		recorded = req.Header.Get("User-Agent")
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "https://example.com/feed.xml", nil)
	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if !strings.Contains(recorded, "Pulse") || strings.Contains(recorded, "Go-http-client") {
		t.Fatalf("recorded User-Agent = %q, want a descriptive Pulse UA", recorded)
	}
}

func TestUserAgentTransportPreservesExplicit(t *testing.T) {
	var recorded string
	transport := userAgentTransport{base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		recorded = req.Header.Get("User-Agent")
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	})}
	request := httptest.NewRequest(http.MethodGet, "https://example.com/feed.xml", nil)
	request.Header.Set("User-Agent", "custom-bot/9.9")
	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	if recorded != "custom-bot/9.9" {
		t.Fatalf("recorded User-Agent = %q, want custom-bot/9.9 (explicit UA must be preserved)", recorded)
	}
}
