# Command-owned local composition and mode configuration

Status: complete
Dependencies: 01

Add mode=single-process|distributed and app-db-path to typed configuration and
CLI overrides. Preserve flags > environment > file > default precedence. Home
config remains mode 0600 and atomic. Default single-process. Existing hosted config
maps to distributed; personal remote URLs require explicit authenticated migration,
never accidental local fallback. No process starts on bare invocation; show help.

Refactor TUI to receive query client/runtime; startup loading model performs capture,
direct delivery and visibility waiting while preserving Retry/View saved/Quit.
Local web reserves requested listener (fail occupied port before capture), processes
initial work then serves/opens browser, remains foreground. Listener bind and browser
URL differ for 0.0.0.0. Ctrl-C cancels and joins. Public web has no write/admin routes.
--sync=false opens saved data. A second viewer fails with fixed owned-database error.

Wait for this invocation's receipts and affected revisions, plus generation activation;
return diagnostics/timeouts without claiming fresh totals. Empty/repeated capture
must also recover previously accepted pending local work. Do not reset retained data.

Wire local sync to the same runtime with no listener. Distributed TUI rejects before
mutation. Distributed web submits foreground and opens canonical remote dashboard.
Reload remains query-only.

Tests: loading cancellation/retry/saved-data, no HTTP local ingestion/TUI reads,
query visibility, occupied ports/ownership, wildcard URL, browser-open failure,
foreground lifecycle, configuration precedence and remote no-fallback.

Implementation: implemented in this worktree. See PRD validation record.

## Unresolved questions

None.
