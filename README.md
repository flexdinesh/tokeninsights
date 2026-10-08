# TokenInsights

TokenInsights is a token usage dashboard for OpenCode, Pi, Codex, and Claude Code. A Go collector captures sanitized evidence in SQLite. Shared ingestion and processing store token data in DuckDB, either inside one local command or in an authenticated remote container.

The Repo view groups token, model, and provider usage by repository or directory. Location filters apply only there. Missing location data appears as **unknown**; the web view lets you expand an unknown row to see recorded contributing directories when available.

| Web                                                                                  | TUI                                                                       |
| ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| ![TokenInsights browser dashboard with synthetic demo data](assets/tokeninsights-web.png) | ![TokenInsights terminal dashboard with synthetic fixture data](assets/tokeninsights-tui.png) |

## Install

With Homebrew:

```sh
brew install flexdinesh/tap/tokeninsights
```

With Go:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
```

`@latest` installs the latest stable version, published manually from `main`. Use `@v0.1.3` to pin a stable version.

See [release channels and workflow](docs/release.md).

## Run

Single-process mode is the default. No daemon or separate server setup:

```sh
tokeninsights tui                    # collect, ingest, show terminal dashboard
tokeninsights web                    # collect, ingest, serve browser dashboard
tokeninsights web --host 0.0.0.0      # expose read-only dashboard on all IPv4 interfaces
tokeninsights sync                   # collect and ingest without a viewer
tokeninsights tui --sync=false       # query saved usage
tokeninsights web --sync=false
```

`web` remains foreground until terminated; its server stops with it. Default bind is
`127.0.0.1:8765`. With a wildcard bind, the local browser opens a loopback URL. Use
`--open=false` to print the URL without opening a browser. One viewer owns a token
database at a time. Sync requests arriving while a viewer runs are queued locally
and collected/ingested inside that viewer's process. TUI queries and local ingestion
never use HTTP. Browser Reload and TUI `r` only query saved data.

The TUI loading screen shows collection and acceptance progress, then waits for
processing. Failures retain **Retry**, **View saved**, and **Quit**. Local startup
recovers pending processing and unfinished generations before claiming fresh data.

For distributed mode, run one authenticated [server container](docs/deployment.md)
and configure the collector:

```sh
tokeninsights config set mode distributed
tokeninsights config set server-url https://usage.example.com
tokeninsights config set server-token  # secure prompt/stdin; no token argument
tokeninsights sync                     # finite background worker
tokeninsights sync --print             # also submits; stdout is only remote URL
tokeninsights sync --wait              # foreground acceptance for scripts
tokeninsights sync --debug             # capture/acceptance/receipt progress UI
tokeninsights sync status --json
tokeninsights web                      # sync then open remote browser login
```

Background success means the job was saved and its worker started, not that upload
or processing succeeded. Check `sync status`. Workers have a ten-minute bound and
at most three delivery attempts, respecting Retry-After. Later explicit sync replays
retained immutable requests and resumes earlier queued requests for the same
endpoint/credential. Completed job history retains the latest 1,000 terminal jobs; queued/running work is retained.
Jobs never store bearer tokens. Ordinary remote sync supports
an ingest-only token; debug also needs read permission and waits on its own receipts.
Distributed mode has no analytics TUI. Remote failure never starts a local fallback.

Both modes use the same identity, normalization, ambiguity, estimates and deduplication
rules. All four harnesses are collected by default. Plugins invoke `sync --wait`
with their own `--harness`; richer event payloads are not accepted. Useful options:

```sh
tokeninsights sync --harness codex
tokeninsights sync --publish-only       # retry retained evidence without collection
tokeninsights sync --full-refresh       # reread sources/retry quarantine
tokeninsights sync --dry-run            # parse without persistent writes or delivery
```

Equal token counts never establish duplicate identity. Durable native session,
message and request identities govern contributions; all five components remain
atomic. Exact retries return original receipts. Collector deletion, source removal
and interrupted uploads do not erase server history or inflate confirmed totals.
Ambiguous usable counters remain separate estimates; TPS tabs/metrics remain available.

Configuration lives in `${XDG_CONFIG_HOME:-~/.config}/tokeninsights/config.json`,
private and atomically written. Keys: `mode`, `server-url`, `server-token`, `host`,
`port`, `collector-db-path`, `server-db-path`, `app-db-path`. Flags override environment,
then file, then defaults. `TOKENINSIGHTS_MODE`, `TOKENINSIGHTS_ACCESS_TOKEN` and the
role-specific path environment variables are supported. `config get server-token`
only reports whether configured. Legacy hosted configuration maps to distributed.
To return local, remove remote URL/token and set mode `single-process`.

Storage: `collector.sqlite` for source continuity/outbox, `server.duckdb` for token
history/receipts/processing, `app.sqlite` for users/credentials/system state. App and
token databases are paired by identity. Operational sync jobs use
`<canonical-collector-path>.jobs.sqlite`. Defaults live under XDG data home.

Bare invocation prints help. Managed service startup is retired. Before upgrading
an active old service, run `tokeninsights service stop`; stop/status remain migration
commands. Local maintenance uses `data import|reprocess|wait`. Never pair one
`app.sqlite` with another token database or reuse personal history as a hosted account.

## Privacy

Local mode keeps data on your machine. Selecting another server submits whitelisted native evidence, retained indefinitely initially for replay. It stores usage metadata such as token counts, timestamps, models, providers, session identifiers, hashed location keys, and display names. Directory paths use `~/` where a home directory can be identified; otherwise a full directory path may remain in collector storage. It does not store prompts, responses, tool arguments, tool output, source artifact paths, or full Git remote URLs.

Default files under `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/`:

| Role | File | Flag / environment |
| --- | --- | --- |
| Host collector | `collector.sqlite` | `--collector-db-path` / `TOKENINSIGHTS_COLLECTOR_DB_PATH` |
| Token data | `server.duckdb` | `--server-db-path` / `TOKENINSIGHTS_SERVER_DB_PATH` |
| Application | `app.sqlite` | `--app-db-path` / `TOKENINSIGHTS_APP_DB_PATH` |
| Sync jobs | `<collector-path>.jobs.sqlite` | Derived from collector path |

The old `tokeninsights.sqlite` remains untouched. Retained sources rebuild the fresh collector and populate the fresh server through ingestion; verified sibling server.sqlite imports read-only into new default DuckDB. The two files cannot alias one another. Server storage cannot be opened as collector storage or subjected to collector recovery.

Storage uses collector schema 19, DuckDB schema 2, application SQLite schema 1 and job SQLite schema 1. Verified collector schemas 16/17/18 upgrade additively while preserving exact saved requests. A verified DuckDB-1 upgrade stages a personal schema-2 database, checks retained identities, receipts and token components, then publishes it with a recoverable previous copy. Hosted databases start fresh; opening one kind as the other rejects. Incompatible/newer contracts reject without mutation. Current-schema collector data-generation rebuilds remain local; they cannot delete server history.

Rebuild earlier PR databases from retained sources into fresh files. Normalized source times must be valid Unix milliseconds; invalid observations remain server evidence with diagnostics. Filename-derived Pi/Claude sessions remain raw-only until native session evidence exists. Local ingestion runs directly within the owning command; its web listener exposes no ingestion route. Hosted access uses per-user tokens; it does not restore the removed shared-server-token mode.

Raw provider and model values retain the harness names. Stored canonical values used by filters and dashboards map Pi `openai-codex` to `openai`, map `fireworks-ai` to `fireworks`, and shorten Fireworks model names by removing `accounts/fireworks/models/`. Server processing canonicalizes names in each projection.

Server exposes processed metadata to reachable dashboard clients. Default localhost binding keeps the local experience on this machine. Publication sends directory basenames and stable location keys, not collector-local directory paths or private content.

## Documentation

[ADR 0009](docs/adr/0009-single-process-and-distributed-compositions.md) records the approved runtime composition; the [development guide](docs/development.md) defines verification gates.

- [CLI reference](packages/cli/README.md)
- [Development guide](docs/development.md)
- [System boundaries](docs/system.md) and [storage/processing contract](docs/design.md)
- [Docker and hosted deployment](docs/deployment.md)
- [Collector/server architecture](docs/collector-server-architecture.md) — ownership, raw acceptance, storage roles, and recovery guarantees.
- [Collector/ingestion failure tests](docs/collector-ingestion-tests.md) — guarantees, synthetic fixtures, executable coverage, and future acceptance gates.
- [Completion plugins](docs/plugins.md) — thin completion hooks, install artifacts, and host verification scope.


## Evidence, processing and upgrades

DuckDB uses a shared 1 GB memory budget for ingestion, processing and analytics.

Distributed `sync --wait` waits for acceptance; local viewers also wait for processing. Browser Confirmed / Estimated selects separate data;
estimates never inflate confirmed totals. Unusable evidence retains diagnostics.
Fresh default server.duckdb imports verified sibling server.sqlite read-only,
preserving history, identity and receipts. Partial rebuilds preserve unmatched
history. Custom paths, after stopping local service:

    tokeninsights data import --server-db-path NEW.duckdb --legacy-server-db-path OLD.sqlite
    tokeninsights data reprocess
    tokeninsights data wait

Source stays intact. Reprocessing keeps the published generation until complete.
Local data maintenance has a 30-second visibility wait. TUI queries confirmed usage.
CGO/C/C++ toolchain builds embedded DuckDB. Production native archives need no JS.

Application pairing also persists `<canonical-token-path>.application.json`, containing
only the application instance ID. Keep this guard with both databases in stopped
backups. A missing/replaced app database fails closed instead of re-importing stale
legacy credentials. Restore the matched set; do not delete the guard to bypass recovery.
