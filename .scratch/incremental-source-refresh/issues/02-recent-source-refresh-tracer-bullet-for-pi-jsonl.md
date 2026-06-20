# Recent Source Refresh Tracer Bullet For Pi JSONL

Type: AFK

Status: completed

## Parent

Incremental Source Refresh PRD

## What to build

Add the first end-to-end Recent Source Refresh path for Pi JSONL Durable Sources. After a successful source refresh, TokenInsights should persist Local-only Continuity Metadata for that Pi source. On later syncs, Pi sources whose modification metadata is older than the 48-hour freshness window before the last successful source refresh should be treated as up to date and skipped. Pi sources inside the freshness window should still be fully parsed, with raw dedupe and the normalization work queue preserving correctness.

This slice includes schema changes if source refresh state was not already added by the normalization queue slice, so implementation must get explicit schema approval before modifying table structures or schema files.

## Acceptance criteria

- [x] Pi source refresh state is stored as Local-only Continuity Metadata and is not used for viewer analytics.
- [x] A first sync of a Pi JSONL source full-parses the source and records successful source refresh state after commit.
- [x] A later sync skips an unchanged Pi source whose modification metadata is older than `last_successful_source_refresh_at_ms - 48h`.
- [x] A later sync fully parses a Pi source whose modification metadata is inside the 48-hour freshness window.
- [x] A later sync fully parses a Pi source whose modification metadata changed after the previous successful source refresh.
- [x] Skipped up-to-date Pi sources create completed lightweight ingest audit rows with zero raw facts and zero observations.
- [x] Dry-run previews Pi freshness behavior without writing ingest runs, source refresh state, observations, raw facts, or work items.
- [x] If source refresh state is missing, stale, or unusable, Pi falls back to full parse.
- [x] Raw fact dedupe still prevents duplicate analytics when a recent Pi source is reparsed.
- [x] Pending normalization work is enqueued only for newly inserted raw facts.
- [x] Tests cover old unchanged, recent, touched, missing-state, and dry-run Pi source cases.
- [x] README, CLI README, design docs, and tests are updated together.

## Blocked by

- Issue 1 - Incremental Normalization Work Queue For Token Usage
