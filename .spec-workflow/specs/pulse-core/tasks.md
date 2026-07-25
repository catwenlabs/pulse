# Pulse Core Tasks

- [x] 1. Bootstrap Go backend and local development
  - Create `go.mod`, `cmd/pulse`, configuration loading and graceful shutdown.
  - Add Docker Compose with PostgreSQL health check.
  - Add migration runner and development commands.
  - Write configuration and startup tests before implementation.
  - _Requirements: 10.1, 10.2_

- [x] 2. Define Source domain and Driver contract
  - Write tests for SourceSpec validation, normalized locator and duplicate semantics.
  - Implement Source, Trigger, Checkpoint, Candidate and Driver types.
  - Implement Driver Registry with deterministic unknown-driver errors.
  - _Requirements: 1.1, 1.2, 3.1-3.4_

- [x] 3. Add PostgreSQL Source persistence
  - Write integration tests for create, duplicate rejection, pause and archive.
  - Add migrations for `sources` and `source_checkpoints`.
  - Implement PostgreSQL Adapter behind interfaces owned by Source/Ingestion.
  - _Requirements: 1.1-1.5_

- [x] 4. Implement Acquisition queue and Lease
  - Write concurrent integration tests proving `SKIP LOCKED` single ownership.
  - Add `acquisitions` migration and enqueue/claim/retry/succeed transitions.
  - Implement expiration and recovery of abandoned Leases.
  - _Requirements: 2.1-2.3, 10.2_

- [x] 5. Implement Entry identity and transactional Pipeline
  - Write tests for identity priority, insert, update and retry idempotency.
  - Add `entries` and Tombstone migrations.
  - Atomically commit Entry Upsert and Checkpoint.
  - Prove rollback leaves Checkpoint unchanged.
  - _Requirements: 2.3-2.5, 4.1-4.6_

- [x] 6. Expose first REST vertical slice
  - Define OpenAPI for create/list/get Source, manual run and list Entry.
  - Write handler tests before implementation.
  - Add SSE event envelope without premature event types.
  - _Requirements: 1, 2, 8.1_

- [x] 7. Implement RSS/Atom/JSON Feed Driver
  - Add golden fixtures and contract tests for formats, malformed input and updates.
  - Use conditional HTTP requests and safe response limits.
  - Add OPML import/export.
  - _Requirements: 5.1, 9.5, 10.5_

- [x] 8. Build Source management UI
  - Bootstrap React application and typed OpenAPI-aligned client.
  - Write component tests for Source list, creation, pause and diagnostic states.
  - Implement responsive Source screens.
  - _Requirements: 1, 9.1_

- [x] 9. Build configuration test and preview flow
  - Implement ephemeral test Acquisition and Candidate preview.
  - Build step-based wizard with field errors and identity preview.
  - Add browser E2E tests for RSS creation.
  - _Requirements: 6.1, 6.3, 9.1_

- [x] 10. Implement JSON API Driver and pagination
  - Test JSONPath mapping and page/next-link/cursor pagination.
  - Enforce page, Entry, size and duration limits.
  - Extend wizard with mapping and pagination steps.
  - _Requirements: 5.2, 6.1, 6.4, 6.5_

- [x] 11. Implement static HTML Driver
  - Test single-document and collection extraction.
  - Add CSS Selector mapping and zero-result failure semantics.
  - Build visual selector preview.
  - _Requirements: 5.3, 6.2, 6.6_

- [x] 12. Implement Webhook, Manual and File Drivers
  - Test per-Source Webhook secret validation, rotation, JSON limit and idempotency.
  - Implement explicitly created Manual Source and URL enhancement.
  - Restrict File Source to read-only import roots.
  - _Requirements: 5.4-5.6, 9.3, 10.4_

- [x] 13. Implement Reader, search and organization
  - Add Inbox, read/starred/hidden/later, note and display-title behavior.
  - Add one-level Folder, tags, Views and PostgreSQL search.
  - Write API, component and critical journey tests.
  - _Requirements: 8.1-8.5_

- [x] 14. Implement Rule and Effect Outbox
  - Test structured AST, update re-evaluation, provenance and idempotent Effects.
  - Add in-app and generic Webhook delivery with retry.
  - Add historical preview/replay with notifications disabled by default.
  - _Requirements: 7.1-7.6_

- [x] 15. Complete diagnostics, security and operations
  - Implement health views, redaction and seven-day failure snapshots.
  - Add SSRF-safe HTTP Client and security tests.
  - Implement backup, retention, verification and export commands.
  - Verify race detection, coverage, migrations, E2E and Docker deployment.
  - _Requirements: 9, 10_

- [x] 16. Optimize the Reader for high-throughput reading
  - Keep one-level folders and subscriptions visible in a fixed desktop sidebar.
  - Render a dense article stream with source, title, summary and time.
  - Expand article content and actions inline without leaving the stream.
  - Add server-side Source filtering and responsive reader behavior.
  - Cover navigation, filtering, inline expansion and source creation feedback.
  - _Requirements: 8.1-8.6_

- [x] 17. Preserve rich Feed content in the Reader
  - Render sanitized semantic HTML instead of flattening Entry content to text.
  - Add responsive typography for headings, lists, quotes, code, tables and images.
  - Parse `content:encoded`, Media RSS images and image Enclosures.
  - Version Feed checkpoints so existing Sources are reprocessed once after parser upgrades.
  - Verify rich content, image loading, XSS removal and overflow behavior.
  - _Requirements: 8.7, 9.5_
