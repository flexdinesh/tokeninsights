# Preserve `--no-sync` as table-only loading

Type: AFK

Status: ready-for-agent

## Parent

Implicit View Sync Progress PRD

## What to build

Keep `view --no-sync` and `tokeninsights --no-sync` as pure read-only viewer paths after the progress TUI change. These commands should skip the sync progress state entirely and use only the existing canonical table loading behavior while querying already-normalized data.

## Acceptance criteria

- [ ] `tokeninsights view --no-sync` does not render harness sync progress.
- [ ] `tokeninsights --no-sync` does not render harness sync progress.
- [ ] `--no-sync` skips raw ingest and normalization.
- [ ] `--no-sync` preserves the read-only missing/incompatible database behavior.
- [ ] `--no-sync` still shows the existing canonical table loading state while dashboard rows are queried.
- [ ] Tests verify that no harness progress statuses appear in `--no-sync` rendered output.
- [ ] README, CLI README, and design documentation keep `--no-sync` documented as the read-only escape hatch.

## Blocked by

- Issue 1
