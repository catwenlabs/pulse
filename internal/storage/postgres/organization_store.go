package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/catwenlabs/pulse/internal/organization"
	"github.com/catwenlabs/pulse/internal/source"
)

type OrganizationStore struct {
	pool *pgxpool.Pool
}

func NewOrganizationStore(pool *pgxpool.Pool) *OrganizationStore {
	return &OrganizationStore{pool: pool}
}

func (store *OrganizationStore) CreateFolder(ctx context.Context, name string) (organization.Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return organization.Folder{}, fmt.Errorf("folder name is required")
	}
	var folder organization.Folder
	err := store.pool.QueryRow(ctx, `
		INSERT INTO folders (name, navigation_position)
		VALUES (
			$1,
			(SELECT COALESCE(MAX(navigation_position) + 1, 0) FROM folders)
		)
		RETURNING id, name, 0, ARRAY[]::text[]
	`, name).Scan(&folder.ID, &folder.Name, &folder.SourceCount, &folder.SourceIDs)
	if err != nil {
		return organization.Folder{}, fmt.Errorf("create folder: %w", err)
	}
	return folder, nil
}

func (store *OrganizationStore) ListFolders(ctx context.Context) ([]organization.Folder, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT
			folder.id,
			folder.name,
			count(source.id)::integer,
			COALESCE(
				array_agg(source.id::text ORDER BY source_folders.navigation_position, lower(source.name), source.id)
					FILTER (WHERE source.id IS NOT NULL),
				ARRAY[]::text[]
			)
		FROM folders AS folder
		LEFT JOIN source_folders ON source_folders.folder_id = folder.id
		LEFT JOIN sources AS source
			ON source.id = source_folders.source_id
			AND source.archived_at IS NULL
		GROUP BY folder.id, folder.name
		ORDER BY folder.navigation_position, lower(folder.name), folder.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	defer rows.Close()
	var result []organization.Folder
	for rows.Next() {
		var folder organization.Folder
		if err := rows.Scan(&folder.ID, &folder.Name, &folder.SourceCount, &folder.SourceIDs); err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}
		result = append(result, folder)
	}
	return result, rows.Err()
}

func (store *OrganizationStore) DeleteFolder(ctx context.Context, id string) error {
	if _, err := store.pool.Exec(ctx, "DELETE FROM folders WHERE id = $1", id); err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}
	return nil
}

func (store *OrganizationStore) AddSourceToFolder(ctx context.Context, folderID string, sourceID source.ID) error {
	_, err := store.pool.Exec(ctx, `
		INSERT INTO source_folders (folder_id, source_id, navigation_position)
		SELECT $1, $2, COALESCE(MAX(navigation_position) + 1, 0)
		FROM source_folders
		WHERE folder_id = $1
		ON CONFLICT DO NOTHING
	`, folderID, sourceID)
	if err != nil {
		return fmt.Errorf("add source to folder: %w", err)
	}
	return nil
}

func (store *OrganizationStore) RemoveSourceFromFolder(ctx context.Context, folderID string, sourceID source.ID) error {
	_, err := store.pool.Exec(ctx, `
		DELETE FROM source_folders WHERE folder_id = $1 AND source_id = $2
	`, folderID, sourceID)
	if err != nil {
		return fmt.Errorf("remove source from folder: %w", err)
	}
	return nil
}

func (store *OrganizationStore) ReorderRootSources(ctx context.Context, ids []source.ID) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin root Source reorder: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	allIDs, err := queryNavigationIDs(ctx, tx, `
		SELECT id::text
		FROM sources
		WHERE archived_at IS NULL
		ORDER BY navigation_position, lower(name), id
		FOR UPDATE
	`)
	if err != nil {
		return fmt.Errorf("list root Source order: %w", err)
	}
	rootIDs, err := queryNavigationIDs(ctx, tx, `
		SELECT source.id::text
		FROM sources AS source
		WHERE source.archived_at IS NULL
		  AND NOT EXISTS (
			SELECT 1
			FROM source_folders AS membership
			WHERE membership.source_id = source.id
		  )
		ORDER BY source.navigation_position, lower(source.name), source.id
	`)
	if err != nil {
		return fmt.Errorf("list unfiled Source order: %w", err)
	}
	requested := make([]string, len(ids))
	for index, id := range ids {
		requested[index] = string(id)
	}
	if err := validateNavigationOrder("source_ids", requested, rootIDs); err != nil {
		return err
	}

	rootSet := make(map[string]struct{}, len(rootIDs))
	for _, id := range rootIDs {
		rootSet[id] = struct{}{}
	}
	orderedRootIDs := make([]string, len(allIDs))
	rootIndex := 0
	for index, id := range allIDs {
		if _, ok := rootSet[id]; ok {
			orderedRootIDs[index] = requested[rootIndex]
			rootIndex++
			continue
		}
		orderedRootIDs[index] = id
	}
	for position, id := range orderedRootIDs {
		if _, err := tx.Exec(ctx, `
			UPDATE sources
			SET navigation_position = $2, updated_at = now()
			WHERE id = $1 AND archived_at IS NULL
		`, id, position); err != nil {
			return fmt.Errorf("persist root Source order: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit root Source reorder: %w", err)
	}
	return nil
}

func (store *OrganizationStore) ReorderFolders(ctx context.Context, ids []string) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Folder reorder: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := queryNavigationIDs(ctx, tx, `
		SELECT id::text
		FROM folders
		ORDER BY navigation_position, lower(name), id
		FOR UPDATE
	`)
	if err != nil {
		return fmt.Errorf("list Folder order: %w", err)
	}
	if err := validateNavigationOrder("folder_ids", ids, current); err != nil {
		return err
	}
	for position, id := range ids {
		if _, err := tx.Exec(ctx, `
			UPDATE folders
			SET navigation_position = $2
			WHERE id = $1
		`, id, position); err != nil {
			return fmt.Errorf("persist Folder order: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Folder reorder: %w", err)
	}
	return nil
}

func (store *OrganizationStore) ReorderFolderSources(ctx context.Context, folderID string, ids []source.ID) error {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin Folder Source reorder: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := queryNavigationIDs(ctx, tx, `
		SELECT source.id::text
		FROM source_folders AS membership
		JOIN sources AS source ON source.id = membership.source_id
		WHERE membership.folder_id = $1
		  AND source.archived_at IS NULL
		ORDER BY membership.navigation_position, lower(source.name), source.id
	`, folderID)
	if err != nil {
		return fmt.Errorf("list Folder Source order: %w", err)
	}
	requested := make([]string, len(ids))
	for index, id := range ids {
		requested[index] = string(id)
	}
	if err := validateNavigationOrder("source_ids", requested, current); err != nil {
		return err
	}
	for position, id := range ids {
		if _, err := tx.Exec(ctx, `
			UPDATE source_folders
			SET navigation_position = $3
			WHERE folder_id = $1 AND source_id = $2
		`, folderID, id, position); err != nil {
			return fmt.Errorf("persist Folder Source order: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit Folder Source reorder: %w", err)
	}
	return nil
}

type organizationQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func queryNavigationIDs(ctx context.Context, query organizationQuerier, statement string, args ...any) ([]string, error) {
	rows, err := query.Query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func validateNavigationOrder(field string, requested, current []string) error {
	if len(requested) != len(current) {
		return &organization.OrderValidationError{
			Field:   field,
			Message: "order must contain every item in this navigation scope exactly once",
		}
	}
	currentSet := make(map[string]struct{}, len(current))
	for _, id := range current {
		currentSet[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		if _, ok := currentSet[id]; !ok {
			return &organization.OrderValidationError{
				Field:   field,
				Message: "order contains an item outside this navigation scope",
			}
		}
		if _, ok := seen[id]; ok {
			return &organization.OrderValidationError{
				Field:   field,
				Message: "order contains a duplicate item",
			}
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (store *OrganizationStore) CreateView(ctx context.Context, input organization.View) (organization.View, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return organization.View{}, fmt.Errorf("view name is required")
	}
	query, err := json.Marshal(input.Query)
	if err != nil {
		return organization.View{}, fmt.Errorf("encode view query: %w", err)
	}
	var result organization.View
	var raw json.RawMessage
	err = store.pool.QueryRow(ctx, `
		INSERT INTO views (name, query)
		VALUES ($1, $2)
		RETURNING id, name, query
	`, input.Name, query).Scan(&result.ID, &result.Name, &raw)
	if err != nil {
		return organization.View{}, fmt.Errorf("create view: %w", err)
	}
	if err := json.Unmarshal(raw, &result.Query); err != nil {
		return organization.View{}, fmt.Errorf("decode created view: %w", err)
	}
	return result, nil
}

func (store *OrganizationStore) UpdateView(ctx context.Context, input organization.View) (organization.View, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return organization.View{}, fmt.Errorf("view name is required")
	}
	query, err := json.Marshal(input.Query)
	if err != nil {
		return organization.View{}, fmt.Errorf("encode view query: %w", err)
	}
	var result organization.View
	var raw json.RawMessage
	err = store.pool.QueryRow(ctx, `
		UPDATE views
		SET name = $2, query = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, name, query
	`, input.ID, input.Name, query).Scan(&result.ID, &result.Name, &raw)
	if err != nil {
		return organization.View{}, fmt.Errorf("update view: %w", err)
	}
	if err := json.Unmarshal(raw, &result.Query); err != nil {
		return organization.View{}, fmt.Errorf("decode updated view: %w", err)
	}
	return result, nil
}

func (store *OrganizationStore) ListViews(ctx context.Context) ([]organization.View, error) {
	rows, err := store.pool.Query(ctx, "SELECT id, name, query FROM views ORDER BY lower(name), id")
	if err != nil {
		return nil, fmt.Errorf("list views: %w", err)
	}
	defer rows.Close()
	var result []organization.View
	for rows.Next() {
		var view organization.View
		var raw json.RawMessage
		if err := rows.Scan(&view.ID, &view.Name, &raw); err != nil {
			return nil, fmt.Errorf("scan view: %w", err)
		}
		if err := json.Unmarshal(raw, &view.Query); err != nil {
			return nil, fmt.Errorf("decode view: %w", err)
		}
		result = append(result, view)
	}
	return result, rows.Err()
}

func (store *OrganizationStore) DeleteView(ctx context.Context, id string) error {
	if _, err := store.pool.Exec(ctx, "DELETE FROM views WHERE id = $1", id); err != nil {
		return fmt.Errorf("delete view: %w", err)
	}
	return nil
}
