package postgres

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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

	summaries, err := store.List(ctx, "")
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

func buildTestImageEpub(t *testing.T) []byte {
	t.Helper()
	png := "\x89PNG\r\n\x1a\nfake-image-bytes"
	buf := &bytes.Buffer{}
	writer := zip.NewWriter(buf)
	entries := map[string]string{
		"META-INF/container.xml": `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles>
</container>`,
		"OEBPS/content.opf": `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:identifier id="bookid">urn:uuid:image-book</dc:identifier>
    <dc:title>图文之书</dc:title>
  </metadata>
  <manifest><item id="c1" href="chapter1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c1"/></spine>
</package>`,
		"OEBPS/chapter1.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>图</h1>
<p><img src="../images/pic.png" alt="插图"/></p></body></html>`,
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
	image, err := writer.Create("images/pic.png")
	if err != nil {
		t.Fatalf("create image entry: %v", err)
	}
	if _, err := image.Write([]byte(png)); err != nil {
		t.Fatalf("write image entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

func TestDocumentStoreReadsAssetsFromOriginal(t *testing.T) {
	pool := testPool(t)
	store := NewDocumentStore(pool)
	ctx := context.Background()

	saved, err := store.Import(ctx, document.ImportRequest{
		Filename: "book.epub",
		Content:  buildTestImageEpub(t),
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	fetched, err := store.Get(ctx, saved.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !strings.Contains(fetched.Chapters[0].ContentHTML, `src="images/pic.png"`) {
		t.Errorf("chapter ContentHTML = %q, want canonical image src", fetched.Chapters[0].ContentHTML)
	}

	content, contentType, err := store.ReadAsset(ctx, saved.ID, "images/pic.png")
	if err != nil {
		t.Fatalf("ReadAsset() error = %v", err)
	}
	if contentType != "image/png" {
		t.Errorf("ReadAsset() contentType = %q, want image/png", contentType)
	}
	if !bytes.Contains(content, []byte("fake-image-bytes")) {
		t.Errorf("ReadAsset() content = %q, want the image bytes", content)
	}

	// Non-image and missing entries are not served.
	if _, _, err := store.ReadAsset(ctx, saved.ID, "OEBPS/content.opf"); !errors.Is(err, document.ErrNotFound) {
		t.Errorf("ReadAsset(non-image) error = %v, want document.ErrNotFound", err)
	}
	if _, _, err := store.ReadAsset(ctx, saved.ID, "images/missing.png"); !errors.Is(err, document.ErrNotFound) {
		t.Errorf("ReadAsset(missing) error = %v, want document.ErrNotFound", err)
	}
	if _, _, err := store.ReadAsset(ctx, document.ID("00000000-0000-0000-0000-000000000000"), "images/pic.png"); !errors.Is(err, document.ErrNotFound) {
		t.Errorf("ReadAsset(missing document) error = %v, want document.ErrNotFound", err)
	}
}

func TestDocumentStoreNotes(t *testing.T) {
	pool := testPool(t)
	store := NewDocumentStore(pool)
	ctx := context.Background()

	saved, err := store.Import(ctx, document.ImportRequest{
		Filename: "reading-notes.txt",
		Content:  []byte("第一段内容。"),
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	created, err := store.CreateNote(ctx, saved.ID, document.NoteInput{
		ChapterIndex: 0, Highlight: "第一段内容。", Text: "重要",
	})
	if err != nil {
		t.Fatalf("CreateNote() error = %v", err)
	}
	if created.ID == "" || created.DocumentID != saved.ID || created.HighlightedAt == nil {
		t.Errorf("created = %+v, want ID, DocumentID, HighlightedAt", created)
	}

	notes, err := store.ListNotes(ctx, saved.ID)
	if err != nil {
		t.Fatalf("ListNotes() error = %v", err)
	}
	if len(notes) != 1 || notes[0].ID != created.ID || notes[0].Text != "重要" {
		t.Errorf("notes = %+v", notes)
	}

	// A chapter beyond the document's chapter count is rejected.
	_, err = store.CreateNote(ctx, saved.ID, document.NoteInput{ChapterIndex: 5, Highlight: "越界"})
	var validationErr *document.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("out-of-range CreateNote() error = %v, want document.ValidationError", err)
	}

	// Notes on unknown documents are not found.
	missing := document.ID("00000000-0000-0000-0000-000000000000")
	if _, err := store.CreateNote(ctx, missing, document.NoteInput{ChapterIndex: 0, Highlight: "x"}); err != document.ErrNotFound {
		t.Errorf("CreateNote(missing) error = %v, want document.ErrNotFound", err)
	}
	if _, err := store.ListNotes(ctx, missing); err != document.ErrNotFound {
		t.Errorf("ListNotes(missing) error = %v, want document.ErrNotFound", err)
	}
}

func TestDocumentStoreImportNotesMatchesAndStoresUnmatched(t *testing.T) {
	pool := testPool(t)
	store := NewDocumentStore(pool)
	ctx := context.Background()

	doc, err := store.Import(ctx, document.ImportRequest{
		Filename: "书.txt",
		Content:  []byte("第一章内容。"),
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	when := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	file := document.NoteImportFile{Notes: []document.NoteImport{
		{BookTitle: "书", Highlight: "高亮一", Note: "想法", HighlightedAt: &when},
		{BookTitle: "没导入的书", BookAuthor: "某作者", ChapterIndex: 2, Location: "loc-9", Highlight: "高亮二"},
	}}

	summary, err := store.ImportNotes(ctx, file)
	if err != nil {
		t.Fatalf("ImportNotes() error = %v", err)
	}
	if summary.Imported != 1 || summary.Unmatched != 1 {
		t.Fatalf("ImportNotes() summary = %+v, want 1 imported 1 unmatched", summary)
	}

	notes, err := store.ListNotes(ctx, doc.ID)
	if err != nil {
		t.Fatalf("ListNotes() error = %v", err)
	}
	if len(notes) != 1 || notes[0].Highlight != "高亮一" || notes[0].Text != "想法" {
		t.Fatalf("ListNotes() = %+v, want the matched imported note", notes)
	}
	if notes[0].Source != "import" {
		t.Errorf("matched note Source = %q, want import", notes[0].Source)
	}
	if notes[0].HighlightedAt == nil || !notes[0].HighlightedAt.Equal(when) {
		t.Errorf("matched note HighlightedAt = %v, want %v", notes[0].HighlightedAt, when)
	}

	unmatched, err := store.ListUnmatchedNotes(ctx)
	if err != nil {
		t.Fatalf("ListUnmatchedNotes() error = %v", err)
	}
	if len(unmatched) != 1 {
		t.Fatalf("ListUnmatchedNotes() = %+v, want the unmatched note", unmatched)
	}
	entry := unmatched[0]
	if entry.DocumentID != "" || entry.BookTitle != "没导入的书" || entry.BookAuthor != "某作者" {
		t.Errorf("unmatched note = %+v, want book identity and no document", entry)
	}
	if entry.Highlight != "高亮二" || entry.ChapterIndex != 2 || entry.Location != "loc-9" {
		t.Errorf("unmatched note payload = %+v", entry)
	}
}

func TestDocumentStoreLinkNoteAssignsUnmatchedNote(t *testing.T) {
	pool := testPool(t)
	store := NewDocumentStore(pool)
	ctx := context.Background()

	doc, err := store.Import(ctx, document.ImportRequest{
		Filename: "目标书.txt",
		Content:  []byte("内容。"),
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if _, err := store.ImportNotes(ctx, document.NoteImportFile{Notes: []document.NoteImport{
		{BookTitle: "别名", Highlight: "高亮"},
	}}); err != nil {
		t.Fatalf("ImportNotes() error = %v", err)
	}
	unmatched, err := store.ListUnmatchedNotes(ctx)
	if err != nil || len(unmatched) != 1 {
		t.Fatalf("ListUnmatchedNotes() = %v, %v; want one note", unmatched, err)
	}
	noteID := unmatched[0].ID

	if err := store.LinkNote(ctx, noteID, doc.ID); err != nil {
		t.Fatalf("LinkNote() error = %v", err)
	}
	linked, err := store.ListNotes(ctx, doc.ID)
	if err != nil {
		t.Fatalf("ListNotes() error = %v", err)
	}
	if len(linked) != 1 || linked[0].ID != noteID || linked[0].DocumentID != doc.ID {
		t.Fatalf("ListNotes() = %+v, want the linked note", linked)
	}
	if remaining, err := store.ListUnmatchedNotes(ctx); err != nil || len(remaining) != 0 {
		t.Errorf("ListUnmatchedNotes() = %v, %v; want empty", remaining, err)
	}

	if err := store.LinkNote(ctx, noteID, doc.ID); err != document.ErrNotFound {
		t.Errorf("LinkNote(already linked) error = %v, want document.ErrNotFound", err)
	}
	if err := store.LinkNote(ctx, "00000000-0000-0000-0000-000000000000", doc.ID); err != document.ErrNotFound {
		t.Errorf("LinkNote(missing note) error = %v, want document.ErrNotFound", err)
	}
}

func TestDocumentStoreListAllNotesAcrossBooksAndSearch(t *testing.T) {
	pool := testPool(t)
	store := NewDocumentStore(pool)
	ctx := context.Background()

	doc, err := store.Import(ctx, document.ImportRequest{
		Filename: "思考的书.txt",
		Content:  []byte("内容。"),
	})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if _, err := store.CreateNote(ctx, doc.ID, document.NoteInput{ChapterIndex: 0, Highlight: "系统一自动运行"}); err != nil {
		t.Fatalf("CreateNote() error = %v", err)
	}
	if _, err := store.ImportNotes(ctx, document.NoteImportFile{Notes: []document.NoteImport{
		{BookTitle: "思考的书", Highlight: "直觉判断", Note: "和复杂性呼应"},
		{BookTitle: "另一本书", Highlight: "第二本书的高亮"},
	}}); err != nil {
		t.Fatalf("ImportNotes() error = %v", err)
	}

	notes, err := store.ListAllNotes(ctx, "")
	if err != nil {
		t.Fatalf("ListAllNotes() error = %v", err)
	}
	if len(notes) != 3 {
		t.Fatalf("ListAllNotes() = %d notes, want 3: %+v", len(notes), notes)
	}
	var linked, importedLinked, unmatched *document.Note
	for index := range notes {
		switch notes[index].Highlight {
		case "系统一自动运行":
			linked = &notes[index]
		case "直觉判断":
			importedLinked = &notes[index]
		case "第二本书的高亮":
			unmatched = &notes[index]
		}
	}
	if linked == nil || linked.DocumentID != doc.ID || linked.Source != "pulse" || linked.BookTitle != "思考的书" {
		t.Errorf("pulse note = %+v, want linked to the document with its book identity", linked)
	}
	if importedLinked == nil || importedLinked.DocumentID != doc.ID || importedLinked.Source != "import" {
		t.Errorf("imported note = %+v, want linked and marked import", importedLinked)
	}
	if unmatched == nil || unmatched.DocumentID != "" || unmatched.BookTitle != "另一本书" {
		t.Errorf("unmatched note = %+v, want book identity and no document", unmatched)
	}

	matched, err := store.ListAllNotes(ctx, "复杂性")
	if err != nil {
		t.Fatalf("ListAllNotes(search) error = %v", err)
	}
	if len(matched) != 1 || matched[0].Highlight != "直觉判断" {
		t.Errorf("ListAllNotes(复杂性) = %+v, want the note whose note text matches", matched)
	}

	byBook, err := store.ListAllNotes(ctx, "另一本")
	if err != nil {
		t.Fatalf("ListAllNotes(search book) error = %v", err)
	}
	if len(byBook) != 1 || byBook[0].BookTitle != "另一本书" {
		t.Errorf("ListAllNotes(另一本) = %+v, want the unmatched book's note", byBook)
	}
}

func TestDocumentStoreListSearchesChapters(t *testing.T) {
	pool := testPool(t)
	store := NewDocumentStore(pool)
	ctx := context.Background()

	if _, err := store.Import(ctx, document.ImportRequest{
		Filename: "复杂性.txt",
		Content:  []byte("这一章讨论秩序的涌现。"),
	}); err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if _, err := store.Import(ctx, document.ImportRequest{
		Filename: "另一本.txt",
		Content:  []byte("完全不同的内容。"),
	}); err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	byTitle, err := store.List(ctx, "复杂性")
	if err != nil {
		t.Fatalf("List(search) error = %v", err)
	}
	if len(byTitle) != 1 || byTitle[0].Title != "复杂性" {
		t.Errorf("List(复杂性) = %+v, want the titled book", byTitle)
	}

	byChapter, err := store.List(ctx, "秩序的涌现")
	if err != nil {
		t.Fatalf("List(search chapters) error = %v", err)
	}
	if len(byChapter) != 1 || byChapter[0].Title != "复杂性" {
		t.Errorf("List(秩序的涌现) = %+v, want the book whose chapter matches", byChapter)
	}

	all, err := store.List(ctx, "")
	if err != nil || len(all) != 2 {
		t.Errorf("List() = %d, %v; want both books", len(all), err)
	}
}
