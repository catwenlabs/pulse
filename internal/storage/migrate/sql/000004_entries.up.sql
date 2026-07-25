CREATE TABLE entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id),
    identity_key text NOT NULL,
    external_id text NOT NULL DEFAULT '',
    canonical_url text NOT NULL DEFAULT '',
    source_title text NOT NULL DEFAULT '',
    author text NOT NULL DEFAULT '',
    summary text NOT NULL DEFAULT '',
    content_html text NOT NULL DEFAULT '',
    published_at timestamptz,
    discovered_at timestamptz NOT NULL DEFAULT now(),
    source_updated_at timestamptz NOT NULL DEFAULT now(),
    source_deleted boolean NOT NULL DEFAULT false,
    display_title text NOT NULL DEFAULT '',
    note text NOT NULL DEFAULT '',
    read_at timestamptz,
    starred_at timestamptz,
    saved_at timestamptz,
    hidden_at timestamptz,
    UNIQUE (source_id, identity_key)
);

CREATE TABLE entry_tombstones (
    source_id uuid NOT NULL REFERENCES sources(id),
    identity_key text NOT NULL,
    deleted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source_id, identity_key)
);

CREATE INDEX entries_inbox_idx
    ON entries (discovered_at DESC)
    WHERE hidden_at IS NULL;
