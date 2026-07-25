package rule

import (
	"testing"

	"github.com/wenpengfei/pulse/internal/entry"
)

func TestConditionSupportsBooleanASTAndEntryFields(t *testing.T) {
	item := entry.Entry{
		SourceID: "source-1", SourceTitle: "Go concurrency patterns",
		Author: "Ada", Summary: "Structured concurrency", CanonicalURL: "https://example.com/go",
	}
	condition := Condition{
		All: []Condition{
			{Field: FieldSource, Operator: OperatorEquals, Value: "source-1"},
			{Any: []Condition{
				{Field: FieldTitle, Operator: OperatorContains, Value: "concurrency"},
				{Field: FieldAuthor, Operator: OperatorEquals, Value: "Lin"},
			}},
			{Not: &Condition{Field: FieldURL, Operator: OperatorContains, Value: "/blocked"}},
		},
	}
	matched, err := condition.Match(item)
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if !matched {
		t.Fatal("Match() = false")
	}
}

func TestConditionRejectsInvalidAST(t *testing.T) {
	if _, err := (Condition{Field: "unknown", Operator: OperatorContains, Value: "x"}).Match(entry.Entry{}); err == nil {
		t.Fatal("Match() error = nil")
	}
	if _, err := (Condition{All: []Condition{{}}, Field: FieldTitle}).Match(entry.Entry{}); err == nil {
		t.Fatal("ambiguous condition error = nil")
	}
}

func TestEvaluateReevaluatesUpdatedEntryAndCreatesStableEffects(t *testing.T) {
	rule := Rule{
		ID: "rule-1", Version: 3, Enabled: true,
		Condition: Condition{Field: FieldTitle, Operator: OperatorContains, Value: "urgent"},
		Actions: []Action{
			{Kind: ActionTag, Value: "priority"},
			{Kind: ActionWebhook, Value: "https://hooks.example/notify"},
		},
	}
	first, err := Evaluate(rule, entry.Entry{ID: "entry-1", SourceTitle: "Urgent update"})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !first.Matched || len(first.Actions) != 2 || first.Actions[1].EffectKey == "" {
		t.Fatalf("result = %+v", first)
	}
	second, err := Evaluate(rule, entry.Entry{ID: "entry-1", SourceTitle: "Ordinary update"})
	if err != nil {
		t.Fatalf("updated Evaluate() error = %v", err)
	}
	if second.Matched || len(second.Actions) != 0 {
		t.Fatalf("updated result = %+v", second)
	}
	repeated, _ := Evaluate(rule, entry.Entry{ID: "entry-1", SourceTitle: "Urgent update"})
	if repeated.Actions[1].EffectKey != first.Actions[1].EffectKey {
		t.Errorf("effect keys differ: %q / %q", first.Actions[1].EffectKey, repeated.Actions[1].EffectKey)
	}
}

func TestValidateRejectsInvalidStructuredRule(t *testing.T) {
	tests := []Rule{
		{Name: "no condition", Actions: []Action{{Kind: ActionTag, Value: "x"}}},
		{
			Name:      "invalid action",
			Condition: Condition{Field: FieldTitle, Operator: OperatorContains, Value: "Go"},
			Actions:   []Action{{Kind: ActionKind("run-code"), Value: "unsafe"}},
		},
		{
			Name:      "invalid time",
			Condition: Condition{Field: FieldPublished, Operator: OperatorAfter, Value: "tomorrow"},
			Actions:   []Action{{Kind: ActionStar}},
		},
	}
	for _, definition := range tests {
		t.Run(definition.Name, func(t *testing.T) {
			if err := Validate(definition); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}
