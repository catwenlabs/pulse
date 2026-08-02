package httpserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/opml"
	"github.com/catwenlabs/pulse/internal/organization"
	"github.com/catwenlabs/pulse/internal/preview"
	"github.com/catwenlabs/pulse/internal/source"
	"github.com/catwenlabs/pulse/internal/story"
)

type fakeSourceRepository struct{}

func (fakeSourceRepository) Create(_ context.Context, spec source.Spec) (source.Source, error) {
	return source.Source{ID: "created", Name: spec.Name}, nil
}
func (fakeSourceRepository) List(context.Context) ([]source.Source, error) {
	return []source.Source{{ID: "listed"}}, nil
}
func (fakeSourceRepository) Get(_ context.Context, id source.ID) (source.Source, error) {
	return source.Source{ID: id, Kind: source.KindRSS, Locator: "https://example.com/old"}, nil
}
func (fakeSourceRepository) Update(_ context.Context, id source.ID, spec source.Spec) (source.Source, error) {
	return source.Source{ID: id, Name: spec.Name, Kind: spec.Kind, Locator: spec.Locator}, nil
}
func (fakeSourceRepository) SetEnabled(_ context.Context, id source.ID, enabled bool) error {
	return nil
}
func (fakeSourceRepository) Archive(context.Context, source.ID) error {
	return nil
}
func (fakeSourceRepository) SetSecretRef(context.Context, source.ID, string) error {
	return nil
}
func (fakeSourceRepository) Health(_ context.Context, id source.ID) (source.Health, error) {
	return source.Health{SourceID: id, Status: "ok"}, nil
}

type archivedSourceRepository struct {
	fakeSourceRepository
}

func (archivedSourceRepository) Get(_ context.Context, id source.ID) (source.Source, error) {
	archivedAt := time.Now()
	return source.Source{ID: id, ArchivedAt: &archivedAt}, nil
}

type fakeQueue struct{}

func (fakeQueue) Enqueue(_ context.Context, request ingestion.EnqueueRequest) (ingestion.Acquisition, error) {
	return ingestion.Acquisition{ID: "queued", SourceID: request.SourceID}, nil
}

type fakeEntries struct{}

func (fakeEntries) List(context.Context, int) ([]entry.Entry, error) {
	return []entry.Entry{{ID: "entry"}}, nil
}

type fakeStories struct{}

func (fakeStories) Search(context.Context, story.Query) ([]story.Story, error) {
	return []story.Story{{ID: "story"}}, nil
}

func (fakeStories) SearchPage(context.Context, story.Query) (story.Page, error) {
	return story.Page{Stories: []story.Story{{ID: "story"}}, TotalStories: 1}, nil
}

func (fakeStories) Get(_ context.Context, id story.ID) (story.Story, error) {
	return story.Story{ID: id}, nil
}

func (fakeStories) Update(_ context.Context, id story.ID, _ story.Patch) (story.Story, error) {
	return story.Story{ID: id}, nil
}

func (fakeStories) SetRepresentative(_ context.Context, id story.ID, _ entry.ID) (story.Story, error) {
	return story.Story{ID: id}, nil
}

func (fakeStories) MarkRead(context.Context, string) (int64, error) {
	return 1, nil
}
func (fakeStories) MergeManual(_ context.Context, from story.ID, into story.ID, _ story.MergeOptions) error {
	if from == into {
		return story.ErrSelfMerge
	}
	return nil
}
func (fakeStories) Split(_ context.Context, storyID story.ID, entryID entry.ID, _ story.SplitOptions) (story.ID, error) {
	return story.ID("split-" + string(entryID)), nil
}
func (fakeStories) AddTag(_ context.Context, _ story.ID, name string) (entry.Tag, error) {
	return entry.Tag{ID: "tag", Name: name}, nil
}
func (fakeStories) RemoveTag(context.Context, story.ID, string) error { return nil }
func (fakeEntries) Get(_ context.Context, id entry.ID) (entry.Entry, error) {
	return entry.Entry{ID: id}, nil
}
func (fakeEntries) Delete(context.Context, entry.ID, bool) error { return nil }

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
func (fakeOrganization) ReorderRootSources(context.Context, []source.ID) error { return nil }
func (fakeOrganization) ReorderFolders(context.Context, []string) error        { return nil }
func (fakeOrganization) ReorderFolderSources(context.Context, string, []source.ID) error {
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
		fakeStories{},
		nil,
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
	edited, err := backend.UpdateSource(ctx, "source", "Renamed", "https://example.com/new")
	if err != nil || edited.Name != "Renamed" || edited.Locator != "https://example.com/new" {
		t.Fatalf("UpdateSource() = %+v, %v", edited, err)
	}
	updated, err := backend.SetSourceEnabled(ctx, "source", false)
	if err != nil || updated.ID != "source" || updated.Enabled {
		t.Fatalf("SetSourceEnabled() = %+v, %v", updated, err)
	}
	if err := backend.SetSourceSecret(ctx, "source", "secret"); err != nil {
		t.Fatalf("SetSourceSecret() error = %v", err)
	}
	if err := backend.ArchiveSource(ctx, "source"); err != nil {
		t.Fatalf("ArchiveSource() error = %v", err)
	}
	if health, err := backend.GetSourceHealth(ctx, "source"); err != nil || health.Status != "ok" {
		t.Fatalf("GetSourceHealth() = %+v, %v", health, err)
	}
	queued, err := backend.Enqueue(ctx, ingestion.EnqueueRequest{SourceID: "source"})
	if err != nil || queued.ID != "queued" {
		t.Fatalf("Enqueue() = %+v, %v", queued, err)
	}
	if got, err := backend.GetEntry(ctx, "entry"); err != nil || got.ID != "entry" {
		t.Fatalf("GetEntry() = %+v, %v", got, err)
	}
	if stories, err := backend.ListStories(ctx, story.Query{Limit: 10}); err != nil || len(stories) != 1 {
		t.Fatalf("ListStories() = %+v, %v", stories, err)
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

func TestBackendHidesArchivedSource(t *testing.T) {
	backend := NewBackend(
		archivedSourceRepository{},
		fakeQueue{},
		fakeEntries{},
		fakeOPMLRepository{},
		fakePreviewer{},
		fakeOrganization{},
		fakeStories{},
		nil,
	)

	if _, err := backend.GetSource(context.Background(), "archived"); !errors.Is(err, source.ErrNotFound) {
		t.Fatalf("GetSource() error = %v, want ErrNotFound", err)
	}
}
