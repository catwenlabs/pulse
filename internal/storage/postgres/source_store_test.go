package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wenpengfei/pulse/internal/source"
	"github.com/wenpengfei/pulse/internal/storage/migrate"
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
	if _, err := pool.Exec(context.Background(), "TRUNCATE sources, folders CASCADE"); err != nil {
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
