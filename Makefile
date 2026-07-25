GOCACHE ?= /tmp/pulse-go-cache

.PHONY: test test-race vet run compose-config backup backup-verify export-config export-entry e2e

test:
	GOCACHE=$(GOCACHE) go test -cover ./...

test-race:
	GOCACHE=$(GOCACHE) go test -race ./...

vet:
	GOCACHE=$(GOCACHE) go vet ./...

run:
	GOCACHE=$(GOCACHE) go run ./cmd/pulse

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
