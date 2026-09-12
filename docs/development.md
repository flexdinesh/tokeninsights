# Development

Use Node 26+ and pnpm for monorepo development. Go production builds remain independent of both.

```bash
# Check out the repo.
git clone git@github.com:flexdinesh/tokeninsights.git && cd tokeninsights

# Install workspace dependencies.
pnpm install

# Format, verify formatting, and lint Go plus TypeScript/React.
pnpm run format
pnpm run format:check
pnpm run lint

# Run tests.
pnpm run test

# Build the binary.
pnpm run build

# Install the local CLI build as a binary.
pnpm run install:cli
```

The Go CLI lives in `packages/cli`. Equivalent direct Go commands are:

```bash
cd packages/cli

go test ./...
go build -o bin/tokeninsights ./cmd/tokeninsights
go install ./cmd/tokeninsights
```

## Fixture Data

The shared development and pipeline fixture lives under `packages/cli/testdata/conformance/sync-first-basic/source/`. It keeps compact, harness-native durable source shapes for OpenCode, Pi, Codex, and Claude Code—roughly two sessions per harness—while using synthetic IDs, timestamps, token counts, providers, models, and paths. It excludes conversation text, tool payloads, credentials, request data, user paths, and other identifying values. Never commit raw local harness databases or transcripts.

```sh
# Recreate .tokeninsights-dev/ and its normalized fixture database
pnpm run dev:data

# Build, prepare fixture data, and open the all-time TUI without syncing
pnpm run dev:cli

# Build, prepare fixture data, and serve it on loopback without startup sync
pnpm run dev:web
```

`dev:data` copies the sanitized JSONL sources, materializes OpenCode SQLite from reviewable `source.sql`, and writes `.tokeninsights-dev/tokeninsights.sqlite`. The generated directory is ignored and safe to recreate. `pnpm run start:web` remains the normal local-source server command.

## Build Tooling

This is a pnpm monorepo containing Go and TypeScript packages. Root Node tooling is owned by the private `@tokeninsights/build-tools` workspace package in `tools/build`. Its scripts use native, erasable TypeScript supported directly by Node 26+. It handles schema validation, repository cleanup, fixture preparation and safety tests, generated-web checks, and Homebrew formula generation. Root pnpm scripts remain stable, thin orchestration aliases. The language-neutral schema source remains at `schema/schema.sql` outside the package workspace.

Formatting uses `gofmt` for Go and Oxfmt for TypeScript, JavaScript, and React. Linting uses the repository-pinned `golangci-lint` for Go and Oxlint for TypeScript, JavaScript, and React. Use `pnpm run format` to write formatting changes, `pnpm run format:check` in verification, and `pnpm run lint` for both language stacks.

Node, npm, pnpm, `node_modules`, and this tooling package are build-, test-, and development-only. Direct Go builds and installs consume committed web assets and require no JavaScript tooling:

```sh
cd packages/cli
go build -o bin/tokeninsights ./cmd/tokeninsights
go install ./cmd/tokeninsights
```

The production host runs only the native `tokeninsights` binary. Its embedded browser JavaScript is served as static bytes by Go and executes only in the browser; Go runtime code must not invoke Node, npm, or pnpm.

## Web Dashboard

React source lives in `packages/web` and builds with Vite. The built assets are checked into `packages/cli/internal/server/static` and embedded with `go:embed`, so direct Go builds/installs include the web dashboard without requiring Node at runtime. After frontend edits, run `pnpm run build:web` and include the generated changes. `pnpm run build` and `pnpm run install:cli` build the frontend automatically.

```sh
# Build and run the embedded application
pnpm run build
./packages/cli/bin/tokeninsights serve --week

# For hot reload, optionally restrict the API to loopback
./packages/cli/bin/tokeninsights serve --host 127.0.0.1 --week

# Hot-reloading frontend (in another shell; proxies API to 127.0.0.1:8765)
pnpm --filter @tokeninsights/web run dev

# Focused frontend tests and type checking
pnpm --filter @tokeninsights/web run test

# Browser tests against the built binary, with isolated synthetic data
pnpm --filter @tokeninsights/web exec playwright install chromium
pnpm run test:web-e2e

# Verify committed web assets match source (requires a clean asset directory)
pnpm run check-web
```

Go HTTP tests cover canonical aggregation, pagination, facets, shared sync jobs, asset serving, listener lifecycle, and date boundaries. React tests cover URL state, multi-select interactions, and cancellation of obsolete filter requests. `pnpm run test` runs both suites. After building, `pnpm run test:web-e2e` launches the built binary with a separate generated 80-session synthetic dataset and verifies startup/manual sync, all six views, filters/back navigation, pagination, failure inspection, themes, narrow layouts, keyboard navigation, and 200% font scaling. The larger E2E dataset is retained because the compact shared fixture cannot exercise pagination. CI and release workflows run these browser checks too.

## Skipping Actions

`[skip ci]` can be used as a temporary escape hatch when a commit should skip
GitHub Actions, such as a docs-only change that should not run release
automation.

```bash
git commit -m "docs: update readme [skip ci]"
```
