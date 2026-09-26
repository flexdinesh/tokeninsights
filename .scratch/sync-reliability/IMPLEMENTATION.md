# Sync reliability — implemented

Schema approved by user. V13 metadata upgrade preserves V11/V12 raw and canonical usage; migration runs on the next sync. Live usage DB was not modified during verification.

- Replace fixed Scanner limit with bounded, cancellation-aware JSONL snapshots; defer incomplete tails and changing files for retry.
- Continue recoverable source failures; publish normalized usage per harness.
- Persist job/harness/source status, actual timestamps, publication revisions, and last successful normalized all-harness check.
- Derive conservative calendar coverage; placeholders never affect analytics or session counts.
- Share progress between processes, recover interrupted jobs, retain partial results, expose retry.
- Apply Impeccable only after backend completion. Preserve existing dashboards; add coverage disclosure and honest work progress.

Verification: format, lint, full tests (44 web unit tests; all Go packages; 9 build-tool tests), schema/API checks, production build, embedded asset check, and all 9 browser tests pass. Desktop/mobile inspected in bounded passes. Impeccable detector returned no findings. Native Go build ran sync and served embedded UI without Node/npm/pnpm in PATH. TUI exercised in the project PTY against an isolated fixture with current-day usage and a deferred active-file tail. Previously failing live Codex artifact parses in read-only dry-run.

Future refactor: derive UI progress copies from a shared semantic contract; keep source-count work separate from confirmed coverage.
