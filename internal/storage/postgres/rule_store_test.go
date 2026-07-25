package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/rule"
)

func TestRuleStoreReplayRetractsDerivedTagsAndKeepsEffectsIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	src := createTestSource(t, sourceStore, "rule-replay-source")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "rule-replay-entry")
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{{
		ExternalID: "entry", Title: "Urgent report",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	entries, err := entryStore.List(ctx, 1)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	store := NewRuleStore(pool)
	definition, err := store.Create(ctx, rule.Rule{
		Name: "Urgent",
		Condition: rule.Condition{
			Field: rule.FieldTitle, Operator: rule.OperatorContains, Value: "urgent",
		},
		Actions: []rule.Action{
			{Kind: rule.ActionTag, Value: "priority"},
			{Kind: rule.ActionNotification, Value: "Urgent item"},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	result, err := store.Replay(ctx, definition.ID, false)
	if err != nil || result.Matched != 1 || result.Effects != 0 {
		t.Fatalf("Replay(false) = %+v, %v", result, err)
	}
	var derived int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM entry_tags
		WHERE entry_id = $1 AND origin = 'rule'
	`, entries[0].ID).Scan(&derived); err != nil {
		t.Fatalf("count derived tags: %v", err)
	}
	if derived != 1 {
		t.Fatalf("derived tags = %d", derived)
	}

	if _, err := pool.Exec(ctx, "UPDATE entries SET source_title = 'Ordinary report' WHERE id = $1", entries[0].ID); err != nil {
		t.Fatalf("update entry: %v", err)
	}
	if _, err := store.Replay(ctx, definition.ID, false); err != nil {
		t.Fatalf("non-match Replay() error = %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM entry_tags
		WHERE entry_id = $1 AND origin = 'rule'
	`, entries[0].ID).Scan(&derived); err != nil {
		t.Fatalf("count retracted tags: %v", err)
	}
	if derived != 0 {
		t.Errorf("derived tags after non-match = %d", derived)
	}

	if _, err := pool.Exec(ctx, "UPDATE entries SET source_title = 'Urgent again' WHERE id = $1", entries[0].ID); err != nil {
		t.Fatalf("restore entry: %v", err)
	}
	first, err := store.Replay(ctx, definition.ID, true)
	if err != nil || first.Effects != 1 {
		t.Fatalf("Replay(true) = %+v, %v", first, err)
	}
	if _, err := store.Replay(ctx, definition.ID, true); err != nil {
		t.Fatalf("repeated Replay() error = %v", err)
	}
	var effects int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM effects WHERE rule_id = $1", definition.ID).Scan(&effects); err != nil {
		t.Fatalf("count effects: %v", err)
	}
	if effects != 1 {
		t.Errorf("effect count = %d", effects)
	}
}

func TestEntryCommitReevaluatesRulesInSameTransaction(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	ruleStore := NewRuleStore(pool)
	src := createTestSource(t, sourceStore, "rule-commit-source")
	definition, err := ruleStore.Create(ctx, rule.Rule{
		Name: "Go entries",
		Condition: rule.Condition{
			Field: rule.FieldTitle, Operator: rule.OperatorContains, Value: "Go",
		},
		Actions: []rule.Action{
			{Kind: rule.ActionTag, Value: "golang"},
			{Kind: rule.ActionNotification, Value: "New Go entry"},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "rule-commit-entry")
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{{
		ExternalID: "entry", Title: "Go release",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	var tags, effects int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM entry_tags WHERE origin = 'rule' AND rule_id = $1
	`, definition.ID).Scan(&tags); err != nil {
		t.Fatalf("count rule tags: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM effects WHERE rule_id = $1
	`, definition.ID).Scan(&effects); err != nil {
		t.Fatalf("count effects: %v", err)
	}
	if tags != 1 || effects != 1 {
		t.Fatalf("atomic rule results tags=%d effects=%d", tags, effects)
	}

	update := claimTestAcquisition(t, acquisitionStore, src.ID, "rule-commit-update")
	if err := entryStore.CommitBatch(ctx, update, "worker", []ingestion.Candidate{{
		ExternalID: "entry", Title: "Rust release",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("update CommitBatch() error = %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM entry_tags WHERE origin = 'rule' AND rule_id = $1
	`, definition.ID).Scan(&tags); err != nil {
		t.Fatalf("count retracted tags: %v", err)
	}
	if tags != 0 {
		t.Errorf("rule tags after source update = %d", tags)
	}
}
