# TokenInsights

TokenInsights is a token usage dashboard for OpenCode, Pi, Codex, and Claude Code. A host collector extracts sanitized native evidence into a SQLite outbox and submits it to a DuckDB server for asynchronous processing. Terminal and browser dashboards query that server.

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

Run `tokeninsights tui` for the terminal dashboard or `tokeninsights web` for the browser dashboard. Both sync by default. `sync` collects without opening a viewer; `service` manages the local personal server. The dashboards read the same REST API.

Open the browser dashboard:

The Graphite & Lime layout pairs a compact status header and static usage summary
with short charts, horizontal filters, and dense tables. Dark mode uses graphite
and bright lime; light mode uses pure white, Electric Lime actions (`#b8f500`),
and deeper lime for readable selections and chart lines.
Models, Providers, Harnesses, and Repo charts show each group's percentage of the
full filtered token total above its bar and in its tooltip. Dimension filter labels
also show the percentage.

```sh
tokeninsights web
```

`web` ensures the local personal server, opens its dashboard and syncs, with terminal and browser progress. Use `web --sync=false` to open saved usage or `web --host 0.0.0.0` to bind the local dashboard on another interface. A running service with different bind settings requires `service restart`. Browser launch failure prints the dashboard URL. Bare invocation ensures the selected local server or reports the configured endpoint. Server startup alone never collects harness data.

Collect and publish, then open the terminal dashboard:

```sh
tokeninsights tui
```

`sync` defaults to all four harnesses. Collector SQLite retains whitelisted native evidence, continuity and a durable outbox. Server DuckDB stores accepted evidence and asynchronously processes confirmed facts and separate estimates. Collector never normalizes new usage.

`tui` requires a personal server and collects and publishes all four harnesses inside a loading screen before querying available processed data. It shows per-harness activity and acknowledged upload progress. Failures offer **r Retry**, **v View saved**, and **q Quit**; viewing saved data skips collection. Use `tui --sync=false` for query-only startup. Browser **Reload** and dashboard `r` query saved data; viewer filters never narrow collection.

```sh
tokeninsights sync --harness codex
tokeninsights sync --publish-only           # retry retained uploads, no source discovery
tokeninsights tui --sync=false             # query saved usage without collection
tokeninsights web --sync=false             # open saved usage without collection
tokeninsights service start
tokeninsights service status
tokeninsights service stop
tokeninsights service restart
```

Collection and delivery report separate outcomes. If delivery fails, locally captured evidence and pending batches remain for the next manual `sync` or `sync --publish-only`. There is no automatic background retry. Acknowledgement means evidence, receipt and processing work committed. Processing is asynchronous; acceptance does not imply query visibility. Lost acknowledgements replay the saved request; reconstructed stable fact IDs also dedupe uploads after collector storage is deleted and rebuilt.

Equal token counts never establish duplicate identity. Source-native session/message/request identities distinguish facts; mutable token values are payloads. Claude's supported source-timestamp revisions replace one native request contribution; unproven or equal-revision conflicts become durable ambiguity. Usable ambiguous counters appear separately as estimated; unusable evidence remains queryable for debugging. Source disappearance or collector reset does not delete server history.

Location enrichment carries hashed keys, display labels and explicit provenance. Source-native contribution identity governs deduplication; location labels and equal counters never establish that two observations represent distinct consumption.

The local web/API binds `127.0.0.1:8765` by default. The TUI can also query a remote personal server:

```sh
tokeninsights tui --server-url https://example.test
tokeninsights sync --server-url https://example.test
```

Client preferences live in `${XDG_CONFIG_HOME:-~/.config}/tokeninsights/config.json`.
Configure a remote destination once; sync, TUI and completion plugins use it:

```sh
tokeninsights config set server-url http://remote-machine:8765
tokeninsights config get server-url
tokeninsights sync
tokeninsights tui
tokeninsights config remove server-url  # return to managed local
```

On the remote machine, explicitly run the separate server:

```sh
tokeninsights-server --listen 0.0.0.0:8765 --server-db-path /var/lib/tokeninsights/server.duckdb
```

A **personal server** is unauthenticated and serves one dataset. It may be the managed local service or a foreground server on another machine. A **hosted server** authenticates multiple users and keeps their datasets isolated in one shared DuckDB database. Both use the same evidence processing and analytics rules; server startup never collects.

For a hosted destination, configure the client once. `config set server-token` reads the token securely rather than taking it as a command-line argument:

```sh
tokeninsights config set server-kind hosted
tokeninsights config set server-url https://usage.example.com
tokeninsights config set server-token
tokeninsights sync
tokeninsights web
```

`web` syncs with terminal progress before opening a hosted dashboard. The browser requires token login and shows processing state, without collector progress. `tui` rejects hosted destinations even with `--sync=false`. Clients verify the configured kind and required server capabilities before collection; remote failure never starts a local fallback. Token rotation preserves delivery progress; switching users creates a separate dataset binding.

The hosted executable runs with `--kind hosted --public-url https://usage.example.com`. Administrators create users and read/ingest tokens through a private Unix socket. See [Docker and hosted deployment](docs/deployment.md) for provisioning, TLS and persistence. `tokeninsights server run`, `--token` and `TOKENINSIGHTS_SERVER_TOKEN` remain removed.

```sh
tokeninsights config set host 0.0.0.0
tokeninsights config set port 8765
tokeninsights service restart
```

Config keys: `server-kind`, `server-url`, `server-token`, `host`, `port`, `collector-db-path`, `server-db-path`. Kind defaults to `personal`. Tokens are stored in the private config file; `config get server-token` reports whether one is configured without exposing it. `TOKENINSIGHTS_SERVER_KIND` and `TOKENINSIGHTS_ACCESS_TOKEN` override kind and token.
Precedence: flags > environment > file > defaults. `get` reads stored preferences
or defaults, not environment overrides. `--config-file PATH` or
`TOKENINSIGHTS_CONFIG_PATH` selects a file. An explicitly empty `--server-url` /
`TOKENINSIGHTS_SERVER_URL` selects local. Changing bind settings requires restart.
See [system design](docs/system.md).

Service state/discovery directories remain private. Client configuration selects routing/storage/bind preferences; private runtime records describe effective local service state, not harness roots; `--reload-sources` and the old `refresh` command are removed. Server startup and viewer reconnection never collect; TUI startup runs sync unless `--sync=false` is set.

Capture uses up to four source readers with one SQLite writer. Deterministic record-limit failures are quarantined locally: unchanged files are skipped on later syncs, but collection remains incomplete. File changes, a parser policy update, or `sync --full-refresh` retry them. Quarantine preserves prior evidence and checkpoints; it never marks missing usage as captured. Oversized records proven irrelevant are streamed past without retaining their contents.

Repeated collection verifies persisted continuity and skips unchanged sources. Eligible Pi files parse verified appended records; Codex resumes native context; server resolves ancestry. OpenCode scans snapshots to capture revised rows. Changed sources fall back to full parsing where needed. Active JSONL files are read to a captured extent, and incomplete trailing records wait for a later sync. Source diagnostics remain local. Managed personal clients publish bounded, sanitized progress over the private control socket for the personal dashboard; hosted browsers never request it. The server displays available usage without claiming that missing uploads prove empty or checked days.

The terminal dashboard uses a full-width table, filtered token readouts, and a light/dark Instrument desk theme. Press `f` for filters or `?` for keys. Server ingestion failures retain saved usage; failed reads offer Reload while preserving filters.

Common filters:

```sh
tokeninsights tui --today
tokeninsights tui --week --harness codex
tokeninsights tui --provider openai --model gpt-5
tokeninsights tui --all-time
```

Supported periods are `--today`, `--yesterday`, `--week`, `--month`, `--year`, and `--all-time`. See the [CLI reference](packages/cli/README.md) for all commands and options.

Advanced host maintenance lives under `tokeninsights collector normalize|reset-canonical|reset-all`. Previous command names and flags are removed; use the role-specific commands and database flags.

## Privacy

Local mode keeps data on your machine. Selecting another server submits whitelisted native evidence, retained indefinitely initially for replay. It stores usage metadata such as token counts, timestamps, models, providers, session identifiers, hashed location keys, and display names. Directory paths use `~/` where a home directory can be identified; otherwise a full directory path may remain in collector storage. It does not store prompts, responses, tool arguments, tool output, source artifact paths, or full Git remote URLs.

Default files under `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/`:

| Role | File | Flag / environment |
| --- | --- | --- |
| Host collector | `collector.sqlite` | `--collector-db-path` / `TOKENINSIGHTS_COLLECTOR_DB_PATH` |
| Canonical server | `server.duckdb` | `--server-db-path` / `TOKENINSIGHTS_SERVER_DB_PATH` |

The old `tokeninsights.sqlite` remains untouched. Retained sources rebuild the fresh collector and populate the fresh server through ingestion; verified sibling server.sqlite imports read-only into new default DuckDB. The two files cannot alias one another. Server storage cannot be opened as collector storage or subjected to collector recovery.

Storage uses collector schema 19 and DuckDB schema 2. Verified collector schemas 16/17/18 upgrade additively while preserving exact saved requests. A verified DuckDB-1 upgrade stages a personal schema-2 database, checks retained identities, receipts and token components, then publishes it with a recoverable previous copy. Hosted databases start fresh; opening one kind as the other rejects. Incompatible/newer contracts reject without mutation. Current-schema collector data-generation rebuilds remain local; they cannot delete server history.

Rebuild earlier PR databases from retained sources into fresh files. Normalized source times must be valid Unix milliseconds; invalid observations remain server evidence with diagnostics. Filename-derived Pi/Claude sessions remain raw-only until native session evidence exists. Personal server public writes and administration remain private to the owner transport. Hosted access uses per-user tokens; it does not restore the removed shared-server-token mode.

Raw provider and model values retain the harness names. Stored canonical values used by filters and dashboards map Pi `openai-codex` to `openai`, map `fireworks-ai` to `fireworks`, and shorten Fireworks model names by removing `accounts/fireworks/models/`. Server processing canonicalizes names in each projection.

Server exposes processed metadata to reachable dashboard clients. Default localhost binding keeps the local experience on this machine. Publication sends directory basenames and stable location keys, not collector-local directory paths or private content.

## Documentation

- [CLI reference](packages/cli/README.md)
- [Development guide](docs/development.md)
- [System boundaries](docs/system.md) and [storage/processing contract](docs/design.md)
- [Docker and hosted deployment](docs/deployment.md)
- [Collector/server architecture](docs/collector-server-architecture.md) — ownership, raw acceptance, storage roles, and recovery guarantees.
- [Collector/ingestion failure tests](docs/collector-ingestion-tests.md) — guarantees, synthetic fixtures, executable coverage, and future acceptance gates.
- [Completion plugins](docs/plugins.md) — thin completion hooks, install artifacts, and host verification scope.


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

Foreground personal servers accept legacy-server-db-path for a new target. Source stays intact.
Reprocessing keeps published generation until complete. Service wait is explicit
30-second maintenance wait; sync does not wait. TUI queries confirmed usage.
CGO/C/C++ toolchain builds embedded DuckDB. Production native archives need no JS.
