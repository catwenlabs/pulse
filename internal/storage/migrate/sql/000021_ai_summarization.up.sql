CREATE TABLE ai_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL CHECK (kind IN ('story_summary', 'digest')),
    target_id uuid NOT NULL,
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'retry', 'completed', 'partial', 'failed', 'dead')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    requested_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz,
    lease_owner text NOT NULL DEFAULT '',
    lease_until timestamptz,
    last_error text NOT NULL DEFAULT ''
);

CREATE INDEX ai_jobs_claim_idx ON ai_jobs(status, available_at, requested_at, id);
CREATE INDEX ai_jobs_target_idx ON ai_jobs(kind, target_id, requested_at DESC);

CREATE TABLE story_ai_summaries (
    story_id uuid PRIMARY KEY,
    status text NOT NULL DEFAULT 'not_requested'
        CHECK (status IN ('not_requested', 'queued', 'running', 'completed', 'partial', 'failed', 'stale')),
    overview text NOT NULL DEFAULT '',
    key_points jsonb NOT NULL DEFAULT '[]'::jsonb,
    source_notes jsonb NOT NULL DEFAULT '[]'::jsonb,
    provider text NOT NULL DEFAULT '',
    model text NOT NULL DEFAULT '',
    prompt_version text NOT NULL DEFAULT '',
    input_fingerprint text NOT NULL DEFAULT '',
    job_id uuid REFERENCES ai_jobs(id) ON DELETE SET NULL,
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX story_ai_summaries_status_idx ON story_ai_summaries(status, updated_at DESC);

CREATE TABLE ai_digests (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mode text NOT NULL DEFAULT 'catch_up',
    status text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'partial', 'failed')),
    story_count integer NOT NULL DEFAULT 0 CHECK (story_count >= 0),
    start_at timestamptz,
    end_at timestamptz,
    overview text NOT NULL DEFAULT '',
    themes jsonb NOT NULL DEFAULT '[]'::jsonb,
    priorities jsonb NOT NULL DEFAULT '[]'::jsonb,
    omissions jsonb NOT NULL DEFAULT '[]'::jsonb,
    provider text NOT NULL DEFAULT '',
    model text NOT NULL DEFAULT '',
    prompt_version text NOT NULL DEFAULT '',
    input_fingerprint text NOT NULL DEFAULT '',
    job_id uuid REFERENCES ai_jobs(id) ON DELETE SET NULL,
    error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_digests_created_idx ON ai_digests(created_at DESC, id DESC);
CREATE INDEX ai_digests_status_idx ON ai_digests(status, updated_at DESC);

CREATE TABLE ai_digest_stories (
    digest_id uuid NOT NULL REFERENCES ai_digests(id) ON DELETE CASCADE,
    label text NOT NULL,
    story_id uuid NOT NULL,
    story_title text NOT NULL DEFAULT '',
    entry_count integer NOT NULL DEFAULT 1 CHECK (entry_count >= 1),
    source_count integer NOT NULL DEFAULT 1 CHECK (source_count >= 1),
    sort_time timestamptz,
    input_fingerprint text NOT NULL DEFAULT '',
    PRIMARY KEY (digest_id, label),
    UNIQUE (digest_id, story_id)
);

CREATE INDEX ai_digest_stories_story_idx ON ai_digest_stories(story_id, digest_id);
