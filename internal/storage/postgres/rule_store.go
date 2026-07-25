package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wenpengfei/pulse/internal/entry"
	"github.com/wenpengfei/pulse/internal/rule"
)

type RuleStore struct {
	pool *pgxpool.Pool
}

func NewRuleStore(pool *pgxpool.Pool) *RuleStore {
	return &RuleStore{pool: pool}
}

func (store *RuleStore) Create(ctx context.Context, definition rule.Rule) (rule.Rule, error) {
	definition.Name = strings.TrimSpace(definition.Name)
	if err := rule.Validate(definition); err != nil {
		return rule.Rule{}, err
	}
	condition, err := json.Marshal(definition.Condition)
	if err != nil {
		return rule.Rule{}, fmt.Errorf("encode rule condition: %w", err)
	}
	actions, err := json.Marshal(definition.Actions)
	if err != nil {
		return rule.Rule{}, fmt.Errorf("encode rule actions: %w", err)
	}
	definition.Version = 1
	definition.Enabled = true
	err = store.pool.QueryRow(ctx, `
		INSERT INTO rules (name, version, enabled, condition, actions)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, definition.Name, definition.Version, definition.Enabled, condition, actions).Scan(&definition.ID)
	if err != nil {
		return rule.Rule{}, fmt.Errorf("create rule: %w", err)
	}
	return definition, nil
}

func (store *RuleStore) Get(ctx context.Context, id string) (rule.Rule, error) {
	return getRule(ctx, store.pool, id)
}

func (store *RuleStore) List(ctx context.Context) ([]rule.Rule, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, name, version, enabled, condition, actions
		FROM rules
		ORDER BY created_at, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	defer rows.Close()
	var result []rule.Rule
	for rows.Next() {
		item, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	return result, nil
}

func (store *RuleStore) Update(ctx context.Context, definition rule.Rule) (rule.Rule, error) {
	definition.Name = strings.TrimSpace(definition.Name)
	if definition.ID == "" {
		return rule.Rule{}, fmt.Errorf("rule ID is required")
	}
	if err := rule.Validate(definition); err != nil {
		return rule.Rule{}, err
	}
	condition, err := json.Marshal(definition.Condition)
	if err != nil {
		return rule.Rule{}, fmt.Errorf("encode rule condition: %w", err)
	}
	actions, err := json.Marshal(definition.Actions)
	if err != nil {
		return rule.Rule{}, fmt.Errorf("encode rule actions: %w", err)
	}
	row := store.pool.QueryRow(ctx, `
		UPDATE rules
		SET name = $2, enabled = $3, condition = $4, actions = $5,
			version = version + 1, updated_at = now()
		WHERE id = $1
		RETURNING id, name, version, enabled, condition, actions
	`, definition.ID, definition.Name, definition.Enabled, condition, actions)
	updated, err := scanRule(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return rule.Rule{}, fmt.Errorf("rule not found: %s", definition.ID)
	}
	if err != nil {
		return rule.Rule{}, fmt.Errorf("update rule: %w", err)
	}
	return updated, nil
}

func (store *RuleStore) Delete(ctx context.Context, id string) error {
	tag, err := store.pool.Exec(ctx, "DELETE FROM rules WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("rule not found: %s", id)
	}
	return nil
}

func (store *RuleStore) Replay(
	ctx context.Context,
	id string,
	effectsEnabled bool,
) (rule.ReplayResult, error) {
	definition, err := store.Get(ctx, id)
	if err != nil {
		return rule.ReplayResult{}, err
	}
	entries, err := store.allEntries(ctx)
	if err != nil {
		return rule.ReplayResult{}, err
	}
	result := rule.ReplayResult{Evaluated: len(entries)}
	for _, item := range entries {
		evaluation, err := rule.Evaluate(definition, item)
		if err != nil {
			return result, fmt.Errorf("evaluate rule for entry %s: %w", item.ID, err)
		}
		if evaluation.Matched {
			result.Matched++
		}
		count, err := store.apply(ctx, definition, item, evaluation, effectsEnabled)
		if err != nil {
			return result, err
		}
		result.Effects += count
	}
	return result, nil
}

func (store *RuleStore) Preview(ctx context.Context, id string) (rule.PreviewResult, error) {
	definition, err := store.Get(ctx, id)
	if err != nil {
		return rule.PreviewResult{}, err
	}
	entries, err := store.allEntries(ctx)
	if err != nil {
		return rule.PreviewResult{}, err
	}
	result := rule.PreviewResult{Evaluated: len(entries), Items: []rule.PreviewItem{}}
	for _, item := range entries {
		evaluation, err := rule.Evaluate(definition, item)
		if err != nil {
			return result, fmt.Errorf("preview rule for entry %s: %w", item.ID, err)
		}
		if !evaluation.Matched {
			continue
		}
		title := item.DisplayTitle
		if title == "" {
			title = item.SourceTitle
		}
		result.Matched++
		result.Items = append(result.Items, rule.PreviewItem{
			EntryID: item.ID, Title: title, Actions: evaluation.Actions,
		})
	}
	return result, nil
}

func (store *RuleStore) allEntries(ctx context.Context) ([]entry.Entry, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT
			id, source_id, identity_key, external_id, canonical_url,
			source_title, display_title, author, summary, content_html,
			published_at, discovered_at, read_at, starred_at, hidden_at, later_at, note
		FROM entries
		ORDER BY discovered_at, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list entries for rule replay: %w", err)
	}
	defer rows.Close()
	var result []entry.Entry
	for rows.Next() {
		item, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan replay entry: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list entries for rule replay: %w", err)
	}
	return result, nil
}

func (store *RuleStore) apply(
	ctx context.Context,
	definition rule.Rule,
	item entry.Entry,
	evaluation rule.Result,
	effectsEnabled bool,
) (int, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin rule application: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	effectCount, err := applyRuleEvaluationTx(
		ctx, tx, definition, item, evaluation, effectsEnabled,
	)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit rule application: %w", err)
	}
	return effectCount, nil
}

func applyRuleEvaluationTx(
	ctx context.Context,
	tx pgx.Tx,
	definition rule.Rule,
	item entry.Entry,
	evaluation rule.Result,
	effectsEnabled bool,
) (int, error) {
	if _, err := tx.Exec(ctx, `
		DELETE FROM entry_tags
		WHERE entry_id = $1 AND origin = 'rule' AND rule_id = $2
	`, item.ID, definition.ID); err != nil {
		return 0, fmt.Errorf("retract derived tags: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM rule_entry_tags WHERE entry_id = $1 AND rule_id = $2
	`, item.ID, definition.ID); err != nil {
		return 0, fmt.Errorf("clear derived tag records: %w", err)
	}

	effectCount := 0
	if evaluation.Matched {
		for _, action := range evaluation.Actions {
			switch action.Kind {
			case rule.ActionTag:
				if err := applyRuleTag(ctx, tx, definition, item.ID, action.Value); err != nil {
					return 0, err
				}
			case rule.ActionRead, rule.ActionStar, rule.ActionHide, rule.ActionLater:
				if err := applyRuleState(ctx, tx, item.ID, action.Kind); err != nil {
					return 0, err
				}
			case rule.ActionNotification, rule.ActionWebhook:
				if effectsEnabled {
					payload, err := json.Marshal(map[string]string{"value": action.Value})
					if err != nil {
						return 0, fmt.Errorf("encode effect payload: %w", err)
					}
					tag, err := tx.Exec(ctx, `
						INSERT INTO effects (
							effect_key, rule_id, rule_version, entry_id, kind, payload
						)
						VALUES ($1, $2, $3, $4, $5, $6)
						ON CONFLICT (effect_key) DO NOTHING
					`, action.EffectKey, definition.ID, definition.Version, item.ID, action.Kind, payload)
					if err != nil {
						return 0, fmt.Errorf("enqueue rule effect: %w", err)
					}
					effectCount += int(tag.RowsAffected())
				}
			}
		}
	}
	return effectCount, nil
}

func applyEnabledRulesTx(ctx context.Context, tx pgx.Tx, entryID entry.ID) error {
	item, err := scanEntry(tx.QueryRow(ctx, `
		SELECT
			id, source_id, identity_key, external_id, canonical_url,
			source_title, display_title, author, summary, content_html,
			published_at, discovered_at, read_at, starred_at, hidden_at, later_at, note
		FROM entries WHERE id = $1
	`, entryID))
	if err != nil {
		return fmt.Errorf("load entry for rule evaluation: %w", err)
	}
	rows, err := tx.Query(ctx, `
		SELECT id, name, version, enabled, condition, actions
		FROM rules WHERE enabled = true ORDER BY created_at, id
	`)
	if err != nil {
		return fmt.Errorf("list enabled rules: %w", err)
	}
	var definitions []rule.Rule
	for rows.Next() {
		definition, err := scanRule(rows)
		if err != nil {
			rows.Close()
			return fmt.Errorf("scan enabled rule: %w", err)
		}
		definitions = append(definitions, definition)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("list enabled rules: %w", err)
	}
	rows.Close()
	for _, definition := range definitions {
		evaluation, err := rule.Evaluate(definition, item)
		if err != nil {
			return fmt.Errorf("evaluate rule %s: %w", definition.ID, err)
		}
		if _, err := applyRuleEvaluationTx(ctx, tx, definition, item, evaluation, true); err != nil {
			return err
		}
	}
	return nil
}

func applyRuleTag(
	ctx context.Context,
	tx pgx.Tx,
	definition rule.Rule,
	entryID entry.ID,
	name string,
) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("rule tag name is required")
	}
	var tagID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO tags (name, normalized_name)
		VALUES ($1, lower($1))
		ON CONFLICT (normalized_name) DO UPDATE SET name = tags.name
		RETURNING id
	`, name).Scan(&tagID); err != nil {
		return fmt.Errorf("create rule tag: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO entry_tags (entry_id, tag_id, origin, rule_id)
		VALUES ($1, $2, 'rule', $3)
		ON CONFLICT DO NOTHING
	`, entryID, tagID, definition.ID); err != nil {
		return fmt.Errorf("link rule tag: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO rule_entry_tags (rule_id, rule_version, entry_id, tag_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (rule_id, entry_id, tag_id)
		DO UPDATE SET rule_version = EXCLUDED.rule_version
	`, definition.ID, definition.Version, entryID, tagID); err != nil {
		return fmt.Errorf("record rule tag: %w", err)
	}
	return nil
}

func applyRuleState(ctx context.Context, tx pgx.Tx, id entry.ID, kind rule.ActionKind) error {
	column := map[rule.ActionKind]string{
		rule.ActionRead: "read_at", rule.ActionStar: "starred_at",
		rule.ActionHide: "hidden_at", rule.ActionLater: "later_at",
	}[kind]
	if column == "" {
		return fmt.Errorf("unsupported state action %q", kind)
	}
	if _, err := tx.Exec(ctx, "UPDATE entries SET "+column+" = COALESCE("+column+", now()) WHERE id = $1", id); err != nil {
		return fmt.Errorf("apply rule state %s: %w", kind, err)
	}
	return nil
}

type ruleQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func getRule(ctx context.Context, querier ruleQuerier, id string) (rule.Rule, error) {
	item, err := scanRule(querier.QueryRow(ctx, `
		SELECT id, name, version, enabled, condition, actions
		FROM rules WHERE id = $1
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return rule.Rule{}, fmt.Errorf("rule not found: %s", id)
	}
	if err != nil {
		return rule.Rule{}, fmt.Errorf("get rule: %w", err)
	}
	return item, nil
}

func scanRule(row entryRow) (rule.Rule, error) {
	var item rule.Rule
	var condition, actions []byte
	if err := row.Scan(
		&item.ID, &item.Name, &item.Version, &item.Enabled, &condition, &actions,
	); err != nil {
		return rule.Rule{}, err
	}
	if err := json.Unmarshal(condition, &item.Condition); err != nil {
		return rule.Rule{}, fmt.Errorf("decode rule condition: %w", err)
	}
	if err := json.Unmarshal(actions, &item.Actions); err != nil {
		return rule.Rule{}, fmt.Errorf("decode rule actions: %w", err)
	}
	return item, nil
}
