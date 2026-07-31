CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE entries
    ADD COLUMN normalized_title text NOT NULL DEFAULT '',
    ADD COLUMN content_hash text NOT NULL DEFAULT '',
    ADD COLUMN content_simhash bigint NOT NULL DEFAULT 0,
    ADD COLUMN embedding vector,
    ADD COLUMN embedding_model text NOT NULL DEFAULT '',
    ADD COLUMN embedding_updated_at timestamptz,
    ADD COLUMN embedding_attempted_at timestamptz;

CREATE TABLE stories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    representative_entry_id uuid REFERENCES entries(id) ON DELETE SET NULL,
    entry_count integer NOT NULL DEFAULT 1 CHECK (entry_count >= 0),
    source_count integer NOT NULL DEFAULT 1 CHECK (source_count >= 0),
    first_published_at timestamptz,
    last_published_at timestamptz,
    clustered_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE story_entries (
    story_id uuid NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    entry_id uuid NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    match_method text NOT NULL DEFAULT 'singleton',
    final_score real,
    embedding_score real,
    title_score real,
    content_score real,
    time_score real,
    critical_conflict boolean NOT NULL DEFAULT false,
    algorithm_version integer NOT NULL DEFAULT 1,
    joined_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (story_id, entry_id),
    UNIQUE (entry_id)
);

INSERT INTO stories (
    representative_entry_id,
    first_published_at,
    last_published_at
)
SELECT
    id,
    coalesce(published_at, discovered_at),
    coalesce(published_at, discovered_at)
FROM entries;

INSERT INTO story_entries (story_id, entry_id)
SELECT story.id, story.representative_entry_id
FROM stories AS story
WHERE story.representative_entry_id IS NOT NULL;

CREATE INDEX stories_activity_idx
    ON stories (last_published_at DESC, id DESC);

CREATE INDEX story_entries_story_idx
    ON story_entries (story_id);

CREATE INDEX entries_normalized_title_trgm_idx
    ON entries USING gin (normalized_title gin_trgm_ops);
