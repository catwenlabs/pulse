package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wenpengfei/pulse/internal/organization"
	"github.com/wenpengfei/pulse/internal/source"
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
		INSERT INTO folders (name)
		VALUES ($1)
		RETURNING id, name, 0
	`, name).Scan(&folder.ID, &folder.Name, &folder.SourceCount)
	if err != nil {
		return organization.Folder{}, fmt.Errorf("create folder: %w", err)
	}
	return folder, nil
}

func (store *OrganizationStore) ListFolders(ctx context.Context) ([]organization.Folder, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT folder.id, folder.name, count(source_folders.source_id)::integer
		FROM folders AS folder
		LEFT JOIN source_folders ON source_folders.folder_id = folder.id
		GROUP BY folder.id, folder.name
		ORDER BY lower(folder.name), folder.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	defer rows.Close()
	var result []organization.Folder
	for rows.Next() {
		var folder organization.Folder
		if err := rows.Scan(&folder.ID, &folder.Name, &folder.SourceCount); err != nil {
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
		INSERT INTO source_folders (folder_id, source_id)
		VALUES ($1, $2)
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
