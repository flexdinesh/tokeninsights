# 2. Constrain `sync --all --source-dir` Discovery To Harness Subdirectories

Execution order: 2

Type: AFK

Blocked by: None - can start immediately

## What to build

Prevent `sync --all --source-dir <root>` from letting Pi or Codex ingest files intended for OpenCode, or vice versa. Multi-harness sync should treat `<root>/<harness>` as the harness boundary and skip harnesses whose subdirectory is absent.

## Acceptance criteria

- [ ] With `sync --all --source-dir <root>`, each harness only scans `<root>/<harness>` when that subdirectory exists.
- [ ] Harnesses with no matching subdirectory are skipped rather than scanning the whole root.
- [ ] Single-harness sync can still scan the explicit source directory directly for ad hoc fixture usage.
- [ ] Tests cover a root containing only `opencode/usage.jsonl` and verify Pi and Codex do not ingest it.

## Blocked by

None - can start immediately.
