# Implicit View Sync Issue Index

Status: ready-for-agent

This document indexes the implementation slices for Implicit View Sync. The slices are ordered by dependency and can be handed to AFK agents in sequence.

## Issues

| Issue | Type | Status | Blocked by | Document |
|-------|------|--------|------------|----------|
| 1. Add Implicit View Sync with `--no-sync` | AFK | ready-for-agent | None | [`01-add-implicit-view-sync-with-no-sync.md`](01-add-implicit-view-sync-with-no-sync.md) |
| 2. Fail closed with targeted harness recovery guidance | AFK | ready-for-agent | Issue 1 | [`02-fail-closed-with-targeted-harness-recovery-guidance.md`](02-fail-closed-with-targeted-harness-recovery-guidance.md) |

## Loading Guidance

For any implementation slice, load:

1. The parent PRD.
2. The current design document.
3. The TokenInsights glossary.
4. The single issue document being implemented.

Do not add schema changes or sync-scoping flags to `view` unless a new product decision changes the PRD.
