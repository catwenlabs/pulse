package postgres

import (
	"context"
	"testing"

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/organization"
	"github.com/catwenlabs/pulse/internal/source"
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

func TestOrganizationStorePersistsIndependentNavigationOrder(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	store := NewOrganizationStore(pool)
	ctx := context.Background()

	rootFirst := createTestSource(t, sourceStore, "root-first")
	rootHidden := createTestSource(t, sourceStore, "root-hidden")
	rootLast := createTestSource(t, sourceStore, "root-last")
	folderOnly := createTestSource(t, sourceStore, "folder-only")

	firstFolder, err := store.CreateFolder(ctx, "First folder")
	if err != nil {
		t.Fatalf("CreateFolder(first) error = %v", err)
	}
	secondFolder, err := store.CreateFolder(ctx, "Second folder")
	if err != nil {
		t.Fatalf("CreateFolder(second) error = %v", err)
	}
	if err := store.AddSourceToFolder(ctx, firstFolder.ID, rootHidden.ID); err != nil {
		t.Fatalf("AddSourceToFolder(first, hidden) error = %v", err)
	}
	if err := store.AddSourceToFolder(ctx, firstFolder.ID, folderOnly.ID); err != nil {
		t.Fatalf("AddSourceToFolder(first, only) error = %v", err)
	}
	if err := store.AddSourceToFolder(ctx, secondFolder.ID, rootHidden.ID); err != nil {
		t.Fatalf("AddSourceToFolder(second, hidden) error = %v", err)
	}

	if err := store.ReorderFolders(ctx, []string{secondFolder.ID, firstFolder.ID}); err != nil {
		t.Fatalf("ReorderFolders() error = %v", err)
	}
	if err := store.ReorderRootSources(ctx, []source.ID{rootLast.ID, rootFirst.ID}); err != nil {
		t.Fatalf("ReorderRootSources() error = %v", err)
	}
	if err := store.ReorderFolderSources(ctx, firstFolder.ID, []source.ID{folderOnly.ID, rootHidden.ID}); err != nil {
		t.Fatalf("ReorderFolderSources() error = %v", err)
	}

	folders, err := store.ListFolders(ctx)
	if err != nil {
		t.Fatalf("ListFolders() error = %v", err)
	}
	if len(folders) != 2 || folders[0].ID != secondFolder.ID || folders[1].ID != firstFolder.ID {
		t.Fatalf("ListFolders() = %+v, want second folder before first folder", folders)
	}
	if got := folders[1].SourceIDs; len(got) != 2 || got[0] != string(folderOnly.ID) || got[1] != string(rootHidden.ID) {
		t.Fatalf("first folder SourceIDs = %v, want [%s %s]", got, folderOnly.ID, rootHidden.ID)
	}
	if got := folders[0].SourceIDs; len(got) != 1 || got[0] != string(rootHidden.ID) {
		t.Fatalf("second folder SourceIDs = %v, want [%s]", got, rootHidden.ID)
	}

	if err := store.RemoveSourceFromFolder(ctx, firstFolder.ID, rootHidden.ID); err != nil {
		t.Fatalf("RemoveSourceFromFolder(first, hidden) error = %v", err)
	}
	// The Source remains assigned to the second Folder and must stay out of root.
	if rootIDs := listRootSourceIDs(t, store, sourceStore, folders); len(rootIDs) != 2 || rootIDs[0] != rootLast.ID || rootIDs[1] != rootFirst.ID {
		t.Fatalf("root SourceIDs while hidden = %v, want [%s %s]", rootIDs, rootLast.ID, rootFirst.ID)
	}
	if err := store.RemoveSourceFromFolder(ctx, secondFolder.ID, rootHidden.ID); err != nil {
		t.Fatalf("RemoveSourceFromFolder(second, hidden) error = %v", err)
	}
	if rootIDs := listRootSourceIDs(t, store, sourceStore, folders); len(rootIDs) != 3 || rootIDs[0] != rootLast.ID || rootIDs[1] != rootHidden.ID || rootIDs[2] != rootFirst.ID {
		t.Fatalf("root SourceIDs after restore = %v, want [%s %s %s]", rootIDs, rootLast.ID, rootHidden.ID, rootFirst.ID)
	}
}

func listRootSourceIDs(t *testing.T, store *OrganizationStore, sourceStore *SourceStore, folders []organization.Folder) []source.ID {
	t.Helper()
	sources, err := sourceStore.List(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	assigned := make(map[string]struct{})
	for _, folder := range folders {
		for _, sourceID := range folder.SourceIDs {
			assigned[sourceID] = struct{}{}
		}
	}
	// The helper intentionally refreshes Folder membership so the assertion also
	// covers the rule that a Source assigned to any Folder is absent from root.
	freshFolders, err := store.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() refresh error = %v", err)
	}
	assigned = make(map[string]struct{})
	for _, folder := range freshFolders {
		for _, sourceID := range folder.SourceIDs {
			assigned[sourceID] = struct{}{}
		}
	}
	result := make([]source.ID, 0, len(sources))
	for _, item := range sources {
		if _, ok := assigned[string(item.ID)]; !ok {
			result = append(result, item.ID)
		}
	}
	return result
}
