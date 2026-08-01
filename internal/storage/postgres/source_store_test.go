package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/source"
	"github.com/catwenlabs/pulse/internal/storage/migrate"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("PULSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PULSE_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := migrate.Run(context.Background(), pool); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	if _, err := pool.Exec(
		context.Background(),
		"TRUNCATE sources, folders, rules, tags, views CASCADE",
	); err != nil {
		t.Fatalf("truncate test data: %v", err)
	}
	return pool
}

func TestSourceStoreCreateAndGet(t *testing.T) {
	pool := testPool(t)
	store := NewSourceStore(pool)
	ctx := context.Background()

	created, err := store.Create(ctx, source.Spec{
		Name:    "Example",
		Kind:    source.KindRSS,
		Locator: "HTTPS://Example.com:443/feed",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("created ID is empty")
	}
	if !created.Enabled {
		t.Fatal("created source is disabled")
	}

	got, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.NormalizedLocator != "https://example.com/feed" {
		t.Errorf("NormalizedLocator = %q", got.NormalizedLocator)
	}
}

func TestSourceStoreRejectsDuplicate(t *testing.T) {
	pool := testPool(t)
	store := NewSourceStore(pool)
	ctx := context.Background()

	first := source.Spec{Name: "First", Kind: source.KindRSS, Locator: "https://example.com/feed"}
	second := source.Spec{Name: "Second", Kind: source.KindRSS, Locator: "HTTPS://EXAMPLE.COM:443/feed"}

	if _, err := store.Create(ctx, first); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if _, err := store.Create(ctx, second); !errors.Is(err, source.ErrDuplicate) {
		t.Fatalf("second Create() error = %v, want ErrDuplicate", err)
	}
}

func TestSourceStoreUpdate(t *testing.T) {
	pool := testPool(t)
	store := NewSourceStore(pool)
	ctx := context.Background()
	created := createTestSource(t, store, "before")

	updated, err := store.Update(ctx, created.ID, source.Spec{
		Name:    "After",
		Kind:    created.Kind,
		Locator: "https://example.com/after",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "After" || updated.Locator != "https://example.com/after" {
		t.Errorf("Update() = %+v", updated)
	}
}

func TestSourceStorePauseAndArchive(t *testing.T) {
	pool := testPool(t)
	store := NewSourceStore(pool)
	ctx := context.Background()

	created, err := store.Create(ctx, source.Spec{
		Name:    "Example",
		Kind:    source.KindManual,
		Locator: "reading-list",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.SetEnabled(ctx, created.ID, false); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if err := store.Archive(ctx, created.ID); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	got, err := store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Enabled {
		t.Error("archived source is enabled")
	}
	if got.ArchivedAt == nil {
		t.Error("ArchivedAt = nil")
	}
}

func TestSourceStoreListExcludesArchived(t *testing.T) {
	pool := testPool(t)
	store := NewSourceStore(pool)
	ctx := context.Background()
	active := createTestSource(t, store, "active")
	archived := createTestSource(t, store, "archived")
	if err := store.Archive(ctx, archived.ID); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != active.ID {
		t.Errorf("List() = %+v, want active source", got)
	}
}

func TestSourceStoreListIncludesUnreadEntryCount(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	ctx := context.Background()

	src := createTestSource(t, sourceStore, "unread-source")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "unread")
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{
		{ExternalID: "unread-one", Title: "Unread one"},
		{ExternalID: "unread-two", Title: "Unread two"},
		{ExternalID: "read-one", Title: "Read one"},
		{ExternalID: "hidden-one", Title: "Hidden one"},
	}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}

	// A second Source with its own unread Entries must not bleed into src's count.
	other := createTestSource(t, sourceStore, "other-source")
	otherAcquisition := claimTestAcquisition(t, acquisitionStore, other.ID, "other")
	if err := entryStore.CommitBatch(ctx, otherAcquisition, "worker", []ingestion.Candidate{
		{ExternalID: "other-unread-a", Title: "Other unread A"},
		{ExternalID: "other-unread-b", Title: "Other unread B"},
		{ExternalID: "other-unread-c", Title: "Other unread C"},
	}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("other CommitBatch() error = %v", err)
	}

	results, err := entryStore.Search(ctx, entry.Query{Limit: 10, SourceID: src.ID})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	byExternal := make(map[string]entry.Entry, len(results))
	for _, item := range results {
		byExternal[item.ExternalID] = item
	}
	yes := true
	if _, err := entryStore.Update(ctx, byExternal["read-one"].ID, entry.Patch{Read: &yes}); err != nil {
		t.Fatalf("mark read Update() error = %v", err)
	}
	if _, err := entryStore.Update(ctx, byExternal["hidden-one"].ID, entry.Patch{Hidden: &yes}); err != nil {
		t.Fatalf("mark hidden Update() error = %v", err)
	}

	got, err := sourceStore.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	counts := make(map[source.ID]int, len(got))
	for _, item := range got {
		counts[item.ID] = item.UnreadCount
	}
	if counts[src.ID] != 2 {
		t.Errorf("src UnreadCount = %d, want 2 (only this Source's unread, non-hidden entries)", counts[src.ID])
	}
	if counts[other.ID] != 3 {
		t.Errorf("other UnreadCount = %d, want 3", counts[other.ID])
	}
}
