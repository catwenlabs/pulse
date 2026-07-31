package postgres

import (
	"context"
	"testing"

	"github.com/catwenlabs/pulse/internal/opml"
)

func TestOPMLStoreImportIsIdempotentAndPreservesFolders(t *testing.T) {
	pool := testPool(t)
	store := NewOPMLStore(pool)
	ctx := context.Background()
	subscriptions := []opml.Subscription{
		{
			Title:   "Example",
			FeedURL: "HTTPS://Example.com:443/feed",
			SiteURL: "https://example.com",
			Folders: []string{"Technology"},
		},
		{
			Title:   "Nested",
			FeedURL: "https://nested.example/feed",
			Folders: []string{"Technology", "AI"},
		},
	}

	first, err := store.Import(ctx, subscriptions)
	if err != nil {
		t.Fatalf("first Import() error = %v", err)
	}
	if first.CreatedSources != 2 || first.ExistingSources != 0 || first.CreatedFolders != 2 {
		t.Errorf("first result = %+v", first)
	}

	second, err := store.Import(ctx, subscriptions)
	if err != nil {
		t.Fatalf("second Import() error = %v", err)
	}
	if second.CreatedSources != 0 || second.ExistingSources != 2 || second.CreatedFolders != 0 {
		t.Errorf("second result = %+v", second)
	}

	exported, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(exported) != 2 {
		t.Fatalf("exported count = %d", len(exported))
	}
	byURL := make(map[string]opml.Subscription, len(exported))
	for _, subscription := range exported {
		byURL[subscription.FeedURL] = subscription
	}
	if got := byURL["https://nested.example/feed"].Folders; len(got) != 2 {
		t.Errorf("nested folders = %v", got)
	}
}
