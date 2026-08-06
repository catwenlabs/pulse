package aichat

import (
	"errors"
	"strings"
	"testing"
)

func msg(role MessageRole, content string, status GenerationStatus) Message {
	return Message{Role: role, Content: content, Status: status}
}

func TestAssembleMemoryAlwaysIncludesSystemPromptAndInitialSelection(t *testing.T) {
	messages := []Message{
		msg(RoleUser, "请解释：E=mc^2", ""),
	}
	built, err := AssembleMemory(messages, len(messages)-1, MemoryOptions{})
	if err != nil {
		t.Fatalf("AssembleMemory() error = %v", err)
	}
	if len(built) != 2 {
		t.Fatalf("built = %d messages, want 2", len(built))
	}
	if built[0].Content != SystemPrompt {
		t.Errorf("first message is not the System Prompt")
	}
	if built[1].Content != "请解释：E=mc^2" {
		t.Errorf("initial selection not preserved verbatim: %q", built[1].Content)
	}
}

func TestAssembleMemoryIncludesOnlyCompleteTurns(t *testing.T) {
	messages := []Message{
		msg(RoleUser, "initial", ""),
		msg(RoleUser, "q1", ""),
		msg(RoleAssistant, "a1", StatusCompleted),
		msg(RoleUser, "q2-failed-prompt", ""),
		msg(RoleAssistant, "partial", StatusFailed),
		msg(RoleUser, "q3", ""),
		msg(RoleAssistant, "a3", StatusCompleted),
		msg(RoleUser, "current", ""),
	}
	built, err := AssembleMemory(messages, len(messages)-1, MemoryOptions{})
	if err != nil {
		t.Fatalf("AssembleMemory() error = %v", err)
	}
	joined := joinContents(built)
	if !strings.Contains(joined, "a1") || !strings.Contains(joined, "a3") {
		t.Errorf("expected completed answers in memory: %q", joined)
	}
	if strings.Contains(joined, "partial") {
		t.Errorf("failed partial content leaked into memory: %q", joined)
	}
	if strings.Contains(joined, "q2-failed-prompt") {
		t.Errorf("user prompt of a failed turn leaked into memory: %q", joined)
	}
	if built[len(built)-1].Content != "current" {
		t.Errorf("current message must be last: %q", built[len(built)-1].Content)
	}
}

func TestAssembleMemoryTrimsToEightCompleteTurns(t *testing.T) {
	messages := []Message{msg(RoleUser, "initial", "")}
	for i := 0; i < 12; i++ {
		messages = append(messages,
			msg(RoleUser, "q", ""),
			msg(RoleAssistant, "a", StatusCompleted),
		)
	}
	messages = append(messages, msg(RoleUser, "current", ""))
	built, err := AssembleMemory(messages, len(messages)-1, MemoryOptions{MaxTurns: 8})
	if err != nil {
		t.Fatalf("AssembleMemory() error = %v", err)
	}
	// system + initial + 8 turns*2 + current = 1 + 1 + 16 + 1 = 19
	if len(built) != 19 {
		t.Fatalf("built = %d messages, want 19", len(built))
	}
}

func TestAssembleMemoryTrimsOldestTurnsWhenTokenBudgetReached(t *testing.T) {
	messages := []Message{msg(RoleUser, "initial", "")}
	big := strings.Repeat("字", 6000) // ~2000 estimated tokens each => ~4000 tokens/turn
	// Three turns of ~4000 tokens each; a 11000-token budget fits only two.
	for i := 0; i < 3; i++ {
		messages = append(messages,
			msg(RoleUser, big, ""),
			msg(RoleAssistant, big, StatusCompleted),
		)
	}
	messages = append(messages, msg(RoleUser, "current", ""))
	built, err := AssembleMemory(messages, len(messages)-1, MemoryOptions{MaxTurns: 8, MaxInputTokens: 11000})
	if err != nil {
		t.Fatalf("AssembleMemory() error = %v", err)
	}
	count := 0
	for _, m := range built {
		if m.Content == big {
			count++
		}
	}
	// Two most recent turns fit (4 big messages); the oldest turn is trimmed.
	if count != 4 {
		t.Errorf("expected exactly two complete turns (4 big messages), got %d", count)
	}
}

func TestAssembleMemoryFailsWhenCoreExceedsProviderInputLimit(t *testing.T) {
	initial := strings.Repeat("字", 12000) // ~4000 tokens
	messages := []Message{
		msg(RoleUser, initial, ""),
		msg(RoleUser, "current", ""),
	}
	_, err := AssembleMemory(messages, len(messages)-1, MemoryOptions{ProviderInputLimit: 1000})
	var budget *MemoryBudgetError
	if !errors.As(err, &budget) {
		t.Fatalf("AssembleMemory() error = %v, want MemoryBudgetError", err)
	}
	if budget.LimitTokens != 1000 {
		t.Errorf("LimitTokens = %d", budget.LimitTokens)
	}
}

func joinContents(messages []ProviderMessage) string {
	var builder strings.Builder
	for _, m := range messages {
		builder.WriteString(m.Content)
		builder.WriteString("|")
	}
	return builder.String()
}
