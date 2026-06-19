# Add Implicit View Sync with `--no-sync`

Type: AFK

Status: ready-for-agent

## Parent

Implicit View Sync PRD

## What to build

Make opening the viewer refresh all supported Durable Sources before the interactive TUI starts. `tokeninsights view` and plain `tokeninsights` should behave like `sync --all` first, including normal default normalization and missing-database creation, then open the TUI over canonical token usage.

Add `--no-sync` as the read-only escape hatch for both `tokeninsights view --no-sync` and `tokeninsights --no-sync`. Viewer filters such as harness, provider, model, session, date range, and Time Bucket must remain display constraints and must not scope ingestion.

## Acceptance criteria

- [ ] `tokeninsights view` performs an implicit all-harness sync before launching the TUI.
- [ ] Running `tokeninsights` with no subcommand performs the same implicit sync before launching the TUI.
- [ ] `tokeninsights view --no-sync` skips both raw ingest and normalization.
- [ ] `tokeninsights --no-sync` behaves like `tokeninsights view --no-sync`.
- [ ] Successful implicit sync does not print the normal sync summary before the TUI launches.
- [ ] Viewer Dimension Filters remain display-only; for example, `view --harness pi` still refreshes all supported harnesses before filtering the viewer to Pi.
- [ ] No sync-scoping flags are added to `view`; custom source and targeted refresh workflows stay on explicit `sync`.
- [ ] Missing database behavior matches `sync` when implicit sync is enabled and preserves old read-only viewer behavior when `--no-sync` is used.
- [ ] README, CLI README, and design documentation describe the new write-before-read command behavior and the `--no-sync` escape hatch.
- [ ] Tests cover the command-level behavior for `view`, no-subcommand `view`, `view --no-sync`, and no-subcommand `--no-sync`.
- [ ] Existing sync, normalize, and viewer filter behavior continues to pass.

## Blocked by

None - can start immediately
