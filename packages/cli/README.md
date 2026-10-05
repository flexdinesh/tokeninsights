# tokeninsights

Collect local token usage, normalize it in host SQLite, publish canonical facts to a SQLite server, and query terminal/browser dashboards. Run `tokeninsights sync`, then `tokeninsights view`. Viewing is read-only by default.

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

`sync`

Collect durable sources into collector raw tables, normalize locally, journal canonical changes, and publish pending normalized batches. All supported harnesses are selected by default; `--all` is an explicit equivalent. The server receives no raw data or parser state. Collection and delivery produce separate summaries, and failures return a nonzero exit status without deleting committed local work or earlier accepted batches.

```sh
tokeninsights sync
tokeninsights sync --harness opencode
tokeninsights sync --harness pi --source-dir /path/to/source-root
tokeninsights sync --dry-run
tokeninsights sync --full-refresh
tokeninsights sync --no-normalize
tokeninsights sync --publish-only
tokeninsights sync --collector-db-path /path/to/collector.sqlite --server-db-path /path/to/server.sqlite
tokeninsights sync --server-url https://example.test
```

`--publish-only` retries retained journal work without source discovery. `--no-normalize` captures raw facts without normalizing new work, but can still publish previously journaled work. `--dry-run` does not write or deliver. Without an explicit server URL, delivery ensures the local server after collection; an explicit URL skips local startup even on failure. `--token` / `TOKENINSIGHTS_SERVER_TOKEN` authenticates that client. Later manual sync retries pending delivery; there is no background agent or automatic retry.

Publication saves immutable request bytes before transport. Server facts and receipts commit atomically, so acknowledgement means queryable data. Replayed batches return their original receipt; stable source-native fact IDs also dedupe recollection after collector SQLite is deleted. Conflicting immutable payloads fail the whole batch, preserving the pending suffix. Claude's approved source-timestamp revision rule permits newer native-request snapshots and rejects equal-time conflicts. Weak identity is diagnosed and withheld rather than guessed from equal counts. Collector reset/source disappearance never retracts server history.

With `sync --all --source-dir`, each harness is read only from its own subdirectory, such as `/path/to/source-root/opencode`; missing subdirectories are skipped. Single-harness sync first looks for a matching harness subdirectory, then falls back to scanning the provided directory directly.

OpenCode sync reads modern SQLite sources named `opencode.db` or `opencode-<channel>.db`, including sessions marked archived in those databases. V1 `message` and V2 `session_message` layouts share the SQLite row ID as message identity; usable V2 data replaces matching V1 data. Pi sync reads JSONL session files from `~/.pi/agent/sessions`, or from a provided Pi source directory; Pi has no harness archive and OS trash is excluded. Codex sync reads rollout JSONL session files from `${CODEX_HOME:-~/.codex}/sessions` and `${CODEX_HOME:-~/.codex}/archived_sessions`, parsing structured `event_msg` token-count records. Claude Code sync reads retained local JSONL transcript files from `${CLAUDE_CONFIG_DIR:-~/.claude}/projects` regardless of UI/server archive state; cloud-only archives are excluded. Successful refreshes persist markers: matching content and current location attribution skip parsing, regardless of source age. Eligible Pi files verify the processed prefix and parse only appended records. Size and mtime alone never prove reuse. `sync --dry-run` previews the same checks without writing; `sync --full-refresh` ignores markers for the requested harness scope without requeueing existing raw facts for canonical rebuild.

Harness discovery runs concurrently. Source preparation uses a worker pool sized by Go's available CPU budget (`GOMAXPROCS`), with at most twice that many outstanding sources. Reads, parsing, and Git inspection run outside write transactions; one writer commits sources in discovery order and batches unchanged-source bookkeeping. Loading status includes content verification, so an unchanged refresh still performs reads. Server startup performs no source reads. New defaults use separate fresh collector/server databases; the legacy mixed file remains untouched.

Token components are additive across harnesses: input excludes cache read/write, and output excludes separately reported reasoning. Pi legacy inclusive-input rows are corrected when their source total proves the old layout. Inconsistent source totals fall back to component sums with diagnostics; non-integer and overflowing counters are rejected.

Codex forks/subagents reuse versioned markers only when their content, location, parser/collector identity, and complete parent chain match. Existing fork sources parse once to establish these markers. Missing, conflicting, cyclic, unreadable, or unstable ancestry falls back to parsing; shared parent parses and verification are cached within the run. Explicit ancestry plus matching turn, provider/model, and complete last/cumulative token metadata identifies replay; rewritten timestamps are not match keys. Verified copies retain the original parent fact identity/time, while uncertain history is retained with diagnostics. Distinct snapshots use deterministic identity fingerprints, with line identity when cumulative identity is absent.

`normalize`

Process pending canonical work from collector raw facts, and journal supported normalized changes in the canonical transaction. After `reset-canonical`, rebuild from requeued raw facts. Publication occurs on a later `sync` or `sync --publish-only`; the server has no normalization command.

Raw provider and model names remain unchanged. Canonical rows used by queries map Pi `openai-codex` to `openai`, map `fireworks-ai` to `fireworks`, and strip `accounts/fireworks/models/` from Fireworks model names. Normal sync and `normalize` also refresh previously stored canonical names.

```sh
tokeninsights normalize
tokeninsights normalize --harness codex
tokeninsights normalize --dry-run
```

`reset-canonical`

Delete collector canonical sessions/messages/usage and normalization diagnostics while retaining raw facts, observations, source continuity, and publication history. Raw facts are requeued for normalization. This does not delete server facts or receipts.

Requires compatible data with no unfinished recovery. It cannot repair incompatible raw token semantics or identities.

```sh
tokeninsights reset-canonical
tokeninsights reset-canonical --confirm
```

`reset-all`

Transactionally recreate collector application tables inside its SQLite file. This clears raw/canonical facts, pending normalization, continuity, journal, and delivery markers. A later sync reparses retained sources under a new stream and server stable IDs dedupe identical facts. Server history and receipts remain untouched. Reset is not an implicit server retraction or reconciliation policy.

```sh
tokeninsights reset-all
tokeninsights reset-all --confirm
```

`view`

Open the interactive terminal UI over REST analytics. Without a server URL it ensures the local server; explicit remote transport needs no local database. Default view, `--no-sync`, query Reload, and filters never collect or normalize. `--sync` performs caller-side all-harness collection/publication before opening; it stops on failure.

```sh
tokeninsights view
tokeninsights view --no-sync              # alias for read-only default
tokeninsights view --sync                 # explicit collection before opening
tokeninsights view --server-url https://example.test
tokeninsights view --today
tokeninsights view --yesterday
tokeninsights view --year --bucket month
tokeninsights view --month --provider openai --model gpt-5
```

Bare `tokeninsights` ensures the background service and prints its URL. Move root viewer flags to `tokeninsights view ...`; `--host`/`--port` are service options only.

Every tab's pinned summary shows `sessions <shown> shown / <synced> synced`, followed by the row count and, except in Context, the filtered token total. `shown` counts distinct sessions matching all active filters across the full result, not just the visible scroll viewport. `synced` counts all distinct sessions with countable canonical usage in this database across all dates and harnesses, ignoring viewer filters. Sessions spanning multiple dates or models are counted once; sessions without countable usage are excluded from both counts.

The default current-month filter can show a small subset of synced sessions. Compare `view --no-sync --month` with `view --no-sync --all-time` using the same `--server-db-path` to inspect date filtering without changing the database. All time removes the preset date restriction but keeps dimension filters and any explicit custom date bounds.

`service start|stop|restart|status|run`

```sh
tokeninsights                         # local query server, print URL; no collection
tokeninsights service start --open
tokeninsights service status --json
tokeninsights service restart
tokeninsights service stop
tokeninsights service run             # foreground; Ctrl+C stops
```

Use `--server-db-path` after the service action. Start/run/restart accept `--host`, `--port`, and `--token` / `TOKENINSIGHTS_SERVER_TOKEN`; start also accepts `--open`. Default binding is `127.0.0.1:8765`; port zero reports the assigned port. Non-loopback binding requires a token. Omitted bind settings reuse saved configuration, and changing a running bind requires restart. Startup never collects or imports legacy storage; incompatible server files are rejected without deletion. Saved configuration contains server settings, not harness roots. `--reload-sources` and `refresh` are removed.

Private lifecycle control uses a Unix socket; public REST handles normalized ingestion and queries only. Held/unreachable ownership and busy ports fail without process takeover. Status is read-only; stopped status exits 3, usage exits 2, other failures exit 1. `serve` is a deprecated alias for `service run`; its old `--no-sync` is obsolete because startup never collects.

`server run`

```sh
tokeninsights server run --server-db-path /path/to/server.sqlite
# Set TOKENINSIGHTS_SERVER_TOKEN before exposing a non-loopback server.
tokeninsights server run --host 0.0.0.0 --port 8765
```

Foreground remote composition shares the canonical store, ingestion core, and query handlers. Non-loopback serving requires an explicit token. Use `--token` or `TOKENINSIGHTS_SERVER_TOKEN`; CLI clients send bearer auth, and browser Basic auth uses the token as its password. Remote provisioning, TLS deployment, account administration, and login/reboot autostart remain later work. The configured owner is currently `default`, not a multi-account service.

The browser provides Tokens, Models, Providers, Harnesses, Sessions, Context, and Repo views, charts, faceted filters, custom dates, session-ID search, sorting, pagination, and themes. Repo groups by repository/directory; location filters apply only there. Unknown groups remain part of totals and can reveal recorded contributing directories. Published directory names are basenames, not collector-local full paths. URL state preserves query scope/navigation. Reporting periods use the server timezone and Monday-start weeks.

Browser **Reload** and TUI `r` query committed data. Empty state points to `tokeninsights sync`; no server endpoint starts a collector. Source-day coverage is absent because unavailable uploads cannot establish checked/empty days. Failed reads preserve filters and available saved results. TUI quits cancel reads, not already committed collection/ingestion transactions.

Producer hostname comes from committed normalized delivery metadata, returning `unknown` without a label and `multiple machines` when labels differ. It is never replaced with the serving machine's hostname. Browser requests remain same-origin, without a destination selector or advertised CORS access. Explicit CLI `--server-url` / `TOKENINSIGHTS_SERVER_URL` selects another server and bypasses local bootstrap.

| Endpoint | Purpose |
| --- | --- |
| `GET /api/v1/instance` | Versions, runtime/database identity, producer labels, reporting timezone, defaults |
| `GET /api/v1/sync` | Read-only readiness and canonical revision; compatibility-named status |
| `GET /api/v1/usage` | Filtered summary, chart, rows, revision, last committed ingestion |
| `GET /api/v1/usage/facets` | Filter facets/session search |
| `GET /api/v1/ingestion/capabilities` | Supported normalized versions/limits and database identity |
| `POST /api/v1/ingestion/batches` | Atomic canonical ingestion with durable committed receipt |

`POST /api/v1/sync` is rejected. Ingestion V1 allows at most 256 entries and 1 MiB per batch, 256-byte strings, and nonnegative integers/aggregate counters within `9007199254740991`. Unsupported versions, private/unknown fields, duplicate JSON keys, invalid identities/totals, and conflicting payloads fail explicitly. Admission is bounded to four concurrent ingestions; busy responses keep pending requests retryable.

[`docs/openapi.yaml`](../../docs/openapi.yaml) is the repository-only REST contract; it is not served at runtime. `pnpm run generate:api` creates committed Go/TypeScript query models and schemas; `pnpm run check-api` checks drift. Native Go builds consume committed output and embedded assets, without Node.

## Database

Default role paths:

| Role | Default file | Override |
| --- | --- | --- |
| Collector | `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/collector.sqlite` | `--collector-db-path`, `TOKENINSIGHTS_COLLECTOR_DB_PATH` |
| Server | `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/server.sqlite` | `--server-db-path`, `TOKENINSIGHTS_SERVER_DB_PATH` |

The former `tokeninsights.sqlite` remains untouched. Retained harness artifacts populate fresh collector/server storage through normal collection and ingestion; no legacy import is implemented. Role checks precede mutation; a collector file cannot serve queries and a server file cannot enter producer recovery/reset. Identical, symlink-equivalent, and existing hard-linked collector/server paths are rejected.

Prefer role flags. `sync`, `normalize`, and collector reset retain `--db-path` as a collector-path alias; `view` retains it as a server-path alias. Service/server commands use `--server-db-path`. Legacy `TOKENINSIGHTS_DB_PATH` does not select either new default.

Collector schema V15/data generation 6 retains metadata-only raw facts, continuity, normalization work, canonical usage, journal snapshots, saved batches, and per-destination acknowledgements. Server schema V1 retains canonical query tables, producer labels, persistent database identity/revision, and durable receipts; it has no harness/source/raw tables. Protocol, identity, and semantics versions are independently validated at ingestion.

Collector recovery is local and depends on retained sources. Server compatibility cannot reset/resync history from source artifacts; incompatible storage is rejected without deletion. Resetting collector canonical state retains publication history; resetting all collector state starts a new stream. Ordinary reupload still dedupes stable fact IDs, and neither action deletes server facts.

Local delivery binds an endpoint plus server database ID, allowing a replacement local server to replay retained journal work under a new binding. Explicit remote destinations refuse silent database-ID changes rather than advance an old cursor against a different dataset. No automatic backflow/retraction is implemented. Authentication tokens and credentials stay outside journal payloads, receipts, and canonical IDs; authenticated endpoint selection does not change fact identity.

## View Arguments

`--no-sync`

Alias for the read-only default. It may start the local query server, but never discovers sources, normalizes, or publishes. An explicit server URL skips local startup.

`--sync`

Collect and publish all harnesses before opening. Conflicts with `--no-sync`. Query filters do not narrow collection.

`--server-url URL`, `--token TOKEN`

Select an existing server and authentication token. Defaults come from `TOKENINSIGHTS_SERVER_URL` and `TOKENINSIGHTS_SERVER_TOKEN`.

`--server-db-path PATH`, `--collector-db-path PATH`

Select local server storage and the collector used only by explicit `--sync`. Default read-only remote view opens neither file.

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

Token columns come from countable rows in `canonical_token_usage`:

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
