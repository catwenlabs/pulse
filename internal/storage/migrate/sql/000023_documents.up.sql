CREATE TABLE documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    identifier text NOT NULL DEFAULT '',
    title text NOT NULL,
    author text NOT NULL DEFAULT '',
    original_filename text NOT NULL DEFAULT '',
    original bytea NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE document_chapters (
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    chapter_index integer NOT NULL,
    title text NOT NULL DEFAULT '',
    content_html text NOT NULL,
    PRIMARY KEY (document_id, chapter_index)
);

CREATE TABLE document_progress (
    document_id uuid PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
    chapter_index integer NOT NULL,
    scroll_ratio double precision NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE document_notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    chapter_index integer NOT NULL,
    highlight text NOT NULL,
    note text NOT NULL DEFAULT '',
    highlight_color text NOT NULL DEFAULT '',
    highlighted_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX document_notes_document_idx ON document_notes (document_id, chapter_index);
