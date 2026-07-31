package rule

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/catwenlabs/pulse/internal/entry"
)

var ErrNoEffect = errors.New("no effect available")

type Field string
type Operator string
type ActionKind string

const (
	FieldSource    Field = "source"
	FieldTitle     Field = "title"
	FieldAuthor    Field = "author"
	FieldBody      Field = "body"
	FieldURL       Field = "url"
	FieldPublished Field = "published_at"

	OperatorEquals   Operator = "equals"
	OperatorContains Operator = "contains"
	OperatorPrefix   Operator = "prefix"
	OperatorBefore   Operator = "before"
	OperatorAfter    Operator = "after"

	ActionTag          ActionKind = "tag"
	ActionStar         ActionKind = "star"
	ActionRead         ActionKind = "read"
	ActionHide         ActionKind = "hide"
	ActionLater        ActionKind = "later"
	ActionNotification ActionKind = "notification"
	ActionWebhook      ActionKind = "webhook"
)

type Condition struct {
	All      []Condition `json:"all,omitempty"`
	Any      []Condition `json:"any,omitempty"`
	Not      *Condition  `json:"not,omitempty"`
	Field    Field       `json:"field,omitempty"`
	Operator Operator    `json:"operator,omitempty"`
	Value    string      `json:"value,omitempty"`
}

type Action struct {
	Kind  ActionKind `json:"kind"`
	Value string     `json:"value,omitempty"`
}

type Rule struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	Enabled   bool      `json:"enabled"`
	Condition Condition `json:"condition"`
	Actions   []Action  `json:"actions"`
}

type EvaluatedAction struct {
	Action
	EffectKey string `json:"effect_key,omitempty"`
}

type Result struct {
	Matched bool              `json:"matched"`
	Actions []EvaluatedAction `json:"actions"`
}

type ReplayResult struct {
	Evaluated int `json:"evaluated"`
	Matched   int `json:"matched"`
	Effects   int `json:"effects"`
}

type PreviewItem struct {
	EntryID entry.ID          `json:"entry_id"`
	Title   string            `json:"title"`
	Actions []EvaluatedAction `json:"actions"`
}

type PreviewResult struct {
	Evaluated int           `json:"evaluated"`
	Matched   int           `json:"matched"`
	Items     []PreviewItem `json:"items"`
}

type EffectStatus string

const (
	EffectPending   EffectStatus = "pending"
	EffectRunning   EffectStatus = "running"
	EffectRetry     EffectStatus = "retry"
	EffectSucceeded EffectStatus = "succeeded"
	EffectDead      EffectStatus = "dead"
)

type Effect struct {
	ID          string       `json:"id"`
	EffectKey   string       `json:"effect_key"`
	RuleID      string       `json:"rule_id"`
	RuleVersion int          `json:"rule_version"`
	EntryID     entry.ID     `json:"entry_id"`
	Kind        ActionKind   `json:"kind"`
	Value       string       `json:"value"`
	Status      EffectStatus `json:"status"`
	Attempts    int          `json:"attempts"`
}

func Validate(definition Rule) error {
	if strings.TrimSpace(definition.Name) == "" {
		return fmt.Errorf("rule name is required")
	}
	if err := validateCondition(definition.Condition); err != nil {
		return err
	}
	if len(definition.Actions) == 0 {
		return fmt.Errorf("at least one rule action is required")
	}
	for _, action := range definition.Actions {
		if !validAction(action.Kind) {
			return fmt.Errorf("unsupported rule action %q", action.Kind)
		}
		if (action.Kind == ActionTag || action.Kind == ActionNotification || action.Kind == ActionWebhook) &&
			strings.TrimSpace(action.Value) == "" {
			return fmt.Errorf("rule action %q requires a value", action.Kind)
		}
		if action.Kind == ActionWebhook {
			if parsed, err := url.ParseRequestURI(action.Value); err != nil ||
				(parsed.Scheme != "http" && parsed.Scheme != "https") ||
				parsed.Host == "" || parsed.User != nil {
				return fmt.Errorf("rule webhook requires an HTTP(S) URL without embedded credentials")
			}
		}
	}
	return nil
}

func validateCondition(condition Condition) error {
	branches := 0
	if len(condition.All) > 0 {
		branches++
	}
	if len(condition.Any) > 0 {
		branches++
	}
	if condition.Not != nil {
		branches++
	}
	if condition.Field != "" || condition.Operator != "" || condition.Value != "" {
		branches++
	}
	if branches != 1 {
		return fmt.Errorf("condition must contain exactly one of all, any, not, or field expression")
	}
	children := condition.All
	if len(condition.Any) > 0 {
		children = condition.Any
	}
	for _, child := range children {
		if err := validateCondition(child); err != nil {
			return err
		}
	}
	if condition.Not != nil {
		return validateCondition(*condition.Not)
	}
	if len(children) > 0 {
		return nil
	}
	switch condition.Field {
	case FieldSource, FieldTitle, FieldAuthor, FieldBody, FieldURL, FieldPublished:
	default:
		return fmt.Errorf("unsupported rule field %q", condition.Field)
	}
	switch condition.Operator {
	case OperatorEquals, OperatorContains, OperatorPrefix:
	case OperatorBefore, OperatorAfter:
		if condition.Field != FieldPublished {
			return fmt.Errorf("time operators require published_at")
		}
		if _, err := time.Parse(time.RFC3339, condition.Value); err != nil {
			return fmt.Errorf("invalid rule time value: %w", err)
		}
	default:
		return fmt.Errorf("unsupported rule operator %q", condition.Operator)
	}
	return nil
}

func (condition Condition) Match(item entry.Entry) (bool, error) {
	branches := 0
	if len(condition.All) > 0 {
		branches++
	}
	if len(condition.Any) > 0 {
		branches++
	}
	if condition.Not != nil {
		branches++
	}
	if condition.Field != "" || condition.Operator != "" || condition.Value != "" {
		branches++
	}
	if branches != 1 {
		return false, fmt.Errorf("condition must contain exactly one of all, any, not, or field expression")
	}
	if len(condition.All) > 0 {
		for _, child := range condition.All {
			matched, err := child.Match(item)
			if err != nil || !matched {
				return matched, err
			}
		}
		return true, nil
	}
	if len(condition.Any) > 0 {
		for _, child := range condition.Any {
			matched, err := child.Match(item)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	}
	if condition.Not != nil {
		matched, err := condition.Not.Match(item)
		return !matched, err
	}
	return condition.matchLeaf(item)
}

func (condition Condition) matchLeaf(item entry.Entry) (bool, error) {
	var actual string
	switch condition.Field {
	case FieldSource:
		actual = string(item.SourceID)
	case FieldTitle:
		actual = item.DisplayTitle
		if actual == "" {
			actual = item.SourceTitle
		}
	case FieldAuthor:
		actual = item.Author
	case FieldBody:
		actual = item.Summary + " " + item.ContentHTML
	case FieldURL:
		actual = item.CanonicalURL
	case FieldPublished:
		if item.PublishedAt != nil {
			actual = item.PublishedAt.UTC().Format(time.RFC3339)
		}
	default:
		return false, fmt.Errorf("unsupported rule field %q", condition.Field)
	}
	switch condition.Operator {
	case OperatorEquals:
		return strings.EqualFold(strings.TrimSpace(actual), strings.TrimSpace(condition.Value)), nil
	case OperatorContains:
		return strings.Contains(strings.ToLower(actual), strings.ToLower(condition.Value)), nil
	case OperatorPrefix:
		return strings.HasPrefix(strings.ToLower(actual), strings.ToLower(condition.Value)), nil
	case OperatorBefore, OperatorAfter:
		actualTime, err := time.Parse(time.RFC3339, actual)
		if err != nil {
			return false, nil
		}
		expected, err := time.Parse(time.RFC3339, condition.Value)
		if err != nil {
			return false, fmt.Errorf("invalid rule time value: %w", err)
		}
		if condition.Operator == OperatorBefore {
			return actualTime.Before(expected), nil
		}
		return actualTime.After(expected), nil
	default:
		return false, fmt.Errorf("unsupported rule operator %q", condition.Operator)
	}
}

func Evaluate(rule Rule, item entry.Entry) (Result, error) {
	if !rule.Enabled {
		return Result{}, nil
	}
	if rule.ID == "" || rule.Version <= 0 {
		return Result{}, fmt.Errorf("rule ID and positive version are required")
	}
	matched, err := rule.Condition.Match(item)
	if err != nil || !matched {
		return Result{Matched: matched}, err
	}
	result := Result{Matched: true, Actions: make([]EvaluatedAction, 0, len(rule.Actions))}
	for index, action := range rule.Actions {
		if !validAction(action.Kind) {
			return Result{}, fmt.Errorf("unsupported rule action %q", action.Kind)
		}
		evaluated := EvaluatedAction{Action: action}
		if action.Kind == ActionNotification || action.Kind == ActionWebhook {
			evaluated.EffectKey = effectKey(rule, item, index, action)
		}
		result.Actions = append(result.Actions, evaluated)
	}
	return result, nil
}

func effectKey(rule Rule, item entry.Entry, index int, action Action) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		rule.ID,
		strconv.Itoa(rule.Version),
		string(item.ID),
		strconv.Itoa(index),
		string(action.Kind),
		action.Value,
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func validAction(kind ActionKind) bool {
	switch kind {
	case ActionTag, ActionStar, ActionRead, ActionHide, ActionLater, ActionNotification, ActionWebhook:
		return true
	default:
		return false
	}
}
