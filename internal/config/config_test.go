package config

import (
	"strings"
	"testing"
	"time"
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
	if cfg.EmbeddingProvider != "disabled" {
		t.Errorf("EmbeddingProvider = %q, want disabled", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingBaseURL != "http://127.0.0.1:11434" {
		t.Errorf("EmbeddingBaseURL = %q", cfg.EmbeddingBaseURL)
	}
	if cfg.EmbeddingModel != "qwen3-embedding" {
		t.Errorf("EmbeddingModel = %q", cfg.EmbeddingModel)
	}
	if cfg.AIProvider != "disabled" || cfg.AIBaseURL != "http://127.0.0.1:11434/v1" ||
		cfg.AIModel != "qwen3:8b" || cfg.AITimeout != 2*time.Minute || cfg.AIMaxDigestStories != 100 ||
		cfg.AIMaxActiveJobs != 4 {
		t.Errorf("AI defaults = %#v", cfg)
	}
}

func TestLoadParsesRoles(t *testing.T) {
	env := map[string]string{
		"PULSE_HTTP_ADDR":             "127.0.0.1:9090",
		"PULSE_DATABASE_URL":          "postgres://custom",
		"PULSE_ROLES":                 "web, worker ,scheduler",
		"PULSE_WEB_DIR":               "/custom/web",
		"PULSE_IMPORT_ROOTS":          "/imports,/more-imports",
		"PULSE_MASTER_KEY":            "external-key",
		"PULSE_EMBEDDING_PROVIDER":    "ollama",
		"PULSE_EMBEDDING_BASE_URL":    "http://ollama:11434/",
		"PULSE_EMBEDDING_MODEL":       "qwen3-embedding:0.6b",
		"PULSE_AI_PROVIDER":           "openai-compatible",
		"PULSE_AI_BASE_URL":           "https://openrouter.ai/api/v1/",
		"PULSE_AI_API_KEY":            "secret",
		"PULSE_AI_MODEL":              "deepseek/deepseek-chat",
		"PULSE_AI_HEADERS_JSON":       `{"HTTP-Referer":"https://pulse.example"}`,
		"PULSE_AI_TIMEOUT":            "45s",
		"PULSE_AI_MAX_DIGEST_STORIES": "25",
		"PULSE_AI_MAX_ACTIVE_JOBS":    "2",
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
	if cfg.EmbeddingProvider != "ollama" ||
		cfg.EmbeddingBaseURL != "http://ollama:11434" ||
		cfg.EmbeddingModel != "qwen3-embedding:0.6b" {
		t.Errorf("embedding config = %#v", cfg)
	}
	if cfg.AIProvider != "openai-compatible" || cfg.AIBaseURL != "https://openrouter.ai/api/v1" ||
		cfg.AIAPIKey != "secret" || cfg.AIModel != "deepseek/deepseek-chat" ||
		cfg.AITimeout != 45*time.Second || cfg.AIMaxDigestStories != 25 ||
		cfg.AIMaxActiveJobs != 2 ||
		cfg.AIHeaders["HTTP-Referer"] != "https://pulse.example" {
		t.Errorf("AI config = %#v", cfg)
	}
	want := []Role{RoleWeb, RoleWorker, RoleScheduler}
	for i := range want {
		if cfg.Roles[i] != want[i] {
			t.Errorf("Roles[%d] = %q, want %q", i, cfg.Roles[i], want[i])
		}
	}
}

func TestLoadRejectsInvalidEmbeddingConfiguration(t *testing.T) {
	tests := []map[string]string{
		{"PULSE_EMBEDDING_PROVIDER": "unknown"},
		{
			"PULSE_EMBEDDING_PROVIDER": "ollama",
			"PULSE_EMBEDDING_BASE_URL": "file:///tmp/model",
		},
		{
			"PULSE_EMBEDDING_PROVIDER": "ollama",
			"PULSE_EMBEDDING_MODEL":    " ",
		},
	}
	for _, env := range tests {
		if _, err := Load(func(key string) (string, bool) {
			value, ok := env[key]
			return value, ok
		}); err == nil {
			t.Errorf("Load(%v) error = nil", env)
		}
	}
}

func TestLoadRejectsInvalidAIConfiguration(t *testing.T) {
	tests := []map[string]string{
		{"PULSE_AI_PROVIDER": "unknown"},
		{"PULSE_AI_PROVIDER": "ollama", "PULSE_AI_BASE_URL": "file:///tmp/model"},
		{"PULSE_AI_PROVIDER": "ollama", "PULSE_AI_MODEL": " "},
		{"PULSE_AI_HEADERS_JSON": "[]"},
		{"PULSE_AI_TIMEOUT": "0s"},
		{"PULSE_AI_MAX_DIGEST_STORIES": "0"},
		{"PULSE_AI_MAX_ACTIVE_JOBS": "0"},
	}
	for _, env := range tests {
		if _, err := Load(func(key string) (string, bool) {
			value, ok := env[key]
			return value, ok
		}); err == nil {
			t.Errorf("Load(%v) error = nil", env)
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
