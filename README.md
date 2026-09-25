# TokenInsights

TokenInsights is a local token usage dashboard for OpenCode, Pi, Codex, and Claude Code. It reads durable local session data and presents usage in terminal and browser dashboards.

The Repo view groups token, model, and provider usage by repository or directory. Location filters apply only there. Missing location data appears as **unknown**; the web view lets you expand an unknown row to see recorded contributing directories when available.

| Web                                                                                  | TUI                                                                       |
| ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| ![TokenInsights browser dashboard with synthetic demo data](assets/tokeninsights-web-light.png) | ![TokenInsights terminal dashboard with synthetic fixture data](assets/tokeninsights-view-models.png) |

## Install

With Homebrew:

```sh
brew install flexdinesh/tap/tokeninsights
```

With Go:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
```

## Run

Open the browser dashboard:

The Signal Studio layout pairs a compact static usage summary with a wide chart,
sky-and-slate light/dark themes, and filters in a desktop side rail or compact
mobile disclosure.

```sh
tokeninsights serve
```

Open the terminal dashboard:

```sh
tokeninsights
```

The terminal dashboard uses a full-width table, filtered token readouts, and a
light/dark Instrument desk theme. Press `f` for the filter drawer or `?` for keys.

Both refresh supported local sources on startup. The browser server listens at `http://localhost:8765` by default. Use `--host <ipv4>` to bind another interface or `--port <port>` to choose another port.

Common filters:

```sh
tokeninsights view --today
tokeninsights view --week --harness codex
tokeninsights view --provider openai --model gpt-5
tokeninsights view --all-time
```

Supported periods are `--today`, `--yesterday`, `--week`, `--month`, `--year`, and `--all-time`. See the [CLI reference](packages/cli/README.md) for all commands and options.

## Privacy

TokenInsights keeps data on your machine. It stores usage metadata such as token counts, timestamps, models, providers, session identifiers, hashed location keys, and display names. Directory paths use `~/` where a home directory can be identified; otherwise a full directory path may be stored. It does not store prompts, responses, tool arguments, tool output, source artifact paths, or full remote URLs.

The default database is `~/.local/share/tokeninsights/tokeninsights.sqlite`. Override it with `--db-path` or `TOKENINSIGHTS_DB_PATH`.

Raw provider and model values retain the harness names. Stored canonical values used by filters and dashboards map Pi `openai-codex` to `openai` and shorten Fireworks model names by removing `accounts/fireworks/models/`. Normal sync also updates previously stored canonical names.

The web server exposes usage metadata to clients that can reach it. Its default localhost binding limits access to this machine. Only use `--host` with a trusted network address.

## Documentation

- [CLI reference](packages/cli/README.md)
- [Development guide](docs/development.md)
