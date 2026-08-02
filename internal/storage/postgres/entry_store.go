package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/ingestion"
	"github.com/catwenlabs/pulse/internal/pagination"
	"github.com/catwenlabs/pulse/internal/source"
	"github.com/catwenlabs/pulse/internal/story"
)

type EntryStore struct {
	pool    *pgxpool.Pool
	publish func(string)
}

func NewEntryStore(pool *pgxpool.Pool, publishers ...func(string)) *EntryStore {
	var publish func(string)
	if len(publishers) > 0 {
		publish = publishers[0]
	}
	return &EntryStore{pool: pool, publish: publish}
}

// List and Search expose source content for internal maintenance and rule
// processing. Reader state and metadata are intentionally not part of Entry.
func (store *EntryStore) List(ctx context.Context, limit int) ([]entry.Entry, error) {
	return store.Search(ctx, entry.Query{Limit: limit})
}

func (store *EntryStore) Search(ctx context.Context, query entry.Query) ([]entry.Entry, error) {
	items, err := store.searchSourceEntries(ctx, query, true)
	if err != nil {
		return nil, err
	}
	result := make([]entry.Entry, 0, len(items))
	for _, item := range items {
		result = append(result, item.Entry)
	}
	return result, nil
}

// SearchSourceEntries is the Source browsing seam. Reader state and user
// metadata are returned under the real owning Story rather than copied onto
// the source Entry or represented by a synthetic singleton Story.
func (store *EntryStore) SearchSourceEntries(ctx context.Context, query entry.Query) ([]story.SourceEntry, error) {
	return store.searchSourceEntries(ctx, query, true)
}

func (store *EntryStore) searchSourceEntries(ctx context.Context, query entry.Query, sourceOrder bool) ([]story.SourceEntry, error) {
	if query.Limit <= 0 || query.Limit > 201 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	search := strings.TrimSpace(query.Search)
	state := strings.TrimSpace(query.State)
	tag := strings.TrimSpace(query.Tag)
	sourceID := strings.TrimSpace(string(query.SourceID))
	cursor, err := pagination.Decode(query.Cursor, "source_entries", search, state, tag, sourceID)
	if err != nil {
		return nil, err
	}
	var cursorTime any
	var cursorID any
	if cursor.Kind != "" {
		cursorTime = cursor.Time
		cursorID = cursor.ID
	}
	rows, err := store.pool.Query(ctx, `
		SELECT
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at',
			story.id,
			(SELECT count(*)::integer FROM story_entries WHERE story_id = story.id),
			(SELECT count(DISTINCT member_entry.source_id)::integer
			 FROM story_entries AS count_member
			 JOIN entries AS member_entry ON member_entry.id = count_member.entry_id
			 WHERE count_member.story_id = story.id),
			story.display_title, story.note,
			story.read_at, story.starred_at, story.hidden_at, story.later_at,
			COALESCE((
				SELECT jsonb_agg(jsonb_build_object('id', tag.id, 'name', tag.name) ORDER BY tag.name)
				FROM story_tags AS story_tag
				JOIN tags AS tag ON tag.id = story_tag.tag_id
				WHERE story_tag.story_id = story.id
			), '[]'::jsonb)
		FROM entries AS entry
		JOIN sources AS source ON source.id = entry.source_id
		JOIN story_entries AS membership ON membership.entry_id = entry.id
		JOIN stories AS story ON story.id = membership.story_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE
			($2 = '' OR (
				to_tsvector(
					'simple',
					coalesce(entry.source_title, '') || ' ' ||
					coalesce(story.display_title, '') || ' ' ||
					coalesce(story.note, '') || ' ' ||
					coalesce(entry.author, '') || ' ' ||
					coalesce(entry.summary, '') || ' ' ||
					coalesce(entry.content_html, '') || ' ' ||
					coalesce(source.name, '') || ' ' ||
					coalesce(entry_annotation.book_title, '') || ' ' ||
					coalesce(entry_annotation.book_author, '') || ' ' ||
					coalesce(entry_annotation.chapter, '') || ' ' ||
					coalesce(entry_annotation.annotation_note, '')
				) @@ plainto_tsquery('simple', $2)
				OR entry.source_title ILIKE '%' || $2 || '%'
				OR story.display_title ILIKE '%' || $2 || '%'
				OR story.note ILIKE '%' || $2 || '%'
				OR entry.author ILIKE '%' || $2 || '%'
				OR source.name ILIKE '%' || $2 || '%'
				OR lower(
					coalesce(entry.summary, '') || ' ' ||
					coalesce(entry.content_html, '') || ' ' ||
					coalesce(story.note, '') || ' ' ||
					coalesce(entry_annotation.book_title, '') || ' ' ||
					coalesce(entry_annotation.book_author, '') || ' ' ||
					coalesce(entry_annotation.chapter, '') || ' ' ||
					coalesce(entry_annotation.annotation_note, '')
				) LIKE '%' || lower($2) || '%'
				OR word_similarity(lower($2), lower(coalesce(entry.source_title, ''))) >= 0.45
				OR word_similarity(lower($2), lower(coalesce(story.display_title, ''))) >= 0.45
				OR word_similarity(lower($2), lower(coalesce(entry.author, ''))) >= 0.45
				OR word_similarity(lower($2), lower(coalesce(source.name, ''))) >= 0.45
				OR pulse_fuzzy_contains(coalesce(entry.source_title, ''), $2)
				OR pulse_fuzzy_contains(coalesce(story.display_title, ''), $2)
				OR pulse_fuzzy_contains(coalesce(entry.author, ''), $2)
				OR pulse_fuzzy_contains(coalesce(source.name, ''), $2)
			))
			AND (
				$3 = ''
				OR ($3 = 'inbox' AND story.hidden_at IS NULL)
				OR ($3 = 'unread' AND story.hidden_at IS NULL AND story.read_at IS NULL)
				OR ($3 = 'starred' AND story.starred_at IS NOT NULL)
				OR ($3 = 'later' AND story.later_at IS NOT NULL)
				OR ($3 = 'hidden' AND story.hidden_at IS NOT NULL)
			)
			AND (
				$4 = ''
				OR EXISTS (
					SELECT 1
					FROM story_tags AS story_tag
					JOIN tags ON tags.id = story_tag.tag_id
					WHERE story_tag.story_id = story.id
					  AND tags.normalized_name = lower($4)
				)
			)
			AND ($5 = '' OR entry.source_id = $5::uuid)
			AND (
				$7::timestamptz IS NULL
				OR entry.discovered_at < $7::timestamptz
				OR (entry.discovered_at = $7::timestamptz AND entry.id < $8::uuid)
			)
		ORDER BY
			CASE WHEN $9 THEN 0 ELSE CASE WHEN story.read_at IS NULL THEN 1 ELSE 0 END END DESC,
			entry.discovered_at DESC, entry.id DESC
		LIMIT $1
		OFFSET $6
	`,
		query.Limit,
		search, state, tag, query.SourceID,
		query.Offset,
		cursorTime, cursorID,
		sourceOrder,
	)
	if err != nil {
		return nil, fmt.Errorf("search entries: %w", err)
	}
	defer rows.Close()

	result := make([]story.SourceEntry, 0, query.Limit)
	for rows.Next() {
		item, err := scanSourceEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan Source Entry: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list Source Entries: %w", err)
	}
	return result, nil
}

func (store *EntryStore) SearchSourceEntryPage(ctx context.Context, query entry.Query) (story.SourceEntryPage, error) {
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	query.Offset = 0
	search := strings.TrimSpace(query.Search)
	state := strings.TrimSpace(query.State)
	tag := strings.TrimSpace(query.Tag)
	sourceID := strings.TrimSpace(string(query.SourceID))
	total, counts, err := store.sourceReaderCounts(ctx, search, state, tag, sourceID)
	if err != nil {
		return story.SourceEntryPage{}, err
	}
	query.Limit++
	items, err := store.searchSourceEntries(ctx, query, true)
	if err != nil {
		return story.SourceEntryPage{}, fmt.Errorf("search Source Entry page: %w", err)
	}
	page := story.SourceEntryPage{
		Entries:      items,
		TotalEntries: total,
		ReaderCounts: counts,
	}
	if len(items) <= query.Limit-1 {
		return page, nil
	}
	page.Entries = items[:query.Limit-1]
	last := page.Entries[len(page.Entries)-1]
	page.NextCursor, err = pagination.Encode(pagination.Position{
		Kind:     "source_entries",
		Search:   search,
		State:    state,
		Tag:      tag,
		SourceID: sourceID,
		Time:     last.Entry.DiscoveredAt,
		ID:       string(last.Entry.ID),
	})
	if err != nil {
		return story.SourceEntryPage{}, err
	}
	return page, nil
}

func (store *EntryStore) sourceReaderCounts(
	ctx context.Context,
	search string,
	state string,
	tag string,
	sourceID string,
) (int, story.ReaderCounts, error) {
	var total int
	var counts story.ReaderCounts
	err := store.pool.QueryRow(ctx, `
		WITH base AS (
			SELECT entry.id, story.id AS story_id,
				story.read_at, story.starred_at, story.hidden_at, story.later_at
			FROM entries AS entry
			JOIN sources AS source ON source.id = entry.source_id
			JOIN story_entries AS membership ON membership.entry_id = entry.id
			JOIN stories AS story ON story.id = membership.story_id
			LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
			WHERE entry.source_id = $4::uuid
			  AND ($1 = '' OR (
				to_tsvector(
					'simple',
					coalesce(entry.source_title, '') || ' ' ||
					coalesce(story.display_title, '') || ' ' ||
					coalesce(story.note, '') || ' ' ||
					coalesce(entry.author, '') || ' ' ||
					coalesce(entry.summary, '') || ' ' ||
					coalesce(entry.content_html, '') || ' ' ||
					coalesce(source.name, '') || ' ' ||
					coalesce(entry_annotation.book_title, '') || ' ' ||
					coalesce(entry_annotation.book_author, '') || ' ' ||
					coalesce(entry_annotation.chapter, '') || ' ' ||
					coalesce(entry_annotation.annotation_note, '')
				) @@ plainto_tsquery('simple', $1)
				OR entry.source_title ILIKE '%' || $1 || '%'
				OR story.display_title ILIKE '%' || $1 || '%'
				OR story.note ILIKE '%' || $1 || '%'
				OR entry.author ILIKE '%' || $1 || '%'
				OR source.name ILIKE '%' || $1 || '%'
				OR lower(coalesce(entry.summary, '') || ' ' || coalesce(entry.content_html, '') || ' ' || coalesce(story.note, '') || ' ' || coalesce(entry_annotation.book_title, '') || ' ' || coalesce(entry_annotation.book_author, '') || ' ' || coalesce(entry_annotation.chapter, '') || ' ' || coalesce(entry_annotation.annotation_note, '')) LIKE '%' || lower($1) || '%'
				OR word_similarity(lower($1), lower(coalesce(entry.source_title, ''))) >= 0.45
				OR word_similarity(lower($1), lower(coalesce(story.display_title, ''))) >= 0.45
				OR word_similarity(lower($1), lower(coalesce(entry.author, ''))) >= 0.45
				OR word_similarity(lower($1), lower(coalesce(source.name, ''))) >= 0.45
				OR pulse_fuzzy_contains(coalesce(entry.source_title, ''), $1)
				OR pulse_fuzzy_contains(coalesce(story.display_title, ''), $1)
				OR pulse_fuzzy_contains(coalesce(entry.author, ''), $1)
				OR pulse_fuzzy_contains(coalesce(source.name, ''), $1)
			  ))
			  AND ($3 = '' OR EXISTS (
				SELECT 1
				FROM story_tags AS story_tag
				JOIN tags ON tags.id = story_tag.tag_id
				WHERE story_tag.story_id = story.id
				  AND tags.normalized_name = lower($3)
			  ))
		)
		SELECT
			count(*) FILTER (WHERE
				$2 = ''
				OR ($2 = 'inbox' AND hidden_at IS NULL)
				OR ($2 = 'unread' AND hidden_at IS NULL AND read_at IS NULL)
				OR ($2 = 'starred' AND starred_at IS NOT NULL)
				OR ($2 = 'later' AND later_at IS NOT NULL)
				OR ($2 = 'hidden' AND hidden_at IS NOT NULL)
			)::integer,
			count(DISTINCT story_id) FILTER (WHERE hidden_at IS NULL)::integer,
			count(DISTINCT story_id) FILTER (WHERE hidden_at IS NULL AND read_at IS NULL)::integer,
			count(DISTINCT story_id) FILTER (WHERE starred_at IS NOT NULL)::integer,
			count(DISTINCT story_id) FILTER (WHERE later_at IS NOT NULL)::integer,
			count(DISTINCT story_id) FILTER (WHERE hidden_at IS NOT NULL)::integer
		FROM base
	`, search, state, tag, sourceID).Scan(
		&total,
		&counts.InboxStories,
		&counts.UnreadStories,
		&counts.StarredStories,
		&counts.LaterStories,
		&counts.HiddenStories,
	)
	if err != nil {
		return 0, story.ReaderCounts{}, fmt.Errorf("count Source Entry page: %w", err)
	}
	return total, counts, nil
}

func (store *EntryStore) Get(ctx context.Context, id entry.ID) (entry.Entry, error) {
	row := store.pool.QueryRow(ctx, `
		SELECT
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at'
		FROM entries AS entry
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE entry.id = $1
	`, id)
	item, err := scanEntry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return entry.Entry{}, fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	if err != nil {
		return entry.Entry{}, fmt.Errorf("get entry %s: %w", id, err)
	}
	return item, nil
}

func (store *EntryStore) Delete(ctx context.Context, id entry.ID, confirmed bool) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Entry deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var sourceID source.ID
	var identityKey string
	var storyID story.ID
	var displayTitle, note string
	var entryCount int
	var representativeID entry.ID
	err = tx.QueryRow(ctx, `
		SELECT
			entry.source_id, entry.identity_key,
			story.id, story.display_title, story.note,
			story.representative_entry_id,
			(SELECT count(*) FROM story_entries WHERE story_id = story.id)
		FROM entries AS entry
		JOIN story_entries AS member ON member.entry_id = entry.id
		JOIN stories AS story ON story.id = member.story_id
		WHERE entry.id = $1
		FOR UPDATE OF entry, story
	`, id).Scan(
		&sourceID, &identityKey, &storyID, &displayTitle, &note,
		&representativeID, &entryCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("load Entry deletion %s: %w", id, err)
	}
	if entryCount == 1 && !confirmed {
		return &entry.DeletionConfirmationError{
			StoryID:      string(storyID),
			DisplayTitle: displayTitle,
			Note:         note,
			EntryCount:   entryCount,
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO entry_tombstones (source_id, identity_key, deleted_at)
		VALUES ($1, $2, now())
		ON CONFLICT (source_id, identity_key) DO UPDATE SET deleted_at = EXCLUDED.deleted_at
	`, sourceID, identityKey); err != nil {
		return fmt.Errorf("write Entry tombstone %s: %w", id, err)
	}
	if entryCount == 1 {
		if _, err := tx.Exec(ctx, `DELETE FROM stories WHERE id = $1`, storyID); err != nil {
			return fmt.Errorf("delete final Story %s: %w", storyID, err)
		}
	} else if representativeID == id {
		if _, err := tx.Exec(ctx, `
			UPDATE stories
			SET representative_entry_id = (
				SELECT member.entry_id
				FROM story_entries AS member
				WHERE member.story_id = $1 AND member.entry_id <> $2
				ORDER BY member.joined_at ASC, member.entry_id ASC
				LIMIT 1
			), updated_at = now()
			WHERE id = $1
		`, storyID, id); err != nil {
			return fmt.Errorf("repair Story representative %s: %w", storyID, err)
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM entries WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete Entry %s: %w", id, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Entry deletion %s: %w", id, err)
	}
	return nil
}

type entryRow interface {
	Scan(...any) error
}

func scanEntry(row entryRow) (entry.Entry, error) {
	var item entry.Entry
	var annotationJSON []byte
	err := row.Scan(
		&item.ID,
		&item.SourceID,
		&item.IdentityKey,
		&item.ExternalID,
		&item.CanonicalURL,
		&item.SourceTitle,
		&item.Author,
		&item.Summary,
		&item.ContentHTML,
		&item.PublishedAt,
		&item.DiscoveredAt,
		&annotationJSON,
	)
	if err == nil && len(annotationJSON) > 0 && string(annotationJSON) != "null" {
		if err := json.Unmarshal(annotationJSON, &item.Annotation); err != nil {
			return entry.Entry{}, fmt.Errorf("decode entry annotation: %w", err)
		}
	}
	return item, err
}

func scanSourceEntry(row entryRow) (story.SourceEntry, error) {
	var item story.SourceEntry
	var annotationJSON []byte
	var tagsJSON []byte
	err := row.Scan(
		&item.Entry.ID,
		&item.Entry.SourceID,
		&item.Entry.IdentityKey,
		&item.Entry.ExternalID,
		&item.Entry.CanonicalURL,
		&item.Entry.SourceTitle,
		&item.Entry.Author,
		&item.Entry.Summary,
		&item.Entry.ContentHTML,
		&item.Entry.PublishedAt,
		&item.Entry.DiscoveredAt,
		&annotationJSON,
		&item.Story.ID,
		&item.Story.EntryCount,
		&item.Story.SourceCount,
		&item.Story.DisplayTitle,
		&item.Story.Note,
		&item.Story.ReadAt,
		&item.Story.StarredAt,
		&item.Story.HiddenAt,
		&item.Story.LaterAt,
		&tagsJSON,
	)
	if err != nil {
		return story.SourceEntry{}, err
	}
	if err := decodeAnnotation(annotationJSON, &item.Entry); err != nil {
		return story.SourceEntry{}, fmt.Errorf("decode Source Entry annotation: %w", err)
	}
	if len(tagsJSON) > 0 && string(tagsJSON) != "null" {
		if err := json.Unmarshal(tagsJSON, &item.Story.Tags); err != nil {
			return story.SourceEntry{}, fmt.Errorf("decode Story tags: %w", err)
		}
	}
	return item, nil
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (store *EntryStore) CommitBatch(
	ctx context.Context,
	acquisition ingestion.Acquisition,
	owner string,
	candidates []ingestion.Candidate,
	checkpoint json.RawMessage,
) error {
	identified := make([]identifiedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		identityKey, err := entry.Identity(candidate)
		if err != nil {
			return err
		}
		content := candidate.ContentHTML
		if content == "" {
			content = candidate.Summary
		}
		identified = append(identified, identifiedCandidate{
			identityKey: identityKey,
			candidate:   candidate,
			features:    story.BuildFeatures(candidate.Title, content),
		})
	}
	if len(checkpoint) == 0 {
		checkpoint = json.RawMessage(`{}`)
	}

	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin entry batch: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	newCount := 0
	updatedCount := 0
	for _, item := range identified {
		candidate := item.candidate
		var entryID entry.ID
		var inserted bool
		err := tx.QueryRow(ctx, `
			INSERT INTO entries (
				source_id, identity_key, external_id, canonical_url,
				source_title, author, summary, content_html, published_at,
				normalized_title, content_hash, content_simhash
			)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
			WHERE NOT EXISTS (
				SELECT 1
				FROM entry_tombstones
				WHERE source_id = $1 AND identity_key = $2
			)
			ON CONFLICT (source_id, identity_key)
			DO UPDATE SET
				external_id = EXCLUDED.external_id,
				canonical_url = EXCLUDED.canonical_url,
				source_title = EXCLUDED.source_title,
				author = EXCLUDED.author,
				summary = EXCLUDED.summary,
				content_html = EXCLUDED.content_html,
				published_at = EXCLUDED.published_at,
				normalized_title = EXCLUDED.normalized_title,
				content_hash = EXCLUDED.content_hash,
				content_simhash = EXCLUDED.content_simhash,
				embedding = CASE
					WHEN (entries.source_title, entries.summary, entries.content_html, entries.published_at)
						IS DISTINCT FROM
						(EXCLUDED.source_title, EXCLUDED.summary, EXCLUDED.content_html, EXCLUDED.published_at)
					THEN NULL ELSE entries.embedding
				END,
				embedding_model = CASE
					WHEN (entries.source_title, entries.summary, entries.content_html, entries.published_at)
						IS DISTINCT FROM
						(EXCLUDED.source_title, EXCLUDED.summary, EXCLUDED.content_html, EXCLUDED.published_at)
					THEN '' ELSE entries.embedding_model
				END,
				embedding_updated_at = CASE
					WHEN (entries.source_title, entries.summary, entries.content_html, entries.published_at)
						IS DISTINCT FROM
						(EXCLUDED.source_title, EXCLUDED.summary, EXCLUDED.content_html, EXCLUDED.published_at)
					THEN NULL ELSE entries.embedding_updated_at
				END,
				embedding_attempted_at = CASE
					WHEN (entries.source_title, entries.summary, entries.content_html, entries.published_at)
						IS DISTINCT FROM
						(EXCLUDED.source_title, EXCLUDED.summary, EXCLUDED.content_html, EXCLUDED.published_at)
					THEN NULL ELSE entries.embedding_attempted_at
				END,
				source_updated_at = now(),
				source_deleted = false
			RETURNING id, (xmax = 0)
		`,
			acquisition.SourceID,
			item.identityKey,
			candidate.ExternalID,
			candidate.URL,
			candidate.Title,
			candidate.Author,
			candidate.Summary,
			candidate.ContentHTML,
			candidate.PublishedAt,
			item.features.NormalizedTitle,
			item.features.ContentHash,
			int64(item.features.ContentSimHash),
		).Scan(&entryID, &inserted)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("upsert entry %q: %w", item.identityKey, err)
		}
		if candidate.Annotation != nil {
			detail := candidate.Annotation
			if _, err := tx.Exec(ctx, `
				INSERT INTO entry_annotations (
					entry_id, provider, book_identity, book_title, book_author,
					chapter, location, highlight_color, annotation_note, highlighted_at
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (entry_id)
				DO UPDATE SET
					provider = EXCLUDED.provider,
					book_identity = EXCLUDED.book_identity,
					book_title = EXCLUDED.book_title,
					book_author = EXCLUDED.book_author,
					chapter = EXCLUDED.chapter,
					location = EXCLUDED.location,
					highlight_color = EXCLUDED.highlight_color,
					annotation_note = EXCLUDED.annotation_note,
					highlighted_at = EXCLUDED.highlighted_at,
					imported_at = now()
			`, entryID, detail.Provider, detail.BookIdentity, detail.BookTitle,
				detail.BookAuthor, detail.Chapter, detail.Location, detail.HighlightColor,
				detail.AnnotationNote, detail.HighlightedAt); err != nil {
				return fmt.Errorf("upsert annotation for entry %q: %w", item.identityKey, err)
			}
		}
		if _, err := tx.Exec(ctx, `
			WITH created AS (
				INSERT INTO stories (
					representative_entry_id,
					sort_time
				)
				SELECT
					entry.id,
					least(coalesce($2::timestamptz, entry.discovered_at), entry.discovered_at)
				FROM entries AS entry
				WHERE entry.id = $1
				  AND NOT EXISTS (
					SELECT 1 FROM story_entries WHERE entry_id = $1
				  )
				RETURNING id
			)
			INSERT INTO story_entries (story_id, entry_id, match_method, final_score)
			SELECT id, $1, 'singleton', 1 FROM created
		`, entryID, candidate.PublishedAt); err != nil {
			return fmt.Errorf("ensure Story for entry %q: %w", item.identityKey, err)
		}
		if err := applyEnabledRulesTx(ctx, tx, entryID); err != nil {
			return fmt.Errorf("apply rules to entry %q: %w", item.identityKey, err)
		}
		if !inserted {
			if _, err := tx.Exec(ctx, `
				UPDATE stories
				SET clustered_at = NULL, updated_at = now()
				WHERE id = (
					SELECT story_id
					FROM story_entries
					WHERE entry_id = $1
				)
			`, entryID); err != nil {
				return fmt.Errorf("schedule Story refresh for entry %q: %w", item.identityKey, err)
			}
		}
		if inserted {
			newCount++
		} else {
			updatedCount++
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO source_checkpoints (source_id, checkpoint)
		VALUES ($1, $2)
		ON CONFLICT (source_id)
		DO UPDATE SET checkpoint = EXCLUDED.checkpoint, updated_at = now()
	`, acquisition.SourceID, checkpoint); err != nil {
		return fmt.Errorf("save source checkpoint: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE acquisitions
		SET
			status = 'succeeded',
			finished_at = now(),
			candidate_count = $3,
			new_count = $4,
			updated_count = $5,
			lease_owner = '',
			lease_until = NULL,
			last_error = ''
		WHERE id = $1 AND status = 'running' AND lease_owner = $2
	`, acquisition.ID, owner, len(candidates), newCount, updatedCount)
	if err != nil {
		return fmt.Errorf("complete acquisition in batch: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("complete acquisition in batch: lease is not owned by %q", owner)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit entry batch: %w", err)
	}
	if store.publish != nil && len(candidates) > 0 {
		store.publish(string(acquisition.SourceID))
	}
	return nil
}

type identifiedCandidate struct {
	identityKey string
	candidate   ingestion.Candidate
	features    story.Features
}
