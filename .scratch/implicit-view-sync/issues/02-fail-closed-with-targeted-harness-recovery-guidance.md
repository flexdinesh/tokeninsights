# Fail closed with targeted harness recovery guidance

Type: AFK

Status: ready-for-agent

## Parent

Implicit View Sync PRD

## What to build

When Implicit View Sync fails, stop before launching the TUI. The command should surface the sync summary and underlying error, then give users a concrete recovery path: run targeted manual sync for unaffected harnesses with `tokeninsights sync --harness <harness>`, then open the viewer with `tokeninsights view --no-sync`.

This should preserve the existing explicit `sync --all` failure semantics while making the `view` failure path actionable.

## Acceptance criteria

- [ ] If Implicit View Sync returns an error, the TUI is not launched.
- [ ] The failure path prints the sync summary so users can see requested, synced, skipped, failed, raw fact, observation, canonical, and diagnostic counts.
- [ ] The failure path returns or prints the underlying sync error.
- [ ] The failure output recommends targeted manual refresh with `tokeninsights sync --harness <harness>`.
- [ ] The failure output recommends opening `tokeninsights view --no-sync` after manual refresh.
- [ ] The recommendation is only shown for implicit view sync failures and does not change explicit `sync` command output.
- [ ] Tests cover a failing implicit sync path and assert that the TUI does not start.
- [ ] README, CLI README, and design documentation describe the failure behavior and recovery path.

## Blocked by

- Issue 1
