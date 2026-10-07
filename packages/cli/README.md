# tokeninsights

Collect sanitized evidence in SQLite, submit it to a DuckDB server for asynchronous processing, and query terminal/browser dashboards. Run `tokeninsights tui` to sync in a loading screen and open the dashboard; `--sync=false` views saved data only.

## Install

```sh
brew install flexdinesh/tap/tokeninsights
```

Alternative stable install with Go:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
```

Development version from `main`:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@main
```

Supported harness IDs:

```text
opencode
pi
codex
claude-code
```

## Commands

Everyday commands are `service`, `sync`, and `tui`. Advanced host maintenance uses `collector normalize|reset-canonical|reset-all`; `tokeninsights collector --help` lists that namespace. Previous command names and flags are removed. Use `tui`, the `collector` namespace, and role-specific database flags.

`sync`

Capture whitelisted native evidence in collector SQLite and submit raw batches for acceptance. All supported harnesses are selected by default; `--all` is an explicit equivalent. Server retains evidence and asynchronously computes facts. Collection and delivery produce separate summaries, and failures return a nonzero exit status without deleting committed local work or earlier accepted batches.

```sh
tokeninsights sync
tokeninsights sync --harness opencode
tokeninsights sync --harness pi --source-dir /path/to/source-root
tokeninsights sync --dry-run
tokeninsights sync --full-refresh
tokeninsights sync --no-normalize
tokeninsights sync --publish-only
tokeninsights sync --collector-db-path /path/to/collector.sqlite --server-db-path /path/to/server.duckdb
tokeninsights sync --server-url https://example.test
```

`--publish-only` retries retained raw outbox work without source discovery. `--no-normalize` is a deprecated compatibility flag; processing always happens on server. `--dry-run` does not write or deliver. Without an explicit server URL, delivery ensures the local server after collection; an explicit URL skips local startup even on failure. Later manual sync retries pending delivery; there is no background agent or automatic retry.

Delivery saves immutable request bytes before transport. Evidence, item mappings,
receipt and processing work commit atomically. Acceptance does not imply query
visibility. Exact retries return the original receipt plus current processing
status; native contribution IDs prevent recollection from inflating confirmed
usage. Changed bytes under an existing delivery identity conflict atomically.
Safe semantic conflicts become durable ambiguity, with usable counters shown
separately as estimated. Collector reset/source disappearance never retracts history.


With `sync --all --source-dir`, each harness is read only from its own subdirectory, such as `/path/to/source-root/opencode`; missing subdirectories are skipped. Single-harness sync first looks for a matching harness subdirectory, then falls back to scanning the provided directory directly.

OpenCode sync reads modern SQLite sources named `opencode.db` or `opencode-<channel>.db`, including sessions marked archived in those databases. V1 `message` and V2 `session_message` layouts share the SQLite row ID as message identity; usable V2 data replaces matching V1 data. Pi sync reads JSONL session files from `~/.pi/agent/sessions`, or from a provided Pi source directory; Pi has no harness archive and OS trash is excluded. Codex sync reads rollout JSONL session files from `${CODEX_HOME:-~/.codex}/sessions` and `${CODEX_HOME:-~/.codex}/archived_sessions`, parsing structured `event_msg` token-count records. Claude Code sync reads retained local JSONL transcript files from `${CLAUDE_CONFIG_DIR:-~/.claude}/projects` regardless of UI/server archive state; cloud-only archives are excluded. JSONL capture verifies the saved byte prefix, parses appended records and retains
native context. Rewrites start a new lineage; incomplete tails wait. OpenCode
scans a consistent SQLite snapshot because older rows can change without a
trustworthy native cursor. Unchanged sanitized observations are not re-enqueued.
Full refresh rereads safely; dry-run uses temporary storage without delivery.

One collector writer commits each source's evidence and checkpoint together.
Discovery and capture are sequential initially. Prefix verification reads bytes
without reparsing old records. Server startup performs no source reads.

The server owns token interpretation: input excludes cache read/write, output
excludes separately reported reasoning, and Pi inclusive-input corrections require
source proof. All five components and their sum remain one atomic contribution.
Invalid counters cannot create confirmed usage. Codex ancestry/copy proofs use
native session/turn/provider/model plus complete last/cumulative metadata; equal
counts or rewritten timestamps alone do not prove duplication. Missing/conflicting
ancestry is retained as ambiguity outside confirmed totals.


`collector normalize`

Legacy-only maintenance: process retained pre-upgrade raw facts and journal their normalized changes. Later sync delivers legacy requests with their original protocol-1 semantics. New evidence is processed only by the server; use `service reprocess` to rebuild its projection.

Raw provider and model names remain unchanged. Canonical rows used by queries map Pi `openai-codex` to `openai`, map `fireworks-ai` to `fireworks`, and strip `accounts/fireworks/models/` from Fireworks model names. Server processing applies these aliases to each projection; legacy maintenance applies them only to retained collector canonical rows.

```sh
tokeninsights collector normalize
tokeninsights collector normalize --harness codex
tokeninsights collector normalize --dry-run
```

`collector reset-canonical`

Delete collector canonical sessions/messages/usage and normalization diagnostics while retaining raw facts, observations, source continuity, and publication history. Raw facts are requeued for normalization. This does not delete server facts or receipts.

Requires compatible data with no unfinished recovery. It cannot repair incompatible raw token semantics or identities.

```sh
tokeninsights collector reset-canonical
tokeninsights collector reset-canonical --confirm
```

`collector reset-all`

Reject reset while raw evidence remains unacknowledged. Otherwise transactionally recreate collector application tables inside its SQLite file. This clears raw/canonical facts, pending normalization, continuity, journal, and delivery markers. A later sync reparses retained sources under a new stream and server stable IDs dedupe identical facts. Server history and receipts remain untouched. Reset is not an implicit server retraction or reconciliation policy.

```sh
tokeninsights collector reset-all
tokeninsights collector reset-all --confirm
```

`tui`

Open the interactive terminal UI. Its loading screen runs all-harness collection/publication, then reads the same REST analytics API as the browser. Without a server URL it ensures the local server; an explicit URL publishes local collection to that server without starting a local server. `--sync=false` skips collection; query-only remote viewing opens neither local database. Dashboard Reload and filters only query saved data.

Startup shows per-harness activity, accepted upload batches, and pending entries. Failure offers `r Retry`, `v View saved` when a server endpoint is available, and `q Quit`/Ctrl+C. Viewing saved data skips collection. Quitting cancels active work and preserves committed collector/server data; later sync resumes pending delivery. Retrying a data-read failure only retries the query.

```sh
tokeninsights tui
tokeninsights tui --sync=false           # query saved usage without collecting
tokeninsights tui --server-url https://example.test
tokeninsights tui --today
tokeninsights tui --yesterday
tokeninsights tui --year --bucket month
tokeninsights tui --month --provider openai --model gpt-5
```

Bare `tokeninsights` ensures the local service or prints the configured remote URL. Move root viewer flags to `tokeninsights tui ...`; `--host`/`--port` are service options only.

Every tab's pinned summary shows `sessions <shown> shown / <synced> synced`, followed by the row count and, except in Context, the filtered token total. `shown` counts distinct sessions matching all active filters across the full result, not just the visible scroll viewport. `synced` counts all distinct sessions with countable canonical usage in this database across all dates and harnesses, ignoring viewer filters. Sessions spanning multiple dates or models are counted once; sessions without countable usage are excluded from both counts.

The default current-month filter can show a small subset of synced sessions. Compare `tui --month` with `tui --all-time` using the same `--server-db-path` to inspect date filtering without changing the database. All time removes the preset date restriction but keeps dimension filters and any explicit custom date bounds.

`service start|stop|restart|status|run|reprocess|wait|import`

```sh
tokeninsights service start --open
tokeninsights service status --json
tokeninsights service restart
tokeninsights service stop
tokeninsights service run             # foreground; Ctrl+C stops
tokeninsights                         # ensure local or print configured remote URL
```

Use `--server-db-path` after the service action. Start/run/restart accept `--host`
and `--port`; start also accepts `--open`. Default binding is `127.0.0.1:8765`;
port zero reports the assigned port. Local public serving is unauthenticated,
including `0.0.0.0`. File/environment bind preferences apply on the next start or
explicit restart; a running service is reused. Startup never collects; fresh default DuckDB imports verified sibling server.sqlite read-only; incompatible server files reject without deletion. Legacy saved
token protection requires explicit restart, preserving history and receipts;
legacy bind choices import into root config only when unset. `--reload-sources`,
`refresh`, `--token` and `TOKENINSIGHTS_SERVER_TOKEN` are removed.

Private lifecycle control uses a Unix socket; public REST handles queries/dashboard; private socket handles ingestion/reprocessing. Held/unreachable ownership and busy ports fail without process takeover. Status is read-only; stopped status exits 3, usage exits 2, other failures exit 1.

`tokeninsights-server`

```sh
tokeninsights-server --listen 0.0.0.0:8765 --server-db-path /path/to/server.duckdb
tokeninsights config set server-url http://remote-machine:8765
tokeninsights sync
```

The separate foreground remote composition shares raw acceptance, processing and query code
and requires an explicit server database path. It creates no local service
discovery/control state and reads no client config or harness artifacts. Remote v1
is unauthenticated and pools clients under owner `default`; accounts, auth,
alternate backends and deployment provisioning remain future work.
`tokeninsights server run` is removed.

`config set/get/remove`

Preferences default to `${XDG_CONFIG_HOME:-~/.config}/tokeninsights/config.json`.
Keys are `server-url`, `host`, `port`, `collector-db-path`, `server-db-path`.
Use `set KEY VALUE`, `get KEY` or `remove KEY`. Reads create no files; writes are
typed, private, atomic and serialized. Unknown keys/invalid values reject unchanged.
`get` reads preferences/defaults; runtime precedence is flags > environment >
file > defaults. Existing `TOKENINSIGHTS_SERVER_URL` and role path variables remain;
`TOKENINSIGHTS_HOST` and `TOKENINSIGHTS_PORT` override local bind preferences.
Root `--config-file PATH` or `TOKENINSIGHTS_CONFIG_PATH` selects the file.

Sync and TUI select the same destination; an explicit empty URL restores local.
Service commands always manage local. Remote delivery never falls back locally.
A new destination receives retained evidence; existing destinations resume
their own cursors. See [system design](../../docs/system.md).

The browser provides Tokens, Models, Providers, Harnesses, Sessions, Context, and Repo views, charts, faceted filters, custom dates, session-ID search, sorting, pagination, and themes. Repo groups by repository/directory; location filters apply only there. Unknown groups remain part of totals and can reveal recorded contributing directories. Published directory names are basenames, not collector-local full paths. URL state preserves query scope/navigation. Reporting periods use the server timezone and Monday-start weeks.

Browser **Reload** and TUI `r` query committed data. Empty state points to `tokeninsights sync`; no server endpoint starts a collector. Source-day coverage is absent because unavailable uploads cannot establish checked/empty days. Failed reads preserve filters and available saved results. TUI quits cancel reads, not already committed collection/ingestion transactions.

New raw ingestion does not upload a producer hostname. The query label is `unknown`; source identity never comes from a machine label. Browser requests remain same-origin, without a destination selector or advertised CORS access. Explicit CLI `--server-url` / `TOKENINSIGHTS_SERVER_URL` selects another server and bypasses local bootstrap.

| Endpoint | Purpose |
| --- | --- |
| `GET /api/v1/instance` | Versions, runtime/database identity, producer labels, reporting timezone, defaults |
| `GET /api/v1/sync` | Read-only readiness and canonical revision; status |
| `GET /api/v1/usage` | Filtered summary, chart, rows, revision, last committed ingestion |
| `GET /api/v1/usage/facets` | Filter facets/session search |
| `GET /api/v2/ingestion/capabilities` | Raw protocol/limits, database identity and acceptance mode |
| `POST /api/v2/ingestion/batches` | Durable raw acceptance; 202 pending / 200 terminal |\n| `GET /api/v2/ingestion/batches/{stream}/{batch}` | Immutable receipt plus current item outcomes |

`POST /api/v1/sync` is rejected. Raw ingestion allows at most 256 entries and 1 MiB per batch, 256-byte strings, and nonnegative integers/aggregate counters within `9007199254740991`. Unsupported versions, private/unknown fields, duplicate JSON keys, invalid identities/totals, and changed delivery payloads fail explicitly. Protocol 1 remains legacy compatibility only; local public routes expose neither ingestion protocol. Admission is bounded to four concurrent ingestions; busy responses keep pending requests retryable.

[`docs/openapi.yaml`](../../docs/openapi.yaml) is the repository-only REST contract; it is not served at runtime. `pnpm run generate:api` creates committed Go/TypeScript query models and schemas; `pnpm run check-api` checks drift. Native Go builds consume committed output and embedded assets, without Node.

## Database

Default role paths:

| Role | Default file | Override |
| --- | --- | --- |
| Collector | `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/collector.sqlite` | `--collector-db-path`, `TOKENINSIGHTS_COLLECTOR_DB_PATH` |
| Server | `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/server.duckdb` | `--server-db-path`, `TOKENINSIGHTS_SERVER_DB_PATH` |

The former `tokeninsights.sqlite` remains untouched. Retained harness artifacts populate fresh collector/server storage through normal collection and ingestion; verified history import preserves baseline facts and receipts. Role checks precede mutation; a collector file cannot serve queries and a server file cannot enter producer recovery/reset. Identical, symlink-equivalent, and existing hard-linked collector/server paths are rejected.

Use `--collector-db-path` for collection and maintenance, and `--server-db-path` for server storage. `tui` can select both roles. Previous command names, `--db-path`, and `--no-sync` are rejected. Legacy `TOKENINSIGHTS_DB_PATH` does not select either new default.

Collector schema 17 retains raw outbox/continuity/exact delivery state and legacy
tables. Verified schema 16 upgrades additively. DuckDB schema 1 stores raw,
receipts/status, typed facts, separate estimates and retained baselines.
Fresh default DuckDB imports verified sibling server.sqlite read-only; custom
imports require explicit new target. Unsupported contracts reject without reset.
Pending raw prevents collector reset; reset never retracts server data.
Remote database identity stays pinned; deliberate local replacement permits replay.

## TUI Arguments

`--sync`

Collect and publish all harnesses inside the TUI before showing data. Enabled by default; `--sync=false` only queries saved data. Query filters do not narrow collection.

`--server-url URL`

Select an existing server. Defaults come from client config and `TOKENINSIGHTS_SERVER_URL`; an explicit empty value selects local.

`--server-db-path PATH`, `--collector-db-path PATH`

Select local server storage and the collector used by startup sync. Remote TUI with `--sync=false` opens neither file.

`--today`

Show data from today.

`--yesterday`

Show data from yesterday.

`--week`

Show data from the current calendar week.

`--month`

Show data from the current calendar month. This is the default Date Range Filter for interactive view.

`--year`

Show data from the current calendar year.

`--all-time`

Show all canonical data with no period filter.

`--bucket day|week|month|year`

Set the Tokens tab Time Bucket. The default bucket is `day`; week buckets start on Monday.

`--session-id ID`

Filter by canonical harness session ID. Can be repeated or comma-separated.

`--provider ID`

Filter by provider. Can be repeated or comma-separated.

`--model ID`

Filter by model. Can be repeated or comma-separated.

`--harness ID`

Filter by `opencode`, `pi`, `codex`, or `claude-code`. Can be repeated or comma-separated.

`--filter-day-from YYYY-MM-DD`

Inclusive local-day lower bound.

`--filter-day-to YYYY-MM-DD`

Inclusive local-day upper bound.

## Interactive Keys

The Instrument desk layout prioritizes full-screen terminals (120×35 or larger): producer/ingestion status, seven-view navigation, scope controls, a token readout strip, full-width table, pinned session counts, and a short key guide. Canvas, readouts, and drawers use the terminal background; foreground colors adapt to light/dark terminals; bracketed active navigation and a row cursor also identify selection without color. Smaller terminals compact navigation and omit the readout strip when space is scarce; table metrics remain accessible by scrolling.

Press `q` or Ctrl+C to quit. Use tab/shift-tab or number keys 1-7 to switch Aggregation Tabs. Use `d` for Date Range Filter, `g` for Time Bucket or Repo grouping, `s` for sort, and `p`, `m`, or `h` for provider, model, or harness filters. `f` opens a filter menu; Repo adds repository and directory facets there. `?` opens the keyboard guide. Drawers open on the right while the dashboard stays visible. Use up/down or j/k to navigate, Space to toggle multi-select values, Enter to apply, and Escape to cancel without changing scope or table position. `c` clears the draft selection; applying an empty selection includes all values. Choosing a date preset replaces custom date bounds.

Use `up/down` or `j/k` to move through rows, PageUp/PageDown to move a page, `left/right` to scroll horizontally, and `home/end` to jump to the start or end of the horizontal table viewport. `r` reloads canonical usage read-only; it does not sync sources. Failed reads offer this retry action.

The six readouts show total, input, output, reasoning, cache read, and cache write tokens summed from the full filtered result, independently of scrolling. Loading uses placeholders rather than stale values. Context shows an explanation of session peaks instead of additive token readouts.

Tables fit the current terminal width where possible. Summary columns with multiple model, provider, or harness values stack those values vertically within the row instead of collapsing them to a count. Long model, provider, and harness values are truncated in-place; horizontal scrolling is used only when the minimum readable table width is still wider than the viewport.

## Metrics

Token columns come from countable rows in the active `analytics.confirmed` view:

```text
input
output
reasoning
cache R
cache W
total
```

Missing model values are normalized to `unknown`. Missing provider values are normalized to `unknown`, except Claude Code artifact-derived rows, which appear as `maybe-anthropic` with inferred provider provenance.

The active Aggregation Tabs are Tokens, Models, Providers, Harnesses, Sessions, Context, and Repo. Repo groups optional fact-level location attribution by repository or directory. Repo rows list distinct providers, harnesses, and models in the same row. Repository identity combines clones with the same remote. Missing location values display as **unknown**. TPS, request, and tool domains remain future-compatible data domains, but they are not active empty viewer tabs. Preserve `tps avg`, `tps mean`, and `tps median` when durable timing becomes available; no timing is inferred from token counts. The Sessions tab also shows derived `ctx used`, the peak prompt-side context load for the session. Context groups session peaks by harness, provider, and model, showing `sessions`, `avg ctx`, `median ctx`, and `max ctx`.

Session IDs are shortened in table output. Model names with `/` are shortened to the last path segment where a compact display is needed.


## Evidence, processing and upgrades

Sync waits for acceptance. Browser Confirmed / Estimated selects separate data;
estimates never inflate confirmed totals. Unusable evidence retains diagnostics.
Fresh default server.duckdb imports verified sibling server.sqlite read-only,
preserving history, identity and receipts. Partial rebuilds preserve unmatched
history. Custom paths, after stopping local service:

    tokeninsights service import --server-db-path NEW.duckdb --legacy-server-db-path OLD.sqlite
    tokeninsights service start
    tokeninsights service reprocess
    tokeninsights service wait

Remote accepts legacy-server-db-path for new target. Source stays intact.
Reprocessing keeps published generation until complete. Service wait is explicit
30-second maintenance wait; sync does not wait. TUI queries confirmed usage.
CGO/C/C++ toolchain builds embedded DuckDB. Production native archives need no JS.
