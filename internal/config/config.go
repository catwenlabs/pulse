package config

import (
	"fmt"
	"strings"
)

type Role string

const (
	RoleWeb       Role = "web"
	RoleScheduler Role = "scheduler"
	RoleWorker    Role = "worker"
	RoleEffect    Role = "effect-worker"
)

var allRoles = []Role{RoleWeb, RoleScheduler, RoleWorker, RoleEffect}

type Config struct {
	HTTPAddr    string
	DatabaseURL string
	WebDir      string
	ImportRoots []string
	MasterKey   string
	Roles       []Role
}

type LookupEnv func(string) (string, bool)

func Load(lookup LookupEnv) (Config, error) {
	cfg := Config{
		HTTPAddr:    "127.0.0.1:8080",
		DatabaseURL: "postgres://pulse:pulse@postgres:5432/pulse?sslmode=disable",
		WebDir:      "/web",
		ImportRoots: []string{"/data/imports"},
		Roles:       append([]Role(nil), allRoles...),
	}

	if value, ok := lookup("PULSE_HTTP_ADDR"); ok && strings.TrimSpace(value) != "" {
		cfg.HTTPAddr = strings.TrimSpace(value)
	}
	if value, ok := lookup("PULSE_DATABASE_URL"); ok && strings.TrimSpace(value) != "" {
		cfg.DatabaseURL = strings.TrimSpace(value)
	}
	if value, ok := lookup("PULSE_WEB_DIR"); ok && strings.TrimSpace(value) != "" {
		cfg.WebDir = strings.TrimSpace(value)
	}
	if value, ok := lookup("PULSE_IMPORT_ROOTS"); ok && strings.TrimSpace(value) != "" {
		cfg.ImportRoots = splitNonEmpty(value)
	}
	if value, ok := lookup("PULSE_MASTER_KEY"); ok {
		cfg.MasterKey = strings.TrimSpace(value)
	}
	if value, ok := lookup("PULSE_ROLES"); ok {
		roles, err := parseRoles(value)
		if err != nil {
			return Config{}, err
		}
		cfg.Roles = roles
	}

	return cfg, nil
}

func splitNonEmpty(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseRoles(value string) ([]Role, error) {
	parts := strings.Split(value, ",")
	roles := make([]Role, 0, len(parts))
	seen := make(map[Role]struct{}, len(parts))

	for _, part := range parts {
		role := Role(strings.TrimSpace(part))
		if !isRole(role) {
			return nil, fmt.Errorf("invalid PULSE_ROLES: unknown role %q", role)
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("invalid PULSE_ROLES: at least one role is required")
	}
	return roles, nil
}

func isRole(role Role) bool {
	for _, allowed := range allRoles {
		if role == allowed {
			return true
		}
	}
	return false
}
