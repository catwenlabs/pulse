package config

import (
	"strings"
	"testing"
)

func TestLoadUsesDefaults(t *testing.T) {
	cfg, err := Load(func(string) (string, bool) {
		return "", false
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Errorf("HTTPAddr = %q, want %q", cfg.HTTPAddr, "127.0.0.1:8080")
	}
	if cfg.DatabaseURL != "postgres://pulse:pulse@postgres:5432/pulse?sslmode=disable" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.WebDir != "/web" {
		t.Errorf("WebDir = %q, want /web", cfg.WebDir)
	}
	if len(cfg.ImportRoots) != 1 || cfg.ImportRoots[0] != "/data/imports" {
		t.Errorf("ImportRoots = %v", cfg.ImportRoots)
	}
	if len(cfg.Roles) != 4 {
		t.Fatalf("Roles count = %d, want 4", len(cfg.Roles))
	}
}

func TestLoadParsesRoles(t *testing.T) {
	env := map[string]string{
		"PULSE_HTTP_ADDR":    "127.0.0.1:9090",
		"PULSE_DATABASE_URL": "postgres://custom",
		"PULSE_ROLES":        "web, worker ,scheduler",
		"PULSE_WEB_DIR":      "/custom/web",
		"PULSE_IMPORT_ROOTS": "/imports,/more-imports",
		"PULSE_MASTER_KEY":   "external-key",
	}

	cfg, err := Load(func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != "127.0.0.1:9090" {
		t.Errorf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "postgres://custom" {
		t.Errorf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.WebDir != "/custom/web" {
		t.Errorf("WebDir = %q", cfg.WebDir)
	}
	if len(cfg.ImportRoots) != 2 || cfg.ImportRoots[1] != "/more-imports" {
		t.Errorf("ImportRoots = %v", cfg.ImportRoots)
	}
	if cfg.MasterKey != "external-key" {
		t.Errorf("MasterKey was not loaded")
	}
	want := []Role{RoleWeb, RoleWorker, RoleScheduler}
	for i := range want {
		if cfg.Roles[i] != want[i] {
			t.Errorf("Roles[%d] = %q, want %q", i, cfg.Roles[i], want[i])
		}
	}
}

func TestLoadRejectsUnknownRole(t *testing.T) {
	_, err := Load(func(key string) (string, bool) {
		if key == "PULSE_ROLES" {
			return "web,unknown", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("Load() error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error = %q, want role name", err)
	}
}

func TestLoadRejectsEmptyRolesAndDeduplicates(t *testing.T) {
	if _, err := Load(func(key string) (string, bool) {
		return "", key == "PULSE_ROLES"
	}); err == nil {
		t.Fatal("empty roles error = nil")
	}

	cfg, err := Load(func(key string) (string, bool) {
		if key == "PULSE_ROLES" {
			return "web,web", true
		}
		return "", false
	})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Roles) != 1 || cfg.Roles[0] != RoleWeb {
		t.Errorf("Roles = %v", cfg.Roles)
	}
}
