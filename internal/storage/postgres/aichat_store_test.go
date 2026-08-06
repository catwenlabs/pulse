package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/catwenlabs/pulse/internal/aichat"
)

func newAIChatStore(t *testing.T) *AIChatStore {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, "TRUNCATE ai_messages, ai_conversations, ai_selection_tools CASCADE"); err != nil {
		t.Fatalf("truncate aichat tables: %v", err)
	}
	return NewAIChatStore(pool)
}

func TestAIChatStoreSeedsStarterToolsOnce(t *testing.T) {
	// The migration seeds three editable starter tools. Verify they are present
	// with {{selection}} templates. They are ordinary rows: disabling or
	// deleting one is no different from a user-created tool.
	pool := testPool(t)
	ctx := context.Background()
	// Reset to a freshly seeded state by re-applying only the seed rows.
	if _, err := pool.Exec(ctx, "TRUNCATE ai_selection_tools CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_selection_tools (name, prompt_template, enabled, position) VALUES
			('AI 解读', '解读：{{selection}}', true, 0),
			('AI 翻译', '翻译：{{selection}}', true, 1),
			('举例说明', '举例：{{selection}}', true, 2)
	`); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	store := NewAIChatStore(pool)
	tools, err := store.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 3 {
		t.Fatalf("expected 3 seeded tools, got %d", len(tools))
	}
}

func TestAIChatStoreRejectsCaseInsensitiveDuplicateToolName(t *testing.T) {
	store := newAIChatStore(t)
	ctx := context.Background()
	if _, err := store.CreateTool(ctx, aichat.ToolInput{Name: "AI 解读", PromptTemplate: "{{selection}}", Enabled: true}); err != nil {
		t.Fatalf("create first: %v", err)
	}
	_, err := store.CreateTool(ctx, aichat.ToolInput{Name: "  ai 解读  ", PromptTemplate: "{{selection}}", Enabled: true})
	var duplicate *aichat.DuplicateToolError
	if !errors.As(err, &duplicate) {
		t.Fatalf("expected DuplicateToolError, got %v", err)
	}
}

func TestAIChatStoreReorderWritesDeterministicOrder(t *testing.T) {
	store := newAIChatStore(t)
	ctx := context.Background()
	a, _ := store.CreateTool(ctx, aichat.ToolInput{Name: "A", PromptTemplate: "{{selection}}", Enabled: true})
	b, _ := store.CreateTool(ctx, aichat.ToolInput{Name: "B", PromptTemplate: "{{selection}}", Enabled: true})
	c, _ := store.CreateTool(ctx, aichat.ToolInput{Name: "C", PromptTemplate: "{{selection}}", Enabled: true})

	ordered, err := store.ReorderTools(ctx, []string{c.ID, a.ID, b.ID})
	if err != nil {
		t.Fatalf("reorder: %v", err)
	}
	if len(ordered) != 3 || ordered[0].ID != c.ID || ordered[1].ID != a.ID || ordered[2].ID != b.ID {
		t.Fatalf("order = %+v", ordered)
	}
}

func TestAIChatStoreConversationCursorOrdering(t *testing.T) {
	store := newAIChatStore(t)
	ctx := context.Background()
	first, _ := createConversation(t, store, "first", "k1")
	createConversation(t, store, "second", "k2")
	createConversation(t, store, "third", "k3")

	// Touch the first conversation so it becomes the most recently active.
	if _, err := store.AppendUserMessage(ctx, aichat.AppendUserMessageParams{
		ConversationID: first.ID, Content: "follow up",
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	page, err := store.ListConversations(ctx, 2, "")
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if !page.HasMore || len(page.Items) != 2 {
		t.Fatalf("first page = %+v", page)
	}
	if page.Items[0].ID != first.ID {
		t.Errorf("most recent first: got %s, want %s", page.Items[0].ID, first.ID)
	}
	next, err := store.ListConversations(ctx, 2, page.NextCursor)
	if err != nil {
		t.Fatalf("next page: %v", err)
	}
	if next.HasMore || len(next.Items) != 1 {
		t.Fatalf("next page = %+v", next)
	}
	bad, err := store.ListConversations(ctx, 2, "not-a-cursor")
	if !errors.Is(err, aichat.ErrInvalidCursor) {
		t.Fatalf("invalid cursor = %v, want ErrInvalidCursor", err)
	}
	_ = bad
}

func TestAIChatStoreConversationDeletionCascadesMessages(t *testing.T) {
	store := newAIChatStore(t)
	ctx := context.Background()
	conv, _ := createConversation(t, store, "root", "k")
	if _, err := store.AppendUserMessage(ctx, aichat.AppendUserMessageParams{ConversationID: conv.ID, Content: "q"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := store.DeleteConversation(ctx, conv.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	messages, err := store.ListMessages(ctx, conv.ID)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected cascaded deletion, got %d messages", len(messages))
	}
}

func TestAIChatStoreSingleActiveAssistantGeneration(t *testing.T) {
	store := newAIChatStore(t)
	ctx := context.Background()
	conv, _ := createConversation(t, store, "text", "k")
	first, created, err := store.StartGeneration(ctx, conv.ID, "g1")
	if err != nil || !created {
		t.Fatalf("first start: created=%v err=%v", created, err)
	}
	if _, _, err := store.StartGeneration(ctx, conv.ID, "g2"); !errors.Is(err, aichat.ErrActiveGeneration) {
		t.Fatalf("second start = %v, want ErrActiveGeneration", err)
	}
	if err := store.CompleteGeneration(ctx, first.ID, aichat.GenerationResult{Status: aichat.StatusCompleted, Content: "done"}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if _, created, err := store.StartGeneration(ctx, conv.ID, "g3"); err != nil || !created {
		t.Fatalf("start after completion: created=%v err=%v", created, err)
	}
}

func TestAIChatStoreIdempotencyUniqueness(t *testing.T) {
	store := newAIChatStore(t)
	ctx := context.Background()
	conv1, msg1, err := store.CreateConversation(ctx, aichat.CreateConversationParams{
		IdempotencyKey: "same", SelectedText: "s", ToolName: "t", PromptTemplate: "{{selection}}", InitialPrompt: "p",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	conv2, msg2, err := store.CreateConversation(ctx, aichat.CreateConversationParams{
		IdempotencyKey: "same", SelectedText: "s", ToolName: "t", PromptTemplate: "{{selection}}", InitialPrompt: "p",
	})
	if err != nil || conv1.ID != conv2.ID || msg1.ID != msg2.ID {
		t.Fatalf("idempotent create diverged: %+v/%+v err=%v", conv1, conv2, err)
	}
	appended1, err := store.AppendUserMessage(ctx, aichat.AppendUserMessageParams{
		ConversationID: conv1.ID, Content: "q", IdempotencyKey: "um",
	})
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	appended2, err := store.AppendUserMessage(ctx, aichat.AppendUserMessageParams{
		ConversationID: conv1.ID, Content: "q", IdempotencyKey: "um",
	})
	if err != nil || appended1.ID != appended2.ID {
		t.Fatalf("idempotent append diverged: %+v/%+v err=%v", appended1, appended2, err)
	}
}

func TestAIChatStoreDurablePartialAndFinalContent(t *testing.T) {
	store := newAIChatStore(t)
	ctx := context.Background()
	conv, _ := createConversation(t, store, "text", "k")
	msg, _, err := store.StartGeneration(ctx, conv.ID, "g")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := store.CheckpointGeneration(ctx, msg.ID, "partial"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	messages, _ := store.ListMessages(ctx, conv.ID)
	if messages[len(messages)-1].Content != "partial" {
		t.Errorf("checkpoint not durable: %+v", messages[len(messages)-1])
	}
	if err := store.CompleteGeneration(ctx, msg.ID, aichat.GenerationResult{
		Status: aichat.StatusFailed, Content: "partial", Provider: "p", Model: "m", Error: "boom",
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	messages, _ = store.ListMessages(ctx, conv.ID)
	last := messages[len(messages)-1]
	if last.Status != aichat.StatusFailed || last.Content != "partial" || last.Provider != "p" || last.Error != "boom" {
		t.Errorf("final persisted = %+v", last)
	}
}

func createConversation(t *testing.T, store *AIChatStore, selection, key string) (aichat.Conversation, aichat.Message) {
	t.Helper()
	conv, msg, err := store.CreateConversation(context.Background(), aichat.CreateConversationParams{
		IdempotencyKey: key, SelectedText: selection, ToolName: "AI 解读",
		PromptTemplate: "解读：{{selection}}", InitialPrompt: "解读：" + selection,
	})
	if err != nil {
		t.Fatalf("createConversation: %v", err)
	}
	return conv, msg
}
