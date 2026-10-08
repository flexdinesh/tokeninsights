# tokeninsights

Single-process and distributed token analytics. Shared Go ingestion/processing uses DuckDB for token history and SQLite for accounts/system state. Local TUI and web commands collect before viewing; remote collectors submit over authenticated HTTP.

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

```sh
tokeninsights tui                       # local collect, ingest, process, direct query
tokeninsights web                       # local collect then foreground web server
tokeninsights web --host 0.0.0.0         # read-only dashboard on all IPv4 interfaces
tokeninsights web --port 0 --open=false  # select available port; print URL
tokeninsights tui --sync=false          # saved usage only
tokeninsights web --sync=false
```

Local commands own storage and processing until exit. No daemon or ingestion HTTP.
One viewer owns the token database; concurrent sync/plugin requests are persisted
and executed inside that viewer. If the owner exits, a waiting sync can take over.
Browser Reload/TUI `r` only query. TUI capture failures offer Retry/View saved/Quit.

```sh
tokeninsights config set mode distributed
tokeninsights config set server-url https://usage.example.com
tokeninsights config set server-token   # secure prompt/stdin, never an argument
tokeninsights sync                      # durable job + finite detached worker
tokeninsights sync --print              # also submit; stdout is URL only
tokeninsights sync --wait               # foreground acceptance
tokeninsights sync --debug              # foreground collection and receipt progress
tokeninsights sync status --json
tokeninsights web                       # sync then remote browser login
```

Remote sync requires bearer authentication; no analytics TUI. Debug needs both read
and ingest, waits on accepted receipts, and falls back to plain progress without a
TTY. Default background completion means worker startup, not server acceptance.
Workers stop within ten minutes and retry transient delivery at most three times,
respecting Retry-After. Durable requests survive; another explicit sync can replay
pending uploads and earlier queued requests for the same endpoint/credential.

Both modes default to all harnesses (`opencode`, `pi`, `codex`, `claude-code`).

```sh
tokeninsights sync --harness pi --source-dir /path/to/source-root
tokeninsights sync --all --source-dir /path/to/all-harnesses
tokeninsights sync --publish-only
tokeninsights sync --full-refresh
tokeninsights sync --dry-run
```

`--all --source-dir` uses each harness's subdirectory. Single-harness collection uses
a matching subdirectory when present, otherwise the given directory. Capture commits
sanitized evidence/checkpoints together; source paths and private content never upload.
Unchanged sources skip; full refresh retries quarantine. Publish-only never reads
sources. Dry-run uses temporary storage and never submits. `--no-normalize` remains
a deprecated flag; shared processing always interprets evidence.

Configuration: `mode`, `server-url`, `server-token`, `host`, `port`, `collector-db-path`,
`server-db-path`, `app-db-path`; legacy `server-kind` remains a migration alias.
Flags > environment > private atomic config > defaults. Use `TOKENINSIGHTS_MODE`,
`TOKENINSIGHTS_ACCESS_TOKEN`, `TOKENINSIGHTS_SERVER_URL` and role-path variables.
`config get` reads preferences and masks tokens. `--config-file` or
`TOKENINSIGHTS_CONFIG_PATH` selects a config file. Remote settings never fall back local.

Bare invocation shows help. Old daemon owners must be explicitly stopped with
`service stop`; `service status` remains for migration. Start/restart/run are retired.
Finite local maintenance:

```sh
tokeninsights data import --legacy-server-db-path OLD.sqlite --server-db-path NEW.duckdb
tokeninsights data reprocess
tokeninsights data wait
tokeninsights collector normalize
tokeninsights collector reset-canonical --confirm
tokeninsights collector reset-all --confirm
```

Remote admin uses the running container's private socket; see [deployment](../../docs/deployment.md).
Collector reset never removes token history, and pending raw evidence prevents reset.
Native contribution identity, all token components, estimates, receipts and processing
generations follow [the shared contract](../../docs/design.md).

## Database

Default role paths:

| Role | Default file | Override |
| --- | --- | --- |
| Collector | `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/collector.sqlite` | `--collector-db-path`, `TOKENINSIGHTS_COLLECTOR_DB_PATH` |
| Token data | `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/server.duckdb` | `--server-db-path`, `TOKENINSIGHTS_SERVER_DB_PATH` |

Application SQLite defaults to `app.sqlite` beside the selected token database; override with `--app-db-path` or `TOKENINSIGHTS_APP_DB_PATH`. Finite jobs use `<canonical-collector-path>.jobs.sqlite`.

The former `tokeninsights.sqlite` remains untouched. Retained harness artifacts populate fresh collector/server storage through normal collection and ingestion; verified history import preserves baseline facts and receipts. Role checks precede mutation; a collector file cannot serve queries and a server file cannot enter producer recovery/reset. Identical, symlink-equivalent, and existing hard-linked collector/server paths are rejected.

Use `--collector-db-path` for collection and maintenance, and `--server-db-path` for server storage. `tui` can select both roles. Previous command names, `--db-path`, and `--no-sync` are rejected. Legacy `TOKENINSIGHTS_DB_PATH` does not select either new default.

Collector schema 19 retains raw outbox/continuity/exact delivery state,
dataset/protocol bindings, local quarantine and legacy tables. Verified schemas 16/17/18 upgrade
additively without changing saved request bytes. Application schema 1 stores accounts and provisioning; jobs schema 1 stores finite
invocations/status without tokens. DuckDB schema 2 retains legacy accounts for one-time
migration and stores dataset-scoped raw evidence/receipts/status/typed facts/estimates/retained baselines.
Verified schema 1 upgrades to personal schema 2 through staged read-only copying,
validation and atomic publication with a recoverable previous file. Hosted starts
fresh; stored server-kind mismatch rejects without assigning existing history.
Fresh default DuckDB imports verified sibling server.sqlite read-only; custom
imports require explicit new target. Unsupported contracts reject without reset.
Pending raw prevents collector reset; reset never retracts server data.
Remote database identity stays pinned; deliberate local replacement permits replay.

## TUI Arguments

`--sync`

Collect and publish all harnesses inside the TUI before showing data. Enabled by default; `--sync=false` only queries saved data. Query filters do not narrow collection.

`--server-db-path PATH`, `--collector-db-path PATH`, `--app-db-path PATH`

Select local role files. TUI always runs in single-process mode; remote URLs reject.

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

    tokeninsights data import --server-db-path NEW.duckdb --legacy-server-db-path OLD.sqlite
    tokeninsights web
    tokeninsights data reprocess
    tokeninsights data wait

Foreground personal accepts legacy-server-db-path for a new target. Source stays intact.
Reprocessing keeps published generation until complete. Service wait is explicit
30-second maintenance wait; sync does not wait. TUI queries confirmed usage.
CGO/C/C++ toolchain builds embedded DuckDB. Production native archives need no JS.

Application pairing also persists `<canonical-token-path>.application.json`, containing
only the application instance ID. Keep this guard with both databases in stopped
backups. A missing/replaced app database fails closed instead of re-importing stale
legacy credentials. Restore the matched set; do not delete the guard to bypass recovery.
