# TokenInsights

Local token usage dashboard for OpenCode, Pi, Codex, and Claude Code.

| Web | TUI |
| --- | --- |
| ![TokenInsights browser dashboard with synthetic demo data](assets/tokeninsights-web.png) | ![TokenInsights terminal dashboard with synthetic fixture data](assets/tokeninsights-tui.png) |

## Install

macOS/Linux, amd64/arm64. MyGo includes the CLI and Go backend.

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-mygo.sh | sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh # CLI, TUI, web
```

Rerun to update. Latest main build: `install-main.sh` for CLI, or append
`| sh -s -- --channel=main` to a product script. Main has no macOS Intel build.

## Use

```sh
tokeninsights                                 # MyGo desktop
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

## Docs

- [CLI reference](docs/cli.md)
- [Remote connections](docs/remote.md)
- [Development](docs/development.md)
- [User guides](docs-website/README.md)
- [Design](docs/design.md) and [ADRs](docs/adr/)

## License

[MIT](LICENSE)
