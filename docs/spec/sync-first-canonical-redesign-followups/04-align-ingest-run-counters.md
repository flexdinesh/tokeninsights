# 4. Make Ingest Run Counters Match Auto-Normalization Results

Execution order: 4

Type: AFK

Blocked by: None - can start immediately

## What to build

Align ingest run count columns with the actual sync-plus-normalize behavior. After a normal sync with auto-normalization enabled, the relevant ingest run rows should reflect canonical facts and diagnostics created from that run's observations, or the docs and tests should explicitly narrow what those counters mean.

## Acceptance criteria

- [ ] Auto-normalized canonical count is reflected in ingest-run audit data, or the counter contract is explicitly renamed or documented as raw-ingest-only.
- [ ] Missing-session normalization diagnostics are represented consistently in run-level counts.
- [ ] Repeated sync and normalize remain idempotent.
- [ ] Tests assert ingest-run counter behavior after successful normalization and missing-session normalization.

## Blocked by

None - can start immediately.
