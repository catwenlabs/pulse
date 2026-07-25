package migrate

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoadPlansOrdersUpMigrations(t *testing.T) {
	files := fstest.MapFS{
		"000002_entries.up.sql":   {Data: []byte("CREATE TABLE entries ();")},
		"000001_sources.up.sql":   {Data: []byte("CREATE TABLE sources ();")},
		"000001_sources.down.sql": {Data: []byte("DROP TABLE sources;")},
		"README.md":               {Data: []byte("ignored")},
	}

	plans, err := LoadPlans(files)
	if err != nil {
		t.Fatalf("LoadPlans() error = %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("plan count = %d, want 2", len(plans))
	}
	if plans[0].Version != 1 || plans[1].Version != 2 {
		t.Errorf("versions = [%d, %d], want [1, 2]", plans[0].Version, plans[1].Version)
	}
}

func TestRunIsIdempotent(t *testing.T) {
	databaseURL := os.Getenv("PULSE_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("PULSE_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := Run(context.Background(), pool); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := Run(context.Background(), pool); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count < 4 {
		t.Errorf("migration count = %d, want at least 4", count)
	}
}

func TestLoadPlansRejectsDuplicateVersion(t *testing.T) {
	files := fstest.MapFS{
		"000001_first.up.sql":  {Data: []byte("SELECT 1;")},
		"000001_second.up.sql": {Data: []byte("SELECT 2;")},
	}

	if _, err := LoadPlans(files); err == nil {
		t.Fatal("LoadPlans() error = nil, want duplicate version error")
	}
}

func TestEmbeddedMigrationsAreValid(t *testing.T) {
	sub, err := fs.Sub(embeddedFiles, "sql")
	if err != nil {
		t.Fatalf("fs.Sub() error = %v", err)
	}
	plans, err := LoadPlans(sub)
	if err != nil {
		t.Fatalf("LoadPlans() error = %v", err)
	}
	if len(plans) == 0 {
		t.Fatal("embedded migrations are empty")
	}
}
