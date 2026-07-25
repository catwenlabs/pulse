# Use PostgreSQL as the only stateful infrastructure

Pulse will use PostgreSQL for domain data, full-text and fuzzy search, scheduled jobs, leases, retries, checkpoints, and the Effect Outbox. Redis and a separate search engine are deliberately excluded from the first release because a single-user deployment does not justify their backup and failure-recovery cost; PostgreSQL also permits multiple Workers through `FOR UPDATE SKIP LOCKED` and leaves search behind a replaceable interface.

## Consequences

Docker Compose requires only Pulse and PostgreSQL as stateful runtime dependencies. English search uses `tsvector`, while initial Chinese search uses normalized text with `pg_trgm`; a dedicated search Adapter can be introduced later without changing Reader behavior.
