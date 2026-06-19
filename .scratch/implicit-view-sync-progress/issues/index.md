# Implicit View Sync Progress Issue Index

Status: ready-for-agent

This document indexes the implementation slices for Implicit View Sync progress. The slices are ordered by dependency and can be handed to AFK agents in sequence.

## Issues

| Issue | Type | Status | Blocked by | Document |
|-------|------|--------|------------|----------|
| 1. Render successful Implicit View Sync progress | AFK | ready-for-agent | None | [`01-render-successful-implicit-view-sync-progress.md`](01-render-successful-implicit-view-sync-progress.md) |
| 2. Preserve `--no-sync` as table-only loading | AFK | ready-for-agent | Issue 1 | [`02-preserve-no-sync-as-table-only-loading.md`](02-preserve-no-sync-as-table-only-loading.md) |
| 3. Exit progress TUI on Implicit View Sync failure | AFK | ready-for-agent | Issue 1 | [`03-exit-progress-tui-on-implicit-sync-failure.md`](03-exit-progress-tui-on-implicit-sync-failure.md) |

## Loading Guidance

For any implementation slice, load:

1. The parent PRD.
2. The current design document.
3. The TokenInsights glossary.
4. The earlier Implicit View Sync PRD if command behavior context is needed.
5. The single issue document being implemented.

Do not add schema changes, source-level progress, concurrent harness sync, or sync-scoping flags to `view` unless a new product decision changes the PRD.
