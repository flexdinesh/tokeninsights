# Extend Recent Source Refresh To Codex And Claude Code JSONL

Type: AFK

Status: completed

## Parent

Incremental Source Refresh PRD

## What to build

Extend the Recent Source Refresh behavior proven by the Pi tracer bullet to Codex JSONL session files and Claude Code JSONL transcript files. Unchanged old JSONL sources should be skipped as up to date, while recent or changed sources should continue using the existing full parser behavior and raw dedupe.

## Acceptance criteria

- [x] Codex JSONL sources persist and use Local-only Continuity Metadata for freshness checks.
- [x] Claude Code JSONL sources persist and use Local-only Continuity Metadata for freshness checks.
- [x] First sync of Codex and Claude Code sources preserves existing full-parse behavior.
- [x] Later sync skips unchanged Codex and Claude Code sources older than the 48-hour freshness window.
- [x] Later sync fully parses Codex and Claude Code sources inside the 48-hour freshness window.
- [x] Later sync fully parses Codex and Claude Code sources whose modification metadata changed.
- [x] Skipped up-to-date sources create completed lightweight ingest audit rows with zero raw facts and zero observations.
- [x] Dry-run previews Codex and Claude Code freshness behavior without writing.
- [x] Missing, stale, or unusable source refresh state falls back to full parse.
- [x] Existing parser diagnostics, duplicate suppression, parent session attribution, provider inference, and raw dedupe behavior are preserved.
- [x] Tests cover old unchanged, recent, touched, and missing-state cases for both harnesses.
- [x] README, CLI README, design docs, and tests are updated together.

## Blocked by

- Issue 2 - Recent Source Refresh Tracer Bullet For Pi JSONL
