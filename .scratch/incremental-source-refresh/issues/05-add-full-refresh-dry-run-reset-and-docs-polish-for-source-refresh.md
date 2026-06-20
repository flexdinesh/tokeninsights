# Add Full Refresh Dry Run Reset And Docs Polish For Source Refresh

Type: AFK

Status: completed

## Parent

Incremental Source Refresh PRD

## What to build

Finish the user-facing command behavior and documentation around Recent Source Refresh and incremental normalization. Add `sync --full-refresh`, ensure dry-run previews source refresh behavior without writing, confirm reset commands handle source refresh state correctly, and update docs so users understand the phased model and escape hatches.

## Acceptance criteria

- [x] `sync --full-refresh` ignores source refresh state for the requested sync scope and full-parses requested sources.
- [x] `sync --full-refresh` updates source refresh state after successful source ingest.
- [x] `sync --full-refresh` does not requeue all existing raw facts for canonical rebuild by default.
- [x] `sync --dry-run` previews freshness-window decisions without writing source refresh state, ingest runs, observations, raw facts, diagnostics, or normalization work.
- [x] `sync --no-normalize` still enqueues pending normalization work for newly inserted raw facts.
- [x] Implicit View Sync processes pre-existing pending normalization work before loading the dashboard.
- [x] `view --no-sync` remains read-only and does not process pending normalization work.
- [x] `reset-canonical --confirm` preserves source refresh state and requeues all existing raw facts for canonical rebuild.
- [x] `reset-all --confirm` clears source refresh state and pending normalization work.
- [x] User-facing sync summaries and progress behavior distinguish absent/skipped harnesses from up-to-date checked sources where the existing UI surface allows it.
- [x] README, CLI README, design docs, and ADR are updated to describe Recent Source Refresh, the 48-hour freshness window, incremental normalization, `--full-refresh`, dry-run behavior, and reset behavior.
- [x] Full schema check, test, and build commands pass.

## Blocked by

- Issue 2 - Recent Source Refresh Tracer Bullet For Pi JSONL
- Issue 3 - Extend Recent Source Refresh To Codex And Claude Code JSONL
- Issue 4 - Apply Recent Source Refresh To OpenCode SQLite
