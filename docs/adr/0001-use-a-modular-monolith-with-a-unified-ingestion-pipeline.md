# Use a modular monolith with a unified ingestion pipeline

Pulse will begin as a single-user modular monolith in which scheduled pulls, pushed payloads, file changes, and imports become persisted Acquisition Commands and pass through one Entry Pipeline. Source-specific behavior is isolated behind Drivers; Entry identity, rules, checkpoints, effects, search, and reading behavior remain shared. This avoids duplicating correctness logic across acquisition methods while preserving the option to move resource-heavy Drivers, such as browser automation, into separate processes later.

## Consequences

Checkpoint advancement, Entry writes, rule results, and Effect creation share one PostgreSQL transaction. External effects use an Outbox because they cannot participate in that transaction. Drivers may return a proposed Checkpoint but may not persist it themselves.
