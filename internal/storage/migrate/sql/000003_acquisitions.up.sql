CREATE TABLE acquisitions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    trigger text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key text NOT NULL,
    status text NOT NULL DEFAULT 'pending',
    priority integer NOT NULL DEFAULT 0,
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    requested_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text NOT NULL DEFAULT '',
    lease_until timestamptz,
    last_error text NOT NULL DEFAULT '',
    UNIQUE (source_id, idempotency_key)
);

CREATE INDEX acquisitions_claim_idx
    ON acquisitions (priority DESC, available_at, requested_at)
    WHERE status IN ('pending', 'retry', 'running');
