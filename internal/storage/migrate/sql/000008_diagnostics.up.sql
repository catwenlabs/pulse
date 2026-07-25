ALTER TABLE acquisitions
    ADD COLUMN started_at timestamptz,
    ADD COLUMN finished_at timestamptz,
    ADD COLUMN candidate_count integer NOT NULL DEFAULT 0;

CREATE TABLE diagnostic_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    acquisition_id uuid REFERENCES acquisitions(id) ON DELETE SET NULL,
    status text NOT NULL,
    summary text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '7 days'
);

CREATE INDEX diagnostic_snapshots_expiry_idx ON diagnostic_snapshots(expires_at);
CREATE INDEX diagnostic_snapshots_source_idx ON diagnostic_snapshots(source_id, created_at DESC);
