package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/catwenlabs/pulse/internal/document"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DocumentStore persists imported documents and their chapters. Import
// dispatches on the filename extension and parses the file into chapters
// before persisting; unsupported files fail validation.
type DocumentStore struct {
	pool *pgxpool.Pool
}

func NewDocumentStore(pool *pgxpool.Pool) *DocumentStore {
	return &DocumentStore{pool: pool}
}

func (store *DocumentStore) Import(ctx context.Context, request document.ImportRequest) (document.Document, error) {
	parsed, err := document.Parse(request.Filename, request.Content)
	if err != nil {
		return document.Document{}, err
	}

	// A book is identified by its OPF identifier, or by title and author
	// when it has none. Re-importing the same book is rejected, not merged.
	var duplicateID document.ID
	duplicateCheck := store.pool.QueryRow(ctx, `
		SELECT id FROM documents
		WHERE ($1 <> '' AND identifier = $1)
		   OR (title = $2 AND author = $3)
		LIMIT 1
	`, parsed.Identifier, parsed.Title, parsed.Author)
	if err := duplicateCheck.Scan(&duplicateID); err == nil {
		return document.Document{}, document.ErrDuplicate
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return document.Document{}, fmt.Errorf("check duplicate document: %w", err)
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return document.Document{}, fmt.Errorf("begin document import: %w", err)
	}
	defer tx.Rollback(ctx)

	var saved document.Document
	err = tx.QueryRow(ctx, `
		INSERT INTO documents (identifier, title, author, original_filename, original)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, identifier, title, author
	`, parsed.Identifier, parsed.Title, parsed.Author, request.Filename, request.Content).Scan(
		&saved.ID, &saved.Identifier, &saved.Title, &saved.Author,
	)
	if err != nil {
		return document.Document{}, fmt.Errorf("insert document: %w", err)
	}
	for _, chapter := range parsed.Chapters {
		if _, err := tx.Exec(ctx, `
			INSERT INTO document_chapters (document_id, chapter_index, title, content_html)
			VALUES ($1, $2, $3, $4)
		`, saved.ID, chapter.Index, chapter.Title, chapter.ContentHTML); err != nil {
			return document.Document{}, fmt.Errorf("insert document chapter: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return document.Document{}, fmt.Errorf("commit document import: %w", err)
	}

	saved.Chapters = parsed.Chapters
	return saved, nil
}

// ReadAsset extracts one image entry from the stored original file. Only
// image entries are served; anything else is not found.
func (store *DocumentStore) ReadAsset(ctx context.Context, id document.ID, entry string) ([]byte, string, error) {
	if document.ImageContentType(entry) == "" {
		return nil, "", document.ErrNotFound
	}
	var original []byte
	err := store.pool.QueryRow(ctx, `
		SELECT original FROM documents WHERE id = $1
	`, id).Scan(&original)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", document.ErrNotFound
	}
	if err != nil {
		return nil, "", fmt.Errorf("read document original: %w", err)
	}
	content, err := document.ReadEpubEntry(original, entry)
	if err != nil {
		return nil, "", document.ErrNotFound
	}
	return content, document.ImageContentType(entry), nil
}

func (store *DocumentStore) Delete(ctx context.Context, id document.ID) error {
	tag, err := store.pool.Exec(ctx, `DELETE FROM documents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete document: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return document.ErrNotFound
	}
	return nil
}

func (store *DocumentStore) Get(ctx context.Context, id document.ID) (document.Document, error) {
	fetched := document.Document{ID: id}
	var progressChapter *int
	var progressRatio *float64
	err := store.pool.QueryRow(ctx, `
		SELECT d.identifier, d.title, d.author,
		       p.chapter_index, p.scroll_ratio
		FROM documents d
		LEFT JOIN document_progress p ON p.document_id = d.id
		WHERE d.id = $1
	`, id).Scan(&fetched.Identifier, &fetched.Title, &fetched.Author, &progressChapter, &progressRatio)
	if errors.Is(err, pgx.ErrNoRows) {
		return document.Document{}, document.ErrNotFound
	}
	if err != nil {
		return document.Document{}, fmt.Errorf("get document: %w", err)
	}
	if progressChapter != nil && progressRatio != nil {
		fetched.Progress = &document.Progress{ChapterIndex: *progressChapter, ScrollRatio: *progressRatio}
	}
	rows, err := store.pool.Query(ctx, `
		SELECT chapter_index, title, content_html
		FROM document_chapters WHERE document_id = $1
		ORDER BY chapter_index
	`, id)
	if err != nil {
		return document.Document{}, fmt.Errorf("list document chapters: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var chapter document.Chapter
		if err := rows.Scan(&chapter.Index, &chapter.Title, &chapter.ContentHTML); err != nil {
			return document.Document{}, fmt.Errorf("scan document chapter: %w", err)
		}
		fetched.Chapters = append(fetched.Chapters, chapter)
	}
	if err := rows.Err(); err != nil {
		return document.Document{}, fmt.Errorf("iterate document chapters: %w", err)
	}
	return fetched, nil
}

func (store *DocumentStore) SaveProgress(ctx context.Context, id document.ID, progress document.Progress) error {
	tag, err := store.pool.Exec(ctx, `
		INSERT INTO document_progress (document_id, chapter_index, scroll_ratio, updated_at)
		SELECT $1, $2, $3, now() WHERE EXISTS (SELECT 1 FROM documents WHERE id = $1)
		ON CONFLICT (document_id) DO UPDATE
		SET chapter_index = EXCLUDED.chapter_index,
		    scroll_ratio = EXCLUDED.scroll_ratio,
		    updated_at = now()
	`, id, progress.ChapterIndex, progress.ScrollRatio)
	if err != nil {
		return fmt.Errorf("save document progress: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return document.ErrNotFound
	}
	return nil
}

func (store *DocumentStore) List(ctx context.Context) ([]document.Summary, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT d.id, d.identifier, d.title, d.author, COUNT(c.chapter_index)
		FROM documents d
		LEFT JOIN document_chapters c ON c.document_id = d.id
		GROUP BY d.id, d.identifier, d.title, d.author
		ORDER BY d.imported_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	defer rows.Close()
	summaries := []document.Summary{}
	for rows.Next() {
		var summary document.Summary
		if err := rows.Scan(&summary.ID, &summary.Identifier, &summary.Title, &summary.Author, &summary.ChapterCount); err != nil {
			return nil, fmt.Errorf("scan document summary: %w", err)
		}
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document summaries: %w", err)
	}
	return summaries, nil
}

// CreateNote persists one highlight written while reading. The chapter must
// exist in the document.
func (store *DocumentStore) CreateNote(ctx context.Context, id document.ID, input document.NoteInput) (document.Note, error) {
	if err := input.Validate(); err != nil {
		return document.Note{}, err
	}
	note := document.Note{DocumentID: id, ChapterIndex: input.ChapterIndex, Highlight: strings.TrimSpace(input.Highlight), Text: strings.TrimSpace(input.Text), Color: strings.TrimSpace(input.Color)}
	err := store.pool.QueryRow(ctx, `
		INSERT INTO document_notes (document_id, chapter_index, highlight, note, highlight_color)
		SELECT $1, $2, $3, $4, $5
		WHERE EXISTS (
			SELECT 1 FROM document_chapters
			WHERE document_id = $1 AND chapter_index = $2
		)
		RETURNING id, highlighted_at
	`, id, input.ChapterIndex, note.Highlight, note.Text, note.Color).Scan(&note.ID, &note.HighlightedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either the document does not exist or the chapter is out of range.
		var chapters int
		countErr := store.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM document_chapters WHERE document_id = $1
		`, id).Scan(&chapters)
		if countErr != nil || chapters == 0 {
			return document.Note{}, document.ErrNotFound
		}
		return document.Note{}, &document.ValidationError{Field: "chapter_index", Message: "chapter does not exist in this document"}
	}
	if err != nil {
		return document.Note{}, fmt.Errorf("insert document note: %w", err)
	}
	return note, nil
}

// ListNotes returns all notes of a document in chapter order.
func (store *DocumentStore) ListNotes(ctx context.Context, id document.ID) ([]document.Note, error) {
	rows, err := store.pool.Query(ctx, noteSelection+`
		WHERE document_id = $1
		ORDER BY chapter_index, highlighted_at
	`, id)
	if err != nil {
		return nil, fmt.Errorf("list document notes: %w", err)
	}
	defer rows.Close()
	notes := []document.Note{}
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate document notes: %w", err)
	}
	if len(notes) == 0 {
		if _, err := store.Get(ctx, id); err != nil {
			return nil, err
		}
	}
	return notes, nil
}

// ImportNotes writes one batch of imported notes, matching each entry's book
// to an existing document by identifier or title+author. Unmatched notes are
// stored without a document for later manual linking — no documents are
// created implicitly.
func (store *DocumentStore) ImportNotes(ctx context.Context, file document.NoteImportFile) (document.NoteImportSummary, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return document.NoteImportSummary{}, fmt.Errorf("begin note import: %w", err)
	}
	defer tx.Rollback(ctx)

	resolved := map[string]document.ID{}
	summary := document.NoteImportSummary{}
	for _, entry := range file.Notes {
		key := entry.BookIdentifier + "\x00" + entry.BookTitle + "\x00" + entry.BookAuthor
		docID, cached := resolved[key]
		if !cached {
			docID, err = matchImportedBook(ctx, tx, entry)
			if err != nil {
				return document.NoteImportSummary{}, err
			}
			resolved[key] = docID
		}
		var linked *string
		if docID != "" {
			value := string(docID)
			linked = &value
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO document_notes (
				document_id, book_identifier, book_title, book_author,
				chapter_index, location, highlight, note, highlight_color,
				source, highlighted_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'import', COALESCE($10, now()))
		`, linked, entry.BookIdentifier, entry.BookTitle, entry.BookAuthor,
			entry.ChapterIndex, entry.Location, strings.TrimSpace(entry.Highlight),
			strings.TrimSpace(entry.Note), strings.TrimSpace(entry.Color), entry.HighlightedAt)
		if err != nil {
			return document.NoteImportSummary{}, fmt.Errorf("insert imported note: %w", err)
		}
		if docID != "" {
			summary.Imported++
		} else {
			summary.Unmatched++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return document.NoteImportSummary{}, fmt.Errorf("commit note import: %w", err)
	}
	return summary, nil
}

// matchImportedBook resolves one imported book identity to an existing
// document, mirroring the import duplicate rule: identifier first, then
// title and author together. An empty ID means unmatched.
func matchImportedBook(ctx context.Context, tx pgx.Tx, entry document.NoteImport) (document.ID, error) {
	var id document.ID
	err := tx.QueryRow(ctx, `
		SELECT id FROM documents
		WHERE ($1 <> '' AND identifier = $1)
		   OR (title = $2 AND author = $3)
		LIMIT 1
	`, entry.BookIdentifier, entry.BookTitle, entry.BookAuthor).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("match imported book: %w", err)
	}
	return id, nil
}

// LinkNote manually assigns one unmatched imported note to a document.
// Already-linked or missing notes, and missing documents, are not found.
func (store *DocumentStore) LinkNote(ctx context.Context, noteID string, id document.ID) (document.Note, error) {
	var note document.Note
	err := store.pool.QueryRow(ctx, `
		UPDATE document_notes SET document_id = $2
		WHERE id = $1::uuid AND document_id IS NULL
			AND EXISTS (SELECT 1 FROM documents WHERE id = $2)
		RETURNING id, document_id, book_identifier, book_title, book_author,
		          chapter_index, location, highlight, note, highlight_color, source, highlighted_at
	`, noteID, id).Scan(
		&note.ID, new(string), &note.BookIdentifier, &note.BookTitle, &note.BookAuthor,
		&note.ChapterIndex, &note.Location, &note.Highlight, &note.Text, &note.Color,
		&note.Source, &note.HighlightedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return document.Note{}, document.ErrNotFound
	}
	if err != nil {
		return document.Note{}, fmt.Errorf("link document note: %w", err)
	}
	note.DocumentID = id
	return note, nil
}

// ListUnmatchedNotes returns imported notes that have no document yet.
func (store *DocumentStore) ListUnmatchedNotes(ctx context.Context) ([]document.Note, error) {
	rows, err := store.pool.Query(ctx, noteSelection+`
		WHERE document_id IS NULL
		ORDER BY book_title, book_author, highlighted_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list unmatched notes: %w", err)
	}
	defer rows.Close()
	notes := []document.Note{}
	for rows.Next() {
		note, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unmatched notes: %w", err)
	}
	return notes, nil
}

// noteSelection is the shared column list for reading document_notes rows.
const noteSelection = `
	SELECT id, document_id, book_identifier, book_title, book_author,
	       chapter_index, location, highlight, note, highlight_color, source, highlighted_at
	FROM document_notes
`

// scanNote reads one document_notes row, mapping a NULL document_id to the
// empty ID so unmatched notes round-trip.
func scanNote(row pgx.Row) (document.Note, error) {
	var note document.Note
	var documentID *string
	err := row.Scan(&note.ID, &documentID, &note.BookIdentifier, &note.BookTitle, &note.BookAuthor,
		&note.ChapterIndex, &note.Location, &note.Highlight, &note.Text, &note.Color,
		&note.Source, &note.HighlightedAt)
	if err != nil {
		return document.Note{}, fmt.Errorf("scan document note: %w", err)
	}
	if documentID != nil {
		note.DocumentID = document.ID(*documentID)
	}
	return note, nil
}
