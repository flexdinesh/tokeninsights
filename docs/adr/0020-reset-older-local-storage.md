# Reset older local storage after confirmation

Status: Accepted. Date: 23 October 2026.

## Decision

Local commands (`tui`, `web`, `data`, and non-dry-run `sync`/`sync status`) inspect
their SQLite stores read-only before opening them. A store whose role matches and
whose `user_version` is lower than this build supports was written by an older
release. After the user confirms in a terminal, TokenInsights moves it aside to
`<file>.v<old>.bak` (plus `-wal`/`-shm`; a taken name gains a UTC timestamp) and
the command creates a current store.

- Token and application stores reset as a pair with the `.application.json` guard:
  a new token store has a new database ID.
- An older application store resets alone with its guard; credentials are recreated.
- Collector and sync-job stores reset alone.
- Newer, foreign, or mismatched files, or files owned by another process, block
  every reset and print the cause, path, and next step without changing files.
- Without a terminal, older stores are reported and left unchanged.
- Remote servers never reset; deployed storage still rejects without mutation.

This supersedes the "reject without mutation" and "does not authorize deleting
users' files" clauses of ADRs 0010 and 0012 for local stores only. Files are moved,
never deleted.

## Consequences

Collector evidence is immutable and retained, so resetting only the token store
replays history, and resetting only the collector keeps local history while stable
identities deduplicate reread logs. Resetting both loses usage whose harness logs
are gone; the prompt warns, and the `.bak` files allow manual recovery. Background
plugin syncs fail until the user runs a local command in a terminal. No schema SQL
or migration path changes.
