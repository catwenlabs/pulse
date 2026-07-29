package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/opml"
	"github.com/wenpengfei/pulse/internal/organization"
	"github.com/wenpengfei/pulse/internal/preview"
	"github.com/wenpengfei/pulse/internal/rule"
	"github.com/wenpengfei/pulse/internal/source"
)

type fakeBackend struct {
	createSource    func(context.Context, source.Spec) (source.Source, error)
	listSources     func(context.Context) ([]source.Source, error)
	getSource       func(context.Context, source.ID) (source.Source, error)
	updateSource    func(context.Context, source.ID, string, string) (source.Source, error)
	setEnabled      func(context.Context, source.ID, bool) (source.Source, error)
	archiveSource   func(context.Context, source.ID) error
	setSecret       func(context.Context, source.ID, string) error
	getSourceHealth func(context.Context, source.ID) (source.Health, error)
	listFolders     func(context.Context) ([]organization.Folder, error)
	enqueue         func(context.Context, ingestion.EnqueueRequest) (ingestion.Acquisition, error)
	listEntries     func(context.Context, int) ([]entry.Entry, error)
	searchEntries   func(context.Context, entry.Query) ([]entry.Entry, error)
	getEntry        func(context.Context, entry.ID) (entry.Entry, error)
	updateEntry     func(context.Context, entry.ID, entry.Patch) (entry.Entry, error)
	markEntriesRead func(context.Context, source.ID) (int64, error)
	addEntryTag     func(context.Context, entry.ID, string) (entry.Tag, error)
	removeEntryTag  func(context.Context, entry.ID, string) error
	importOPML      func(context.Context, []opml.Subscription) (opml.ImportResult, error)
	exportOPML      func(context.Context) ([]opml.Subscription, error)
	previewSource   func(context.Context, source.Spec) (preview.Result, error)
	replayRule      func(context.Context, string, bool) (rule.ReplayResult, error)
}

func (fake fakeBackend) CreateSource(ctx context.Context, spec source.Spec) (source.Source, error) {
	return fake.createSource(ctx, spec)
}

func (fake fakeBackend) ListSources(ctx context.Context) ([]source.Source, error) {
	return fake.listSources(ctx)
}

func (fake fakeBackend) GetSource(ctx context.Context, id source.ID) (source.Source, error) {
	return fake.getSource(ctx, id)
}

func (fake fakeBackend) SetSourceEnabled(ctx context.Context, id source.ID, enabled bool) (source.Source, error) {
	return fake.setEnabled(ctx, id, enabled)
}

func (fake fakeBackend) UpdateSource(ctx context.Context, id source.ID, name, locator string) (source.Source, error) {
	return fake.updateSource(ctx, id, name, locator)
}

func (fake fakeBackend) ArchiveSource(ctx context.Context, id source.ID) error {
	return fake.archiveSource(ctx, id)
}

func (fake fakeBackend) SetSourceSecret(ctx context.Context, id source.ID, secret string) error {
	return fake.setSecret(ctx, id, secret)
}

func (fake fakeBackend) GetSourceHealth(ctx context.Context, id source.ID) (source.Health, error) {
	return fake.getSourceHealth(ctx, id)
}

func (fake fakeBackend) CreateFolder(context.Context, string) (organization.Folder, error) {
	return organization.Folder{ID: "folder"}, nil
}
func (fake fakeBackend) ListFolders(ctx context.Context) ([]organization.Folder, error) {
	if fake.listFolders != nil {
		return fake.listFolders(ctx)
	}
	return []organization.Folder{}, nil
}
func (fake fakeBackend) DeleteFolder(context.Context, string) error                      { return nil }
func (fake fakeBackend) AddSourceToFolder(context.Context, string, source.ID) error      { return nil }
func (fake fakeBackend) RemoveSourceFromFolder(context.Context, string, source.ID) error { return nil }
func (fake fakeBackend) CreateView(_ context.Context, view organization.View) (organization.View, error) {
	view.ID = "view"
	return view, nil
}
func (fake fakeBackend) UpdateView(_ context.Context, view organization.View) (organization.View, error) {
	return view, nil
}
func (fake fakeBackend) ListViews(context.Context) ([]organization.View, error) {
	return []organization.View{}, nil
}
func (fake fakeBackend) DeleteView(context.Context, string) error { return nil }

func (fake fakeBackend) Enqueue(
	ctx context.Context,
	request ingestion.EnqueueRequest,
) (ingestion.Acquisition, error) {
	return fake.enqueue(ctx, request)
}

func (fake fakeBackend) ListEntries(ctx context.Context, limit int) ([]entry.Entry, error) {
	return fake.listEntries(ctx, limit)
}

func (fake fakeBackend) SearchEntries(ctx context.Context, query entry.Query) ([]entry.Entry, error) {
	return fake.searchEntries(ctx, query)
}

func (fake fakeBackend) GetEntry(ctx context.Context, id entry.ID) (entry.Entry, error) {
	return fake.getEntry(ctx, id)
}

func (fake fakeBackend) UpdateEntry(ctx context.Context, id entry.ID, patch entry.Patch) (entry.Entry, error) {
	return fake.updateEntry(ctx, id, patch)
}

func (fake fakeBackend) MarkEntriesRead(ctx context.Context, sourceID source.ID) (int64, error) {
	return fake.markEntriesRead(ctx, sourceID)
}

func (fake fakeBackend) AddEntryTag(ctx context.Context, id entry.ID, name string) (entry.Tag, error) {
	return fake.addEntryTag(ctx, id, name)
}

func (fake fakeBackend) RemoveEntryTag(ctx context.Context, id entry.ID, tagID string) error {
	return fake.removeEntryTag(ctx, id, tagID)
}

func (fake fakeBackend) ImportOPML(
	ctx context.Context,
	subscriptions []opml.Subscription,
) (opml.ImportResult, error) {
	return fake.importOPML(ctx, subscriptions)
}

func (fake fakeBackend) ExportOPML(ctx context.Context) ([]opml.Subscription, error) {
	return fake.exportOPML(ctx)
}

func (fake fakeBackend) PreviewSource(ctx context.Context, spec source.Spec) (preview.Result, error) {
	return fake.previewSource(ctx, spec)
}
func (fake fakeBackend) CreateRule(_ context.Context, definition rule.Rule) (rule.Rule, error) {
	definition.ID = "rule"
	return definition, nil
}
func (fake fakeBackend) ListRules(context.Context) ([]rule.Rule, error) {
	return []rule.Rule{}, nil
}
func (fake fakeBackend) GetRule(context.Context, string) (rule.Rule, error) {
	return rule.Rule{ID: "rule"}, nil
}
func (fake fakeBackend) UpdateRule(_ context.Context, definition rule.Rule) (rule.Rule, error) {
	return definition, nil
}
func (fake fakeBackend) DeleteRule(context.Context, string) error { return nil }
func (fake fakeBackend) PreviewRule(context.Context, string) (rule.PreviewResult, error) {
	return rule.PreviewResult{}, nil
}
func (fake fakeBackend) ReplayRule(ctx context.Context, id string, effects bool) (rule.ReplayResult, error) {
	return fake.replayRule(ctx, id, effects)
}

func TestReplayRuleDefaultsEffectsOff(t *testing.T) {
	backend := completeFakeBackend()
	backend.replayRule = func(_ context.Context, id string, effects bool) (rule.ReplayResult, error) {
		if id != "rule-1" || effects {
			t.Errorf("ReplayRule(%q, %v)", id, effects)
		}
		return rule.ReplayResult{Evaluated: 2, Matched: 1}, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodPost, "/api/v1/rules/rule-1/replay", nil,
	))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"matched":1`) {
		t.Errorf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestListFoldersEncodesEmptyArrayForNilResult(t *testing.T) {
	backend := completeFakeBackend()
	backend.listFolders = func(context.Context) ([]organization.Folder, error) {
		return nil, nil
	}
	response := httptest.NewRecorder()

	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/api/v1/folders", nil,
	))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Body.String() != "[]\n" {
		t.Errorf("body = %q, want an empty JSON array", response.Body.String())
	}
}

func TestExportEntryMarkdown(t *testing.T) {
	backend := completeFakeBackend()
	backend.getEntry = func(context.Context, entry.ID) (entry.Entry, error) {
		return entry.Entry{
			ID: "entry-1", SourceTitle: "A: title", Author: "Alice",
			CanonicalURL: "https://example.com/item", ContentHTML: "<p>Hello <strong>world</strong></p>",
		}, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/api/v1/entries/entry-1/export.md", nil,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "# A: title") ||
		!strings.Contains(response.Body.String(), "Hello world") {
		t.Errorf("markdown = %s", response.Body.String())
	}
	if got := response.Header().Get("Content-Disposition"); !strings.Contains(got, "entry-1.md") {
		t.Errorf("Content-Disposition = %q", got)
	}
}

func TestCreateSource(t *testing.T) {
	backend := completeFakeBackend()
	backend.createSource = func(_ context.Context, spec source.Spec) (source.Source, error) {
		if spec.Name != "Example" || spec.Kind != source.KindRSS {
			t.Errorf("unexpected spec: %+v", spec)
		}
		return source.Source{ID: "source-1", Name: spec.Name, Kind: spec.Kind, Enabled: true}, nil
	}
	body := bytes.NewBufferString(`{
		"name": "Example",
		"kind": "rss",
		"locator": "https://example.com/feed"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", body)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	NewHandler(backend).ServeHTTP(response, req)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var created source.Source
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID != "source-1" {
		t.Errorf("ID = %q", created.ID)
	}
}

func TestDeleteSourceArchivesIt(t *testing.T) {
	backend := completeFakeBackend()
	var archived source.ID
	backend.archiveSource = func(_ context.Context, id source.ID) error {
		archived = id
		return nil
	}
	response := httptest.NewRecorder()

	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodDelete, "/api/v1/sources/source-1", nil,
	))

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if archived != "source-1" {
		t.Errorf("archived source = %q", archived)
	}
}

func TestPreviewSource(t *testing.T) {
	backend := completeFakeBackend()
	backend.previewSource = func(_ context.Context, spec source.Spec) (preview.Result, error) {
		if spec.Kind != source.KindRSS || spec.Locator != "https://example.com/feed" {
			t.Errorf("spec = %+v", spec)
		}
		return preview.Result{Candidates: []preview.Candidate{{
			Title: "First", IdentityKey: "external:item-1",
		}}}, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPost, "/api/v1/sources/preview", bytes.NewBufferString(
			`{"name":"Example","kind":"rss","locator":"https://example.com/feed"}`,
		)),
	)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("external:item-1")) {
		t.Errorf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestCreateSourceReturnsValidationProblem(t *testing.T) {
	backend := completeFakeBackend()
	backend.createSource = func(context.Context, source.Spec) (source.Source, error) {
		return source.Source{}, &source.ValidationError{Field: "locator", Message: "invalid"}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources",
		bytes.NewBufferString(`{"name":"Bad","kind":"rss","locator":"bad"}`))
	response := httptest.NewRecorder()

	NewHandler(backend).ServeHTTP(response, req)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", response.Code)
	}
}

func TestRunSourceEnqueuesManualAcquisition(t *testing.T) {
	backend := completeFakeBackend()
	backend.getSource = func(context.Context, source.ID) (source.Source, error) {
		return source.Source{ID: "source-1", Enabled: true}, nil
	}
	backend.enqueue = func(_ context.Context, request ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
		if request.SourceID != "source-1" || request.Trigger != ingestion.TriggerManual {
			t.Errorf("unexpected enqueue request: %+v", request)
		}
		return ingestion.Acquisition{ID: "acquisition-1", Status: ingestion.StatusPending}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources/source-1/runs", nil)
	response := httptest.NewRecorder()

	NewHandler(backend).ServeHTTP(response, req)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestListSourcesAndEntries(t *testing.T) {
	backend := completeFakeBackend()
	backend.listSources = func(context.Context) ([]source.Source, error) {
		return []source.Source{{ID: "source-1", Name: "One"}}, nil
	}
	backend.listEntries = func(_ context.Context, limit int) ([]entry.Entry, error) {
		if limit != 25 {
			t.Errorf("limit = %d, want 25", limit)
		}
		return []entry.Entry{{ID: "entry-1", SourceTitle: "Entry"}}, nil
	}
	backend.searchEntries = func(_ context.Context, query entry.Query) ([]entry.Entry, error) {
		if query.Limit != 25 || query.Offset != 50 {
			t.Errorf("query = %+v, want limit 25 offset 50", query)
		}
		return []entry.Entry{{ID: "entry-1", SourceTitle: "Entry"}}, nil
	}

	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/api/v1/sources", want: "source-1"},
		{path: "/api/v1/entries?limit=25&offset=50", want: "entry-1"},
	} {
		response := httptest.NewRecorder()
		NewHandler(backend).ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(test.want)) {
			t.Errorf("%s status = %d, body = %s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestReaderSearchAndPatch(t *testing.T) {
	backend := completeFakeBackend()
	backend.searchEntries = func(_ context.Context, query entry.Query) ([]entry.Entry, error) {
		if query.Search != "go" || query.State != "unread" || query.Tag != "tech" {
			t.Errorf("query = %+v", query)
		}
		return []entry.Entry{{ID: "entry-1", SourceTitle: "Go"}}, nil
	}
	backend.updateEntry = func(_ context.Context, id entry.ID, patch entry.Patch) (entry.Entry, error) {
		if id != "entry-1" || patch.Read == nil || !*patch.Read ||
			patch.Starred == nil || !*patch.Starred {
			t.Errorf("patch %s = %+v", id, patch)
		}
		return entry.Entry{ID: id, SourceTitle: "Go"}, nil
	}
	searchResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		searchResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/entries?q=go&state=unread&tag=tech", nil),
	)
	if searchResponse.Code != http.StatusOK || !bytes.Contains(searchResponse.Body.Bytes(), []byte("entry-1")) {
		t.Errorf("search status = %d, body = %s", searchResponse.Code, searchResponse.Body.String())
	}
	patchResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		patchResponse,
		httptest.NewRequest(
			http.MethodPatch,
			"/api/v1/entries/entry-1",
			bytes.NewBufferString(`{"read":true,"starred":true}`),
		),
	)
	if patchResponse.Code != http.StatusOK {
		t.Errorf("patch status = %d, body = %s", patchResponse.Code, patchResponse.Body.String())
	}
}

func TestMarkEntriesReadScopesToSourceWhenRequested(t *testing.T) {
	backend := completeFakeBackend()
	backend.markEntriesRead = func(_ context.Context, sourceID source.ID) (int64, error) {
		if sourceID != "source-1" {
			t.Errorf("source id = %q", sourceID)
		}
		return 7, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		response,
		httptest.NewRequest(
			http.MethodPatch,
			"/api/v1/entries?source_id=source-1",
			bytes.NewBufferString(`{"read":true}`),
		),
	)
	if response.Code != http.StatusOK || response.Body.String() != "{\"updated_count\":7}\n" {
		t.Errorf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestListEntriesRejectsInvalidLimit(t *testing.T) {
	response := httptest.NewRecorder()
	NewHandler(completeFakeBackend()).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/entries?limit=500", nil),
	)
	if response.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", response.Code)
	}
}

func TestListEntriesFiltersBySource(t *testing.T) {
	backend := completeFakeBackend()
	backend.searchEntries = func(_ context.Context, query entry.Query) ([]entry.Entry, error) {
		if query.SourceID != "source-1" {
			t.Errorf("SourceID = %q", query.SourceID)
		}
		return []entry.Entry{}, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/api/v1/entries?source_id=source-1", nil,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestGetSource(t *testing.T) {
	backend := completeFakeBackend()
	backend.getSource = func(_ context.Context, id source.ID) (source.Source, error) {
		return source.Source{ID: id, Name: "Found"}, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/api/v1/sources/source-42", nil),
	)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("source-42")) {
		t.Errorf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSetSourceEnabled(t *testing.T) {
	backend := completeFakeBackend()
	backend.setEnabled = func(_ context.Context, id source.ID, enabled bool) (source.Source, error) {
		if id != "source-42" || enabled {
			t.Errorf("SetSourceEnabled(%q, %v)", id, enabled)
		}
		return source.Source{ID: id, Enabled: enabled}, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPatch, "/api/v1/sources/source-42", bytes.NewBufferString(`{"enabled":false}`)),
	)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"enabled":false`)) {
		t.Errorf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestUpdateSourceMetadata(t *testing.T) {
	backend := completeFakeBackend()
	backend.updateSource = func(_ context.Context, id source.ID, name, locator string) (source.Source, error) {
		if id != "source-42" || name != "Renamed" || locator != "https://example.com/new" {
			t.Errorf("UpdateSource(%q, %q, %q)", id, name, locator)
		}
		return source.Source{ID: id, Name: name, Locator: locator}, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		response,
		httptest.NewRequest(http.MethodPatch, "/api/v1/sources/source-42", bytes.NewBufferString(
			`{"name":"Renamed","locator":"https://example.com/new"}`,
		)),
	)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"name":"Renamed"`)) {
		t.Errorf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestWebhookValidatesSecretAndUsesStableIdempotency(t *testing.T) {
	backend := completeFakeBackend()
	backend.getSource = func(context.Context, source.ID) (source.Source, error) {
		return source.Source{
			ID: "source-hook", Kind: source.KindWebhook, Enabled: true, SecretRef: "top-secret",
		}, nil
	}
	var keys []string
	backend.enqueue = func(_ context.Context, request ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
		keys = append(keys, request.IdempotencyKey)
		if request.Trigger != ingestion.TriggerWebhook || !bytes.Contains(request.Payload, []byte(`"Pushed"`)) {
			t.Errorf("enqueue request = %+v", request)
		}
		return ingestion.Acquisition{ID: "hook-job", Status: ingestion.StatusPending}, nil
	}
	handler := NewHandler(backend)
	for range 2 {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/webhooks/source-hook",
			bytes.NewBufferString(`{"id":"one","title":"Pushed"}`),
		)
		request.Header.Set("X-Pulse-Webhook-Secret", "top-secret")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusAccepted {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] {
		t.Errorf("idempotency keys = %v", keys)
	}
}

func TestWebhookRejectsWrongSecret(t *testing.T) {
	backend := completeFakeBackend()
	backend.getSource = func(context.Context, source.ID) (source.Source, error) {
		return source.Source{
			ID: "source-hook", Kind: source.KindWebhook, Enabled: true, SecretRef: "correct",
		}, nil
	}
	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/webhooks/source-hook", bytes.NewBufferString(`{"title":"Pushed"}`),
	)
	request.Header.Set("X-Pulse-Webhook-Secret", "wrong")
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", response.Code)
	}
}

func TestManualEntryAndSecretRotation(t *testing.T) {
	backend := completeFakeBackend()
	backend.getSource = func(_ context.Context, id source.ID) (source.Source, error) {
		return source.Source{ID: id, Kind: source.KindManual, Enabled: true}, nil
	}
	backend.enqueue = func(_ context.Context, request ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
		return ingestion.Acquisition{ID: "manual-job", Status: ingestion.StatusPending}, nil
	}
	var rotated string
	backend.setSecret = func(_ context.Context, id source.ID, secret string) error {
		rotated = secret
		return nil
	}

	manualResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		manualResponse,
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/sources/manual-source/entries",
			bytes.NewBufferString(`{"title":"Manual entry"}`),
		),
	)
	if manualResponse.Code != http.StatusAccepted {
		t.Errorf("manual status = %d, body = %s", manualResponse.Code, manualResponse.Body.String())
	}

	secretResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		secretResponse,
		httptest.NewRequest(http.MethodPost, "/api/v1/sources/manual-source/secret", nil),
	)
	if secretResponse.Code != http.StatusOK || !strings.HasPrefix(rotated, "sha256:") ||
		bytes.Contains(secretResponse.Body.Bytes(), []byte(rotated)) {
		t.Errorf("secret status = %d, rotated = %q, body = %s",
			secretResponse.Code, rotated, secretResponse.Body.String())
	}
}

func TestManualEntryRejectsUnsafeSnapshotURLBeforeEnqueue(t *testing.T) {
	backend := completeFakeBackend()
	backend.getSource = func(_ context.Context, id source.ID) (source.Source, error) {
		return source.Source{ID: id, Kind: source.KindManual, Enabled: true}, nil
	}
	backend.enqueue = func(context.Context, ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
		t.Fatal("unsafe URL must not be enqueued")
		return ingestion.Acquisition{}, nil
	}

	for _, value := range []string{
		`{"title":"Unsafe","url":"javascript:alert(1)"}`,
		`{"title":"Credentialed","url":"https://user:secret@example.com/article"}`,
	} {
		response := httptest.NewRecorder()
		NewHandler(backend).ServeHTTP(
			response,
			httptest.NewRequest(
				http.MethodPost,
				"/api/v1/sources/manual-source/entries",
				bytes.NewBufferString(value),
			),
		)
		if response.Code != http.StatusBadRequest {
			t.Errorf("payload %s status = %d, body = %s", value, response.Code, response.Body.String())
		}
	}
}

func TestAnnotationImportQueuesWholeBatch(t *testing.T) {
	backend := completeFakeBackend()
	backend.getSource = func(_ context.Context, id source.ID) (source.Source, error) {
		return source.Source{ID: id, Kind: source.KindAnnotations, Enabled: true}, nil
	}
	var queued ingestion.EnqueueRequest
	backend.enqueue = func(_ context.Context, request ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
		queued = request
		return ingestion.Acquisition{ID: "annotation-job", Status: ingestion.StatusPending}, nil
	}
	body := `{"annotations":[{"provider":"kindle","book_title":"Deep Work","highlight":"Focus."}]}`
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sources/annotation-source/annotations",
		bytes.NewBufferString(body),
	)
	request.Header.Set("Idempotency-Key", "annotation-import-1")
	response := httptest.NewRecorder()

	NewHandler(backend).ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if queued.SourceID != "annotation-source" || queued.Trigger != ingestion.TriggerImport ||
		queued.IdempotencyKey != "annotation-import-1" || string(queued.Payload) != body {
		t.Errorf("queued = %#v", queued)
	}
}

func TestAnnotationImportRejectsWrongSourceKind(t *testing.T) {
	backend := completeFakeBackend()
	backend.getSource = func(_ context.Context, id source.ID) (source.Source, error) {
		return source.Source{ID: id, Kind: source.KindManual, Enabled: true}, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		response,
		httptest.NewRequest(
			http.MethodPost,
			"/api/v1/sources/manual-source/annotations",
			bytes.NewBufferString(`{"annotations":[]}`),
		),
	)
	if response.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", response.Code)
	}
}

func TestAnnotationImportRejectsInvalidBatchBeforeQueueing(t *testing.T) {
	backend := completeFakeBackend()
	backend.getSource = func(_ context.Context, id source.ID) (source.Source, error) {
		return source.Source{ID: id, Kind: source.KindAnnotations, Enabled: true}, nil
	}
	queued := false
	backend.enqueue = func(_ context.Context, request ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
		queued = true
		return ingestion.Acquisition{}, nil
	}
	for _, body := range []string{
		`{"annotations":[]}`,
		`{"annotations":[{"provider":"kindle","book_title":"","highlight":"text"}]}`,
		`{"annotations":[{"provider":"` + strings.Repeat("x", 65) + `","book_title":"Book","highlight":"text"}]}`,
	} {
		response := httptest.NewRecorder()
		NewHandler(backend).ServeHTTP(
			response,
			httptest.NewRequest(
				http.MethodPost,
				"/api/v1/sources/annotation-source/annotations",
				bytes.NewBufferString(body),
			),
		)
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %q status = %d, want 400", body, response.Code)
		}
	}
	if queued {
		t.Fatal("invalid annotation batch was queued")
	}
}

func TestHTTPDomainErrors(t *testing.T) {
	tests := []struct {
		name       string
		request    *http.Request
		configure  func(*fakeBackend)
		wantStatus int
	}{
		{
			name:       "invalid JSON",
			request:    httptest.NewRequest(http.MethodPost, "/api/v1/sources", bytes.NewBufferString(`{`)),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:    "duplicate source",
			request: httptest.NewRequest(http.MethodPost, "/api/v1/sources", bytes.NewBufferString(`{"name":"x","kind":"manual","locator":"x"}`)),
			configure: func(backend *fakeBackend) {
				backend.createSource = func(context.Context, source.Spec) (source.Source, error) {
					return source.Source{}, source.ErrDuplicate
				}
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "missing source",
			request:    httptest.NewRequest(http.MethodGet, "/api/v1/sources/missing", nil),
			wantStatus: http.StatusNotFound,
		},
		{
			name:    "source list failure",
			request: httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil),
			configure: func(backend *fakeBackend) {
				backend.listSources = func(context.Context) ([]source.Source, error) {
					return nil, errors.New("database unavailable")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:    "entry list failure",
			request: httptest.NewRequest(http.MethodGet, "/api/v1/entries", nil),
			configure: func(backend *fakeBackend) {
				backend.searchEntries = func(context.Context, entry.Query) ([]entry.Entry, error) {
					return nil, errors.New("database unavailable")
				}
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := completeFakeBackend()
			if test.configure != nil {
				test.configure(&backend)
			}
			response := httptest.NewRecorder()
			NewHandler(backend).ServeHTTP(response, test.request)
			if response.Code != test.wantStatus {
				t.Errorf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestOPMLImportAndExport(t *testing.T) {
	backend := completeFakeBackend()
	backend.importOPML = func(_ context.Context, subscriptions []opml.Subscription) (opml.ImportResult, error) {
		if len(subscriptions) != 1 || subscriptions[0].Folders[0] != "Tech" {
			t.Errorf("subscriptions = %+v", subscriptions)
		}
		return opml.ImportResult{CreatedSources: 1, CreatedFolders: 1}, nil
	}
	backend.exportOPML = func(context.Context) ([]opml.Subscription, error) {
		return []opml.Subscription{{
			Title: "Example", FeedURL: "https://example.com/feed", Folders: []string{"Tech"},
		}}, nil
	}

	importRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/opml/import",
		bytes.NewBufferString(`<opml><body><outline text="Tech"><outline text="Example" xmlUrl="https://example.com/feed"/></outline></body></opml>`),
	)
	importResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(importResponse, importRequest)
	if importResponse.Code != http.StatusOK || !bytes.Contains(importResponse.Body.Bytes(), []byte(`"created_sources":1`)) {
		t.Errorf("import status = %d, body = %s", importResponse.Code, importResponse.Body.String())
	}

	exportResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		exportResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/opml/export", nil),
	)
	if exportResponse.Code != http.StatusOK ||
		exportResponse.Header().Get("Content-Type") != "text/x-opml; charset=utf-8" ||
		!bytes.Contains(exportResponse.Body.Bytes(), []byte(`xmlUrl="https://example.com/feed"`)) {
		t.Errorf("export status = %d, headers = %v, body = %s",
			exportResponse.Code, exportResponse.Header(), exportResponse.Body.String())
	}
}

func completeFakeBackend() fakeBackend {
	return fakeBackend{
		createSource: func(context.Context, source.Spec) (source.Source, error) {
			return source.Source{}, errors.New("unexpected CreateSource")
		},
		listSources: func(context.Context) ([]source.Source, error) {
			return nil, nil
		},
		getSource: func(context.Context, source.ID) (source.Source, error) {
			return source.Source{}, source.ErrNotFound
		},
		setEnabled: func(context.Context, source.ID, bool) (source.Source, error) {
			return source.Source{}, errors.New("unexpected SetSourceEnabled")
		},
		archiveSource: func(context.Context, source.ID) error {
			return errors.New("unexpected ArchiveSource")
		},
		setSecret: func(context.Context, source.ID, string) error {
			return errors.New("unexpected SetSourceSecret")
		},
		getSourceHealth: func(context.Context, source.ID) (source.Health, error) {
			return source.Health{}, errors.New("unexpected GetSourceHealth")
		},
		enqueue: func(context.Context, ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
			return ingestion.Acquisition{}, errors.New("unexpected Enqueue")
		},
		listEntries: func(context.Context, int) ([]entry.Entry, error) {
			return nil, nil
		},
		searchEntries: func(context.Context, entry.Query) ([]entry.Entry, error) {
			return nil, nil
		},
		getEntry: func(context.Context, entry.ID) (entry.Entry, error) {
			return entry.Entry{}, entry.ErrNotFound
		},
		updateEntry: func(context.Context, entry.ID, entry.Patch) (entry.Entry, error) {
			return entry.Entry{}, errors.New("unexpected UpdateEntry")
		},
		markEntriesRead: func(context.Context, source.ID) (int64, error) {
			return 0, errors.New("unexpected MarkEntriesRead")
		},
		addEntryTag: func(context.Context, entry.ID, string) (entry.Tag, error) {
			return entry.Tag{}, errors.New("unexpected AddEntryTag")
		},
		removeEntryTag: func(context.Context, entry.ID, string) error {
			return errors.New("unexpected RemoveEntryTag")
		},
		importOPML: func(context.Context, []opml.Subscription) (opml.ImportResult, error) {
			return opml.ImportResult{}, errors.New("unexpected ImportOPML")
		},
		exportOPML: func(context.Context) ([]opml.Subscription, error) {
			return nil, errors.New("unexpected ExportOPML")
		},
		previewSource: func(context.Context, source.Spec) (preview.Result, error) {
			return preview.Result{}, errors.New("unexpected PreviewSource")
		},
	}
}
