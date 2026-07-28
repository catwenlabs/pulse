GOCACHE ?= /tmp/pulse-go-cache
DEV_COMPOSE := docker compose -f compose.yaml -f compose.dev.yaml
DEV_DATABASE_URL ?= postgres://pulse:pulse@127.0.0.1:54321/pulse?sslmode=disable
DEV_IMPORT_ROOTS ?= ./imports

.PHONY: test test-race vet run dev dev-db-up dev-db-down dev-db-logs dev-api dev-web-install dev-web compose-config backup backup-verify export-config export-entry e2e

test:
	GOCACHE=$(GOCACHE) go test -cover ./...

test-race:
	GOCACHE=$(GOCACHE) go test -race ./...

vet:
	GOCACHE=$(GOCACHE) go vet ./...

run:
	GOCACHE=$(GOCACHE) go run ./cmd/pulse

dev:
	sh scripts/dev.sh

dev-db-up:
	$(DEV_COMPOSE) up -d postgres

dev-db-down:
	$(DEV_COMPOSE) down

dev-db-logs:
	$(DEV_COMPOSE) logs -f --tail=200 postgres

dev-api:
	PULSE_DATABASE_URL="$(DEV_DATABASE_URL)" \
	PULSE_IMPORT_ROOTS="$(DEV_IMPORT_ROOTS)" \
	$(MAKE) run

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
