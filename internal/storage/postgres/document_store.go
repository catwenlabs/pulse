package postgres

import (
	"context"
	"errors"
	"fmt"

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

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return document.Document{}, fmt.Errorf("begin document import: %w", err)
	}
	defer tx.Rollback(ctx)

	var saved document.Document
	err = tx.QueryRow(ctx, `
		INSERT INTO documents (identifier, title, author)
		VALUES ($1, $2, $3)
		RETURNING id, identifier, title, author
	`, parsed.Identifier, parsed.Title, parsed.Author).Scan(
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
