GOCACHE ?= /tmp/pulse-go-cache
DEV_COMPOSE := docker compose -f compose.yaml -f compose.dev.yaml
ENV_FILE ?= .env
ENV_EXAMPLE_FILE ?= .env.example
PULSE_HTTP_ADDR ?=
PULSE_DATABASE_URL ?=
PULSE_IMPORT_ROOTS ?=
PULSE_EMBEDDING_PROVIDER ?=
PULSE_EMBEDDING_BASE_URL ?=
PULSE_EMBEDDING_MODEL ?=

LOAD_ENV = if [ -f "$(ENV_FILE)" ]; then set -a; . "$(ENV_FILE)"; set +a; elif [ -f "$(ENV_EXAMPLE_FILE)" ]; then set -a; . "$(ENV_EXAMPLE_FILE)"; set +a; else echo "missing $(ENV_FILE) and $(ENV_EXAMPLE_FILE)" >&2; exit 1; fi

.PHONY: test test-race vet run dev dev-db-up dev-db-down dev-db-logs dev-api dev-web-install dev-web compose-config backup backup-verify export-config export-entry e2e

test:
	GOCACHE=$(GOCACHE) go test -cover ./...

test-race:
	GOCACHE=$(GOCACHE) go test -race ./...

vet:
	GOCACHE=$(GOCACHE) go vet ./...

run:
	@set -eu; \
	$(LOAD_ENV); \
	GOCACHE="$(GOCACHE)" go run ./cmd/pulse

dev:
	sh scripts/dev.sh

dev-db-up:
	$(DEV_COMPOSE) up -d postgres

dev-db-down:
	$(DEV_COMPOSE) down

dev-db-logs:
	$(DEV_COMPOSE) logs -f --tail=200 postgres

dev-api:
	@set -eu; \
	$(LOAD_ENV); \
	PULSE_HTTP_ADDR="$(if $(PULSE_HTTP_ADDR),$(PULSE_HTTP_ADDR),127.0.0.1:8080)" \
	PULSE_DATABASE_URL="$(if $(PULSE_DATABASE_URL),$(PULSE_DATABASE_URL),postgres://pulse:pulse@127.0.0.1:54321/pulse?sslmode=disable)" \
	PULSE_IMPORT_ROOTS="$(if $(PULSE_IMPORT_ROOTS),$(PULSE_IMPORT_ROOTS),./imports)" \
	PULSE_EMBEDDING_PROVIDER="$(if $(PULSE_EMBEDDING_PROVIDER),$(PULSE_EMBEDDING_PROVIDER),ollama)" \
	PULSE_EMBEDDING_BASE_URL="$(if $(PULSE_EMBEDDING_BASE_URL),$(PULSE_EMBEDDING_BASE_URL),http://127.0.0.1:11434)" \
	PULSE_EMBEDDING_MODEL="$(if $(PULSE_EMBEDDING_MODEL),$(PULSE_EMBEDDING_MODEL),qwen3-embedding)" \
	GOCACHE="$(GOCACHE)" go run ./cmd/pulse

dev-web-install:
	cd web && npm ci

dev-web:
	cd web && npm run dev

compose-config:
	docker compose config --quiet

backup:
	sh scripts/backup.sh

backup-verify:
	test -n "$(FILE)"
	gzip -t "$(FILE)"

export-config:
	curl --fail --show-error http://localhost:8080/api/v1/export/config -o pulse-config.json

export-entry:
	test -n "$(ID)"
	curl --fail --show-error "http://localhost:8080/api/v1/entries/$(ID)/export.md" -o "$(ID).md"

e2e:
	sh scripts/e2e-rss.sh
