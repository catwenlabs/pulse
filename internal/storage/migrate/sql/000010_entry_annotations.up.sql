CREATE TABLE entry_annotations (
    entry_id uuid PRIMARY KEY REFERENCES entries(id) ON DELETE CASCADE,
    provider text NOT NULL,
    book_identity text NOT NULL DEFAULT '',
    book_title text NOT NULL,
    book_author text NOT NULL DEFAULT '',
    chapter text NOT NULL DEFAULT '',
    location text NOT NULL DEFAULT '',
    highlight_color text NOT NULL DEFAULT '',
    annotation_note text NOT NULL DEFAULT '',
    highlighted_at timestamptz,
    imported_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX entry_annotations_book_idx
    ON entry_annotations (provider, book_identity);

CREATE INDEX entry_annotations_highlighted_idx
    ON entry_annotations (highlighted_at DESC);
