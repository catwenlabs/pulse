CREATE TABLE sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    driver_kind text NOT NULL,
    locator text NOT NULL,
    normalized_locator text NOT NULL,
    config jsonb NOT NULL DEFAULT '{}'::jsonb,
    secret_ref text NOT NULL DEFAULT '',
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    UNIQUE (driver_kind, normalized_locator)
);

CREATE TABLE source_checkpoints (
    source_id uuid PRIMARY KEY REFERENCES sources(id) ON DELETE CASCADE,
    checkpoint jsonb NOT NULL DEFAULT '{}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sources_enabled_idx
    ON sources (enabled)
    WHERE archived_at IS NULL;
