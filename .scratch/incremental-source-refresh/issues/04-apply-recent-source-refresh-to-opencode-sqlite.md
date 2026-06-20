# Apply Recent Source Refresh To OpenCode SQLite

Type: AFK

Status: completed

## Parent

Incremental Source Refresh PRD

## What to build

Apply Recent Source Refresh to OpenCode SQLite Durable Sources using database file modification metadata. Unchanged old OpenCode databases should be skipped as up to date. Recent or changed OpenCode databases should continue using the existing full table parser; true row cursors remain out of scope for this slice.

## Acceptance criteria

- [x] OpenCode SQLite sources persist and use Local-only Continuity Metadata for freshness checks.
- [x] First sync of an OpenCode SQLite source preserves existing full-parse behavior.
- [x] Later sync skips an unchanged OpenCode SQLite source older than the 48-hour freshness window.
- [x] Later sync fully parses an OpenCode SQLite source inside the 48-hour freshness window.
- [x] Later sync fully parses an OpenCode SQLite source whose modification metadata changed.
- [x] Skipped up-to-date OpenCode sources create completed lightweight ingest audit rows with zero raw facts and zero observations.
- [x] Dry-run previews OpenCode freshness behavior without writing.
- [x] Missing, stale, or unusable source refresh state falls back to full parse.
- [x] Existing OpenCode SQLite schema validation, assistant-row filtering, duplicate suppression, and raw dedupe behavior are preserved.
- [x] True OpenCode row cursors are not implemented in this slice.
- [x] Tests cover old unchanged, recent, touched, missing-state, and invalid-source cases.
- [x] README, CLI README, design docs, and tests are updated together.

## Blocked by

- Issue 2 - Recent Source Refresh Tracer Bullet For Pi JSONL
