package postgres

import (
	"context"
	"testing"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/organization"
)

func TestOrganizationStoreFoldersAndViews(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	src := createTestSource(t, sourceStore, "organization-source")
	store := NewOrganizationStore(pool)
	ctx := context.Background()

	folder, err := store.CreateFolder(ctx, " Tech ")
	if err != nil {
		t.Fatalf("CreateFolder() error = %v", err)
	}
	if err := store.AddSourceToFolder(ctx, folder.ID, src.ID); err != nil {
		t.Fatalf("AddSourceToFolder() error = %v", err)
	}
	folders, err := store.ListFolders(ctx)
	if err != nil || len(folders) != 1 || folders[0].SourceCount != 1 ||
		len(folders[0].SourceIDs) != 1 || folders[0].SourceIDs[0] != string(src.ID) {
		t.Fatalf("ListFolders() = %+v, %v", folders, err)
	}
	if err := sourceStore.Archive(ctx, src.ID); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
	folders, err = store.ListFolders(ctx)
	if err != nil || len(folders) != 1 || folders[0].SourceCount != 0 || len(folders[0].SourceIDs) != 0 {
		t.Fatalf("ListFolders() after archive = %+v, %v", folders, err)
	}
	if err := store.RemoveSourceFromFolder(ctx, folder.ID, src.ID); err != nil {
		t.Fatalf("RemoveSourceFromFolder() error = %v", err)
	}

	view, err := store.CreateView(ctx, organization.View{
		Name: "Unread Go", Query: entry.Query{Search: "go", State: "unread", Tag: "tech"},
	})
	if err != nil {
		t.Fatalf("CreateView() error = %v", err)
	}
	view.Name = "Unread engineering"
	updated, err := store.UpdateView(ctx, view)
	if err != nil || updated.Name != view.Name {
		t.Fatalf("UpdateView() = %+v, %v", updated, err)
	}
	views, err := store.ListViews(ctx)
	if err != nil || len(views) != 1 || views[0].Query.State != "unread" {
		t.Fatalf("ListViews() = %+v, %v", views, err)
	}
	if err := store.DeleteView(ctx, view.ID); err != nil {
		t.Fatalf("DeleteView() error = %v", err)
	}
}
