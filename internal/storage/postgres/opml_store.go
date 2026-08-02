package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/catwenlabs/pulse/internal/opml"
	"github.com/catwenlabs/pulse/internal/source"
)

type OPMLStore struct {
	pool *pgxpool.Pool
}

func NewOPMLStore(pool *pgxpool.Pool) *OPMLStore {
	return &OPMLStore{pool: pool}
}

func (store *OPMLStore) Import(
	ctx context.Context,
	subscriptions []opml.Subscription,
) (opml.ImportResult, error) {
	type validatedSubscription struct {
		spec    source.ValidatedSpec
		siteURL string
		folders []string
	}
	validated := make([]validatedSubscription, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		name := strings.TrimSpace(subscription.Title)
		if name == "" {
			name = strings.TrimSpace(subscription.FeedURL)
		}
		spec, err := (source.Spec{
			Name:    name,
			Kind:    source.KindRSS,
			Locator: subscription.FeedURL,
		}).Validate()
		if err != nil {
			return opml.ImportResult{}, fmt.Errorf("validate OPML subscription %q: %w", name, err)
		}
		validated = append(validated, validatedSubscription{
			spec:    spec,
			siteURL: strings.TrimSpace(subscription.SiteURL),
			folders: normalizedFolderNames(subscription.Folders),
		})
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return opml.ImportResult{}, fmt.Errorf("begin OPML import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var result opml.ImportResult
	for _, subscription := range validated {
		config, err := json.Marshal(map[string]string{"site_url": subscription.siteURL})
		if err != nil {
			return opml.ImportResult{}, fmt.Errorf("encode OPML source config: %w", err)
		}

		var sourceID source.ID
		err = tx.QueryRow(ctx, `
			INSERT INTO sources (
				name, driver_kind, locator, normalized_locator, config,
				navigation_position
			)
			VALUES (
				$1, $2, $3, $4, $5,
				(SELECT COALESCE(MAX(navigation_position) + 1, 0) FROM sources)
			)
			ON CONFLICT (driver_kind, normalized_locator) DO NOTHING
			RETURNING id
		`,
			subscription.spec.Name,
			subscription.spec.Kind,
			subscription.spec.Locator,
			subscription.spec.NormalizedLocator,
			config,
		).Scan(&sourceID)
		switch {
		case err == nil:
			result.CreatedSources++
		case errors.Is(err, pgx.ErrNoRows):
			result.ExistingSources++
			if err := tx.QueryRow(ctx, `
				SELECT id
				FROM sources
				WHERE driver_kind = $1 AND normalized_locator = $2
			`, subscription.spec.Kind, subscription.spec.NormalizedLocator).Scan(&sourceID); err != nil {
				return opml.ImportResult{}, fmt.Errorf("find existing OPML source: %w", err)
			}
		default:
			return opml.ImportResult{}, fmt.Errorf("create OPML source: %w", err)
		}

		for _, folderName := range subscription.folders {
			folderID, created, err := upsertFolder(ctx, tx, folderName)
			if err != nil {
				return opml.ImportResult{}, err
			}
			if created {
				result.CreatedFolders++
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO source_folders (source_id, folder_id, navigation_position)
				SELECT $1, $2, COALESCE(MAX(navigation_position) + 1, 0)
				FROM source_folders
				WHERE folder_id = $2
				ON CONFLICT DO NOTHING
			`, sourceID, folderID); err != nil {
				return opml.ImportResult{}, fmt.Errorf("link OPML source to folder: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return opml.ImportResult{}, fmt.Errorf("commit OPML import: %w", err)
	}
	return result, nil
}

func (store *OPMLStore) List(ctx context.Context) ([]opml.Subscription, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT
			s.id,
			s.name,
			s.locator,
			COALESCE(s.config->>'site_url', ''),
			COALESCE(f.name, '')
		FROM sources s
		LEFT JOIN source_folders sf ON sf.source_id = s.id
		LEFT JOIN folders f ON f.id = sf.folder_id
		WHERE s.driver_kind = 'rss' AND s.archived_at IS NULL
		ORDER BY s.name, s.id, f.name
	`)
	if err != nil {
		return nil, fmt.Errorf("list OPML subscriptions: %w", err)
	}
	defer rows.Close()

	var result []opml.Subscription
	index := make(map[source.ID]int)
	for rows.Next() {
		var sourceID source.ID
		var title, feedURL, siteURL, folder string
		if err := rows.Scan(&sourceID, &title, &feedURL, &siteURL, &folder); err != nil {
			return nil, fmt.Errorf("scan OPML subscription: %w", err)
		}
		position, ok := index[sourceID]
		if !ok {
			position = len(result)
			index[sourceID] = position
			result = append(result, opml.Subscription{
				Title: title, FeedURL: feedURL, SiteURL: siteURL,
			})
		}
		if folder != "" {
			result[position].Folders = append(result[position].Folders, folder)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list OPML subscriptions: %w", err)
	}
	return result, nil
}

func upsertFolder(
	ctx context.Context,
	tx pgx.Tx,
	name string,
) (id string, created bool, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO folders (name, navigation_position)
		VALUES (
			$1,
			(SELECT COALESCE(MAX(navigation_position) + 1, 0) FROM folders)
		)
		ON CONFLICT (lower(name)) DO NOTHING
		RETURNING id
	`, name).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("create OPML folder %q: %w", name, err)
	}
	if err := tx.QueryRow(ctx,
		"SELECT id FROM folders WHERE lower(name) = lower($1)",
		name,
	).Scan(&id); err != nil {
		return "", false, fmt.Errorf("find OPML folder %q: %w", name, err)
	}
	return id, false, nil
}

func normalizedFolderNames(names []string) []string {
	result := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		key := strings.ToLower(name)
		if name == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, name)
	}
	return result
}
