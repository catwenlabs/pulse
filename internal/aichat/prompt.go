package aichat

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// placeholderPattern matches mustache-style tokens. The inner identifier is
// trimmed, so "{{ selection }}" is equivalent to "{{selection}}". Any token
// whose identifier is not "selection" is treated as an unknown placeholder.
var placeholderPattern = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// NormalizeToolName trims surrounding whitespace. Names are compared
// case-insensitively after normalization for uniqueness.
func NormalizeToolName(name string) string {
	return strings.TrimSpace(name)
}

// ValidateToolInput enforces the documented tool field constraints: name length
// after trimming (1–40), prompt template length (1–4000), presence of
// {{selection}}, and absence of unknown placeholders.
func ValidateToolInput(input ToolInput) error {
	name := NormalizeToolName(input.Name)
	if name == "" {
		return &ValidationError{Field: "name", Message: "tool name is required"}
	}
	if utf8.RuneCountInString(name) > MaxToolNameLength {
		return &ValidationError{
			Field:   "name",
			Message: fmt.Sprintf("tool name must be at most %d characters", MaxToolNameLength),
		}
	}
	template := input.PromptTemplate
	if strings.TrimSpace(template) == "" {
		return &ValidationError{Field: "prompt_template", Message: "prompt template is required"}
	}
	if utf8.RuneCountInString(template) > MaxPromptTemplateLength {
		return &ValidationError{
			Field:   "prompt_template",
			Message: fmt.Sprintf("prompt template must be at most %d characters", MaxPromptTemplateLength),
		}
	}
	if !placeholderPattern.MatchString(template) {
		return &ValidationError{
			Field:   "prompt_template",
			Message: fmt.Sprintf("prompt template must reference the selection via %s", SelectionPlaceholder),
		}
	}
	if unknown := firstUnknownPlaceholder(template); unknown != "" {
		return &ValidationError{
			Field:   "prompt_template",
			Message: fmt.Sprintf("prompt template uses unknown placeholder {{%s}}; only %s is supported", unknown, SelectionPlaceholder),
		}
	}
	return nil
}

// firstUnknownPlaceholder returns the trimmed identifier of the first token in
// the template that is not "selection", or "" when every token is valid.
func firstUnknownPlaceholder(template string) string {
	matches := placeholderPattern.FindAllStringSubmatch(template, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		identifier := strings.TrimSpace(match[1])
		if identifier != "selection" {
			return identifier
		}
	}
	return ""
}

// SelectionUsageCount reports how many times {{selection}} is referenced. The
// spec permits repeated uses but warns about token cost; callers use this only
// for diagnostics, not rejection.
func SelectionUsageCount(template string) int {
	return countSelectionPlaceholders(template)
}

func countSelectionPlaceholders(template string) int {
	count := 0
	for _, match := range placeholderPattern.FindAllStringSubmatch(template, -1) {
		if len(match) >= 2 && strings.TrimSpace(match[1]) == "selection" {
			count++
		}
	}
	return count
}

// ExpandTemplate substitutes the selection into every {{selection}} token. It
// rejects templates with unknown placeholders so a broken tool can never
// silently omit the selected material.
func ExpandTemplate(template, selection string) (string, error) {
	if unknown := firstUnknownPlaceholder(template); unknown != "" {
		return "", &ValidationError{
			Field:   "prompt_template",
			Message: fmt.Sprintf("prompt template uses unknown placeholder {{%s}}; only %s is supported", unknown, SelectionPlaceholder),
		}
	}
	expanded := placeholderPattern.ReplaceAllStringFunc(template, func(token string) string {
		match := placeholderPattern.FindStringSubmatch(token)
		if len(match) >= 2 && strings.TrimSpace(match[1]) == "selection" {
			return selection
		}
		return token
	})
	return expanded, nil
}

// NormalizeSelection validates and normalizes the client-supplied selection. It
// preserves internal whitespace and line breaks, trims empty boundaries, and
// rejects whitespace-only or oversized selections. Oversized selections are
// rejected outright rather than silently truncated.
func NormalizeSelection(selection string) (string, error) {
	trimmed := strings.TrimSpace(selection)
	if trimmed == "" {
		return "", ErrSelectionRequired
	}
	count := utf8.RuneCountInString(trimmed)
	if count > MaxSelectionCharacters {
		return "", &SelectionSizeError{Count: count, Limit: MaxSelectionCharacters}
	}
	return trimmed, nil
}

// normalizeWhitespace collapses runs of whitespace into single spaces for
// display excerpts and labels.
func normalizeWhitespace(value string) string {
	var builder strings.Builder
	inSpace := false
	for _, r := range value {
		if isSpaceRune(r) {
			if !inSpace && builder.Len() > 0 {
				builder.WriteRune(' ')
			}
			inSpace = true
			continue
		}
		inSpace = false
		builder.WriteRune(r)
	}
	return strings.TrimSpace(builder.String())
}

func isSpaceRune(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\v', '\f', '\r', '', ' ':
		return true
	}
	return false
}
