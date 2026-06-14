# Durable Source Harness Exploration Issue Index

This document indexes the independently executable Durable Source harness exploration issues. Each harness issue lives in a separate document so future exploration sessions can load only the relevant harness scope.

Last audited: 2026-06-13.

## Implementation Status Summary

| Issue | Status | Document | Notes |
|-------|--------|----------|-------|
| 1. Explore OpenCode Modern SQLite Durable Sources | Evidence drafted | [`01-opencode-modern-sqlite.md`](01-opencode-modern-sqlite.md) | SQLite-only. Legacy JSON message storage is out of scope. |
| 2. Explore Pi Session Durable Sources | Evidence drafted | [`02-pi-sessions.md`](02-pi-sessions.md) | Pi JSONL sessions only. Oh My Pi is out of scope. |
| 3. Explore Codex Session Durable Sources | Implemented | [`03-codex-sessions.md`](03-codex-sessions.md) | Stateful token-count interpretation implemented without schema changes. |

## Session Loading Guidance

For a harness exploration session, load:

1. [`PRD.md`](../PRD.md)
2. [`docs/design.md`](../../../docs/design.md)
3. The single harness issue document being executed

Do not load the other harness issue documents unless the current issue explicitly requires comparison.
