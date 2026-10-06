# TokenInsights

TokenInsights is a local token usage dashboard for OpenCode, Pi, Codex, and Claude Code. A host collector extracts sanitized native evidence into a SQLite outbox and submits it to a DuckDB server for asynchronous processing. Terminal and browser dashboards query that server.

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

Use `service` for the local server, `sync` for manual collection, and `tui` for the terminal dashboard. The TUI and browser read the same REST API.

Open the browser dashboard:

The Graphite & Lime layout pairs a compact status header and static usage summary
with short charts, horizontal filters, and dense tables. Dark mode uses graphite
and bright lime; light mode uses pure white, Electric Lime actions (`#b8f500`),
and deeper lime for readable selections and chart lines.
Models, Providers, Harnesses, and Repo charts show each group's percentage of the
full filtered token total above its bar and in its tooltip. Dimension filter labels
also show the percentage.

```sh
tokeninsights service start --open
```

`service start` starts the local query/ingestion service or prints its URL. `--open` opens the dashboard when possible; SSH/headless sessions skip browser launch. Bare invocation ensures the selected local server or prints the configured remote endpoint without starting local. Startup creates an empty server database when needed and never collects harness data.

Collect and publish, then open the terminal dashboard:

```sh
tokeninsights sync
tokeninsights tui
```

`sync` defaults to all four harnesses. Collector SQLite retains whitelisted native evidence, continuity and a durable outbox. Server DuckDB stores accepted evidence and asynchronously processes confirmed facts and separate estimates. Collector never normalizes new usage.

`tui` collects and publishes all four harnesses inside a loading screen before querying available processed data. It shows per-harness activity and acknowledged upload progress. Failures offer **r Retry**, **v View saved**, and **q Quit**; viewing saved data skips collection. Use `tui --sync=false` for query-only startup. Browser **Reload** and dashboard `r` query saved data; viewer filters never narrow collection.

```sh
tokeninsights sync --harness codex
tokeninsights sync --publish-only           # retry retained uploads, no source discovery
tokeninsights tui --sync=false             # query saved usage without collection
tokeninsights service start
tokeninsights service status
tokeninsights service stop
tokeninsights service restart
```

Collection and delivery report separate outcomes. If delivery fails, locally captured evidence and pending batches remain for the next manual `sync` or `sync --publish-only`. There is no automatic background retry. Acknowledgement means evidence, receipt and processing work committed. Processing is asynchronous; acceptance does not imply query visibility. Lost acknowledgements replay the saved request; reconstructed stable fact IDs also dedupe uploads after collector storage is deleted and rebuilt.

Equal token counts never establish duplicate identity. Source-native session/message/request identities distinguish facts; mutable token values are payloads. Claude's supported source-timestamp revisions replace one native request contribution; unproven or equal-revision conflicts become durable ambiguity. Usable ambiguous counters appear separately as estimated; unusable evidence remains queryable for debugging. Source disappearance or collector reset does not delete server history.

Location enrichment carries hashed keys, display labels and explicit provenance. Source-native contribution identity governs deduplication; location labels and equal counters never establish that two observations represent distinct consumption.

The local web/API binds `127.0.0.1:8765` by default. The TUI uses the same REST queries as the browser, including when it connects directly to another server:

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

Both deployments share ingestion/query code. Remote collects nothing and accepts
many clients into one shared dataset. Both public servers are unauthenticated;
auth, accounts, alternate storage/queues and deployment provisioning remain future
work. `tokeninsights server run`, `--token` and `TOKENINSIGHTS_SERVER_TOKEN` are
removed. Remote failure retains local publication work without falling back.

```sh
tokeninsights config set host 0.0.0.0
tokeninsights config set port 8765
tokeninsights service restart
```

Config keys: `server-url`, `host`, `port`, `collector-db-path`, `server-db-path`.
Precedence: flags > environment > file > defaults. `get` reads stored preferences
or defaults, not environment overrides. `--config-file PATH` or
`TOKENINSIGHTS_CONFIG_PATH` selects a file. An explicitly empty `--server-url` /
`TOKENINSIGHTS_SERVER_URL` selects local. Changing bind settings requires restart.
See [system design](docs/system.md).

Service state/discovery directories remain private. Client configuration selects routing/storage/bind preferences; private runtime records describe effective local service state, not harness roots; `--reload-sources` and the old `refresh` command are removed. Server startup and viewer reconnection never collect; TUI startup runs sync unless `--sync=false` is set.

Repeated collection verifies persisted continuity and skips unchanged sources. Eligible Pi files parse verified appended records; Codex resumes native context; server resolves ancestry. OpenCode scans snapshots to capture revised rows. Changed sources fall back to full parsing where needed. Active JSONL files are read to a captured extent, and incomplete trailing records wait for a later sync. Collector progress and source diagnostics remain local; the server displays available usage without claiming that missing uploads prove empty or checked days.

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

Storage uses collector schema 17 and DuckDB schema 1. Verified schema 16 upgrades additively; incompatible contracts reject without mutation. Current-schema collector data-generation rebuilds remain local; they cannot delete server history.

Rebuild earlier PR databases from retained sources into fresh files. Normalized source times must be valid Unix milliseconds; invalid observations remain server evidence with diagnostics. Filename-derived Pi/Claude sessions remain raw-only until native session evidence exists. Existing token-protected services require explicit `service restart` to run without public auth; database identity and receipts survive. Restart imports legacy bind preferences into the client config when unset.

Raw provider and model values retain the harness names. Stored canonical values used by filters and dashboards map Pi `openai-codex` to `openai`, map `fireworks-ai` to `fireworks`, and shorten Fireworks model names by removing `accounts/fireworks/models/`. Server processing canonicalizes names in each projection.

Server exposes processed metadata to reachable dashboard clients. Default localhost binding keeps the local experience on this machine. Publication sends directory basenames and stable location keys, not collector-local directory paths or private content.

## Documentation

- [CLI reference](packages/cli/README.md)
- [Development guide](docs/development.md)
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

Remote accepts legacy-server-db-path for new target. Source stays intact.
Reprocessing keeps published generation until complete. Service wait is explicit
30-second maintenance wait; sync does not wait. TUI queries confirmed usage.
CGO/C/C++ toolchain builds embedded DuckDB. Production native archives need no JS.
