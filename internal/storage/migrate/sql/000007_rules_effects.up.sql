CREATE TABLE rules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    version integer NOT NULL DEFAULT 1 CHECK (version > 0),
    enabled boolean NOT NULL DEFAULT true,
    condition jsonb NOT NULL,
    actions jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rule_entry_tags (
    rule_id uuid NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    rule_version integer NOT NULL,
    entry_id uuid NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (rule_id, entry_id, tag_id)
);

CREATE TABLE effects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    effect_key text NOT NULL UNIQUE,
    rule_id uuid NOT NULL REFERENCES rules(id) ON DELETE CASCADE,
    rule_version integer NOT NULL,
    entry_id uuid NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('notification', 'webhook')),
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'retry', 'succeeded', 'dead')),
    attempts integer NOT NULL DEFAULT 0,
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text NOT NULL DEFAULT '',
    lease_until timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    delivered_at timestamptz
);

CREATE INDEX effects_claim_idx ON effects(status, available_at, created_at);

CREATE TABLE notifications (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    effect_id uuid NOT NULL UNIQUE REFERENCES effects(id) ON DELETE CASCADE,
    entry_id uuid NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    message text NOT NULL,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
