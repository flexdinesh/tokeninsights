# Development

TokenInsights is a pnpm monorepo with a Go CLI and a Vite/React browser application. `mise.toml` pins Go, Node, and pnpm for local development and GitHub Actions. Direct Go builds require Go 1.26+, without a C/C++ or JavaScript toolchain. Race verification requires CGO.

## Development builds

For development builds, install directly from `main`:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@main
```

## Setup

```sh
git clone git@github.com:flexdinesh/tokeninsights.git
cd tokeninsights
mise trust
mise install
mise run setup
mise run setup:browser
```

Install [mise](https://mise.jdx.dev/getting-started.html) first. `setup` installs frozen workspace dependencies and registers the [Husky](https://typicode.github.io/husky/get-started.html) pre-push hook through pnpm's `prepare` script. `setup:browser` installs Chromium once; Linux hosts missing browser libraries can use `mise exec -- pnpm --filter @tokeninsights/web exec playwright install --with-deps chromium`.

## Commands

Run commands from the repository root unless noted otherwise.

### Local development

| Purpose | Command |
| --- | --- |
| Full local pre-push verification | `mise run check:push` or `pnpm run check:push` |
| Install workspace dependencies | `pnpm install` |
| Run Go API and hot-reloading web app | `pnpm run dev` |
| Recreate sanitized fixture data | `pnpm run dev:data` |
| Open TUI with fixture data | `pnpm run dev:cli` |
| Run Go API with fixture data | `pnpm run dev:server` |
| Run hot-reloading web app against Go API | `pnpm run dev:web` |
| Run web app with synthetic API responses | `pnpm run dev:web:mock` |
| Build frontend into Go and run web sync/dashboard | `pnpm run start:web` |

### Verification and generation

| Purpose | Command |
| --- | --- |
| Format files | `pnpm run format` |
| Check formatting | `pnpm run format:check` |
| Lint Go and TypeScript | `pnpm run lint` |
| Run all tests | `pnpm run test` |
| Run Go race-detector suite | `pnpm run test:race` |
| Verify real server image on both backends | `pnpm run test:container` |
| Validate SQLite/PostgreSQL schema copies | `pnpm run check-schema` |
| Validate generated API files | `pnpm run check-api` |
| Regenerate API files | `pnpm run generate:api` |
| Build and sync embedded web assets | `pnpm run build:web` |
| Check committed web assets | `pnpm run check-web` |
| Run browser end-to-end tests | `pnpm run test:web-e2e` |
| Build production binary and validate contracts | `pnpm run build` |

The pre-push hook clears Git-local environment variables before running `mise run check:push`, so fixture Git commands operate on their own repositories rather than the repository being pushed. Verification covers formatting, lint, SQLite/PostgreSQL/API contracts, unit/conformance tests, all Go race tests, a frontend rebuild with committed-asset comparison, native build, browser E2E, and real-container contracts on SQLite/PostgreSQL. Docker is required for container contracts even when an external PostgreSQL test DSN is supplied. Checks fail fast and never repair tracked files. Regenerate stale API/assets explicitly before committing and pushing. There is no pre-commit test suite.

Locally reproducible verification belongs in `check:push`, not CI or release preparation. New CI checks must document coverage unavailable on the developer host in both workflow and PR; rerunning local checks is not a reason. CI builds both binaries and tests storage on matching Linux/macOS amd64/arm64 runners, providing native platform coverage one host cannot supply. Release additionally packages, validates and publishes generated artifacts. CI only verifies; development installs resolve `main` directly without publication or waiting for CI. Both workflows disable hook installation with `HUSKY=0`. Hooks run locally after dependency setup; GUI clients must have mise on PATH. Root pnpm scripts own commands; mise tasks delegate to those same scripts.

Install Chromium once before browser tests when using pnpm directly:

```sh
mise run setup:browser
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
tokeninsights web
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

## Local scratch files

Keep plans, issues, research, and temporary verification scripts in ignored `.scratch/`
directories. Never force-add scratch files. Preserve durable contracts in `docs/`,
with links only to tracked documentation. Disposable `packages/web/previews/` are also ignored.

## Fixture Data

The shared fixture is under `packages/cli/testdata/conformance/sync-first-basic/source/`. It contains compact, synthetic source data for all supported harnesses and excludes conversations, tool payloads, credentials, request data, user paths, and identifying values. Never commit raw local harness databases or transcripts.

`dev:data` resets only the controlled `.tokeninsights-dev/collector.sqlite` and
`server.sqlite` data tables, then recreates synthetic source/home
subdirectories. Stop the fixture viewer first; live or unreachable ownership
prevents recreation. Existing DB and lock inodes, unrelated files, and the old
`tokeninsights.sqlite` are preserved. Wrong-role databases are rejected before
either role is reset.

Fixture preparation runs production single-process sync against sanitized sources.
Direct delivery uses the same acceptance contract as authenticated HTTP, then waits
for processing and releases ownership. The fixture application's database pairing
follows its controlled reset. `dev:cli` queries the saved fixture directly;
`dev:server` runs `web --sync=false --open=false` on 127.0.0.1:8765. `dev` runs
that server and Vite together. Browser Reload never collects sources. Vite proxies
`/api` to the Go server; `dev:web:mock` runs without Go or local harness data.

The [collector rebuild fixture](../packages/cli/testdata/conformance/collector-rebuild/README.md)
pins independent logical facts and reparse/copy/streaming scenarios. Publication,
collectorstore, ingestion, and integration tests exercise real SQLite/HTTP
transactions, replay, receipts, compatibility, limits, and manual delivery
recovery. See [the failure contract](collector-ingestion-tests.md) for executable
coverage and remaining failure-test gaps. Fixtures must remain synthetic and
semantic expected results must never be weakened to match duplication or loss.

Current raw-path startup and source-writer benchmarks use temporary synthetic
fixtures; they never open a user's collector or token database:

```sh
cd packages/cli
go test ./internal/collector -run '^$' -bench '^BenchmarkLocalStartup$' -benchtime=3x -count=3
go test ./internal/rawcollectorstore -run '^$' -bench '^BenchmarkCaptureRecords$' -benchtime=5x -count=5
```

Startup measures owner opening, capture/direct acceptance, processing visibility
and first query for first ingest, unchanged restart and append. It excludes fixture
setup, warm-up and owner shutdown. The small Pi fixture is not a mixed-harness or
large-history latency claim. Writer benchmarks separate new observations from
replay. Compare repeated samples on one machine/toolchain; optimize measured
stages while preserving independent semantic fixtures.

From the repository root, execute all benchmark bodies once (including live
PostgreSQL) before pushing:

```sh
pnpm run bench:smoke
```

Smoke uses small shared storage history but executes every benchmark family, so
fixture SQL and semantic checks run even though normal tests do not run benchmarks.
It runs in pre-push verification. No latency
threshold determines success. Separately, `pnpm run bench:storage` measures the full
retained-history matrix with fixed iterations and sequential packages; it is not a
CI timing gate. Use a disk-backed temporary directory as described below.

See the [storage and ingestion baseline](ingestion-performance.md) for workload
sizes, timing boundaries, engine-specific size accounting and profiling commands.

## Build Tooling

The private `@tokeninsights/build-tools` workspace under `tools/build` owns schema validation, fixture preparation and safety checks, embedded-web checks, and Homebrew formula generation. Root pnpm scripts are the stable entry points.

Go uses `gofmt` and the repository-pinned `golangci-lint`. TypeScript, JavaScript, and React use Oxfmt and Oxlint. Node tooling is development-only; the production `tokeninsights` binary does not require a JavaScript runtime.

## Web Dashboard

React source lives in `packages/web`. Vite stages output in ignored `packages/web/dist`; the build tooling copies it to committed `packages/cli/internal/server/static` assets for `go:embed`.

Go tests cover SQL aggregation, API behavior, raw ingestion/replay, processing generations and collector outbox/acknowledgements, assets, listener lifecycle, and date boundaries. React tests cover URL state, filtering, and cancelled requests. Browser tests launch the built binary with isolated synthetic data and verify the full dashboard.

## Skipping CI

Use `[skip ci]` only for a deliberate docs-only commit that should skip GitHub Actions:

```sh
git commit -m "docs: update readme [skip ci]"
```

## Reference

- [Design and architecture](design.md)
- [Docker and hosted deployment](deployment.md)
- [CLI reference](../packages/cli/README.md)
- [OpenAPI contract](openapi.yaml)
- [Release guide](release.md)

Native CI/release Linux/macOS amd64/arm64 jobs build with `CGO_ENABLED=0` and test on matching runners. See [design](design.md) for current storage and acceptance contracts.

Live PostgreSQL verification: `pnpm test` and `pnpm test:race` start a pinned
PostgreSQL 18 Docker container and remove it afterward. Alternatively set
`TOKENINSIGHTS_TEST_POSTGRES_DSN` to a test server with CREATEDB privileges; tests
create random databases and delete only those. Never use production credentials.
Direct `go test` skips PostgreSQL cases without that variable; root verification
always supplies a live database and fails if it cannot start one.
