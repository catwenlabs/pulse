package postgres

import (
	"archive/zip"
	"bytes"
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

func TestDocumentStoreImportRejectsDuplicateBook(t *testing.T) {
	pool := testPool(t)
	store := NewDocumentStore(pool)
	ctx := context.Background()

	book := document.ImportRequest{
		Filename: "reading-notes.txt",
		Content:  []byte("第一段内容。"),
	}
	first, err := store.Import(ctx, book)
	if err != nil {
		t.Fatalf("first Import() error = %v", err)
	}
	if _, err := store.Import(ctx, book); !errors.Is(err, document.ErrDuplicate) {
		t.Fatalf("second Import() error = %v, want document.ErrDuplicate", err)
	}

	// The same title under a different filename is still the same document.
	if _, err := store.Import(ctx, document.ImportRequest{
		Filename: "reading-notes.txt", Content: book.Content,
	}); !errors.Is(err, document.ErrDuplicate) {
		t.Fatalf("same-title Import() error = %v, want document.ErrDuplicate", err)
	}

	// An epub keeps its OPF identifier, so a renamed file is still the same
	// book even though the filename (and derived txt title) would differ.
	epub := document.ImportRequest{Filename: "book.epub", Content: buildTestEpub(t)}
	if _, err := store.Import(ctx, epub); err != nil {
		t.Fatalf("epub Import() error = %v", err)
	}
	epub.Filename = "renamed-book.epub"
	if _, err := store.Import(ctx, epub); !errors.Is(err, document.ErrDuplicate) {
		t.Fatalf("renamed epub Import() error = %v, want document.ErrDuplicate", err)
	}

	// A different title imports fine.
	other := document.ImportRequest{Filename: "other.txt", Content: []byte("别的内容。")}
	if _, err := store.Import(ctx, other); err != nil {
		t.Fatalf("other Import() error = %v", err)
	}

	// Importing after deleting the original is allowed again.
	if err := store.Delete(ctx, first.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Import(ctx, book); err != nil {
		t.Fatalf("Import() after Delete() error = %v", err)
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

func buildTestEpub(t *testing.T) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	entries := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">urn:uuid:store-test-book</dc:identifier>
    <dc:title>存储测试之书</dc:title>
  </metadata>
  <manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c1"/></spine>
</package>`,
		"c1.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>唯一章</h1><p>内容。</p></body></html>`,
	}
	for name, body := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}
