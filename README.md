# <picture><source media="(prefers-color-scheme: dark)" srcset="assets/brand/lockup-dark.svg"><img src="assets/brand/lockup-light.svg" alt="TokenInsights" height="40"></picture>

Local token usage dashboard for OpenCode, Pi, Codex, and Claude Code.

| Web | TUI |
| --- | --- |
| ![TokenInsights browser dashboard with synthetic demo data](assets/tokeninsights-web.png) | ![TokenInsights terminal dashboard with synthetic fixture data](assets/tokeninsights-tui.png) |

## Install

macOS/Linux, amd64/arm64. Installs the desktop app, CLI and Go backend, and puts the
CLI on PATH.

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh -s -- --cli-only # servers without a display
```

Rerun to update, repeating `--cli-only` on servers. Latest main build: `install-main.sh`.
Main has no macOS Intel build. `--no-modify-path` leaves shell profiles untouched.
Updates recognize installer-owned MyGo CLI links from before the desktop rename,
without deleting the old product files.

## Use

```sh
tokeninsights                                 # desktop
tokeninsights tui
tokeninsights web
tokeninsights connect ssh://you@devbox
tokeninsights tui --connection ssh://you@devbox
tokeninsights remote                          # temporary TLS gateway; prints connect command
```

Viewers share one SQLite backend on the selected machine. Launch captures usage;
explicit **Refresh** captures again. Filters and query reload read saved usage.
There is no periodic scanning. `--capture=false` skips launch capture while pending
processing resumes. An unavailable desktop returns an actionable error.

SSH can start the backend on the target. TCP requires a running `remote` gateway;
its printed connection string expires when the gateway stops. Connections are
explicit and never fall back to another machine's data.
Missing SSH commands report remote diagnostics and guidance to set `remote.command`.

## Docs

- [CLI reference](docs/cli.md)
- [Remote connections](docs/remote.md)
- [Development](docs/development.md)
- [User guides](docs-website/README.md)
- [Design](docs/design.md) and [ADRs](docs/adr/)

## License

[MIT](LICENSE)
