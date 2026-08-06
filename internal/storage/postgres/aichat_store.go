package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/catwenlabs/pulse/internal/aichat"
)

// AIChatStore persists the independent AI Chat domain. It contains persistence
// details only; all domain decisions live in the aichat service.
type AIChatStore struct {
	pool *pgxpool.Pool
}

func NewAIChatStore(pool *pgxpool.Pool) *AIChatStore {
	return &AIChatStore{pool: pool}
}

func (store *AIChatStore) ListTools(ctx context.Context) ([]aichat.SelectionTool, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, name, prompt_template, enabled, position, created_at, updated_at
		FROM ai_selection_tools
		ORDER BY position, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list selection tools: %w", err)
	}
	defer rows.Close()
	tools, err := scanTools(rows)
	if err != nil {
		return nil, err
	}
	return tools, nil
}

func (store *AIChatStore) GetEnabledTool(ctx context.Context, id string) (aichat.SelectionTool, error) {
	tool, err := store.scanTool(ctx, `
		SELECT id, name, prompt_template, enabled, position, created_at, updated_at
		FROM ai_selection_tools
		WHERE id = $1 AND enabled
	`, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return aichat.SelectionTool{}, fmt.Errorf("%w: %s", aichat.ErrToolNotFound, id)
	}
	if err != nil {
		return aichat.SelectionTool{}, err
	}
	return tool, nil
}

func (store *AIChatStore) CreateTool(ctx context.Context, input aichat.ToolInput) (aichat.SelectionTool, error) {
	tool, err := store.scanTool(ctx, `
		INSERT INTO ai_selection_tools (name, prompt_template, enabled, position)
		VALUES ($1, $2, $3, (SELECT COALESCE(MAX(position) + 1, 0) FROM ai_selection_tools))
		RETURNING id, name, prompt_template, enabled, position, created_at, updated_at
	`, input.Name, input.PromptTemplate, input.Enabled)
	if err != nil {
		if isUniqueViolation(err) {
			return aichat.SelectionTool{}, &aichat.DuplicateToolError{Name: input.Name}
		}
		return aichat.SelectionTool{}, fmt.Errorf("create selection tool: %w", err)
	}
	return tool, nil
}

func (store *AIChatStore) UpdateTool(ctx context.Context, id string, input aichat.ToolInput) (aichat.SelectionTool, error) {
	tool, err := store.scanTool(ctx, `
		UPDATE ai_selection_tools
		SET name = $2, prompt_template = $3, enabled = $4, updated_at = now()
		WHERE id = $1
		RETURNING id, name, prompt_template, enabled, position, created_at, updated_at
	`, id, input.Name, input.PromptTemplate, input.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return aichat.SelectionTool{}, fmt.Errorf("%w: %s", aichat.ErrToolNotFound, id)
	}
	if err != nil {
		if isUniqueViolation(err) {
			return aichat.SelectionTool{}, &aichat.DuplicateToolError{Name: input.Name}
		}
		return aichat.SelectionTool{}, fmt.Errorf("update selection tool: %w", err)
	}
	return tool, nil
}

func (store *AIChatStore) DeleteTool(ctx context.Context, id string) error {
	tag, err := store.pool.Exec(ctx, `DELETE FROM ai_selection_tools WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete selection tool: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", aichat.ErrToolNotFound, id)
	}
	return nil
}

func (store *AIChatStore) ReorderTools(ctx context.Context, ids []string) ([]aichat.SelectionTool, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin reorder selection tools: %w", err)
	}
	defer tx.Rollback(ctx)
	for position, id := range ids {
		if _, err := tx.Exec(ctx, `
			UPDATE ai_selection_tools SET position = $2, updated_at = updated_at WHERE id = $1
		`, id, position); err != nil {
			return nil, fmt.Errorf("reorder selection tool %s: %w", id, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit reorder selection tools: %w", err)
	}
	return store.ListTools(ctx)
}

func (store *AIChatStore) CreateConversation(
	ctx context.Context,
	params aichat.CreateConversationParams,
) (aichat.Conversation, aichat.Message, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return aichat.Conversation{}, aichat.Message{}, fmt.Errorf("begin create conversation: %w", err)
	}
	defer tx.Rollback(ctx)

	if params.IdempotencyKey != "" {
		var conversationID string
		if err := tx.QueryRow(ctx, `
			SELECT id FROM ai_conversations WHERE idempotency_key = $1
		`, params.IdempotencyKey).Scan(&conversationID); err == nil {
			conversation, msgErr := store.readConversationTx(ctx, tx, conversationID)
			if msgErr != nil {
				return aichat.Conversation{}, aichat.Message{}, msgErr
			}
			messages, listErr := store.listMessagesTx(ctx, tx, conversationID)
			if listErr != nil || len(messages) == 0 {
				return aichat.Conversation{}, aichat.Message{}, fmt.Errorf("load idempotent conversation: %w", errors.Join(listErr, aichat.ErrMessageNotFound))
			}
			return conversation, messages[0], nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return aichat.Conversation{}, aichat.Message{}, fmt.Errorf("lookup idempotent conversation: %w", err)
		}
	}

	var conversation aichat.Conversation
	err = tx.QueryRow(ctx, `
		INSERT INTO ai_conversations (selected_text, tool_name, prompt_template, idempotency_key)
		VALUES ($1, $2, $3, $4)
		RETURNING id, selected_text, tool_name, prompt_template, created_at, updated_at
	`, params.SelectedText, params.ToolName, params.PromptTemplate, params.IdempotencyKey).
		Scan(&conversation.ID, &conversation.SelectedText, &conversation.ToolName,
			&conversation.PromptTemplate, &conversation.CreatedAt, &conversation.UpdatedAt)
	if err != nil {
		return aichat.Conversation{}, aichat.Message{}, fmt.Errorf("create conversation: %w", err)
	}
	var message aichat.Message
	err = tx.QueryRow(ctx, `
		INSERT INTO ai_messages (conversation_id, role, content)
		VALUES ($1, 'user', $2)
		RETURNING id, conversation_id, role, content, status, provider, model,
		          prompt_tokens, completion_tokens, finish_reason, error, created_at, updated_at
	`, conversation.ID, params.InitialPrompt).Scan(
		&message.ID, &message.ConversationID, &message.Role, &message.Content, &message.Status,
		&message.Provider, &message.Model, &message.PromptTokens, &message.CompletionTokens,
		&message.FinishReason, &message.Error, &message.CreatedAt, &message.UpdatedAt,
	)
	if err != nil {
		return aichat.Conversation{}, aichat.Message{}, fmt.Errorf("create initial message: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return aichat.Conversation{}, aichat.Message{}, fmt.Errorf("commit create conversation: %w", err)
	}
	return conversation, message, nil
}

func (store *AIChatStore) ListConversations(ctx context.Context, limit int, cursor string) (aichat.ConversationPage, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, selected_text, tool_name, prompt_template, created_at, updated_at
		FROM ai_conversations
	`
	args := []any{}
	if cursor != "" {
		updatedAt, id, ok := decodeConversationCursor(cursor)
		if !ok {
			return aichat.ConversationPage{}, aichat.ErrInvalidCursor
		}
		query += " WHERE (updated_at, id) < ($2, $3)"
		args = append(args, updatedAt, id)
	}
	query += " ORDER BY updated_at DESC, id DESC LIMIT $1"
	args = append([]any{limit + 1}, args...)

	rows, err := store.pool.Query(ctx, query, args...)
	if err != nil {
		return aichat.ConversationPage{}, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()
	var items []aichat.Conversation
	for rows.Next() {
		var conversation aichat.Conversation
		if err := rows.Scan(&conversation.ID, &conversation.SelectedText, &conversation.ToolName,
			&conversation.PromptTemplate, &conversation.CreatedAt, &conversation.UpdatedAt); err != nil {
			return aichat.ConversationPage{}, fmt.Errorf("scan conversation: %w", err)
		}
		items = append(items, conversation)
	}
	if items == nil {
		items = []aichat.Conversation{}
	}
	page := aichat.ConversationPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		keeper := page.Items[limit-1]
		page.NextCursor = encodeConversationCursor(keeper.UpdatedAt, keeper.ID)
		page.HasMore = true
	}
	return page, nil
}

func (store *AIChatStore) GetConversation(ctx context.Context, id string) (aichat.Conversation, error) {
	return store.readConversationTx(ctx, store.pool, id)
}

func (store *AIChatStore) DeleteConversation(ctx context.Context, id string) error {
	tag, err := store.pool.Exec(ctx, `DELETE FROM ai_conversations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", aichat.ErrConversationNotFound, id)
	}
	return nil
}

func (store *AIChatStore) ListMessages(ctx context.Context, conversationID string) ([]aichat.Message, error) {
	return store.listMessagesTx(ctx, store.pool, conversationID)
}

func (store *AIChatStore) AppendUserMessage(ctx context.Context, params aichat.AppendUserMessageParams) (aichat.Message, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return aichat.Message{}, fmt.Errorf("begin append user message: %w", err)
	}
	defer tx.Rollback(ctx)

	if params.IdempotencyKey != "" {
		message, err := store.findMessageByKeyTx(ctx, tx, params.IdempotencyKey)
		if err == nil {
			return message, tx.Commit(ctx)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return aichat.Message{}, fmt.Errorf("lookup idempotent user message: %w", err)
		}
	}

	var message aichat.Message
	err = tx.QueryRow(ctx, `
		INSERT INTO ai_messages (conversation_id, role, content, idempotency_key)
		VALUES ($1, 'user', $2, $3)
		RETURNING id, conversation_id, role, content, status, provider, model,
		          prompt_tokens, completion_tokens, finish_reason, error, created_at, updated_at
	`, params.ConversationID, params.Content, params.IdempotencyKey).Scan(
		&message.ID, &message.ConversationID, &message.Role, &message.Content, &message.Status,
		&message.Provider, &message.Model, &message.PromptTokens, &message.CompletionTokens,
		&message.FinishReason, &message.Error, &message.CreatedAt, &message.UpdatedAt,
	)
	if err != nil {
		return aichat.Message{}, fmt.Errorf("append user message: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE ai_conversations SET updated_at = now() WHERE id = $1`, params.ConversationID); err != nil {
		return aichat.Message{}, fmt.Errorf("touch conversation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return aichat.Message{}, fmt.Errorf("commit append user message: %w", err)
	}
	return message, nil
}

func (store *AIChatStore) StartGeneration(ctx context.Context, conversationID, idempotencyKey string) (aichat.Message, bool, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return aichat.Message{}, false, fmt.Errorf("begin start generation: %w", err)
	}
	defer tx.Rollback(ctx)

	if idempotencyKey != "" {
		existing, lookupErr := store.findMessageByKeyTx(ctx, tx, idempotencyKey)
		if lookupErr == nil {
			return existing, false, tx.Commit(ctx)
		}
		if !errors.Is(lookupErr, pgx.ErrNoRows) {
			return aichat.Message{}, false, fmt.Errorf("lookup idempotent generation: %w", lookupErr)
		}
	}

	var active int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM ai_messages
		WHERE conversation_id = $1 AND role = 'assistant' AND status = 'streaming'
	`, conversationID).Scan(&active); err != nil {
		return aichat.Message{}, false, fmt.Errorf("check active generation: %w", err)
	}
	if active > 0 {
		return aichat.Message{}, false, aichat.ErrActiveGeneration
	}

	var message aichat.Message
	err = tx.QueryRow(ctx, `
		INSERT INTO ai_messages (conversation_id, role, content, status, idempotency_key)
		VALUES ($1, 'assistant', '', 'streaming', $2)
		RETURNING id, conversation_id, role, content, status, provider, model,
		          prompt_tokens, completion_tokens, finish_reason, error, created_at, updated_at
	`, conversationID, idempotencyKey).Scan(
		&message.ID, &message.ConversationID, &message.Role, &message.Content, &message.Status,
		&message.Provider, &message.Model, &message.PromptTokens, &message.CompletionTokens,
		&message.FinishReason, &message.Error, &message.CreatedAt, &message.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			// A concurrent request opened the generation first.
			return aichat.Message{}, false, aichat.ErrActiveGeneration
		}
		return aichat.Message{}, false, fmt.Errorf("start generation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE ai_conversations SET updated_at = now() WHERE id = $1`, conversationID); err != nil {
		return aichat.Message{}, false, fmt.Errorf("touch conversation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return aichat.Message{}, false, fmt.Errorf("commit start generation: %w", err)
	}
	return message, true, nil
}

func (store *AIChatStore) PeekGeneration(ctx context.Context, _, idempotencyKey string) (aichat.Message, bool, error) {
	if idempotencyKey == "" {
		return aichat.Message{}, false, nil
	}
	message, err := store.findMessageByKeyTx(ctx, store.pool, idempotencyKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return aichat.Message{}, false, nil
	}
	if err != nil {
		return aichat.Message{}, false, err
	}
	return message, true, nil
}

func (store *AIChatStore) CheckpointGeneration(ctx context.Context, messageID, content string) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE ai_messages
		SET content = $2, status = 'streaming', updated_at = now()
		WHERE id = $1 AND status = 'streaming'
	`, messageID, content)
	if err != nil {
		return fmt.Errorf("checkpoint generation: %w", err)
	}
	return nil
}

func (store *AIChatStore) CompleteGeneration(ctx context.Context, messageID string, result aichat.GenerationResult) error {
	tag, err := store.pool.Exec(ctx, `
		UPDATE ai_messages
		SET content = $2, status = $3, provider = $4, model = $5,
		    prompt_tokens = $6, completion_tokens = $7, finish_reason = $8,
		    error = $9, updated_at = now()
		WHERE id = $1
	`,
		messageID, result.Content, result.Status, result.Provider, result.Model,
		result.PromptTokens, result.CompletionTokens, result.FinishReason, result.Error,
	)
	if err != nil {
		return fmt.Errorf("complete generation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", aichat.ErrMessageNotFound, messageID)
	}
	return nil
}

const messageColumns = `
	SELECT id, conversation_id, role, content, status, provider, model,
	       prompt_tokens, completion_tokens, finish_reason, error, created_at, updated_at
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMessageRow(row rowScanner) (aichat.Message, error) {
	var message aichat.Message
	err := row.Scan(
		&message.ID, &message.ConversationID, &message.Role, &message.Content, &message.Status,
		&message.Provider, &message.Model, &message.PromptTokens, &message.CompletionTokens,
		&message.FinishReason, &message.Error, &message.CreatedAt, &message.UpdatedAt,
	)
	return message, err
}

func (store *AIChatStore) listMessagesTx(ctx context.Context, runner queryRunner, conversationID string) ([]aichat.Message, error) {
	rows, err := runner.Query(ctx, messageColumns+`
		FROM ai_messages
		WHERE conversation_id = $1
		ORDER BY created_at, id
	`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	messages := []aichat.Message{}
	for rows.Next() {
		message, err := scanMessageRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, message)
	}
	return messages, nil
}

func (store *AIChatStore) findMessageByKeyTx(ctx context.Context, runner queryRunner, key string) (aichat.Message, error) {
	row := runner.QueryRow(ctx, messageColumns+`
		FROM ai_messages
		WHERE idempotency_key = $1
	`, key)
	return scanMessageRow(row)
}

func (store *AIChatStore) readConversationTx(ctx context.Context, runner queryRunner, id string) (aichat.Conversation, error) {
	var conversation aichat.Conversation
	err := runner.QueryRow(ctx, `
		SELECT id, selected_text, tool_name, prompt_template, created_at, updated_at
		FROM ai_conversations
		WHERE id = $1
	`, id).Scan(&conversation.ID, &conversation.SelectedText, &conversation.ToolName,
		&conversation.PromptTemplate, &conversation.CreatedAt, &conversation.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return aichat.Conversation{}, fmt.Errorf("%w: %s", aichat.ErrConversationNotFound, id)
	}
	if err != nil {
		return aichat.Conversation{}, fmt.Errorf("get conversation: %w", err)
	}
	return conversation, nil
}

type queryRunner interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func scanTools(rows pgx.Rows) ([]aichat.SelectionTool, error) {
	tools := []aichat.SelectionTool{}
	for rows.Next() {
		var tool aichat.SelectionTool
		if err := rows.Scan(&tool.ID, &tool.Name, &tool.PromptTemplate, &tool.Enabled,
			&tool.Position, &tool.CreatedAt, &tool.UpdatedAtAt); err != nil {
			return nil, fmt.Errorf("scan selection tool: %w", err)
		}
		tools = append(tools, tool)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read selection tools: %w", err)
	}
	return tools, nil
}

func (store *AIChatStore) scanTool(ctx context.Context, sql string, args ...any) (aichat.SelectionTool, error) {
	var tool aichat.SelectionTool
	err := store.pool.QueryRow(ctx, sql, args...).Scan(
		&tool.ID, &tool.Name, &tool.PromptTemplate, &tool.Enabled,
		&tool.Position, &tool.CreatedAt, &tool.UpdatedAtAt,
	)
	return tool, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// encodeConversationCursor opaquely encodes the last row of a page so the
// client cannot read or forge internal timestamps.
func encodeConversationCursor(when time.Time, id string) string {
	payload, err := json.Marshal(struct {
		U string `json:"u"`
		I string `json:"i"`
	}{when.UTC().Format(time.RFC3339Nano), id})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeConversationCursor(cursor string) (time.Time, string, bool) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(cursor))
	if err != nil {
		return time.Time{}, "", false
	}
	var decoded struct {
		U string `json:"u"`
		I string `json:"i"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil || decoded.U == "" || decoded.I == "" {
		return time.Time{}, "", false
	}
	when, err := time.Parse(time.RFC3339Nano, decoded.U)
	if err != nil {
		return time.Time{}, "", false
	}
	return when, decoded.I, true
}
