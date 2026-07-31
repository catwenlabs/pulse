# ADR 0004: Group cross-Source Entries into Stories

## Status

Accepted

## Context

Different Sources often publish or syndicate reports about the same event. Pulse currently
deduplicates only within one Source through `(source_id, identity_key)`. Flattening every
Entry into the inbox preserves provenance but repeatedly presents the same news.

Cross-Source similarity is probabilistic. Combining or deleting Entries would lose Source
provenance, revisions, and independently authored coverage.

## Decision

Introduce Story as a read and discovery projection over Entries:

- every Entry belongs to exactly one Story;
- every Story contains at least one Entry;
- an unmatched Entry starts in a single-Entry Story;
- aggregation moves Entry membership but never merges or deletes Entry content;
- the main inbox lists Stories, while Source-specific browsing continues to list Entries;
- Story reader state is synchronized to all member Entries.

New Entries receive their single-Entry Story in the same transaction as the Entry. Similarity
aggregation runs after that transaction and never delays Checkpoint movement.

Candidate retrieval is the union of traditional text candidates and embedding candidates.
The final decision combines normalized titles, content hashes, SimHash, publication time,
and embedding similarity. Critical date and number conflicts veto automatic aggregation.
The decision and its component scores are persisted on Story membership.

Embedding is optional derived data. The initial adapter uses local Ollama with
`qwen3-embedding`, configured by:

```text
PULSE_EMBEDDING_PROVIDER
PULSE_EMBEDDING_BASE_URL
PULSE_EMBEDDING_MODEL
```

A missing embedding is represented by `NULL`. Failure falls back to traditional matching;
a periodic pass retries missing embeddings without a separate persistent embedding job
state machine.

PostgreSQL remains the only stateful infrastructure. The `vector` extension stores vectors;
Ollama is an optional stateless compute dependency.

## Consequences

- The inbox has one stable top-level type even when no related coverage exists.
- Source provenance and every original Entry remain available.
- Story membership can improve after an embedding becomes available.
- False-positive control remains more important than maximizing recall.
- PostgreSQL deployments must provide the `vector` extension.
- Model changes require regenerating embeddings because vectors from different models are
  not comparable.

## Note: matching and recompute refinements

Two hard-match and conflict refinements extend the original decision:

- **Canonical URL hard-match.** When two Entries share a non-empty canonical URL, they
  aggregate immediately (match method `url`), ahead of content-hash and title scoring. This
  makes syndicated reposts with rewritten titles aggregate reliably. Critical conflict
  detection still vetoes first, so a same-URL pair that differs on a critical number or
  direction is not aggregated.
- **Critical conflict detection** covers numbers (e.g. `2025` vs `2026`) and opposite
  directions (e.g. `加息` vs `降息`). Subject/entity conflicts (`人物/公司/地点`) are **not**
  implemented: `Entry` carries no entity data, and honest subject-conflict detection requires
  named-entity extraction plus a schema migration and backfill. That is deferred to a separate
  task rather than approximated with a noisy title heuristic.
- **On-demand recompute.** `POST /api/v1/stories/recompute` drains pending aggregation
  immediately (re-evaluating single-Entry Stories and embedding backfill) instead of waiting
  for the background tick, which is useful after a model or algorithm change. It only
  re-evaluates single-Entry Stories; it never auto-splits an existing multi-Entry Story, to
  avoid membership churn. An HTTP-triggered pass is serialized against the background loop so
  the two cannot run concurrently.
