# Development

Run from repo root. `mise tasks ls` lists every task. User commands: [CLI](cli.md).

## Install from main

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-main.sh | sh                                # CLI
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-mygo.sh | sh -s -- --channel=main          # MyGo
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-electron.sh | sh -s -- --channel=main      # Electron
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-server.sh | sh -s -- --channel=main        # server
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install.sh | sh                                    # back to stable
fish_add_path ~/.local/bin                     # if not on PATH
```

## Run main build

Main replaces stable at `~/.local/bin/tokeninsights`; same commands as [CLI](cli.md).

```sh
tokeninsights --version                        # confirm main build
cat ~/.local/share/tokeninsights/products/cli/.channel  # stable|main
tokeninsights tui
tokeninsights tui --sync=false
tokeninsights web
tokeninsights web --open=mygo                  # MyGo installed from main
tokeninsights web --open=electron              # Electron installed from main
tokeninsights web --open=browser
tokeninsights tui --endpoint ssh://you@devbox  # remote needs matching protocol version
tokeninsights web --endpoint ssh://you@devbox
tokeninsights sync --wait
tokeninsights-server --version                 # server installed from main
```

## Setup

```sh
git clone git@github.com:flexdinesh/tokeninsights.git && cd tokeninsights
mise trust && mise install                     # pinned Go/Node/pnpm (mise 2026.10.18)
mise run setup                                 # deps + Husky hooks
mise run setup:browser                         # Chromium for e2e
```

## Dev loop (fixture data)

```sh
mise run dev:data                              # recreate synthetic fixture DBs in .tokeninsights-dev/
mise run dev                                   # Go API + hot-reload web; prints both URLs
mise run dev -- --api-port 8765 --web-port 5173  # explicit ports; 0 = auto
mise run dev:cli                               # TUI on fixture data
mise run dev:server -- --api-port 0            # Go API only
TOKENINSIGHTS_DEV_API_URL=http://127.0.0.1:PORT mise run dev:web -- --port 0  # web against that API
mise run dev:web:mock                          # web with synthetic API, no Go
mise run dev:docs                              # docs site at http://localhost:5173/
```

## Build and run local binary

```sh
mise run build:cli                             # packages/cli/bin/{tokeninsights,tokeninsights-backend}, packages/server/bin/tokeninsights-server
./packages/cli/bin/tokeninsights tui
./packages/cli/bin/tokeninsights tui --sync=false --all-time
./packages/cli/bin/tokeninsights web
./packages/cli/bin/tokeninsights web --port 0 --open=false
./packages/cli/bin/tokeninsights web --open=browser
./packages/cli/bin/tokeninsights web --open=mygo      # needs MyGo installed
./packages/cli/bin/tokeninsights web --open=electron  # needs Electron installed
mise run start:web                             # build:cli + web
```

## SSH endpoint

```sh
./packages/cli/bin/tokeninsights tui --endpoint ssh://you@devbox
./packages/cli/bin/tokeninsights web --endpoint ssh://you@devbox
./packages/cli/bin/tokeninsights data wait --endpoint ssh://you@devbox
./packages/cli/bin/tokeninsights config set remote.command /home/you/.local/bin/tokeninsights  # remote binary path
tokeninsights-mygo --endpoint=ssh://you@devbox # installed MyGo, Linux
open -a "TokenInsights MyGo" --args --endpoint=ssh://you@devbox  # installed MyGo, macOS
```

## Desktop experiments

```sh
mise run build:desktop                         # dist/tokeninsights-{mygo,electron}_<os>_<arch>.tar.gz
tar -xzf dist/tokeninsights-mygo_linux_amd64.tar.gz -C /tmp && /tmp/tokeninsights-mygo_linux_amd64/tokeninsights-mygo
tar -xzf dist/tokeninsights-electron_linux_amd64.tar.gz -C /tmp && /tmp/tokeninsights-electron_linux_amd64/tokeninsights-electron
pnpm --filter @tokeninsights/desktop-electron run install:runtime  # pinned Electron runtime
TOKENINSIGHTS_NATIVE_CAPTURE_DIR=/tmp/mygo-shots go test ./packages/desktop-mygo/...  # MyGo PNG captures
mise run check:desktop                         # installed Electron GUI check, sandboxed, on host
```

## Remote server

```sh
mkdir -p /tmp/ti-server && TOKENINSIGHTS_BOOTSTRAP_USERNAME=alice TOKENINSIGHTS_BOOTSTRAP_PASSWORD=change-me-please-123 \
  ./packages/server/bin/tokeninsights-server --listen 127.0.0.1:8766 --server-db-path /tmp/ti-server/server.sqlite --admin-socket /tmp/ti-server/admin.sock
./packages/server/bin/tokeninsights-server admin --admin-socket /tmp/ti-server/admin.sock token create USER_ID --scopes read,ingest
TOKENINSIGHTS_BOOTSTRAP_USERNAME=alice TOKENINSIGHTS_BOOTSTRAP_PASSWORD=... docker compose up -d --build  # SQLite image on 127.0.0.1:8766
docker compose -f deploy/compose.postgres.yaml up -d --build                     # PostgreSQL
docker compose exec tokeninsights tokeninsights-server admin token create USER_ID --scopes read,ingest
```

Client against it, isolated config:

```sh
C="--config-file /tmp/ti-server/client.json"
./packages/cli/bin/tokeninsights $C config set distributed.server-url http://127.0.0.1:8766
./packages/cli/bin/tokeninsights $C config set distributed.server-token
./packages/cli/bin/tokeninsights $C sync --wait
./packages/cli/bin/tokeninsights $C sync --debug
./packages/cli/bin/tokeninsights $C sync status
./packages/cli/bin/tokeninsights $C browse
```

## Format, lint, test

```sh
mise run format
mise run lint
mise run test                                  # all, via Dagger, live PostgreSQL
mise run test:js
mise run test:go
mise run test:race
mise run test:web-e2e
mise run test:container                        # after Dockerfile/deploy changes
mise run check-schema
mise run check-api
mise run generate:api
mise run build:web                             # needed before direct go build/test
mise run build                                 # all binaries + plugins
mise run bench:smoke
mise run bench:storage
```

## Validate before PR

Commit first; tracked tree must be clean.

```sh
mise run check:pr                              # full suite on HEAD
mise run check:pr -- --group go                # format|static|web|tooling|go|benchmarks|distribution|browser|deployment
mise run check:pr -- --force                   # ignore cached receipts
mise run check:pr -- --setup-engine            # create missing managed Dagger engine
mise run check:status                          # receipt status, no engine
mise run check:cleanup                         # prune old run artifacts
mise run check:desktop                         # also required before PR
```

Artifacts: `<git-common-dir>/tokeninsights-validation/v2/runs/<run-id>/`. Engine: `tokeninsights-validation-engine-v0.21.11`; never prune or restart it.

## Direct Go

```sh
mise run build:web
cd packages/cli && go test ./... && go build -o bin/tokeninsights ./cmd/tokeninsights
```

## Toolchain bump

Edit `mise.toml`, `mise lock`, then match Dockerfile images/pnpm, root `package.json` `packageManager`, `go.mod`; `mise run check:toolchain`.

## Misc

```sh
git commit -m "docs: update readme [skip ci]"  # skip CI on docs-only main push
```

Scratch notes go in ignored `.scratch/`. More: [design](design.md), [deployment](deployment.md), [release](release.md), [benchmarks](ingestion-performance.md), [OpenAPI](openapi.yaml).
