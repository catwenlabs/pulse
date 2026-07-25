package migrate

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

//go:embed sql/*.up.sql
var embeddedFiles embed.FS

type Plan struct {
	Version int64
	Name    string
	SQL     string
}

func LoadPlans(files fs.FS) ([]Plan, error) {
	var plans []Plan
	versions := make(map[int64]string)

	err := fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
			return nil
		}

		version, err := parseVersion(entry.Name())
		if err != nil {
			return err
		}
		if previous, ok := versions[version]; ok {
			return fmt.Errorf("duplicate migration version %d in %q and %q", version, previous, path)
		}

		body, err := fs.ReadFile(files, path)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", path, err)
		}
		versions[version] = path
		plans = append(plans, Plan{Version: version, Name: filepath.Base(path), SQL: string(body)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load migrations: %w", err)
	}

	sort.Slice(plans, func(i, j int) bool {
		return plans[i].Version < plans[j].Version
	})
	return plans, nil
}

func EmbeddedPlans() ([]Plan, error) {
	files, err := fs.Sub(embeddedFiles, "sql")
	if err != nil {
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	return LoadPlans(files)
}

func parseVersion(name string) (int64, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("migration %q must start with a numeric version and underscore", name)
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse migration version from %q: %w", name, err)
	}
	return version, nil
}
