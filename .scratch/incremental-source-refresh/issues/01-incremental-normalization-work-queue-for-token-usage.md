# Incremental Normalization Work Queue For Token Usage

Type: AFK

Status: completed

## Parent

Incremental Source Refresh PRD

## What to build

Add a durable pending normalization work queue for token usage so ordinary normalization processes only raw facts that need canonicalization. Newly inserted raw facts should enqueue token-usage work even when sync is run with `--no-normalize`. Successful normalization should write either canonical token usage or a normalization diagnostic, then remove the work item in the same transaction.

This slice includes schema changes, so implementation must get explicit schema approval before modifying table structures or schema files.

## Acceptance criteria

- [x] Newly inserted raw token facts enqueue pending `token_usage` normalization work.
- [x] Repeated sync of an already-known raw fact does not create duplicate pending work.
- [x] `sync --no-normalize` writes raw facts and leaves their pending normalization work available for a later normalize.
- [x] Ordinary `normalize` processes pending token-usage work instead of scanning all raw token facts by default.
- [x] A work item that produces canonical token usage is removed only when the canonical write commits.
- [x] A work item that produces a missing-session diagnostic is removed only when the diagnostic write commits.
- [x] Failed normalization leaves pending work available for retry.
- [x] `normalize --dry-run` reports pending work by default and writes nothing.
- [x] `reset-canonical --confirm` deletes canonical facts and diagnostics, then requeues all existing raw token facts for token-usage normalization.
- [x] `reset-all --confirm` clears pending normalization work with the rest of the database.
- [x] Ingest-run canonical and diagnostic counters remain idempotent and do not double count repeated normalize runs.
- [x] Schema source, embedded schema, schema constants, README, design docs, and tests are updated together.
- [x] Existing sync, normalize, viewer, schema check, test, and build commands pass.

## Blocked by

None - can start immediately
