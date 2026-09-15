# Development

TokenInsights is a pnpm monorepo with a Go CLI and a Vite/React browser application. Use Node 26+, pnpm 11+, and Go 1.26+.

## Setup

```sh
git clone git@github.com:flexdinesh/tokeninsights.git
cd tokeninsights
pnpm install
```

## Commands

Run commands from the repository root unless noted otherwise.

### Local development

| Purpose | Command |
| --- | --- |
| Install workspace dependencies | `pnpm install` |
| Run Go API and hot-reloading web app | `pnpm run dev` |
| Recreate sanitized fixture data | `pnpm run dev:data` |
| Open TUI with fixture data | `pnpm run dev:cli` |
| Run Go API with fixture data | `pnpm run dev:server` |
| Run hot-reloading web app against Go API | `pnpm run dev:web` |
| Run web app with synthetic API responses | `pnpm run dev:web:mock` |
| Build frontend into Go and run final setup | `pnpm run start:web` |

### Verification and generation

| Purpose | Command |
| --- | --- |
| Format files | `pnpm run format` |
| Check formatting | `pnpm run format:check` |
| Lint Go and TypeScript | `pnpm run lint` |
| Run all tests | `pnpm run test` |
| Validate SQLite schema copies | `pnpm run check-schema` |
| Validate generated API files | `pnpm run check-api` |
| Regenerate API files | `pnpm run generate:api` |
| Build and sync embedded web assets | `pnpm run build:web` |
| Check committed web assets | `pnpm run check-web` |
| Run browser end-to-end tests | `pnpm run test:web-e2e` |
| Run all checks and build production binary | `pnpm run build` |

Install Chromium once before the browser end-to-end tests:

```sh
pnpm --filter @tokeninsights/web exec playwright install chromium
```

### Local CLI installation

Build the frontend, embed it in Go, and install `tokeninsights` into Go's binary directory:

```sh
pnpm run install:cli
```

If `tokeninsights` is not found, add Go's binary directory to `PATH`. For Fish:

```fish
fish_add_path (go env GOPATH)/bin
```

Verify the installed CLI and embedded browser application:

```sh
command -v tokeninsights
tokeninsights --version
tokeninsights serve
```

### Direct Go commands

Committed web assets let Go build and install without Node or pnpm:

```sh
cd packages/cli
go test ./...
go build -o bin/tokeninsights ./cmd/tokeninsights
go install ./cmd/tokeninsights
```

Direct Go commands use the currently committed embedded web assets. Use `pnpm run install:cli` when frontend changes must be included.

## Fixture Data

The shared fixture is under `packages/cli/testdata/conformance/sync-first-basic/source/`. It contains compact, synthetic source data for all supported harnesses and excludes conversations, tool payloads, credentials, request data, user paths, and identifying values. Never commit raw local harness databases or transcripts.

`dev:data` recreates the ignored `.tokeninsights-dev/` directory and its normalized database. `dev` runs the fixture-backed Go server and Vite together. Vite proxies `/api` to `127.0.0.1:8765`. `dev:web:mock` runs without Go or local harness data.

## Build Tooling

The private `@tokeninsights/build-tools` workspace under `tools/build` owns schema validation, fixture preparation and safety checks, embedded-web checks, and Homebrew formula generation. Root pnpm scripts are the stable entry points.

Go uses `gofmt` and the repository-pinned `golangci-lint`. TypeScript, JavaScript, and React use Oxfmt and Oxlint. Node tooling is development-only; the production `tokeninsights` binary does not require a JavaScript runtime.

## Web Dashboard

React source lives in `packages/web`. Vite stages output in ignored `packages/web/dist`; the build tooling copies it to committed `packages/cli/internal/server/static` assets for `go:embed`.

Go tests cover aggregation, API behavior, sync coordination, assets, listener lifecycle, and date boundaries. React tests cover URL state, filtering, and cancelled requests. Browser tests launch the built binary with isolated synthetic data and verify the full dashboard.

## Skipping CI

Use `[skip ci]` only for a deliberate docs-only commit that should skip GitHub Actions:

```sh
git commit -m "docs: update readme [skip ci]"
```

## Reference

- [Design and architecture](design.md)
- [CLI reference](../packages/cli/README.md)
- [OpenAPI contract](openapi.yaml)
- [Release guide](release.md)
