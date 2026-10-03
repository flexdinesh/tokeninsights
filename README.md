# TokenInsights

TokenInsights is a local token usage dashboard for OpenCode, Pi, Codex, and Claude Code. It reads durable local session data and presents usage in terminal and browser dashboards.

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

Open the browser dashboard:

The Graphite & Lime layout pairs a compact status header and static usage summary
with short charts, horizontal filters, and dense tables. Dark mode uses graphite
and bright lime; light mode uses warm white and deep lime for readable selections
and chart lines.
Models, Providers, Harnesses, and Repo charts show each group's percentage of the
full filtered token total above its bar and in its tooltip. Dimension filter labels
also show the percentage.

```sh
tokeninsights --open
```

Bare invocation starts the background service or prints its current URL. `--open` explicitly opens the dashboard when possible; SSH/headless sessions skip browser launch. Startup creates an empty database when needed and never syncs.

Service startup creates private directories with mode `0700` and tightens existing service directories owned by you, including state directories from older installs. Symlinks and directories owned by another user are rejected. `service status` leaves permissions unchanged.

Open the terminal dashboard:

```sh
tokeninsights view
```

The terminal dashboard uses a full-width table, filtered token readouts, and a
light/dark Instrument desk theme. Press `f` for the filter drawer or `?` for keys.

The TUI requests refresh on each opening and shows saved usage while it runs. Closing it leaves the service and accepted refresh running. The web reads saved data on opening; its **Refresh** button syncs on demand. `view --no-sync` stays read-only and does not start the service.

```sh
tokeninsights service start
tokeninsights service status
tokeninsights service stop
tokeninsights service restart
tokeninsights service start --host 0.0.0.0 --port 8765
tokeninsights refresh --wait
```

The web/API binds `127.0.0.1:8765` by default. `--host` affects only that listener; the TUI always uses local SQLite and a private Unix socket. Wildcard binding supports remote browsers on trusted networks. Binding and source roots persist across restarts. Use `service restart --reload-sources` to capture changed source environment settings. Login/reboot autostart and authentication are deferred.

Requests made after source capture queue one shared follow-up refresh. Explicit CLI sync/normalize keep their caller's options and sources. An explicit reset cancels pending ordinary refresh, preventing automatic reimport. Unfinished queued requests are lost on a daemon crash; durable pipeline work retains interrupted status. The service performs no periodic ingest.

Sync progress persists across restarts and is shared by CLI, TUI, and web. It distinguishes discovery, source checks, normalization, failures, and interrupted jobs. Daily coverage shows unknown/pending/partial/checked states; missing usage is not presented as zero before sources are checked. Last sync means a successful normalized all-harness refresh. Large JSONL tool-output records no longer hit a fixed 16 MiB Scanner limit, and a failed source does not prevent later sources from syncing. Active files are read to a captured extent; incomplete trailing records wait for the next sync.

Daily source coverage is shared by TUI and web. Unknown days show `—`, confirmed empty days show zero; calendar markers never affect usage totals. The TUI polls committed revisions and offers `u` to retry sync (disabled with `--no-sync`). Ordinary failures retain saved usage.

Daily sync status appears beside dates as compact markers. Web markers expose status and check time on hover and to screen readers; detailed source coverage stays collapsed below results. The TUI keeps days checked on the sync progress line; `?` explains its day markers.

When a new sync starts, previous completed day markers stay blank until the current check confirms them. Saved usage remains visible; pending/updating/failure markers reflect actual source work. `--no-sync` retains saved coverage markers.

The browser also withholds stale empty-day zeros and completion labels in expanded coverage until the current check confirms them. Session search failures offer retry without clearing the search or selected filters. Chart load failures keep navigation, summaries, and tables available, with an explicit page reload to recover.

Repeated syncs verify persisted markers and skip unchanged sources after a successful ingest. Codex fork markers also verify the complete parent chain, avoiding repeated replay parsing. OpenCode checks parser-relevant SQLite rows, so unrelated database writes do not reparse messages. Eligible Pi session files use a verified byte cursor to parse only appended records. Verification still reads source content to detect rewrites; loading progress includes these checks. Discovery and source preparation use multiple cores, with bounded workers and one SQLite writer that batches unchanged-source bookkeeping. Normalization skips identifier refresh when its rule marker is current. Changed sources retain full parsing where incremental replay is unsafe. CLI and schema remain unchanged; existing fork sources establish their new markers on the next sync. The V11–V13-to-V14 metadata upgrade preserves existing usage; older incompatible upgrades rebuild from retained source artifacts.

Common filters:

```sh
tokeninsights view --today
tokeninsights view --week --harness codex
tokeninsights view --provider openai --model gpt-5
tokeninsights view --all-time
```

Supported periods are `--today`, `--yesterday`, `--week`, `--month`, `--year`, and `--all-time`. See the [CLI reference](packages/cli/README.md) for all commands and options.

## Privacy

TokenInsights keeps data on your machine. It stores usage metadata such as token counts, timestamps, models, providers, session identifiers, ingesting-machine hostnames, hashed location keys, and display names. Directory paths use `~/` where a home directory can be identified; otherwise a full directory path may be stored. It does not store prompts, responses, tool arguments, tool output, source artifact paths, or full remote URLs.

The default database is `~/.local/share/tokeninsights/tokeninsights.sqlite`. Override it with `--db-path` or `TOKENINSIGHTS_DB_PATH`.

Raw provider and model values retain the harness names. Stored canonical values used by filters and dashboards map Pi `openai-codex` to `openai`, map `fireworks-ai` to `fireworks`, and shorten Fireworks model names by removing `accounts/fireworks/models/`. Normal sync also updates previously stored canonical names.

The web server exposes usage metadata to clients that can reach it. Its default localhost binding limits access to this machine. Only use `--host` with a trusted network address.

## Documentation

- [CLI reference](packages/cli/README.md)
- [Development guide](docs/development.md)
