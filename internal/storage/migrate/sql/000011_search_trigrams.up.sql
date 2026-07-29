CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX entries_source_title_trgm_idx
    ON entries USING gin (source_title gin_trgm_ops);

CREATE INDEX entries_display_title_trgm_idx
    ON entries USING gin (display_title gin_trgm_ops);

CREATE INDEX entries_author_trgm_idx
    ON entries USING gin (author gin_trgm_ops);

CREATE INDEX sources_name_trgm_idx
    ON sources USING gin (name gin_trgm_ops);
