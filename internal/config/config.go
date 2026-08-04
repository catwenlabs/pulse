package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
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
	HTTPAddr           string
	DatabaseURL        string
	WebDir             string
	ImportRoots        []string
	MasterKey          string
	Roles              []Role
	EmbeddingProvider  string
	EmbeddingBaseURL   string
	EmbeddingModel     string
	AIProvider         string
	AIBaseURL          string
	AIAPIKey           string
	AIHeaders          map[string]string
	AIModel            string
	AITimeout          time.Duration
	AIMaxDigestStories int
	AIMaxActiveJobs    int
}

type LookupEnv func(string) (string, bool)

func Load(lookup LookupEnv) (Config, error) {
	cfg := Config{
		HTTPAddr:           "127.0.0.1:8080",
		DatabaseURL:        "postgres://pulse:pulse@postgres:5432/pulse?sslmode=disable",
		WebDir:             "/web",
		ImportRoots:        []string{"/data/imports"},
		Roles:              append([]Role(nil), allRoles...),
		EmbeddingProvider:  "disabled",
		EmbeddingBaseURL:   "http://127.0.0.1:11434",
		EmbeddingModel:     "qwen3-embedding",
		AIProvider:         "disabled",
		AIBaseURL:          "http://127.0.0.1:11434/v1",
		AIModel:            "qwen3:8b",
		AITimeout:          2 * time.Minute,
		AIMaxDigestStories: 100,
		AIMaxActiveJobs:    4,
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
	if value, ok := lookup("PULSE_EMBEDDING_PROVIDER"); ok {
		cfg.EmbeddingProvider = strings.ToLower(strings.TrimSpace(value))
	}
	if value, ok := lookup("PULSE_EMBEDDING_BASE_URL"); ok && strings.TrimSpace(value) != "" {
		cfg.EmbeddingBaseURL = strings.TrimRight(strings.TrimSpace(value), "/")
	}
	if value, ok := lookup("PULSE_EMBEDDING_MODEL"); ok {
		cfg.EmbeddingModel = strings.TrimSpace(value)
	}
	if value, ok := lookup("PULSE_AI_PROVIDER"); ok {
		cfg.AIProvider = strings.ToLower(strings.TrimSpace(value))
	}
	if value, ok := lookup("PULSE_AI_BASE_URL"); ok && strings.TrimSpace(value) != "" {
		cfg.AIBaseURL = strings.TrimRight(strings.TrimSpace(value), "/")
	}
	if value, ok := lookup("PULSE_AI_API_KEY"); ok {
		cfg.AIAPIKey = strings.TrimSpace(value)
	}
	if value, ok := lookup("PULSE_AI_MODEL"); ok {
		cfg.AIModel = strings.TrimSpace(value)
	}
	if value, ok := lookup("PULSE_AI_HEADERS_JSON"); ok && strings.TrimSpace(value) != "" {
		headers, err := parseAIHeaders(value)
		if err != nil {
			return Config{}, err
		}
		cfg.AIHeaders = headers
	}
	if value, ok := lookup("PULSE_AI_TIMEOUT"); ok && strings.TrimSpace(value) != "" {
		timeout, err := time.ParseDuration(strings.TrimSpace(value))
		if err != nil || timeout <= 0 {
			return Config{}, fmt.Errorf("invalid PULSE_AI_TIMEOUT: positive duration required")
		}
		cfg.AITimeout = timeout
	}
	if value, ok := lookup("PULSE_AI_MAX_DIGEST_STORIES"); ok && strings.TrimSpace(value) != "" {
		limit, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || limit <= 0 {
			return Config{}, fmt.Errorf("invalid PULSE_AI_MAX_DIGEST_STORIES: positive integer required")
		}
		cfg.AIMaxDigestStories = limit
	}
	if value, ok := lookup("PULSE_AI_MAX_ACTIVE_JOBS"); ok && strings.TrimSpace(value) != "" {
		limit, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || limit <= 0 {
			return Config{}, fmt.Errorf("invalid PULSE_AI_MAX_ACTIVE_JOBS: positive integer required")
		}
		cfg.AIMaxActiveJobs = limit
	}
	if cfg.EmbeddingProvider != "disabled" && cfg.EmbeddingProvider != "ollama" {
		return Config{}, fmt.Errorf(
			"invalid PULSE_EMBEDDING_PROVIDER: unknown provider %q",
			cfg.EmbeddingProvider,
		)
	}
	embeddingURL, err := url.Parse(cfg.EmbeddingBaseURL)
	if err != nil || (embeddingURL.Scheme != "http" && embeddingURL.Scheme != "https") ||
		embeddingURL.Host == "" {
		return Config{}, fmt.Errorf("invalid PULSE_EMBEDDING_BASE_URL: absolute HTTP(S) URL required")
	}
	if cfg.EmbeddingProvider == "ollama" && cfg.EmbeddingModel == "" {
		return Config{}, fmt.Errorf("invalid PULSE_EMBEDDING_MODEL: model is required")
	}
	if cfg.AIProvider != "disabled" && cfg.AIProvider != "ollama" && cfg.AIProvider != "openai-compatible" {
		return Config{}, fmt.Errorf("invalid PULSE_AI_PROVIDER: unknown provider %q", cfg.AIProvider)
	}
	if cfg.AIProvider != "disabled" {
		aiURL, err := url.Parse(cfg.AIBaseURL)
		if err != nil || (aiURL.Scheme != "http" && aiURL.Scheme != "https") || aiURL.Host == "" {
			return Config{}, fmt.Errorf("invalid PULSE_AI_BASE_URL: absolute HTTP(S) URL required")
		}
		if cfg.AIModel == "" {
			return Config{}, fmt.Errorf("invalid PULSE_AI_MODEL: model is required")
		}
	}

	return cfg, nil
}

func parseAIHeaders(value string) (map[string]string, error) {
	var headers map[string]string
	if err := json.Unmarshal([]byte(value), &headers); err != nil {
		return nil, fmt.Errorf("invalid PULSE_AI_HEADERS_JSON: JSON object required")
	}
	if headers == nil {
		return nil, fmt.Errorf("invalid PULSE_AI_HEADERS_JSON: JSON object required")
	}
	for name, headerValue := range headers {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(headerValue) == "" {
			return nil, fmt.Errorf("invalid PULSE_AI_HEADERS_JSON: header names and values are required")
		}
	}
	return headers, nil
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
