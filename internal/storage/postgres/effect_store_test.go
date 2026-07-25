package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/ingestion"
	"github.com/wenpengfei/pulse/internal/rule"
)

func TestEffectStoreIsIdempotentAndCompletesNotification(t *testing.T) {
	pool := testPool(t)
	sourceStore := NewSourceStore(pool)
	acquisitionStore := NewAcquisitionStore(pool)
	entryStore := NewEntryStore(pool)
	ctx := context.Background()
	src := createTestSource(t, sourceStore, "effect-source")
	acquisition := claimTestAcquisition(t, acquisitionStore, src.ID, "effect-entry")
	if err := entryStore.CommitBatch(ctx, acquisition, "worker", []ingestion.Candidate{{
		ExternalID: "entry", Title: "Urgent",
	}}, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("CommitBatch() error = %v", err)
	}
	entries, err := entryStore.List(ctx, 1)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var ruleID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO rules (name, condition, actions)
		VALUES ('Notify', '{}', '[]')
		RETURNING id
	`).Scan(&ruleID); err != nil {
		t.Fatalf("insert rule: %v", err)
	}
	definition := rule.Rule{ID: ruleID, Version: 1}
	action := rule.EvaluatedAction{
		Action:    rule.Action{Kind: rule.ActionNotification, Value: "Matched"},
		EffectKey: "stable-effect-key",
	}
	store := NewEffectStore(pool)
	first, err := store.Enqueue(ctx, definition, entries[0].ID, action)
	if err != nil {
		t.Fatalf("first Enqueue() error = %v", err)
	}
	second, err := store.Enqueue(ctx, definition, entries[0].ID, action)
	if err != nil {
		t.Fatalf("second Enqueue() error = %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("effect IDs = %q / %q", first.ID, second.ID)
	}
	claimed, err := store.Claim(ctx, "effect-worker", time.Minute)
	if err != nil || claimed.ID != first.ID {
		t.Fatalf("Claim() = %+v, %v", claimed, err)
	}
	if err := store.Succeed(ctx, claimed, "effect-worker"); err != nil {
		t.Fatalf("Succeed() error = %v", err)
	}
	var notifications int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM notifications WHERE entry_id = $1",
		entry.ID(entries[0].ID),
	).Scan(&notifications); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if notifications != 1 {
		t.Errorf("notification count = %d", notifications)
	}
}
