# 1. Make Source Ingest Atomic

Execution order: 1

Type: AFK

Blocked by: None - can start immediately

## What to build

Ensure each discovered source ingest is committed atomically. If any raw fact, observation, or diagnostic write fails for a source, that source should not leave partially written raw rows or observations behind, and the run summary should accurately reflect what was persisted.

## Acceptance criteria

- [x] A source ingest uses one transaction for ingest run creation, raw fact writes, observation writes, diagnostics, and run completion.
- [x] A mid-source write failure leaves no partial raw facts or observations for that source.
- [x] Failed source runs are still represented clearly as failed ingest attempts.
- [x] Auto-normalization only considers facts that were successfully committed.
- [x] Tests cover a valid row followed by a write failure and assert rollback and summary behavior.

## Blocked by

None - can start immediately.
