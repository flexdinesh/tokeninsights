# TokenInsights

TokenInsights is a local token usage dashboard for OpenCode, Pi, Codex, and Claude Code. A host collector reads durable session data, normalizes it locally, and publishes canonical facts to a SQLite server. Terminal and browser dashboards query that server.

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

`service start` starts the local query/ingestion service or prints its URL. `--open` opens the dashboard when possible; SSH/headless sessions skip browser launch. Bare invocation also ensures the local server. Startup creates an empty server database when needed and never collects harness data.

Collect and publish, then open the terminal dashboard:

```sh
tokeninsights sync
tokeninsights tui
```

`sync` defaults to all four harnesses. The collector retains metadata-only raw facts, normalized usage, source continuity, and a durable publication journal in its own SQLite database. Only normalized facts and their session/message references cross ingestion. The server never reads harness files or runs parsers.

Both dashboards read committed server data. Browser **Reload** and TUI `r` reload queries; neither starts collection. `tui --sync` explicitly collects and publishes before opening. Viewer filters select saved results, not which harnesses get collected.

```sh
tokeninsights sync --harness codex
tokeninsights sync --publish-only           # retry retained uploads, no source discovery
tokeninsights tui --sync
tokeninsights service start
tokeninsights service status
tokeninsights service stop
tokeninsights service restart
```

Collection and delivery report separate outcomes. If delivery fails, locally committed facts and pending batches remain for the next manual `sync` or `sync --publish-only`. There is no automatic background retry. An acknowledgement means the batch's facts and receipt committed together and are queryable. Lost acknowledgements replay the saved request; reconstructed stable fact IDs also dedupe uploads after collector storage is deleted and rebuilt.

Equal token counts never establish duplicate identity. Source-native session/message/request identities distinguish facts; mutable token values are payloads. Claude's supported source-timestamp revisions replace one native request contribution; unproven or equal-revision conflicts fail explicitly. Missing stable evidence is withheld from publication with diagnostics. Source disappearance or collector reset does not delete server history.

Harnesses can publish the same location with different repository provenance. When keys and display names match, the server keeps the strongest evidence (`harness`, `git-remote`, `git-common-dir`, then `opencode-project`) without changing token counts. Pending batches blocked by a provenance-only `reference_conflict` can resume with `sync --publish-only` after upgrading and restarting the server.

The local web/API binds `127.0.0.1:8765` by default. The TUI uses the same REST queries as the browser, including when it connects directly to another server:

```sh
tokeninsights tui --server-url https://example.test
tokeninsights sync --server-url https://example.test
```

An explicit server URL skips local startup. `tokeninsights server run` provides the shared foreground server composition. Non-loopback serving requires a token; CLI clients use `--token` or `TOKENINSIGHTS_SERVER_TOKEN`, and browser authentication uses the same token as its Basic-auth password. Remote provisioning, TLS deployment, account management, and login/reboot autostart remain later work.

Service state/discovery directories remain private. Saved configuration contains server settings, not harness roots; `--reload-sources` and the old `refresh` command are removed. Neither startup nor viewer reconnection can collect local sources.

Repeated collection verifies persisted continuity and skips unchanged sources. Eligible Pi files parse verified appended records; Codex replay verifies complete ancestry; OpenCode fingerprints parser-relevant SQLite rows. Changed sources fall back to full parsing where needed. Active JSONL files are read to a captured extent, and incomplete trailing records wait for a later sync. Collector progress and source diagnostics remain local; the server displays available usage without claiming that missing uploads prove empty or checked days.

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

Local mode keeps data on your machine. Selecting another server publishes normalized usage metadata to that destination. It stores usage metadata such as token counts, timestamps, models, providers, session identifiers, ingesting-machine hostnames, hashed location keys, and display names. Directory paths use `~/` where a home directory can be identified; otherwise a full directory path may remain in collector storage. It does not store prompts, responses, tool arguments, tool output, source artifact paths, or full Git remote URLs.

Default files under `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/`:

| Role | File | Flag / environment |
| --- | --- | --- |
| Host collector | `collector.sqlite` | `--collector-db-path` / `TOKENINSIGHTS_COLLECTOR_DB_PATH` |
| Canonical server | `server.sqlite` | `--server-db-path` / `TOKENINSIGHTS_SERVER_DB_PATH` |

The old `tokeninsights.sqlite` remains untouched. Retained sources rebuild the fresh collector and populate the fresh server through ingestion; legacy import is outside this change. The two files cannot alias one another. Server storage cannot be opened as collector storage or subjected to collector recovery.

Storage accepts only the current collector schema 16 and server schema 2. Previous schemas are rejected without mutation. Current-schema collector data-generation rebuilds remain local; they cannot delete server history.

Rebuild earlier PR databases from retained sources into fresh files. Normalized source times must be valid Unix milliseconds; invalid observations remain collector-local diagnostics. Filename-derived Pi/Claude sessions remain raw-only until native session evidence exists. Changing a running service token requires `service restart`.

Raw provider and model values retain the harness names. Stored canonical values used by filters and dashboards map Pi `openai-codex` to `openai`, map `fireworks-ai` to `fireworks`, and shorten Fireworks model names by removing `accounts/fireworks/models/`. Normal sync also updates previously stored canonical names.

The server exposes normalized usage metadata to its authorized clients. Default localhost binding keeps the local experience on this machine. Publication sends directory basenames and stable location keys, not collector-local directory paths or raw facts.

## Documentation

- [CLI reference](packages/cli/README.md)
- [Development guide](docs/development.md)
- [Collector/server architecture](docs/collector-server-architecture.md) — ownership, normalized publication, storage roles, and recovery guarantees.
- [Collector/ingestion failure tests](docs/collector-ingestion-tests.md) — guarantees, synthetic fixtures, executable coverage, and future acceptance gates.
- [Completion plugins](docs/plugins.md) — thin completion hooks, install artifacts, and host verification scope.
