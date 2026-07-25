package httpserver

import (
	"context"
	"testing"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/opml"
	"github.com/wenpengfei/pulse/internal/organization"
	"github.com/wenpengfei/pulse/internal/preview"
	"github.com/wenpengfei/pulse/internal/source"
)

type fakeSourceRepository struct{}

func (fakeSourceRepository) Create(_ context.Context, spec source.Spec) (source.Source, error) {
	return source.Source{ID: "created", Name: spec.Name}, nil
}
func (fakeSourceRepository) List(context.Context) ([]source.Source, error) {
	return []source.Source{{ID: "listed"}}, nil
}
func (fakeSourceRepository) Get(_ context.Context, id source.ID) (source.Source, error) {
	return source.Source{ID: id}, nil
}
func (fakeSourceRepository) SetEnabled(_ context.Context, id source.ID, enabled bool) error {
	return nil
}
func (fakeSourceRepository) SetSecretRef(context.Context, source.ID, string) error {
	return nil
}
func (fakeSourceRepository) Health(_ context.Context, id source.ID) (source.Health, error) {
	return source.Health{SourceID: id, Status: "ok"}, nil
}

type fakeQueue struct{}

func (fakeQueue) Enqueue(_ context.Context, request ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
	return ingestion.Acquisition{ID: "queued", SourceID: request.SourceID}, nil
}

type fakeEntries struct{}

func (fakeEntries) List(context.Context, int) ([]entry.Entry, error) {
	return []entry.Entry{{ID: "entry"}}, nil
}
func (fakeEntries) Search(context.Context, entry.Query) ([]entry.Entry, error) {
	return []entry.Entry{{ID: "entry"}}, nil
}
func (fakeEntries) Get(_ context.Context, id entry.ID) (entry.Entry, error) {
	return entry.Entry{ID: id}, nil
}
func (fakeEntries) Update(_ context.Context, id entry.ID, _ entry.Patch) (entry.Entry, error) {
	return entry.Entry{ID: id}, nil
}
func (fakeEntries) AddTag(context.Context, entry.ID, string) (entry.Tag, error) {
	return entry.Tag{ID: "tag", Name: "Go"}, nil
}
func (fakeEntries) RemoveTag(context.Context, entry.ID, string) error {
	return nil
}

type fakeOPMLRepository struct{}

func (fakeOPMLRepository) Import(
	context.Context,
	[]opml.Subscription,
) (opml.ImportResult, error) {
	return opml.ImportResult{CreatedSources: 1}, nil
}

type fakePreviewer struct{}

func (fakePreviewer) Run(context.Context, source.Spec) (preview.Result, error) {
	return preview.Result{Candidates: []preview.Candidate{{IdentityKey: "external:one"}}}, nil
}

type fakeOrganization struct{}

func (fakeOrganization) CreateFolder(context.Context, string) (organization.Folder, error) {
	return organization.Folder{ID: "folder"}, nil
}
func (fakeOrganization) ListFolders(context.Context) ([]organization.Folder, error) {
	return []organization.Folder{{ID: "folder"}}, nil
}
func (fakeOrganization) DeleteFolder(context.Context, string) error { return nil }
func (fakeOrganization) AddSourceToFolder(context.Context, string, source.ID) error {
	return nil
}
func (fakeOrganization) RemoveSourceFromFolder(context.Context, string, source.ID) error {
	return nil
}
func (fakeOrganization) CreateView(context.Context, organization.View) (organization.View, error) {
	return organization.View{ID: "view"}, nil
}
func (fakeOrganization) UpdateView(_ context.Context, view organization.View) (organization.View, error) {
	return view, nil
}
func (fakeOrganization) ListViews(context.Context) ([]organization.View, error) {
	return []organization.View{{ID: "view"}}, nil
}
func (fakeOrganization) DeleteView(context.Context, string) error { return nil }
func (fakeOPMLRepository) List(context.Context) ([]opml.Subscription, error) {
	return []opml.Subscription{{FeedURL: "https://example.com/feed"}}, nil
}

func TestBackendForwardsOperations(t *testing.T) {
	backend := NewBackend(
		fakeSourceRepository{},
		fakeQueue{},
		fakeEntries{},
		fakeOPMLRepository{},
		fakePreviewer{},
		fakeOrganization{},
	)
	ctx := context.Background()

	created, err := backend.CreateSource(ctx, source.Spec{Name: "name"})
	if err != nil || created.ID != "created" {
		t.Fatalf("CreateSource() = %+v, %v", created, err)
	}
	sources, err := backend.ListSources(ctx)
	if err != nil || len(sources) != 1 {
		t.Fatalf("ListSources() = %+v, %v", sources, err)
	}
	got, err := backend.GetSource(ctx, "source")
	if err != nil || got.ID != "source" {
		t.Fatalf("GetSource() = %+v, %v", got, err)
	}
	updated, err := backend.SetSourceEnabled(ctx, "source", false)
	if err != nil || updated.ID != "source" || updated.Enabled {
		t.Fatalf("SetSourceEnabled() = %+v, %v", updated, err)
	}
	if err := backend.SetSourceSecret(ctx, "source", "secret"); err != nil {
		t.Fatalf("SetSourceSecret() error = %v", err)
	}
	if health, err := backend.GetSourceHealth(ctx, "source"); err != nil || health.Status != "ok" {
		t.Fatalf("GetSourceHealth() = %+v, %v", health, err)
	}
	queued, err := backend.Enqueue(ctx, ingestion.EnqueueRequest{SourceID: "source"})
	if err != nil || queued.ID != "queued" {
		t.Fatalf("Enqueue() = %+v, %v", queued, err)
	}
	entries, err := backend.ListEntries(ctx, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("ListEntries() = %+v, %v", entries, err)
	}
	if searched, err := backend.SearchEntries(ctx, entry.Query{Limit: 10}); err != nil || len(searched) != 1 {
		t.Fatalf("SearchEntries() = %+v, %v", searched, err)
	}
	if got, err := backend.GetEntry(ctx, "entry"); err != nil || got.ID != "entry" {
		t.Fatalf("GetEntry() = %+v, %v", got, err)
	}
	imported, err := backend.ImportOPML(ctx, []opml.Subscription{{FeedURL: "https://example.com/feed"}})
	if err != nil || imported.CreatedSources != 1 {
		t.Fatalf("ImportOPML() = %+v, %v", imported, err)
	}
	exported, err := backend.ExportOPML(ctx)
	if err != nil || len(exported) != 1 {
		t.Fatalf("ExportOPML() = %+v, %v", exported, err)
	}
	previewed, err := backend.PreviewSource(ctx, source.Spec{Kind: source.KindRSS})
	if err != nil || len(previewed.Candidates) != 1 {
		t.Fatalf("PreviewSource() = %+v, %v", previewed, err)
	}
}
