# Exit progress TUI on Implicit View Sync failure

Type: AFK

Status: ready-for-agent

## Parent

Implicit View Sync Progress PRD

## What to build

When Implicit View Sync fails while the progress TUI is active, stop before rendering the dashboard table. The command should exit the TUI and print the same terminal sync summary, underlying error, and targeted harness recovery guidance used by the existing Implicit View Sync failure path.

The failure should remain simple: no in-TUI recovery workflow and no stale canonical table after failed implicit sync.

## Acceptance criteria

- [ ] A failing harness can be represented as `failed` before the TUI exits.
- [ ] If Implicit View Sync fails, the dashboard table is not rendered.
- [ ] If Implicit View Sync fails, the TUI exits.
- [ ] The terminal output includes the sync summary.
- [ ] The terminal error includes the underlying sync failure.
- [ ] The terminal guidance recommends `tokeninsights sync --harness <harness>`.
- [ ] The terminal guidance recommends `tokeninsights view --no-sync` after manual refresh.
- [ ] No in-TUI retry or recovery workflow is added.
- [ ] Tests cover a failing implicit sync path from progress state through terminal summary/error/guidance.
- [ ] README, CLI README, and design documentation describe the failure behavior.

## Blocked by

- Issue 1
