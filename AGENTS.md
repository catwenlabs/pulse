# Pulse Agent Guide

This file defines repository-level instructions for AI coding agents and automated contributors. Human-facing usage and deployment instructions live in `README.md`.

## Required Reading

Before changing code:

1. Read `CONTEXT.md` for the domain language.
2. Read `docs/architecture.md` when the change affects module boundaries, persistence, ingestion, concurrency, or deployment.
3. Treat `api/openapi.yaml` as the HTTP API contract.
4. Inspect the nearest tests before modifying behavior.

Use the project terms Source, Driver, Trigger, Acquisition, Candidate, Entry, Checkpoint, Identity Key, Tombstone, Rule, View, Folder, and Effect exactly as defined in `CONTEXT.md`.

## Runtime Contract

| Component | Contract |
| --- | --- |
| Go | `1.25`; process entrypoint is `./cmd/pulse` |
| Node.js | `22`; frontend lives in `./web` |
| PostgreSQL | `17`; the only stateful infrastructure |
| Application | `http://localhost:8080` |
| Vite development server | `http://localhost:5173` |
| Local development database | `127.0.0.1:54321` |
| Health endpoint | `GET /healthz` |
| Container image | `ghcr.io/wenpengfei/pulse` |

The default process enables `web,scheduler,worker,effect-worker`. Database migrations run automatically during application startup.

## Repository Map

| Path | Responsibility |
| --- | --- |
| `cmd/pulse/` | Process entrypoint, role assembly, and lifecycle |
| `internal/source/` | Source domain model |
| `internal/ingestion/` | Acquisition and the unified ingestion pipeline |
| `internal/drivers/` | Feed, JSON API, HTML, push, and file Drivers |
| `internal/entry/` | Entry identity and content model |
| `internal/rule/` | Structured rule evaluation |
| `internal/effect/` | Effect processing |
| `internal/storage/` | PostgreSQL adapters and SQL migrations |
| `internal/transport/httpserver/` | HTTP API and static frontend delivery |
| `web/src/` | React client |
| `api/openapi.yaml` | HTTP API source of truth |
| `docs/architecture.md` | Architecture, transaction, and failure model |
| `docs/adr/` | Architecture decisions |

## Development Workflow

1. Establish the affected domain and read its implementation and tests.
2. Preserve module boundaries from `docs/architecture.md`.
3. Make the smallest coherent change.
4. Update tests and API contracts alongside behavior.
5. Run the narrowest relevant checks while iterating.
6. Run the full change-specific verification before handing off.

Prefer existing Make targets. Do not duplicate build commands across documentation, CI, and scripts.

## Local Development

Start PostgreSQL, the backend, and the frontend development server together:

```sh
make dev
```

The first run installs frontend dependencies when they are missing. Press `Ctrl+C` to stop services started by this command.

Alternatively, start PostgreSQL separately:

```sh
make dev-db-up
```

Run the backend:

```sh
make dev-api
```

Run the frontend in another terminal:

```sh
make dev-web-install
make dev-web
```

The Vite server proxies `/api` and `/healthz` to the Go backend on port `8080`.
Use `make dev-db-logs` to follow PostgreSQL logs and `make dev-db-down` to stop the development database.

The development defaults can be overridden without changing tracked files:

```sh
make dev-api DEV_DATABASE_URL='postgres://...' DEV_IMPORT_ROOTS='./imports'
```

## Build and Test Commands

| Change scope | Required verification |
| --- | --- |
| Go | `make vet && make test-race` |
| React / TypeScript | `cd web && npm ci && npm run lint && npm test && npm run build` |
| Compose | `docker compose config --quiet` |
| Dockerfile or cross-stack | `docker compose build pulse` |
| RSS browser journey | Start the full application, then run `make e2e` |

For a fast Go test iteration, use:

```sh
make test
```

## API Change Rules

When changing an endpoint or payload:

1. Update `api/openapi.yaml`.
2. Update the HTTP handler and its tests.
3. Update `web/src/api.ts`.
4. Update affected React tests.
5. Preserve redaction of credentials and secrets.

Do not introduce an undocumented endpoint or let frontend types drift from the OpenAPI contract.

## Architecture Boundaries

- Keep the modular monolith. Do not introduce Redis, an external queue, or another stateful dependency without an explicit architecture decision.
- Drivers understand external protocols and return Candidates. They do not modify Entries, Rules, reader state, or Checkpoints directly.
- Checkpoints move only after the complete batch commits successfully.
- Entry, rule results, Effect Outbox records, and Checkpoint updates share the required transaction boundary.
- Network Drivers must use the controlled HTTP client in `internal/platform/httpclient`.
- PostgreSQL adapters contain persistence details, not domain decisions.
- The root `Dockerfile` is the single image build source for local Compose and CI.

## Security and Data Safety

- Never commit, print, or log `PULSE_MASTER_KEY`, tokens, cookies, passwords, webhook secrets, or authentication headers.
- Never regenerate the `PULSE_MASTER_KEY` for an existing database. Doing so makes stored source credentials unreadable.
- Never run `docker compose down -v` unless the user explicitly requests permanent database deletion.
- Keep `imports/` mounted read-only and restrict File Sources to `PULSE_IMPORT_ROOTS`.
- Preserve SSRF, redirect, response-size, unsafe HTML, and credential-redaction protections.
- Treat backup restoration as destructive. Back up the current database before restoring another dump.

## Git Hygiene

- Preserve unrelated user changes.
- Stage only files in the requested scope.
- Do not commit generated frontend directories such as `web/node_modules`, `web/dist`, or `web/coverage`.
- Do not commit `backups`, exports, imported user files, or secrets.
- Use concise commit messages that describe the behavior or documentation change.

## Definition of Done

A change is complete when:

- behavior and tests agree;
- the OpenAPI contract is synchronized when applicable;
- relevant verification commands pass;
- documentation reflects changed operation or deployment behavior;
- no secrets, generated artifacts, or unrelated edits are included;
- the final handoff reports what changed, what was verified, and any remaining risk.
