ALTER TABLE entries
    ADD COLUMN later_at timestamptz;

CREATE TABLE tags (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    normalized_name text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE entry_tags (
    entry_id uuid NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    origin text NOT NULL DEFAULT 'user' CHECK (origin IN ('user', 'rule')),
    rule_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (entry_id, tag_id, origin)
);

CREATE TABLE views (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    query jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX entries_search_idx
    ON entries
    USING gin (
        to_tsvector(
            'simple',
            coalesce(display_title, '') || ' ' ||
            coalesce(source_title, '') || ' ' ||
            coalesce(author, '') || ' ' ||
            coalesce(summary, '') || ' ' ||
            coalesce(content_html, '') || ' ' ||
            coalesce(note, '')
        )
    );

CREATE INDEX entry_tags_tag_entry_idx ON entry_tags(tag_id, entry_id);
