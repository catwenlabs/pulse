CREATE TABLE folders (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX folders_name_ci_unique
    ON folders (lower(name));

CREATE TABLE source_folders (
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    folder_id uuid NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
    PRIMARY KEY (source_id, folder_id)
);

CREATE INDEX source_folders_folder_idx
    ON source_folders (folder_id, source_id);
