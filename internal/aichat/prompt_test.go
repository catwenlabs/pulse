package aichat

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestValidateToolInputAcceptsValidTool(t *testing.T) {
	err := ValidateToolInput(ToolInput{
		Name:           "AI 解读",
		PromptTemplate: "请解释以下内容：{{selection}}",
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("ValidateToolInput() unexpected error = %v", err)
	}
}

func TestValidateToolInputTrimsNameAndToleratesWhitespaceInPlaceholder(t *testing.T) {
	err := ValidateToolInput(ToolInput{
		Name:           "  翻译  ",
		PromptTemplate: "Translate: {{ selection }}",
	})
	if err != nil {
		t.Fatalf("ValidateToolInput() unexpected error = %v", err)
	}
	if got := NormalizeToolName("  翻译  "); got != "翻译" {
		t.Errorf("NormalizeToolName() = %q", got)
	}
}

func TestValidateToolInputRejectsEmptyAndOversizedNames(t *testing.T) {
	cases := []struct {
		name   string
		input  ToolInput
		field  string
	}{
		{
			name:  "empty name",
			input: ToolInput{Name: "   ", PromptTemplate: "{{selection}}"},
			field: "name",
		},
		{
			name:  "oversized name",
			input: ToolInput{Name: strings.Repeat("字", MaxToolNameLength+1), PromptTemplate: "{{selection}}"},
			field: "name",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateToolInput(tc.input)
			var validation *ValidationError
			if !errors.As(err, &validation) || validation.Field != tc.field {
				t.Fatalf("ValidateToolInput() error = %v, want %s ValidationError", err, tc.field)
			}
		})
	}
}

func TestValidateToolInputRequiresSelectionPlaceholder(t *testing.T) {
	err := ValidateToolInput(ToolInput{
		Name:           "举例说明",
		PromptTemplate: "请举一个例子",
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "prompt_template" {
		t.Fatalf("ValidateToolInput() error = %v, want prompt_template ValidationError", err)
	}
}

func TestValidateToolInputRejectsUnknownPlaceholders(t *testing.T) {
	err := ValidateToolInput(ToolInput{
		Name:           "bad",
		PromptTemplate: "Explain {{selection}} using {{context}}",
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "prompt_template" {
		t.Fatalf("ValidateToolInput() error = %v, want prompt_template ValidationError", err)
	}
	if !strings.Contains(validation.Message, "context") {
		t.Errorf("expected error to name the unknown placeholder, got %q", validation.Message)
	}
}

func TestValidateToolInputRejectsOversizedTemplate(t *testing.T) {
	err := ValidateToolInput(ToolInput{
		Name:           "long",
		PromptTemplate: "{{selection}} " + strings.Repeat("x", MaxPromptTemplateLength),
	})
	var validation *ValidationError
	if !errors.As(err, &validation) || validation.Field != "prompt_template" {
		t.Fatalf("ValidateToolInput() error = %v, want prompt_template ValidationError", err)
	}
}

func TestExpandTemplateSubstitutesEverySelection(t *testing.T) {
	expanded, err := ExpandTemplate("First {{selection}} then {{ selection }}", "TEXT")
	if err != nil {
		t.Fatalf("ExpandTemplate() error = %v", err)
	}
	if expanded != "First TEXT then TEXT" {
		t.Errorf("ExpandTemplate() = %q", expanded)
	}
	if count := SelectionUsageCount("{{selection}} {{selection}}"); count != 2 {
		t.Errorf("SelectionUsageCount() = %d, want 2", count)
	}
}

func TestExpandTemplateRejectsUnknownPlaceholder(t *testing.T) {
	if _, err := ExpandTemplate("{{selection}} {{foo}}", "TEXT"); err == nil {
		t.Fatal("ExpandTemplate() expected error for unknown placeholder")
	}
}

func TestNormalizeSelectionPreservesInternalWhitespaceAndRejectsBadInput(t *testing.T) {
	normalized, err := NormalizeSelection("  公式：\n  E=mc^2  ")
	if err != nil {
		t.Fatalf("NormalizeSelection() error = %v", err)
	}
	if normalized != "公式：\n  E=mc^2" {
		t.Errorf("NormalizeSelection() = %q", normalized)
	}

	if _, err := NormalizeSelection("   \n\t  "); err != ErrSelectionRequired {
		t.Errorf("whitespace-only selection error = %v, want ErrSelectionRequired", err)
	}

	oversized := strings.Repeat("字", MaxSelectionCharacters+1)
	_, err = NormalizeSelection(oversized)
	var sizeErr *SelectionSizeError
	if !errors.As(err, &sizeErr) {
		t.Fatalf("oversized selection error = %v, want SelectionSizeError", err)
	}
	if sizeErr.Count != utf8.RuneCountInString(oversized) {
		t.Errorf("SelectionSizeError.Count = %d", sizeErr.Count)
	}
}

func TestConversationExcerptCollapsesAndBounds(t *testing.T) {
	conv := Conversation{SelectedText: "  line one\nline two  "}
	if got := conv.Excerpt(80); got != "line one line two" {
		t.Errorf("Excerpt() = %q", got)
	}

	long := Conversation{SelectedText: strings.Repeat("a", 120)}
	excerpt := long.Excerpt(40)
	if !strings.HasSuffix(excerpt, "…") || len([]rune(excerpt)) != 41 {
		t.Errorf("Excerpt() = %q (len %d)", excerpt, len([]rune(excerpt)))
	}
}
