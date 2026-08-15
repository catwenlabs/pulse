package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/catwenlabs/pulse/internal/document"
)

func TestDocumentStoreRoundTrip(t *testing.T) {
	pool := testPool(t)
	store := NewDocumentStore(pool)
	ctx := context.Background()

	saved, err := store.Import(ctx, document.ImportRequest{
		Filename: "reading-notes.txt",
		Content:  []byte("第一段内容。\n\n第二段内容。"),
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if saved.ID == "" {
		t.Fatalf("Import() returned empty ID")
	}
	if saved.Title != "reading-notes" {
		t.Errorf("Import() Title = %q, want reading-notes", saved.Title)
	}
	if len(saved.Chapters) != 1 || saved.Chapters[0].ContentHTML == "" {
		t.Fatalf("Import() chapters = %+v, want one chapter with content", saved.Chapters)
	}

	fetched, err := store.Get(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if fetched.ID != saved.ID || fetched.Title != "reading-notes" {
		t.Errorf("Get() = %+v", fetched)
	}
	if len(fetched.Chapters) != 1 || fetched.Chapters[0].ContentHTML != saved.Chapters[0].ContentHTML {
		t.Errorf("Get() chapters = %+v, want persisted chapter content", fetched.Chapters)
	}

	summaries, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(summaries) != 1 || summaries[0].ID != saved.ID || summaries[0].ChapterCount != 1 {
		t.Errorf("List() = %+v, want one summary for %q with 1 chapter", summaries, saved.ID)
	}

	if _, err := store.Get(ctx, document.ID("00000000-0000-0000-0000-000000000000")); err != document.ErrNotFound {
		t.Errorf("Get(missing) error = %v, want document.ErrNotFound", err)
	}

	progress := document.Progress{ChapterIndex: 0, ScrollRatio: 0.42}
	if err := store.SaveProgress(ctx, saved.ID, progress); err != nil {
		t.Fatalf("SaveProgress() error = %v", err)
	}
	resumed, err := store.Get(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Get() after SaveProgress() error = %v", err)
	}
	if resumed.Progress == nil {
		t.Fatalf("Get() Progress = nil, want saved progress")
	}
	if *resumed.Progress != progress {
		t.Errorf("Get() Progress = %+v, want %+v", *resumed.Progress, progress)
	}

	if err := store.SaveProgress(ctx, document.ID("00000000-0000-0000-0000-000000000000"), progress); err != document.ErrNotFound {
		t.Errorf("SaveProgress(missing) error = %v, want document.ErrNotFound", err)
	}
}

func TestDocumentStoreImportRejectsUnsupportedFileType(t *testing.T) {
	pool := testPool(t)
	store := NewDocumentStore(pool)

	_, err := store.Import(context.Background(), document.ImportRequest{
		Filename: "paper.pdf",
		Content:  []byte("%PDF-1.7"),
	})
	var validationErr *document.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("Import() error = %v, want document.ValidationError", err)
	}
	if validationErr.Field != "filename" {
		t.Errorf("ValidationError Field = %q, want filename", validationErr.Field)
	}
}
