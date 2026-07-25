package httpserver

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestWebAppServesIndexAssetsAndSPAFallback(t *testing.T) {
	web := fstest.MapFS{
		"index.html":        &fstest.MapFile{Data: []byte("<main>Pulse</main>")},
		"assets/app.js":     &fstest.MapFile{Data: []byte("console.log('pulse')")},
		"assets/ignored.js": &fstest.MapFile{Data: []byte("ignored")},
	}
	handler := NewHandlerWithWeb(nil, web)

	tests := []struct {
		path        string
		wantStatus  int
		wantContent string
	}{
		{path: "/", wantStatus: http.StatusOK, wantContent: "Pulse"},
		{path: "/assets/app.js", wantStatus: http.StatusOK, wantContent: "console.log"},
		{path: "/sources/source-1", wantStatus: http.StatusOK, wantContent: "Pulse"},
		{path: "/api/v1/unknown", wantStatus: http.StatusNotFound},
	}
	for _, test := range tests {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != test.wantStatus {
			t.Errorf("%s status = %d, want %d", test.path, response.Code, test.wantStatus)
		}
		if test.wantContent != "" && !strings.Contains(response.Body.String(), test.wantContent) {
			t.Errorf("%s body = %q, want %q", test.path, response.Body.String(), test.wantContent)
		}
	}

	if _, err := fs.Stat(web, "assets/ignored.js"); err != nil {
		t.Fatalf("test filesystem: %v", err)
	}
}

func TestUnknownRoute(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	response := httptest.NewRecorder()

	NewHandler().ServeHTTP(response, req)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}
