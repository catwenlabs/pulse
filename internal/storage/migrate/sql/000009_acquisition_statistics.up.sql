ALTER TABLE acquisitions
    ADD COLUMN new_count integer NOT NULL DEFAULT 0,
    ADD COLUMN updated_count integer NOT NULL DEFAULT 0;
