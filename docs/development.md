# Development

```bash
# Check out the repo.
git clone git@github.com:flexdinesh/tokeninsights.git && cd tokeninsights

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

## Web Dashboard

React source lives in `packages/web`. The built assets are checked into `packages/cli/internal/server/static` so direct Go builds/installs include the web dashboard. After frontend edits, run `pnpm run build:web` and include the generated changes. `pnpm run build` and `pnpm run install:cli` build the frontend automatically.

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

Go HTTP tests cover canonical aggregation, pagination, facets, shared sync jobs, asset serving, listener lifecycle, and date boundaries. React tests cover URL state, multi-select interactions, and cancellation of obsolete filter requests. `pnpm run test` runs both suites. After building, `pnpm run test:web-e2e` launches the built binary with isolated synthetic data and verifies startup/manual sync, all six views, filters/back navigation, pagination, failure inspection, themes, narrow layouts, keyboard navigation, and 200% font scaling. CI and release workflows run these browser checks too.

## Skipping Actions

`[skip ci]` can be used as a temporary escape hatch when a commit should skip
GitHub Actions, such as a docs-only change that should not run release
automation.

```bash
git commit -m "docs: update readme [skip ci]"
```
