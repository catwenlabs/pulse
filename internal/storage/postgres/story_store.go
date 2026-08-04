package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/catwenlabs/pulse/internal/entry"
	"github.com/catwenlabs/pulse/internal/pagination"
	"github.com/catwenlabs/pulse/internal/story"
)

type StoryStore struct {
	pool *pgxpool.Pool
}

func NewStoryStore(pool *pgxpool.Pool) *StoryStore {
	return &StoryStore{pool: pool}
}

func (store *StoryStore) Search(ctx context.Context, query story.Query) ([]story.Story, error) {
	if query.Limit <= 0 || query.Limit > 201 {
		query.Limit = 50
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	search := strings.TrimSpace(query.Search)
	state := strings.TrimSpace(query.State)
	tag := strings.TrimSpace(query.Tag)
	sourceID := strings.TrimSpace(query.SourceID)
	cursor, err := pagination.Decode(query.Cursor, "stories", search, state, tag, sourceID)
	if err != nil {
		return nil, err
	}
	cursorBucket := -1
	var cursorTime any
	var cursorID any
	if cursor.Kind != "" {
		cursorBucket = cursor.Bucket
		cursorTime = cursor.Time
		cursorID = cursor.ID
	}
	rows, err := store.pool.Query(ctx, `
		WITH aggregates AS (
			SELECT
				member.story_id,
				count(*)::integer AS entry_count,
				count(DISTINCT entry.source_id)::integer AS source_count,
				min(coalesce(entry.published_at, entry.discovered_at)) AS first_published_at,
				max(coalesce(entry.published_at, entry.discovered_at)) AS last_published_at
			FROM story_entries AS member
			JOIN entries AS entry ON entry.id = member.entry_id
			GROUP BY member.story_id
		)
		SELECT
			story.id, story.sort_time, story.display_title, story.note,
			COALESCE((
				SELECT jsonb_agg(jsonb_build_object('id', tag.id, 'name', tag.name) ORDER BY tag.name)
				FROM story_tags AS story_tag
				JOIN tags AS tag ON tag.id = story_tag.tag_id
				WHERE story_tag.story_id = story.id
			), '[]'::jsonb),
			aggregate.entry_count, aggregate.source_count,
			aggregate.first_published_at, aggregate.last_published_at,
			story.read_at, story.starred_at, story.hidden_at, story.later_at,
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at'
		FROM stories AS story
		JOIN aggregates AS aggregate ON aggregate.story_id = story.id
		JOIN entries AS entry ON entry.id = story.representative_entry_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE
			($2 = '' OR (
				story.display_title ILIKE '%' || $2 || '%'
				OR story.note ILIKE '%' || $2 || '%'
				OR EXISTS (
					SELECT 1
					FROM story_entries AS matching_member
					JOIN entries AS matching_entry ON matching_entry.id = matching_member.entry_id
					JOIN sources AS matching_source ON matching_source.id = matching_entry.source_id
					LEFT JOIN entry_annotations AS matching_annotation ON matching_annotation.entry_id = matching_entry.id
					WHERE matching_member.story_id = story.id
					  AND (
						matching_entry.source_title ILIKE '%' || $2 || '%'
						OR matching_entry.author ILIKE '%' || $2 || '%'
						OR matching_entry.summary ILIKE '%' || $2 || '%'
						OR matching_entry.content_html ILIKE '%' || $2 || '%'
						OR matching_source.name ILIKE '%' || $2 || '%'
						OR matching_annotation.book_title ILIKE '%' || $2 || '%'
						OR matching_annotation.book_author ILIKE '%' || $2 || '%'
						OR matching_annotation.chapter ILIKE '%' || $2 || '%'
						OR matching_annotation.annotation_note ILIKE '%' || $2 || '%'
						OR word_similarity(lower($2), lower(matching_entry.source_title)) >= 0.45
						OR pulse_fuzzy_contains(coalesce(matching_entry.source_title, ''), $2)
					  )
				)
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
			AND (
				$5 = ''
				OR EXISTS (
					SELECT 1
					FROM story_entries AS member
					JOIN entries AS source_entry ON source_entry.id = member.entry_id
					WHERE member.story_id = story.id
					  AND source_entry.source_id = $5::uuid
				)
			)
			AND (
				$7 < 0
				OR CASE WHEN story.read_at IS NULL THEN 1 ELSE 0 END < $7
				OR (
					CASE WHEN story.read_at IS NULL THEN 1 ELSE 0 END = $7
					AND (
						story.sort_time < $8::timestamptz
						OR (story.sort_time = $8::timestamptz AND story.id < $9::uuid)
					)
				)
			)
		ORDER BY (story.read_at IS NULL) DESC, story.sort_time DESC, story.id DESC
		LIMIT $1 OFFSET $6
	`, query.Limit, search, state, tag, sourceID, query.Offset,
		cursorBucket, cursorTime, cursorID)
	if err != nil {
		return nil, fmt.Errorf("search Stories: %w", err)
	}
	defer rows.Close()

	result := make([]story.Story, 0, query.Limit)
	for rows.Next() {
		item, err := scanStory(rows)
		if err != nil {
			return nil, fmt.Errorf("scan Story: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search Stories: %w", err)
	}
	if search != "" {
		for index := range result {
			matched, found, err := store.findMatchingEntry(ctx, result[index].ID, search)
			if err != nil {
				return nil, err
			}
			if found {
				result[index].MatchedEntry = &matched
			}
		}
	}
	return result, nil
}

func (store *StoryStore) findMatchingEntry(
	ctx context.Context,
	storyID story.ID,
	search string,
) (entry.Entry, bool, error) {
	row := store.pool.QueryRow(ctx, `
		SELECT
			matching_entry.id, matching_entry.source_id, matching_entry.identity_key,
			matching_entry.external_id, matching_entry.canonical_url,
			matching_entry.source_title, matching_entry.author, matching_entry.summary,
			matching_entry.content_html, matching_entry.published_at,
			matching_entry.discovered_at,
			to_jsonb(matching_annotation) - 'entry_id' - 'imported_at'
		FROM story_entries AS member
		JOIN entries AS matching_entry ON matching_entry.id = member.entry_id
		JOIN sources AS matching_source ON matching_source.id = matching_entry.source_id
		LEFT JOIN entry_annotations AS matching_annotation ON matching_annotation.entry_id = matching_entry.id
		WHERE member.story_id = $1
		  AND (
			matching_entry.source_title ILIKE '%' || $2 || '%'
			OR matching_entry.author ILIKE '%' || $2 || '%'
			OR matching_entry.summary ILIKE '%' || $2 || '%'
			OR matching_entry.content_html ILIKE '%' || $2 || '%'
			OR matching_source.name ILIKE '%' || $2 || '%'
			OR matching_annotation.book_title ILIKE '%' || $2 || '%'
			OR matching_annotation.book_author ILIKE '%' || $2 || '%'
			OR matching_annotation.chapter ILIKE '%' || $2 || '%'
			OR matching_annotation.annotation_note ILIKE '%' || $2 || '%'
			OR word_similarity(lower($2), lower(matching_entry.source_title)) >= 0.45
			OR pulse_fuzzy_contains(coalesce(matching_entry.source_title, ''), $2)
		  )
		ORDER BY member.joined_at ASC, member.entry_id ASC
		LIMIT 1
	`, storyID, search)
	matched, err := scanEntry(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return entry.Entry{}, false, nil
	}
	if err != nil {
		return entry.Entry{}, false, fmt.Errorf("find matching Story Entry: %w", err)
	}
	return matched, true, nil
}

func (store *StoryStore) SearchPage(ctx context.Context, query story.Query) (story.Page, error) {
	if query.Limit <= 0 || query.Limit > 200 {
		query.Limit = 50
	}
	query.Offset = 0
	search := strings.TrimSpace(query.Search)
	state := strings.TrimSpace(query.State)
	tag := strings.TrimSpace(query.Tag)
	sourceID := strings.TrimSpace(query.SourceID)
	total, counts, err := store.storyReaderCounts(ctx, search, state, tag, sourceID)
	if err != nil {
		return story.Page{}, err
	}
	query.Limit++
	items, err := store.Search(ctx, query)
	if err != nil {
		return story.Page{}, fmt.Errorf("search Story page: %w", err)
	}
	page := story.Page{
		Stories:      items,
		TotalStories: total,
		ReaderCounts: counts,
	}
	if len(items) <= query.Limit-1 {
		return page, nil
	}
	page.Stories = items[:query.Limit-1]
	last := page.Stories[len(page.Stories)-1]
	if last.SortTime == nil {
		return story.Page{}, fmt.Errorf("create Story cursor: missing stable sort time")
	}
	page.NextCursor, err = pagination.Encode(pagination.Position{
		Kind:     "stories",
		Search:   search,
		State:    state,
		Tag:      tag,
		SourceID: sourceID,
		Bucket:   boolBucket(last.ReadAt == nil),
		Time:     *last.SortTime,
		ID:       string(last.ID),
	})
	if err != nil {
		return story.Page{}, err
	}
	return page, nil
}

func (store *StoryStore) storyReaderCounts(
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
			SELECT story.id, story.read_at, story.starred_at, story.hidden_at, story.later_at
			FROM stories AS story
			WHERE
				($1 = '' OR (
					story.display_title ILIKE '%' || $1 || '%'
					OR story.note ILIKE '%' || $1 || '%'
					OR EXISTS (
						SELECT 1
						FROM story_entries AS matching_member
						JOIN entries AS matching_entry ON matching_entry.id = matching_member.entry_id
						JOIN sources AS matching_source ON matching_source.id = matching_entry.source_id
						LEFT JOIN entry_annotations AS matching_annotation ON matching_annotation.entry_id = matching_entry.id
						WHERE matching_member.story_id = story.id
						  AND (
							matching_entry.source_title ILIKE '%' || $1 || '%'
							OR matching_entry.author ILIKE '%' || $1 || '%'
							OR matching_entry.summary ILIKE '%' || $1 || '%'
							OR matching_entry.content_html ILIKE '%' || $1 || '%'
							OR matching_source.name ILIKE '%' || $1 || '%'
							OR matching_annotation.book_title ILIKE '%' || $1 || '%'
							OR matching_annotation.book_author ILIKE '%' || $1 || '%'
							OR matching_annotation.chapter ILIKE '%' || $1 || '%'
							OR matching_annotation.annotation_note ILIKE '%' || $1 || '%'
							OR word_similarity(lower($1), lower(matching_entry.source_title)) >= 0.45
							OR pulse_fuzzy_contains(coalesce(matching_entry.source_title, ''), $1)
						  )
					)
				))
				AND (
					$3 = ''
					OR EXISTS (
						SELECT 1
						FROM story_tags AS story_tag
						JOIN tags ON tags.id = story_tag.tag_id
						WHERE story_tag.story_id = story.id
						  AND tags.normalized_name = lower($3)
					)
				)
				AND (
					$4 = ''
					OR EXISTS (
						SELECT 1
						FROM story_entries AS member
						JOIN entries AS source_entry ON source_entry.id = member.entry_id
						WHERE member.story_id = story.id
						  AND source_entry.source_id = $4::uuid
					)
				)
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
			count(*) FILTER (WHERE hidden_at IS NULL)::integer,
			count(*) FILTER (WHERE hidden_at IS NULL AND read_at IS NULL)::integer,
			count(*) FILTER (WHERE starred_at IS NOT NULL)::integer,
			count(*) FILTER (WHERE later_at IS NOT NULL)::integer,
			count(*) FILTER (WHERE hidden_at IS NOT NULL)::integer
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
		return 0, story.ReaderCounts{}, fmt.Errorf("count Story page: %w", err)
	}
	return total, counts, nil
}

func boolBucket(unread bool) int {
	if unread {
		return 1
	}
	return 0
}

func (store *StoryStore) Get(ctx context.Context, id story.ID) (story.Story, error) {
	resolvedID, err := store.resolveID(ctx, id)
	if err != nil {
		return story.Story{}, err
	}
	id = resolvedID
	var item story.Story
	var representativeID entry.ID
	var tagsJSON []byte
	err = store.pool.QueryRow(ctx, `
		WITH aggregates AS (
			SELECT
				count(*)::integer AS entry_count,
				count(DISTINCT entry.source_id)::integer AS source_count,
				min(coalesce(entry.published_at, entry.discovered_at)) AS first_published_at,
				max(coalesce(entry.published_at, entry.discovered_at)) AS last_published_at
			FROM story_entries AS member
			JOIN entries AS entry ON entry.id = member.entry_id
			WHERE member.story_id = $1
		)
		SELECT
			story.id, story.representative_entry_id, story.display_title, story.note,
			COALESCE((
				SELECT jsonb_agg(jsonb_build_object('id', tag.id, 'name', tag.name) ORDER BY tag.name)
				FROM story_tags AS story_tag
				JOIN tags AS tag ON tag.id = story_tag.tag_id
				WHERE story_tag.story_id = story.id
			), '[]'::jsonb),
			aggregate.entry_count, aggregate.source_count,
			aggregate.first_published_at, aggregate.last_published_at,
			story.read_at, story.starred_at, story.hidden_at, story.later_at
		FROM stories AS story
		CROSS JOIN aggregates AS aggregate
		WHERE story.id = $1
	`, id).Scan(
		&item.ID, &representativeID, &item.DisplayTitle, &item.Note, &tagsJSON,
		&item.EntryCount, &item.SourceCount, &item.FirstPublishedAt, &item.LastPublishedAt,
		&item.ReadAt, &item.StarredAt, &item.HiddenAt, &item.LaterAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return story.Story{}, fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	if err != nil {
		return story.Story{}, fmt.Errorf("get Story metadata %s: %w", id, err)
	}
	if len(tagsJSON) > 0 && string(tagsJSON) != "null" {
		if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
			return story.Story{}, fmt.Errorf("decode Story tags %s: %w", id, err)
		}
	}

	rows, err := store.pool.Query(ctx, `
		SELECT
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at'
		FROM story_entries AS member
		JOIN entries AS entry ON entry.id = member.entry_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE member.story_id = $1
		ORDER BY coalesce(entry.published_at, entry.discovered_at) DESC, entry.id DESC
	`, id)
	if err != nil {
		return story.Story{}, fmt.Errorf("get Story entries %s: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		entryItem, scanErr := scanEntry(rows)
		if scanErr != nil {
			return story.Story{}, fmt.Errorf("scan Story Entry: %w", scanErr)
		}
		item.Entries = append(item.Entries, entryItem)
	}
	if err := rows.Err(); err != nil {
		return story.Story{}, fmt.Errorf("get Story %s: %w", id, err)
	}
	if len(item.Entries) == 0 {
		return story.Story{}, fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	item.Representative = item.Entries[0]
	for _, candidate := range item.Entries {
		if candidate.ID == representativeID {
			item.Representative = candidate
			break
		}
	}
	return item, nil
}

func (store *StoryStore) Pending(ctx context.Context, limit int, model string) ([]story.Candidate, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := store.pool.Query(ctx, `
		SELECT
			story.id, story.clustered_at,
			(SELECT count(*)::integer FROM story_entries WHERE story_id = story.id),
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at',
			entry.normalized_title, entry.content_hash, entry.content_simhash,
			coalesce(entry.embedding::text, ''), entry.embedding_model
		FROM stories AS story
		JOIN entries AS entry ON entry.id = story.representative_entry_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
		WHERE (
			story.clustered_at IS NULL
			OR (
				$2 <> ''
				AND (
					(entry.embedding IS NOT NULL AND entry.embedding_model <> $2)
					OR (
						entry.embedding IS NULL
						AND entry.embedding_attempted_at IS NOT NULL
						AND entry.embedding_attempted_at < now() - interval '1 day'
					)
					-- Backfill: entries clustered while embeddings were disabled never had a
					-- vector attempted. Once a model is configured, pick them up so recluster
					-- (and the background worker) gradually re-embed and re-cluster history.
					OR (entry.embedding IS NULL AND entry.embedding_attempted_at IS NULL)
				)
			)
		  )
		ORDER BY
			(story.clustered_at IS NULL) DESC,
			coalesce(entry.embedding_attempted_at, '-infinity') ASC,
			story.created_at ASC
		LIMIT $1
	`, limit, model)
	if err != nil {
		return nil, fmt.Errorf("list pending Stories: %w", err)
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func (store *StoryStore) Candidates(
	ctx context.Context,
	item story.Candidate,
	limit int,
) ([]story.Candidate, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	var embeddingVector any
	if len(item.Features.Embedding) > 0 && item.Features.EmbeddingModel != "" {
		embeddingVector = vectorLiteral(item.Features.Embedding)
	}
	rows, err := store.pool.Query(ctx, `
		WITH lexical_candidates AS (
			SELECT story.id
			FROM stories AS story
			JOIN entries AS entry ON entry.id = story.representative_entry_id
			WHERE story.id <> $1
			  AND (SELECT count(*) FROM story_entries WHERE story_id = $1) = 1
			  AND coalesce(entry.published_at, entry.discovered_at)
			      BETWEEN $2::timestamptz - interval '7 days'
			          AND $2::timestamptz + interval '7 days'
			ORDER BY
				CASE WHEN entry.content_hash <> '' AND entry.content_hash = $3 THEN 1 ELSE 0 END DESC,
				word_similarity(entry.normalized_title, $4) DESC,
				coalesce(entry.published_at, entry.discovered_at) DESC
			LIMIT greatest(1, $7 / 2)
		), semantic_candidates AS (
			SELECT story.id
			FROM stories AS story
			JOIN entries AS entry ON entry.id = story.representative_entry_id
			WHERE story.id <> $1
			  AND (SELECT count(*) FROM story_entries WHERE story_id = $1) = 1
			  AND $5::vector IS NOT NULL
			  AND entry.embedding IS NOT NULL
			  AND entry.embedding_model = $6
			  AND vector_dims(entry.embedding) = vector_dims($5::vector)
			  AND coalesce(entry.published_at, entry.discovered_at)
			      BETWEEN $2::timestamptz - interval '7 days'
			          AND $2::timestamptz + interval '7 days'
			ORDER BY entry.embedding <=> $5::vector
			LIMIT greatest(1, $7 / 2)
		), selected AS (
			SELECT id FROM lexical_candidates
			UNION
			SELECT id FROM semantic_candidates
		)
		SELECT
			story.id, story.clustered_at,
			(SELECT count(*)::integer FROM story_entries WHERE story_id = story.id),
			entry.id, entry.source_id, entry.identity_key, entry.external_id, entry.canonical_url,
			entry.source_title, entry.author, entry.summary, entry.content_html,
			entry.published_at, entry.discovered_at,
			to_jsonb(entry_annotation) - 'entry_id' - 'imported_at',
			entry.normalized_title, entry.content_hash, entry.content_simhash,
			coalesce(entry.embedding::text, ''), entry.embedding_model
		FROM selected
		JOIN stories AS story ON story.id = selected.id
		JOIN entries AS entry ON entry.id = story.representative_entry_id
		LEFT JOIN entry_annotations AS entry_annotation ON entry_annotation.entry_id = entry.id
	`,
		item.StoryID,
		entryTimeForStore(item.Entry),
		item.Features.ContentHash,
		item.Features.NormalizedTitle,
		embeddingVector,
		item.Features.EmbeddingModel,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list Story candidates: %w", err)
	}
	defer rows.Close()
	return scanCandidates(rows)
}

func (store *StoryStore) SaveEmbedding(
	ctx context.Context,
	id entry.ID,
	model string,
	vector []float32,
) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE entries
		SET
			embedding = $2::vector,
			embedding_model = $3,
			embedding_updated_at = now()
		WHERE id = $1
	`, id, vectorLiteral(vector), model)
	if err != nil {
		return fmt.Errorf("save Entry embedding %s: %w", id, err)
	}
	return nil
}

func (store *StoryStore) SaveFeatures(
	ctx context.Context,
	id entry.ID,
	features story.Features,
) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE entries
		SET
			normalized_title = $2,
			content_hash = $3,
			content_simhash = $4
		WHERE id = $1
	`, id, features.NormalizedTitle, features.ContentHash, int64(features.ContentSimHash))
	if err != nil {
		return fmt.Errorf("save Entry Story features %s: %w", id, err)
	}
	return nil
}

func (store *StoryStore) MarkEmbeddingAttempted(ctx context.Context, id entry.ID) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE entries SET embedding_attempted_at = now() WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("mark Entry embedding attempted %s: %w", id, err)
	}
	return nil
}

func (store *StoryStore) Merge(
	ctx context.Context,
	from story.ID,
	into story.ID,
	match story.Result,
) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Story merge: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		SELECT id
		FROM stories
		WHERE id = ANY($1::uuid[])
		ORDER BY id
		FOR UPDATE
	`, []string{string(from), string(into)}); err != nil {
		return fmt.Errorf("lock Stories: %w", err)
	}
	var sourceEntryCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM story_entries
		WHERE story_id = $1
	`, from).Scan(&sourceEntryCount); err != nil {
		return fmt.Errorf("count source Story entries: %w", err)
	}
	if sourceEntryCount != 1 {
		return fmt.Errorf("automatic Story merge requires a singleton source Story, got %d entries", sourceEntryCount)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE story_entries
		SET
			story_id = $2,
			match_method = $3,
			final_score = $4,
			embedding_score = $5,
			title_score = $6,
			content_score = $7,
			time_score = $8,
			critical_conflict = $9,
			algorithm_version = 1,
			joined_at = now()
		WHERE story_id = $1
	`, from, into, match.Method, match.FinalScore, match.EmbeddingScore,
		match.TitleScore, match.ContentScore, match.TimeScore, match.CriticalConflict)
	if err != nil {
		return fmt.Errorf("move Story entries: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE stories AS story
		SET
			read_at = CASE WHEN story.read_at IS NULL THEN source_story.read_at WHEN source_story.read_at IS NULL THEN story.read_at ELSE greatest(story.read_at, source_story.read_at) END,
			starred_at = CASE WHEN story.starred_at IS NULL THEN source_story.starred_at WHEN source_story.starred_at IS NULL THEN story.starred_at ELSE greatest(story.starred_at, source_story.starred_at) END,
			hidden_at = CASE WHEN story.hidden_at IS NULL THEN source_story.hidden_at WHEN source_story.hidden_at IS NULL THEN story.hidden_at ELSE greatest(story.hidden_at, source_story.hidden_at) END,
			later_at = CASE WHEN story.later_at IS NULL THEN source_story.later_at WHEN source_story.later_at IS NULL THEN story.later_at ELSE greatest(story.later_at, source_story.later_at) END,
			clustered_at = now(),
			updated_at = now()
		FROM stories AS source_story
		WHERE story.id = $1 AND source_story.id = $2
	`, into, from); err != nil {
		return fmt.Errorf("refresh target Story: %w", err)
	}
	if err := mergeStoryMetadataTx(ctx, tx, from, into, story.MergeOptions{}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM stories WHERE id = $1", from); err != nil {
		return fmt.Errorf("delete empty Story: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Story merge: %w", err)
	}
	return nil
}

func (store *StoryStore) MergeManual(
	ctx context.Context,
	from story.ID,
	into story.ID,
	options story.MergeOptions,
) error {
	resolvedFrom, err := store.resolveID(ctx, from)
	if err != nil {
		return err
	}
	resolvedInto, err := store.resolveID(ctx, into)
	if err != nil {
		return err
	}
	from, into = resolvedFrom, resolvedInto
	if from == into {
		return story.ErrSelfMerge
	}
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin manual Story merge: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		SELECT id
		FROM stories
		WHERE id = ANY($1::uuid[])
		ORDER BY id
		FOR UPDATE
	`, []string{string(from), string(into)}); err != nil {
		return fmt.Errorf("lock Stories: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE story_entries
		SET
			story_id = $2,
			match_method = $3,
			final_score = NULL,
			embedding_score = NULL,
			title_score = NULL,
			content_score = NULL,
			time_score = NULL,
			critical_conflict = false,
			algorithm_version = 1,
			joined_at = now()
		WHERE story_id = $1
	`, from, into, story.MatchManual)
	if err != nil {
		return fmt.Errorf("move Story entries: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE stories AS story
		SET
			read_at = CASE WHEN story.read_at IS NULL THEN source_story.read_at WHEN source_story.read_at IS NULL THEN story.read_at ELSE greatest(story.read_at, source_story.read_at) END,
			starred_at = CASE WHEN story.starred_at IS NULL THEN source_story.starred_at WHEN source_story.starred_at IS NULL THEN story.starred_at ELSE greatest(story.starred_at, source_story.starred_at) END,
			hidden_at = CASE WHEN story.hidden_at IS NULL THEN source_story.hidden_at WHEN source_story.hidden_at IS NULL THEN story.hidden_at ELSE greatest(story.hidden_at, source_story.hidden_at) END,
			later_at = CASE WHEN story.later_at IS NULL THEN source_story.later_at WHEN source_story.later_at IS NULL THEN story.later_at ELSE greatest(story.later_at, source_story.later_at) END,
			clustered_at = now(),
			updated_at = now()
		FROM stories AS source_story
		WHERE story.id = $1 AND source_story.id = $2
	`, into, from); err != nil {
		return fmt.Errorf("refresh target Story: %w", err)
	}

	if err := mergeStoryMetadataTx(ctx, tx, from, into, options); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, "DELETE FROM stories WHERE id = $1", from); err != nil {
		return fmt.Errorf("delete empty Story: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit manual Story merge: %w", err)
	}
	return nil
}

func (store *StoryStore) Split(
	ctx context.Context,
	storyID story.ID,
	entryID entry.ID,
	options story.SplitOptions,
) (story.ID, error) {
	if (options.CopyDisplayTitle && options.MoveDisplayTitle) ||
		(options.CopyNote && options.MoveNote) ||
		(options.CopyTags && options.MoveTags) {
		return "", fmt.Errorf("split metadata cannot be copied and moved at the same time")
	}
	resolvedID, err := store.resolveID(ctx, storyID)
	if err != nil {
		return "", err
	}
	storyID = resolvedID
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin Story split: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		SELECT id FROM stories WHERE id = $1 FOR UPDATE
	`, storyID); err != nil {
		return "", fmt.Errorf("lock Story: %w", err)
	}
	var memberCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM story_entries WHERE story_id = $1
	`, storyID).Scan(&memberCount); err != nil {
		return "", fmt.Errorf("count Story entries for split: %w", err)
	}
	if memberCount <= 1 {
		return "", fmt.Errorf("cannot split the final Entry from a Story")
	}

	var newID story.ID
	err = tx.QueryRow(ctx, `
		INSERT INTO stories (
			representative_entry_id,
			sort_time,
			read_at,
			starred_at,
			hidden_at,
			later_at,
			clustered_at,
			display_title,
			note
		)
		SELECT
			entry.id,
			least(coalesce(entry.published_at, entry.discovered_at), entry.discovered_at),
			original.read_at,
			original.starred_at,
			original.hidden_at,
			original.later_at,
			now(),
			CASE WHEN $3 OR $4 THEN original.display_title ELSE '' END,
			CASE WHEN $5 OR $6 THEN original.note ELSE '' END
		FROM entries AS entry
		JOIN stories AS original ON original.id = $1
		WHERE entry.id = $2
		  AND EXISTS (SELECT 1 FROM story_entries WHERE story_id = $1 AND entry_id = $2)
		RETURNING stories.id
	`, storyID, entryID, options.CopyDisplayTitle, options.MoveDisplayTitle,
		options.CopyNote, options.MoveNote).Scan(&newID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("%w: entry %s in story %s", entry.ErrNotFound, entryID, storyID)
		}
		return "", fmt.Errorf("create split Story: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE story_entries
		SET
			story_id = $3,
			match_method = $4,
			final_score = NULL,
			embedding_score = NULL,
			title_score = NULL,
			content_score = NULL,
			time_score = NULL,
			critical_conflict = false,
			algorithm_version = 1,
			joined_at = now()
		WHERE story_id = $1 AND entry_id = $2
	`, storyID, entryID, newID, story.MatchManual); err != nil {
		return "", fmt.Errorf("move split entry: %w", err)
	}
	if options.MoveDisplayTitle || options.MoveNote {
		if _, err := tx.Exec(ctx, `
			UPDATE stories
			SET
				display_title = CASE WHEN $2 THEN '' ELSE display_title END,
				note = CASE WHEN $3 THEN '' ELSE note END,
				updated_at = now()
			WHERE id = $1
		`, storyID, options.MoveDisplayTitle, options.MoveNote); err != nil {
			return "", fmt.Errorf("move split Story metadata: %w", err)
		}
	}
	if options.CopyTags || options.MoveTags {
		if options.CopyTags {
			if _, err := tx.Exec(ctx, `
				INSERT INTO story_tags (story_id, tag_id, origin, rule_id, created_at)
				SELECT $2, tag_id, origin, rule_id, created_at
				FROM story_tags
				WHERE story_id = $1 AND origin = 'user'
				ON CONFLICT DO NOTHING
			`, storyID, newID); err != nil {
				return "", fmt.Errorf("copy split Story tags: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO story_rule_tags (rule_id, rule_version, entry_id, story_id, tag_id, created_at)
				SELECT rule_id, rule_version, entry_id, $2, tag_id, created_at
				FROM story_rule_tags
				WHERE story_id = $1 AND entry_id = $3
				ON CONFLICT DO NOTHING
			`, storyID, newID, entryID); err != nil {
				return "", fmt.Errorf("copy split Story rule tags: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO story_tags (story_id, tag_id, origin, rule_id, created_at)
				SELECT DISTINCT ON (tag_id) $2, tag_id, 'rule', rule_id, created_at
				FROM story_rule_tags
				WHERE story_id = $2
				ORDER BY tag_id, created_at, rule_id
				ON CONFLICT DO NOTHING
			`, storyID, newID); err != nil {
				return "", fmt.Errorf("copy split Story rule tags: %w", err)
			}
		} else {
			if _, err := tx.Exec(ctx, `
				UPDATE story_tags SET story_id = $2 WHERE story_id = $1 AND origin = 'user'
			`, storyID, newID); err != nil {
				return "", fmt.Errorf("move split Story tags: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE story_rule_tags SET story_id = $3 WHERE story_id = $1 AND entry_id = $2
			`, storyID, entryID, newID); err != nil {
				return "", fmt.Errorf("move split Story rule tags: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				DELETE FROM story_tags
				WHERE story_id IN ($1, $2) AND origin = 'rule'
			`, storyID, newID); err != nil {
				return "", fmt.Errorf("refresh split Story rule tags: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO story_tags (story_id, tag_id, origin, rule_id, created_at)
				SELECT DISTINCT ON (story_id, tag_id) story_id, tag_id, 'rule', rule_id, created_at
				FROM story_rule_tags
				WHERE story_id IN ($1, $2)
				ORDER BY story_id, tag_id, created_at, rule_id
				ON CONFLICT DO NOTHING
			`, storyID, newID); err != nil {
				return "", fmt.Errorf("rebuild split Story rule tags: %w", err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE stories AS story
		SET
			representative_entry_id = (
				SELECT member.entry_id
				FROM story_entries AS member
				WHERE member.story_id = story.id
				ORDER BY member.joined_at ASC
				LIMIT 1
			),
			clustered_at = now(),
			updated_at = now()
		WHERE story.id = $1
	`, storyID); err != nil {
		return "", fmt.Errorf("refresh original Story: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit Story split: %w", err)
	}
	return newID, nil
}

func mergeStoryMetadataTx(
	ctx context.Context,
	tx pgx.Tx,
	from story.ID,
	into story.ID,
	options story.MergeOptions,
) error {
	var targetTitle, targetNote, sourceTitle, sourceNote string
	if err := tx.QueryRow(ctx, `
		SELECT target.display_title, target.note, source.display_title, source.note
		FROM stories AS target
		JOIN stories AS source ON source.id = $2
		WHERE target.id = $1
		FOR UPDATE OF target, source
	`, into, from).Scan(&targetTitle, &targetNote, &sourceTitle, &sourceNote); err != nil {
		return fmt.Errorf("load Stories for merge: %w", err)
	}
	if targetTitle != "" && sourceTitle != "" && targetTitle != sourceTitle {
		if options.DisplayTitle == nil {
			return fmt.Errorf("%w: display title", story.ErrMetadataConflict)
		}
	}
	if targetNote != "" && sourceNote != "" && targetNote != sourceNote {
		if options.Note == nil {
			return fmt.Errorf("%w: Note", story.ErrMetadataConflict)
		}
	}
	mergedTitle := targetTitle
	if options.DisplayTitle != nil {
		mergedTitle = *options.DisplayTitle
	} else if mergedTitle == "" {
		mergedTitle = sourceTitle
	}
	mergedNote := targetNote
	if options.Note != nil {
		mergedNote = *options.Note
	} else if mergedNote == "" {
		mergedNote = sourceNote
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO story_tags (story_id, tag_id, origin, rule_id, created_at)
		SELECT $2, tag_id, origin, rule_id, created_at
		FROM story_tags
		WHERE story_id = $1
		ON CONFLICT (story_id, tag_id, origin) DO NOTHING
	`, from, into); err != nil {
		return fmt.Errorf("union Story tags: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE story_rule_tags
		SET story_id = $2
		WHERE story_id = $1
	`, from, into); err != nil {
		return fmt.Errorf("move Story rule tags: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE story_aliases
		SET canonical_story_id = $2
		WHERE canonical_story_id = $1
	`, from, into); err != nil {
		return fmt.Errorf("flatten Story aliases: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO story_aliases (alias_id, canonical_story_id)
		VALUES ($1, $2)
		ON CONFLICT (alias_id) DO UPDATE SET canonical_story_id = EXCLUDED.canonical_story_id
	`, from, into); err != nil {
		return fmt.Errorf("record Story alias: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE stories
		SET
			display_title = $2,
			note = $3,
			updated_at = now()
		WHERE id = $1
	`, into, mergedTitle, mergedNote); err != nil {
		return fmt.Errorf("union Story metadata: %w", err)
	}
	return nil
}

func (store *StoryStore) MarkClustered(ctx context.Context, id story.ID) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE stories
		SET
			clustered_at = coalesce(clustered_at, now()),
			updated_at = CASE WHEN clustered_at IS NULL THEN now() ELSE updated_at END
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("mark Story clustered %s: %w", id, err)
	}
	return nil
}

func (store *StoryStore) Update(
	ctx context.Context,
	id story.ID,
	patch story.Patch,
) (story.Story, error) {
	resolvedID, err := store.resolveID(ctx, id)
	if err != nil {
		return story.Story{}, err
	}
	id = resolvedID
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return story.Story{}, fmt.Errorf("begin Story update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE stories
		SET
			read_at = CASE WHEN $2 THEN CASE WHEN $3 THEN coalesce(read_at, now()) ELSE NULL END ELSE read_at END,
			starred_at = CASE WHEN $4 THEN CASE WHEN $5 THEN coalesce(starred_at, now()) ELSE NULL END ELSE starred_at END,
			hidden_at = CASE WHEN $6 THEN CASE WHEN $7 THEN coalesce(hidden_at, now()) ELSE NULL END ELSE hidden_at END,
			later_at = CASE WHEN $8 THEN CASE WHEN $9 THEN coalesce(later_at, now()) ELSE NULL END ELSE later_at END,
			display_title = CASE WHEN $10 THEN $11 ELSE display_title END,
			note = CASE WHEN $12 THEN $13 ELSE note END,
			updated_at = now()
		WHERE id = $1
	`, id,
		patch.Read != nil, boolValue(patch.Read),
		patch.Starred != nil, boolValue(patch.Starred),
		patch.Hidden != nil, boolValue(patch.Hidden),
		patch.Later != nil, boolValue(patch.Later),
		patch.DisplayTitle != nil, stringValue(patch.DisplayTitle),
		patch.Note != nil, stringValue(patch.Note),
	)
	if err != nil {
		return story.Story{}, fmt.Errorf("update Story %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return story.Story{}, fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	if err := tx.Commit(ctx); err != nil {
		return story.Story{}, fmt.Errorf("commit Story update %s: %w", id, err)
	}
	return store.Get(ctx, id)
}

func (store *StoryStore) SetRepresentative(
	ctx context.Context,
	id story.ID,
	entryID entry.ID,
) (story.Story, error) {
	resolvedID, err := store.resolveID(ctx, id)
	if err != nil {
		return story.Story{}, err
	}
	tag, err := store.pool.Exec(ctx, `
		UPDATE stories
		SET representative_entry_id = $2, updated_at = now()
		WHERE id = $1
		  AND EXISTS (
			SELECT 1 FROM story_entries
			WHERE story_id = $1 AND entry_id = $2
		  )
	`, resolvedID, entryID)
	if err != nil {
		return story.Story{}, fmt.Errorf("set Story representative: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return story.Story{}, fmt.Errorf("%w: Entry %s is not a member of Story %s", entry.ErrNotFound, entryID, resolvedID)
	}
	return store.Get(ctx, resolvedID)
}

func (store *StoryStore) MarkRead(ctx context.Context, sourceID string, storyIDs []string) (int64, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin mark Stories read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var rows pgx.Rows
	if len(storyIDs) > 0 {
		rows, err = tx.Query(ctx, `
			UPDATE stories AS story
			SET read_at = now(), updated_at = now()
			WHERE story.read_at IS NULL
			  AND (
				story.id = ANY($1::uuid[])
				OR EXISTS (
					SELECT 1
					FROM story_aliases AS alias
					WHERE alias.alias_id = ANY($1::uuid[])
					  AND alias.canonical_story_id = story.id
				)
			  )
			RETURNING story.id
		`, storyIDs)
	} else {
		rows, err = tx.Query(ctx, `
			UPDATE stories AS story
			SET read_at = now(), updated_at = now()
			WHERE story.read_at IS NULL
			  AND (
				$1 = ''
				OR EXISTS (
					SELECT 1
					FROM story_entries AS member
					JOIN entries AS entry ON entry.id = member.entry_id
					WHERE member.story_id = story.id AND entry.source_id = $1::uuid
				)
			  )
			RETURNING story.id
		`, sourceID)
	}
	if err != nil {
		return 0, fmt.Errorf("mark Stories read: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan marked Story: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("mark Stories read: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit mark Stories read: %w", err)
	}
	return int64(len(ids)), nil
}

func (store *StoryStore) AddTag(ctx context.Context, id story.ID, name string) (entry.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return entry.Tag{}, fmt.Errorf("tag name is required")
	}
	resolvedID, err := store.resolveID(ctx, id)
	if err != nil {
		return entry.Tag{}, err
	}
	id = resolvedID
	var tag entry.Tag
	err = store.pool.QueryRow(ctx, `
		WITH selected_tag AS (
			INSERT INTO tags (name, normalized_name)
			VALUES ($2, lower($2))
			ON CONFLICT (normalized_name) DO UPDATE SET name = tags.name
			RETURNING id, name
		), linked AS (
			INSERT INTO story_tags (story_id, tag_id, origin)
			SELECT $1, id, 'user' FROM selected_tag
			ON CONFLICT DO NOTHING
		)
		SELECT id, name FROM selected_tag
	`, id, name).Scan(&tag.ID, &tag.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return entry.Tag{}, fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	if err != nil {
		return entry.Tag{}, fmt.Errorf("add tag to Story %s: %w", id, err)
	}
	return tag, nil
}

func (store *StoryStore) RemoveTag(ctx context.Context, id story.ID, tagID string) error {
	resolvedID, err := store.resolveID(ctx, id)
	if err != nil {
		return err
	}
	id = resolvedID
	if _, err := store.pool.Exec(ctx, `
		DELETE FROM story_tags
		WHERE story_id = $1 AND tag_id = $2 AND origin = 'user'
	`, id, tagID); err != nil {
		return fmt.Errorf("remove tag from Story %s: %w", id, err)
	}
	return nil
}

type storyRow interface {
	Scan(...any) error
}

func scanStory(row storyRow) (story.Story, error) {
	var item story.Story
	var representative entry.Entry
	var annotationJSON []byte
	var tagsJSON []byte
	err := row.Scan(
		&item.ID,
		&item.SortTime,
		&item.DisplayTitle,
		&item.Note,
		&tagsJSON,
		&item.EntryCount,
		&item.SourceCount,
		&item.FirstPublishedAt,
		&item.LastPublishedAt,
		&item.ReadAt,
		&item.StarredAt,
		&item.HiddenAt,
		&item.LaterAt,
		&representative.ID,
		&representative.SourceID,
		&representative.IdentityKey,
		&representative.ExternalID,
		&representative.CanonicalURL,
		&representative.SourceTitle,
		&representative.Author,
		&representative.Summary,
		&representative.ContentHTML,
		&representative.PublishedAt,
		&representative.DiscoveredAt,
		&annotationJSON,
	)
	if err != nil {
		return story.Story{}, err
	}
	if err := decodeAnnotation(annotationJSON, &representative); err != nil {
		return story.Story{}, err
	}
	if len(tagsJSON) > 0 && string(tagsJSON) != "null" {
		if err := json.Unmarshal(tagsJSON, &item.Tags); err != nil {
			return story.Story{}, fmt.Errorf("decode Story tags: %w", err)
		}
	}
	item.Representative = representative
	return item, nil
}

func scanCandidates(rows pgx.Rows) ([]story.Candidate, error) {
	var result []story.Candidate
	for rows.Next() {
		var item story.Candidate
		var annotationJSON []byte
		var simHash int64
		var vector string
		err := rows.Scan(
			&item.StoryID,
			&item.ClusteredAt,
			&item.EntryCount,
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
			&item.Features.NormalizedTitle,
			&item.Features.ContentHash,
			&simHash,
			&vector,
			&item.Features.EmbeddingModel,
		)
		if err != nil {
			return nil, err
		}
		item.Features.ContentSimHash = uint64(simHash)
		item.Features.Embedding, err = parseVector(vector)
		if err != nil {
			return nil, err
		}
		item.Features.CanonicalURL = item.Entry.CanonicalURL
		if err := decodeAnnotation(annotationJSON, &item.Entry); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func vectorLiteral(vector []float32) string {
	var result strings.Builder
	result.WriteByte('[')
	for index, value := range vector {
		if index > 0 {
			result.WriteByte(',')
		}
		result.WriteString(strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	result.WriteByte(']')
	return result.String()
}

func parseVector(value string) ([]float32, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	value = strings.TrimPrefix(strings.TrimSuffix(value, "]"), "[")
	parts := strings.Split(value, ",")
	result := make([]float32, len(parts))
	for index, part := range parts {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return nil, fmt.Errorf("parse embedding vector: %w", err)
		}
		result[index] = float32(parsed)
	}
	return result, nil
}

func entryTimeForStore(item entry.Entry) time.Time {
	if item.PublishedAt != nil {
		return *item.PublishedAt
	}
	return item.DiscoveredAt
}

func decodeAnnotation(value []byte, item *entry.Entry) error {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	if err := json.Unmarshal(value, &item.Annotation); err != nil {
		return err
	}
	return nil
}

func (store *StoryStore) resolveID(ctx context.Context, id story.ID) (story.ID, error) {
	var resolved story.ID
	err := store.pool.QueryRow(ctx, `
		WITH RECURSIVE chain(id) AS (
			SELECT $1::uuid
			UNION
			SELECT alias.canonical_story_id
			FROM story_aliases AS alias
			JOIN chain ON chain.id = alias.alias_id
		)
		SELECT chain.id
		FROM chain
		JOIN stories AS story ON story.id = chain.id
		LIMIT 1
	`, id).Scan(&resolved)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", entry.ErrNotFound, id)
	}
	if err != nil {
		return "", fmt.Errorf("resolve Story %s: %w", id, err)
	}
	return resolved, nil
}
