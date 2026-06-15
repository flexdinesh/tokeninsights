# Claude Code Harness Support Issue Index

Status: ready-for-agent

This document indexes the implementation slices for Claude Code Harness support. The slices are ordered by dependency and can be handed to AFK agents in sequence.

## Issues

| Issue | Type | Status | Blocked by | Document |
|-------|------|--------|------------|----------|
| 1. Add the `claude-code` harness storage contract | AFK | ready-for-agent | None | [`01-claude-code-storage-contract.md`](01-claude-code-storage-contract.md) |
| 2. Sync modern Claude Code main-session token usage | AFK | ready-for-agent | Issue 1 | [`02-claude-code-main-session-sync.md`](02-claude-code-main-session-sync.md) |
| 3. Attribute Claude Code subagent and duplicate transcript token usage safely | AFK | ready-for-agent | Issue 2 | [`03-claude-code-subagents-and-dedupe.md`](03-claude-code-subagents-and-dedupe.md) |

## Loading Guidance

For any implementation slice, load:

1. The parent PRD.
2. The current design document.
3. The ADR about harness constraint schema versioning.
4. The single issue document being implemented.

Do not reopen Cursor scope unless a new product decision changes the Durable Source boundary.
