CREATE TABLE ai_selection_tools (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    prompt_template text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Case-insensitive tool-name uniqueness. Names are compared after trimming
-- surrounding whitespace, so "AI 解读" and "  ai 解读  " collide.
CREATE UNIQUE INDEX ai_selection_tools_unique_name_idx
    ON ai_selection_tools (lower(trim(name)));
CREATE INDEX ai_selection_tools_order_idx
    ON ai_selection_tools (position, id);

CREATE TABLE ai_conversations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    selected_text text NOT NULL DEFAULT '',
    tool_name text NOT NULL DEFAULT '',
    prompt_template text NOT NULL DEFAULT '',
    idempotency_key text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- A Conversation is uniquely identified by its creation idempotency key, so a
-- transport retry cannot duplicate the Conversation or its first User Message.
CREATE UNIQUE INDEX ai_conversations_idempotency_idx
    ON ai_conversations (idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX ai_conversations_history_idx
    ON ai_conversations (updated_at DESC, id DESC);

CREATE TABLE ai_messages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id uuid NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('user', 'assistant')),
    content text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT ''
        CHECK (status IN ('', 'streaming', 'completed', 'cancelled', 'failed')),
    provider text NOT NULL DEFAULT '',
    model text NOT NULL DEFAULT '',
    prompt_tokens integer NOT NULL DEFAULT 0 CHECK (prompt_tokens >= 0),
    completion_tokens integer NOT NULL DEFAULT 0 CHECK (completion_tokens >= 0),
    finish_reason text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    idempotency_key text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX ai_messages_conversation_idx
    ON ai_messages (conversation_id, created_at, id);

-- At most one streaming Assistant Message per Conversation. The service checks
-- this first; the partial unique index is the durable backstop against races.
CREATE UNIQUE INDEX ai_messages_one_streaming_per_conversation_idx
    ON ai_messages (conversation_id)
    WHERE role = 'assistant' AND status = 'streaming';

-- A User Message or Assistant generation is uniquely identified by its
-- idempotency key, so a transport retry cannot duplicate messages or Provider
-- requests.
CREATE UNIQUE INDEX ai_messages_idempotency_idx
    ON ai_messages (idempotency_key)
    WHERE idempotency_key <> '';

-- Seed three ordinary, editable starter tools. Migrations apply exactly once,
-- so these records are created a single time and are never recreated after a
-- user edits, disables, reorders, or deletes them.
INSERT INTO ai_selection_tools (name, prompt_template, enabled, position) VALUES
    (
        'AI 解读',
        '请解读以下内容，用中文解释其中的关键概念、公式或论证逻辑；必要时使用 Markdown 与 LaTeX 让公式保持可读：' || E'\n\n' || '{{selection}}',
        true,
        0
    ),
    (
        'AI 翻译',
        '请将以下内容翻译成中文，保留专有名词、代码与原有格式；遇到歧义时给出最自然的一种译法：' || E'\n\n' || '{{selection}}',
        true,
        1
    ),
    (
        '举例说明',
        '请为以下内容举一个具体、易懂的例子，帮助读者真正理解它在说什么：' || E'\n\n' || '{{selection}}',
        true,
        2
    );
