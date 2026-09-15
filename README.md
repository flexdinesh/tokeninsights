# TokenInsights

TokenInsights is a local token usage dashboard for OpenCode, Pi, Codex, and Claude Code. It reads durable local session data and presents usage in terminal and browser dashboards.

| Web                                                                                  | TUI                                                                       |
| ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| ![TokenInsights browser dashboard in light mode](assets/tokeninsights-web-light.png) | ![TokenInsights terminal dashboard](assets/tokeninsights-view-models.png) |

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

```sh
tokeninsights serve
```

Open the terminal dashboard:

```sh
tokeninsights
```

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

TokenInsights keeps data on your machine. It stores usage metadata such as token counts, timestamps, models, providers, and session identifiers—not prompts, responses, tool arguments, or tool output.

The default database is `~/.local/share/tokeninsights/tokeninsights.sqlite`. Override it with `--db-path` or `TOKENINSIGHTS_DB_PATH`.

The web server exposes usage metadata to clients that can reach it. Its default localhost binding limits access to this machine. Only use `--host` with a trusted network address.

## Documentation

- [CLI reference](packages/cli/README.md)
- [Development guide](docs/development.md)
