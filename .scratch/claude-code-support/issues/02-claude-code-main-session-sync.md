# Sync modern Claude Code main-session token usage

Type: AFK

Status: ready-for-agent

## Parent

Claude Code Harness Support PRD

## What to build

Add end-to-end sync, normalization, and viewing support for modern Claude Code main-session JSONL transcripts. The adapter should discover current Claude Code transcript files, parse assistant token usage metadata, write deduplicated raw token facts, normalize them into canonical token facts, and make them visible through the existing viewer aggregations and filters.

Keep this slice focused on main session files. Subagent attribution and copied transcript suppression are covered by the follow-up slice.

## Acceptance criteria

- [ ] `tokeninsights sync --harness claude-code` discovers modern Claude Code JSONL transcript files from the current Claude Code projects layout.
- [ ] The adapter honors the current Claude Code configuration root environment variable and falls back to the user's `.claude` directory when it is absent.
- [ ] Single-harness `--source-dir` scans the provided directory directly for JSONL fixtures.
- [ ] `sync --all --source-dir <root>` scans the `claude-code` subdirectory and skips it cleanly when absent.
- [ ] Assistant usage rows produce raw token facts with stable session identity from the transcript file stem.
- [ ] Message identity uses `message.id` when present.
- [ ] Streaming duplicate rows in one main-session file merge by `message.id + requestId` when both exist, or by `message.id` when request ID is absent.
- [ ] Claude Code cache read and cache creation token fields map to existing cache read and cache write token columns.
- [ ] Raw total tokens remain absent when the source does not provide a total; canonical totals are derived by existing normalization.
- [ ] Explicit provider fields are stored when present with provider source `explicit`; missing Claude Code provider canonicalizes to `maybe-anthropic` with provider source `inferred`.
- [ ] Missing model canonicalizes to `unknown`.
- [ ] Claude Code token facts are marked `derived`.
- [ ] A source-level diagnostic explains that Claude Code JSONL usage is transcript-derived.
- [ ] Parser diagnostics avoid private content and full source paths.
- [ ] Fixture-style conformance tests prove raw, observation, canonical, and diagnostic output for a main-session Claude Code source.
- [ ] README and design documentation describe Claude Code main-session sync behavior and token semantics.

## Blocked by

- Issue 1
