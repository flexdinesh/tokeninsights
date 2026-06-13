# Durable Source Harness Exploration Issue Index

This document indexes the independently executable Durable Source harness exploration issues. Each harness issue lives in a separate document so future exploration sessions can load only the relevant harness scope.

Last audited: 2026-06-13.

## Implementation Status Summary

| Issue | Status | Document | Notes |
|-------|--------|----------|-------|
| 1. Explore OpenCode Modern SQLite Durable Sources | Not started | [`01-opencode-modern-sqlite.md`](durable-source-harness-exploration-issues/01-opencode-modern-sqlite.md) | SQLite-only. Legacy JSON message storage is out of scope. |
| 2. Explore Pi Session Durable Sources | Not started | [`02-pi-sessions.md`](durable-source-harness-exploration-issues/02-pi-sessions.md) | Pi only. Oh My Pi is out of scope. |
| 3. Explore Codex Session Durable Sources | Not started | [`03-codex-sessions.md`](durable-source-harness-exploration-issues/03-codex-sessions.md) | Requires stateful token interpretation analysis. |

## Session Loading Guidance

For a harness exploration session, load:

1. [`durable-source-harness-exploration-prd.md`](durable-source-harness-exploration-prd.md)
2. [`../design.md`](../design.md)
3. The single harness issue document being executed

Do not load the other harness issue documents unless the current issue explicitly requires comparison.
