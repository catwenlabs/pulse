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

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/opml"
	"github.com/catwenlabs/pulse/internal/organization"
	"github.com/catwenlabs/pulse/internal/preview"
	"github.com/catwenlabs/pulse/internal/rule"
	"github.com/catwenlabs/pulse/internal/source"
	"github.com/catwenlabs/pulse/internal/story"
)

type fakeBackend struct {
	createSource         func(context.Context, source.Spec) (source.Source, error)
	listSources          func(context.Context) ([]source.Source, error)
	getSource            func(context.Context, source.ID) (source.Source, error)
	updateSource         func(context.Context, source.ID, string, string) (source.Source, error)
	setEnabled           func(context.Context, source.ID, bool) (source.Source, error)
	archiveSource        func(context.Context, source.ID) error
	setSecret            func(context.Context, source.ID, string) error
	getSourceHealth      func(context.Context, source.ID) (source.Health, error)
	listFolders          func(context.Context) ([]organization.Folder, error)
	reorderRootSources   func(context.Context, []source.ID) error
	reorderFolders       func(context.Context, []string) error
	reorderFolderSources func(context.Context, string, []source.ID) error
	enqueue              func(context.Context, ingestion.EnqueueRequest) (ingestion.Acquisition, error)
	listSourceEntries    func(context.Context, source.ID, entry.Query) ([]story.SourceEntry, error)
	listSourceEntryPage  func(context.Context, source.ID, entry.Query) (story.SourceEntryPage, error)
	getEntry             func(context.Context, entry.ID) (entry.Entry, error)
	deleteEntry          func(context.Context, entry.ID, bool) error
	listStories          func(context.Context, story.Query) ([]story.Story, error)
	listStoryPage        func(context.Context, story.Query) (story.Page, error)
	getStory             func(context.Context, story.ID) (story.Story, error)
	updateStory          func(context.Context, story.ID, story.Patch) (story.Story, error)
	setRepresentative    func(context.Context, story.ID, entry.ID) (story.Story, error)
	markStoriesRead      func(context.Context, string) (int64, error)
	mergeStories         func(context.Context, story.ID, story.ID) (story.Story, error)
	splitStory           func(context.Context, story.ID, entry.ID) (story.Story, error)
	recluster            func(context.Context) (int, error)
	importOPML           func(context.Context, []opml.Subscription) (opml.ImportResult, error)
	exportOPML           func(context.Context) ([]opml.Subscription, error)
	previewSource        func(context.Context, source.Spec) (preview.Result, error)
	replayRule           func(context.Context, string, bool) (rule.ReplayResult, error)
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
func (fake fakeBackend) ReorderRootSources(ctx context.Context, ids []source.ID) error {
	if fake.reorderRootSources != nil {
		return fake.reorderRootSources(ctx, ids)
	}
	return nil
}
func (fake fakeBackend) ReorderFolders(ctx context.Context, ids []string) error {
	if fake.reorderFolders != nil {
		return fake.reorderFolders(ctx, ids)
	}
	return nil
}
func (fake fakeBackend) ReorderFolderSources(ctx context.Context, folderID string, ids []source.ID) error {
	if fake.reorderFolderSources != nil {
		return fake.reorderFolderSources(ctx, folderID, ids)
	}
	return nil
}
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

func (fake fakeBackend) ListSourceEntries(ctx context.Context, sourceID source.ID, query entry.Query) ([]story.SourceEntry, error) {
	if fake.listSourceEntries != nil {
		return fake.listSourceEntries(ctx, sourceID, query)
	}
	return []story.SourceEntry{}, nil
}

func (fake fakeBackend) ListSourceEntryPage(ctx context.Context, sourceID source.ID, query entry.Query) (story.SourceEntryPage, error) {
	if fake.listSourceEntryPage != nil {
		return fake.listSourceEntryPage(ctx, sourceID, query)
	}
	items, err := fake.ListSourceEntries(ctx, sourceID, query)
	return story.SourceEntryPage{Entries: items, TotalEntries: len(items)}, err
}

func (fake fakeBackend) GetEntry(ctx context.Context, id entry.ID) (entry.Entry, error) {
	return fake.getEntry(ctx, id)
}

func (fake fakeBackend) DeleteEntry(ctx context.Context, id entry.ID, confirmed bool) error {
	if fake.deleteEntry != nil {
		return fake.deleteEntry(ctx, id, confirmed)
	}
	return nil
}

func (fake fakeBackend) ListStories(ctx context.Context, query story.Query) ([]story.Story, error) {
	return fake.listStories(ctx, query)
}

func (fake fakeBackend) ListStoryPage(ctx context.Context, query story.Query) (story.Page, error) {
	if fake.listStoryPage != nil {
		return fake.listStoryPage(ctx, query)
	}
	items, err := fake.ListStories(ctx, query)
	return story.Page{Stories: items, TotalStories: len(items)}, err
}

func (fake fakeBackend) GetStory(ctx context.Context, id story.ID) (story.Story, error) {
	return fake.getStory(ctx, id)
}

func (fake fakeBackend) UpdateStory(
	ctx context.Context,
	id story.ID,
	patch story.Patch,
) (story.Story, error) {
	return fake.updateStory(ctx, id, patch)
}

func (fake fakeBackend) SetStoryRepresentative(ctx context.Context, storyID story.ID, entryID entry.ID) (story.Story, error) {
	if fake.setRepresentative != nil {
		return fake.setRepresentative(ctx, storyID, entryID)
	}
	return story.Story{ID: storyID, Representative: entry.Entry{ID: entryID}}, nil
}

func (fake fakeBackend) MarkStoriesRead(ctx context.Context, sourceID string) (int64, error) {
	return fake.markStoriesRead(ctx, sourceID)
}

func (fake fakeBackend) MergeStories(
	ctx context.Context,
	from story.ID,
	into story.ID,
	_ story.MergeOptions,
) (story.Story, error) {
	return fake.mergeStories(ctx, from, into)
}

func (fake fakeBackend) SplitStory(
	ctx context.Context,
	storyID story.ID,
	entryID entry.ID,
	_ story.SplitOptions,
) (story.Story, error) {
	return fake.splitStory(ctx, storyID, entryID)
}

func (fake fakeBackend) AddStoryTag(_ context.Context, _ story.ID, name string) (entry.Tag, error) {
	return entry.Tag{ID: "tag", Name: name}, nil
}

func (fake fakeBackend) RemoveStoryTag(context.Context, story.ID, string) error { return nil }

func (fake fakeBackend) Recluster(ctx context.Context) (int, error) {
	return fake.recluster(ctx)
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

func TestReorderNavigation(t *testing.T) {
	backend := completeFakeBackend()
	backend.reorderRootSources = func(_ context.Context, ids []source.ID) error {
		if got := []source.ID{"source-2", "source-1"}; len(ids) != len(got) || ids[0] != got[0] || ids[1] != got[1] {
			t.Errorf("root Source IDs = %v, want %v", ids, got)
		}
		return nil
	}
	backend.reorderFolders = func(_ context.Context, ids []string) error {
		if got := []string{"folder-2", "folder-1"}; len(ids) != len(got) || ids[0] != got[0] || ids[1] != got[1] {
			t.Errorf("Folder IDs = %v, want %v", ids, got)
		}
		return nil
	}
	backend.reorderFolderSources = func(_ context.Context, folderID string, ids []source.ID) error {
		if folderID != "folder-1" {
			t.Errorf("folder ID = %q, want folder-1", folderID)
		}
		if got := []source.ID{"source-2", "source-1"}; len(ids) != len(got) || ids[0] != got[0] || ids[1] != got[1] {
			t.Errorf("Folder Source IDs = %v, want %v", ids, got)
		}
		return nil
	}

	requests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPut, "/api/v1/sources/order", `{"source_ids":["source-2","source-1"]}`},
		{http.MethodPut, "/api/v1/folders/order", `{"folder_ids":["folder-2","folder-1"]}`},
		{http.MethodPut, "/api/v1/folders/folder-1/sources/order", `{"source_ids":["source-2","source-1"]}`},
	}
	for _, test := range requests {
		response := httptest.NewRecorder()
		NewHandler(backend).ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code != http.StatusNoContent {
			t.Errorf("%s %s status = %d, body = %s", test.method, test.path, response.Code, response.Body.String())
		}
	}
}

func TestReorderNavigationMapsValidationErrors(t *testing.T) {
	backend := completeFakeBackend()
	backend.reorderRootSources = func(context.Context, []source.ID) error {
		return &organization.OrderValidationError{Field: "source_ids", Message: "must contain every root Source exactly once"}
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodPut,
		"/api/v1/sources/order",
		strings.NewReader(`{"source_ids":["source-1"]}`),
	))
	if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"field":"source_ids"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
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

func TestPreviewSourceMapsFetchError(t *testing.T) {
	backend := completeFakeBackend()
	backend.previewSource = func(context.Context, source.Spec) (preview.Result, error) {
		return preview.Result{}, errors.Join(ingestion.ErrFetch, errors.New("dial: connection refused"))
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(http.MethodPost,
		"/api/v1/sources/preview", bytes.NewBufferString(
			`{"name":"X","kind":"rss","locator":"https://example.com/feed"}`)))
	if response.Code != http.StatusBadGateway ||
		!bytes.Contains(response.Body.Bytes(), []byte("source_fetch_failed")) ||
		!bytes.Contains(response.Body.Bytes(), []byte("connection refused")) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestPreviewSourceMapsParseError(t *testing.T) {
	backend := completeFakeBackend()
	backend.previewSource = func(context.Context, source.Spec) (preview.Result, error) {
		return preview.Result{}, errors.Join(ingestion.ErrParse, errors.New("invalid XML"))
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(http.MethodPost,
		"/api/v1/sources/preview", bytes.NewBufferString(
			`{"name":"X","kind":"rss","locator":"https://example.com/feed"}`)))
	if response.Code != http.StatusUnprocessableEntity ||
		!bytes.Contains(response.Body.Bytes(), []byte("source_parse_failed")) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
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
	backend.listSourceEntries = func(_ context.Context, sourceID source.ID, query entry.Query) ([]story.SourceEntry, error) {
		if sourceID != "source-1" || query.Limit != 25 || query.Cursor != "cursor-1" {
			t.Errorf("query = %+v, want limit 25 cursor-1", query)
		}
		return []story.SourceEntry{{Entry: entry.Entry{ID: "entry-1", SourceTitle: "Entry"}, Story: story.StoryRef{ID: "story-1"}}}, nil
	}

	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/api/v1/sources", want: "source-1"},
		{path: "/api/v1/sources/source-1/entries?limit=25&cursor=cursor-1", want: "entry-1"},
	} {
		response := httptest.NewRecorder()
		NewHandler(backend).ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(test.want)) {
			t.Errorf("%s status = %d, body = %s", test.path, response.Code, response.Body.String())
		}
	}
}

func TestListSourcesExposesUnreadCount(t *testing.T) {
	backend := completeFakeBackend()
	backend.listSources = func(context.Context) ([]source.Source, error) {
		return []source.Source{{ID: "source-1", Name: "One", UnreadCount: 7}}, nil
	}

	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/sources", nil))

	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"unread_count":7`)) {
		t.Errorf("status = %d, body = %s, want unread_count in response", response.Code, response.Body.String())
	}
}

func TestListAndGetStories(t *testing.T) {
	backend := completeFakeBackend()
	backend.listStories = func(_ context.Context, query story.Query) ([]story.Story, error) {
		if query.Limit != 25 || query.Cursor != "cursor-1" || query.SourceID != "source-1" {
			t.Errorf("query = %+v", query)
		}
		return []story.Story{{
			ID:             "story-1",
			Representative: entry.Entry{ID: "entry-1", SourceTitle: "聚合新闻"},
			EntryCount:     2,
			SourceCount:    2,
		}}, nil
	}
	backend.getStory = func(_ context.Context, id story.ID) (story.Story, error) {
		return story.Story{
			ID:         id,
			Entries:    []entry.Entry{{ID: "entry-1"}, {ID: "entry-2"}},
			EntryCount: 2,
		}, nil
	}

	listResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(listResponse, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/stories?limit=25&cursor=cursor-1&source_id=source-1",
		nil,
	))
	if listResponse.Code != http.StatusOK ||
		!strings.Contains(listResponse.Body.String(), `"source_count":2`) {
		t.Fatalf("list status = %d, body = %s", listResponse.Code, listResponse.Body.String())
	}

	getResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(getResponse, httptest.NewRequest(
		http.MethodGet,
		"/api/v1/stories/story-1",
		nil,
	))
	if getResponse.Code != http.StatusOK ||
		!strings.Contains(getResponse.Body.String(), `"entry-2"`) {
		t.Fatalf("get status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
}

func TestMergeStories(t *testing.T) {
	backend := completeFakeBackend()
	var capturedFrom, capturedInto story.ID
	backend.mergeStories = func(_ context.Context, from, into story.ID) (story.Story, error) {
		capturedFrom, capturedInto = from, into
		return story.Story{ID: into, Representative: entry.Entry{ID: "entry-1"}, EntryCount: 2, SourceCount: 2}, nil
	}

	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/v1/stories/story-1/merge",
		bytes.NewBufferString(`{"into":"story-2"}`),
	))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"source_count":2`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if capturedFrom != "story-1" || capturedInto != "story-2" {
		t.Errorf("captured from = %q, into = %q", capturedFrom, capturedInto)
	}
}

func TestSplitStory(t *testing.T) {
	backend := completeFakeBackend()
	var capturedStory story.ID
	var capturedEntry entry.ID
	backend.splitStory = func(_ context.Context, storyID story.ID, entryID entry.ID) (story.Story, error) {
		capturedStory, capturedEntry = storyID, entryID
		return story.Story{ID: "story-2", Representative: entry.Entry{ID: "entry-2"}, EntryCount: 1, SourceCount: 1}, nil
	}

	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/v1/stories/story-1/split",
		bytes.NewBufferString(`{"entry_id":"entry-2"}`),
	))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"entry-2"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if capturedStory != "story-1" || capturedEntry != "entry-2" {
		t.Errorf("captured story = %q, entry = %q", capturedStory, capturedEntry)
	}
}

func TestMergeStoriesRejectsSelfMerge(t *testing.T) {
	backend := completeFakeBackend()
	backend.mergeStories = func(_ context.Context, from, into story.ID) (story.Story, error) {
		return story.Story{}, story.ErrSelfMerge
	}

	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodPost,
		"/api/v1/stories/story-1/merge",
		bytes.NewBufferString(`{"into":"story-1"}`),
	))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestReclusterStories(t *testing.T) {
	backend := completeFakeBackend()
	backend.recluster = func(context.Context) (int, error) {
		return 3, nil
	}

	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodPost, "/api/v1/stories/recluster", nil,
	))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"processed":3`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestReclusterStoriesUnavailable(t *testing.T) {
	backend := completeFakeBackend()
	// completeFakeBackend defaults recluster to ErrReclusterUnavailable → 503.

	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodPost, "/api/v1/stories/recluster", nil,
	))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestReaderSearchAndPatch(t *testing.T) {
	backend := completeFakeBackend()
	backend.listSourceEntries = func(_ context.Context, sourceID source.ID, query entry.Query) ([]story.SourceEntry, error) {
		if sourceID != "source-1" || query.Search != "go" || query.State != "unread" || query.Tag != "tech" {
			t.Errorf("query = %+v", query)
		}
		return []story.SourceEntry{{Entry: entry.Entry{ID: "entry-1", SourceTitle: "Go"}, Story: story.StoryRef{ID: "story-1"}}}, nil
	}
	backend.updateStory = func(_ context.Context, id story.ID, patch story.Patch) (story.Story, error) {
		if id != "story-1" || patch.Read == nil || !*patch.Read ||
			patch.Starred == nil || !*patch.Starred {
			t.Errorf("patch %s = %+v", id, patch)
		}
		return story.Story{ID: id}, nil
	}
	searchResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		searchResponse,
		httptest.NewRequest(http.MethodGet, "/api/v1/sources/source-1/entries?q=go&state=unread&tag=tech", nil),
	)
	if searchResponse.Code != http.StatusOK || !bytes.Contains(searchResponse.Body.Bytes(), []byte("entry-1")) {
		t.Errorf("search status = %d, body = %s", searchResponse.Code, searchResponse.Body.String())
	}
	patchResponse := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(
		patchResponse,
		httptest.NewRequest(
			http.MethodPatch,
			"/api/v1/stories/story-1",
			bytes.NewBufferString(`{"read":true,"starred":true}`),
		),
	)
	if patchResponse.Code != http.StatusOK {
		t.Errorf("patch status = %d, body = %s", patchResponse.Code, patchResponse.Body.String())
	}
}

func TestMarkEntriesReadScopesToSourceWhenRequested(t *testing.T) {
	backend := completeFakeBackend()
	backend.markStoriesRead = func(_ context.Context, sourceID string) (int64, error) {
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
			"/api/v1/stories?source_id=source-1",
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
		httptest.NewRequest(http.MethodGet, "/api/v1/sources/source-1/entries?limit=500", nil),
	)
	if response.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", response.Code)
	}
}

func TestListEntriesFiltersBySource(t *testing.T) {
	backend := completeFakeBackend()
	backend.listSourceEntries = func(_ context.Context, sourceID source.ID, _ entry.Query) ([]story.SourceEntry, error) {
		if sourceID != "source-1" {
			t.Errorf("SourceID = %q", sourceID)
		}
		return []story.SourceEntry{}, nil
	}
	response := httptest.NewRecorder()
	NewHandler(backend).ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/api/v1/sources/source-1/entries", nil,
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
			request: httptest.NewRequest(http.MethodGet, "/api/v1/sources/source-1/entries", nil),
			configure: func(backend *fakeBackend) {
				backend.listSourceEntries = func(context.Context, source.ID, entry.Query) ([]story.SourceEntry, error) {
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
		getEntry: func(context.Context, entry.ID) (entry.Entry, error) {
			return entry.Entry{}, entry.ErrNotFound
		},
		listStories: func(context.Context, story.Query) ([]story.Story, error) {
			return nil, nil
		},
		getStory: func(context.Context, story.ID) (story.Story, error) {
			return story.Story{}, entry.ErrNotFound
		},
		updateStory: func(context.Context, story.ID, story.Patch) (story.Story, error) {
			return story.Story{}, errors.New("unexpected UpdateStory")
		},
		markStoriesRead: func(context.Context, string) (int64, error) {
			return 0, errors.New("unexpected MarkStoriesRead")
		},
		mergeStories: func(context.Context, story.ID, story.ID) (story.Story, error) {
			return story.Story{}, errors.New("unexpected MergeStories")
		},
		splitStory: func(context.Context, story.ID, entry.ID) (story.Story, error) {
			return story.Story{}, errors.New("unexpected SplitStory")
		},
		recluster: func(context.Context) (int, error) {
			return 0, story.ErrReclusterUnavailable
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
