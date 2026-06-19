# Render successful Implicit View Sync progress

Type: AFK

Status: ready-for-agent

## Parent

Implicit View Sync Progress PRD

## What to build

Move successful Implicit View Sync into the TUI lifecycle. Opening `tokeninsights view` or plain `tokeninsights` should immediately render a full-screen sync progress state, update high-level harness statuses while sync runs sequentially, then transition into the existing dashboard loading and table rendering flow after sync and normalization complete.

The sync pipeline should expose optional harness-scoped progress events for the TUI to consume. Explicit `tokeninsights sync` should keep its current one-line summary output.

## Acceptance criteria

- [ ] `tokeninsights view` launches the TUI before Implicit View Sync completes.
- [ ] Plain `tokeninsights` launches the same progress-enabled TUI path.
- [ ] The progress view shows all supported harnesses from the start.
- [ ] Harness rows start as `pending`.
- [ ] Harness rows update through high-level statuses such as `discovering`, `syncing`, `skipped`, and `synced`.
- [ ] Harness sync remains sequential in supported harness order.
- [ ] The progress view shows `normalizing` while canonical facts are being rebuilt after successful harness scopes.
- [ ] The progress view shows `loading dashboard` before the normal dashboard table appears.
- [ ] The progress view does not render source paths, source IDs, project names, or file-level details.
- [ ] Explicit `tokeninsights sync` output remains the existing one-line summary.
- [ ] Successful sync progress transitions into the existing canonical table loading and rendering behavior.
- [ ] README, CLI README, and design documentation describe the TUI sync progress state.
- [ ] Tests cover progress event order, rendered progress statuses, privacy-safe progress output, and successful transition into dashboard loading.

## Blocked by

None - can start immediately
