# Development

TokenInsights is a pnpm monorepo with a Go CLI and a Vite/React browser application. The public documentation site in `docs-website/` is a Vocs pnpm workspace package. `mise.toml` pins Go, Node, and pnpm for development and owns all repository tasks. Direct Go builds require Go 1.27+, without a C/C++ or JavaScript toolchain; `packages/cli/go.mod` names toolchain `go1.27.3`, so Go 1.27.0-1.27.2 with default `GOTOOLCHAIN=auto` downloads it. Race verification requires CGO. `pnpm-workspace.yaml` enforces a 3-day minimum release age for npm packages everywhere, except the `@opencode/*` and `@earendil-works/*` harness SDKs.

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

Install [mise](https://mise.jdx.dev/getting-started.html) 2026.10.18 or later first. `mise trust` accepts the repository config; `mise install` installs locked tools. `setup` runs `mise deps install`, installing frozen workspace dependencies and registering the [Husky](https://typicode.github.io/husky/get-started.html) pre-push hook through pnpm's `prepare` script. `setup:browser` installs Chromium once; Linux hosts missing browser libraries can use `mise exec -- pnpm --filter @tokeninsights/web exec playwright install --with-deps chromium`.

## Toolchain

`mise.toml` pins exact Go 1.27.3, Node 26.12.1 and pnpm 12.15.0. Committed `mise.lock` records checksums, plus GitHub attestations for pnpm, on Linux/macOS x64/arm64; installs for this project are locked. Other global mise tools are unaffected.

To bump a tool, edit `mise.toml`, run `mise lock`, then update the Dockerfile `golang`/`node` image tags and pnpm install, root `package.json` `packageManager`, and the `packages/cli/go.mod` `toolchain` line. `mise run check:toolchain`, part of `check:push`, fails on any mismatch.

mise sets `GOTOOLCHAIN=local`, so development never silently switches Go toolchains, and puts `node_modules/.bin` on `PATH`. Put personal overrides, such as `TOKENINSIGHTS_TEST_POSTGRES_DSN`, in ignored `mise.local.toml` or `mise.*.local.toml`.

`[deps.pnpm]` requires `experimental = true`. Before `mise run` or `mise exec`, it runs `pnpm install --frozen-lockfile` when `package.json` or `pnpm-lock.yaml` changed or `node_modules` is missing; `--no-deps` skips it.

## Commands

Run commands from the repository root unless noted otherwise. `mise tasks ls` lists every task with its description.

### Local development

| Purpose | Command |
| --- | --- |
| Full local pre-push verification | `mise run check:push` |
| Install workspace dependencies | `mise run setup` |
| Run Go API and hot-reloading web app | `mise run dev` |
| Recreate sanitized fixture data | `mise run dev:data` |
| Open TUI with fixture data | `mise run dev:cli` |
| Run Go API with fixture data | `mise run dev:server` |
| Run hot-reloading web app against Go API | `mise run dev:web` |
| Run web app with synthetic API responses | `mise run dev:web:mock` |
| Build frontend into Go and run local Web capture/dashboard | `mise run start:web` |
| Serve public docs with live reload | `mise run dev:docs` |
| Build public docs as static files | `mise run build:docs` |

### Verification and generation

| Purpose | Command |
| --- | --- |
| Format files | `mise run format` |
| Check formatting | `mise run format:check` |
| Lint Go and TypeScript | `mise run lint` |
| Run all tests | `mise run test` |
| Run TypeScript, plugin and web unit tests | `mise run test:js` |
| Run Go tests without the race detector | `mise run test:go` |
| Run Go race-detector suite | `mise run test:race` |
| Verify real server image on both backends | `mise run test:container` |
| Validate SQLite/PostgreSQL schema copies | `mise run check-schema` |
| Validate generated API files | `mise run check-api` |
| Check tool pins match Dockerfile, `packageManager` and go.mod | `mise run check:toolchain` |
| Regenerate API files | `mise run generate:api` |
| Build and sync embedded web assets; skips unchanged inputs | `mise run build:web` |
| Build the CLI and run browser end-to-end tests | `mise run test:web-e2e` |
| Build production binaries and plugins, validating contracts | `mise run build` |

The pre-push hook clears Git-local environment variables before running `mise run check:push`, so fixture Git commands operate on their own repositories rather than the repository being pushed. Verification covers formatting, lint, SQLite/PostgreSQL/API contracts, toolchain pins, TypeScript/web unit tests, all Go tests once under the race detector, benchmark smoke, native build, distribution and asset-publication checks, browser E2E, and real-container contracts on SQLite/PostgreSQL. Docker is required for container contracts even when an external PostgreSQL test DSN is supplied.

The entire pipeline queues across worktrees using an OS advisory lock at
`<canonical Git common directory>/tokeninsights-check.lock`. Both the hook and
direct `mise run check:push` enter this gate. Only compilation of the small Go
gate helper precedes admission. The task prints waiting, acquisition and execution
durations separately. Canceling a waiter starts no checks; canceling an owner
stops its process group before releasing admission. Parent-process loss also
triggers cleanup. Never delete the lock file: its inode, not its contents, owns
exclusion. Independently detached containers still require their tool's cleanup.
The hidden `check:push:stages` task is an internal implementation detail.

Within admission, format, lint, schema, API, toolchain and TypeScript tasks have
bounded concurrency; race tests, benchmark smoke, CLI build, distribution,
browser E2E and container contracts follow serially. `TOKENINSIGHTS_CHECK_CONCURRENCY`
defaults to `2` for mise jobs, Node test files, plugin workspaces, Go package/test
parallelism and default `GOMAXPROCS`. Explicit `GOMAXPROCS`/`GOFLAGS` remain supported;
explicit Go runner `-p`/`-parallel` arguments override its defaults. Vitest defaults
to two workers, overridable with `TOKENINSIGHTS_VITEST_WORKERS`. Chromium defaults
to one worker; an explicit override is
`mise exec -- pnpm --filter @tokeninsights/web exec playwright test --workers=2`.
Standalone focused test tasks remain available without acquiring
the full-pipeline gate; avoid launching them alongside a queued full run.

Go tasks depend on `build:web`, so one graph builds assets once. `build:web`
records input/output digests under `packages/web/node_modules/.cache` and resyncs
drifted embedded assets. CLI/plugin builds reuse unchanged outputs. Verification
tasks never skip; Go runs use `-count=1` to avoid cached test results. `keep-order`
output prints each task's output after it finishes, including phase duration.
Checks fail fast and never repair tracked files. Regenerate stale API files and
plugin bundles explicitly before committing and pushing. There is no pre-commit
test suite.

Locally reproducible verification belongs in `check:push`, not CI or release preparation. New CI checks must document coverage unavailable on the developer host in both workflow and PR; rerunning local checks is not a reason. CI runs only on main pushes, never on pull requests. It builds both binaries and tests storage on Linux arm64 and macOS amd64/arm64 runners, covering platforms the Linux amd64 developer host cannot; pre-push already covers Linux amd64. Release additionally packages, validates and publishes generated artifacts. CI only verifies; development installs resolve `main` directly without publication or waiting for CI. Both workflows disable hook installation with `HUSKY=0`, and CI pushes to `edge` and release tags use `--no-verify`, so pre-push never runs in CI. Hooks run locally after dependency setup; GUI clients must have mise on PATH. mise tasks own orchestration ([ADR 0021](adr/0021-mise-owns-task-orchestration.md)); root `package.json` keeps only `prepare`, and tasks call per-package leaf scripts. Pre-push does not build docs.

### Local CLI installation

Build the frontend, embed it in Go, and install `tokeninsights` into Go's binary directory:

```sh
mise run install:cli
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

Embedded web assets are ignored on `main` ([ADR 0017](adr/0017-generated-assets-on-installable-refs.md)). Generate them once before direct Go commands:

```sh
mise run build:web
cd packages/cli
go test ./...
go build -o bin/tokeninsights ./cmd/tokeninsights
go install ./cmd/tokeninsights
```

Direct Go commands embed the last generated assets; rerun `mise run build:web` after frontend changes, or use `mise run install:cli`. Inside the repository, mise sets `GOTOOLCHAIN=local`, so direct Go commands use the pinned toolchain.

## Local scratch files

Keep plans, issues, research, and temporary verification scripts in ignored `.scratch/`
directories. Never force-add scratch files. Preserve durable contracts in `docs/`,
with links only to tracked documentation. Disposable `packages/web/previews/` are also ignored.

## Fixture Data

Fixture capture uses the guarded internal local composition entrypoint, not distributed sync.

The shared fixture is under `packages/cli/testdata/conformance/sync-first-basic/source/`. It contains compact, synthetic source data for all supported harnesses and excludes conversations, tool payloads, credentials, request data, user paths, and identifying values. Never commit raw local harness databases or transcripts.

`dev:data` resets only the controlled `.tokeninsights-dev/collector.sqlite` and
`server.sqlite` data tables, then recreates synthetic source/home
subdirectories. Stop the fixture viewer first; live or unreachable ownership
prevents recreation. Existing DB and lock inodes, unrelated files, and the old
`tokeninsights.sqlite` are preserved. Wrong-role databases are rejected before
either role is reset.

Fixture preparation runs the shared local capture, direct ingestion and processing pipeline against sanitized sources.
Direct delivery uses the same acceptance contract as authenticated HTTP, then waits
for processing and releases ownership. The fixture application's database pairing
follows its controlled reset. `dev:cli` queries the saved fixture directly;
`dev:server` runs `web --sync=false --open=false` on an available loopback port.
`dev` discovers that server's actual URL before starting Vite on its own available
loopback port, then prints both URLs. Vite proxies `/api` to that exact server.
Browser Reload never collects sources; `dev:web:mock` needs no Go server or local
harness data.

Run `mise run dev` independently in each worktree. Each owns its fixture databases
and listeners. To choose ports explicitly, use
`mise run dev -- --api-port 8765 --web-port 5173`; either port can be `0` for
automatic allocation. An occupied explicit port fails without falling back.
Stopping the command stops both processes; startup failure also cleans up the
other process. One fixture viewer may run per worktree because it owns those
databases. Ordinary installed CLI database and port defaults are unchanged.

For separate processes, run `mise run dev:server -- --api-port 0`, then pass its
printed URL to `TOKENINSIGHTS_DEV_API_URL=http://127.0.0.1:PORT mise run dev:web -- --port 0`.
The standalone frontend requires an explicit API URL; it never guesses another
worktree's backend. Mock development supports `mise run dev:web:mock -- --port 0`.
Development binds loopback unless a standalone Vite `--host` is explicitly supplied.

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
mise run bench:smoke
```

Smoke uses small shared storage history but executes every benchmark family, so
fixture SQL and semantic checks run even though normal tests do not run benchmarks.
It runs in pre-push verification. No latency
threshold determines success. Separately, `mise run bench:storage` measures the full
retained-history matrix with fixed iterations and sequential packages; it is not a
CI timing gate. Use a disk-backed temporary directory as described below.

See the [storage and ingestion baseline](ingestion-performance.md) for workload
sizes, timing boundaries, engine-specific size accounting and profiling commands.

## Build Tooling

The private `@tokeninsights/build-tools` workspace under `tools/build` owns schema validation, fixture preparation and safety checks, embedded web asset generation, Pi/OpenCode plugin bundles, and Homebrew formula generation. The shared Pi/OpenCode collector runner lives in the private `@tokeninsights/plugin-runtime` workspace under `packages/plugin-runtime`; plugin builds copy it into each `dist/runner.js`. mise tasks are the stable entry points.

Go uses `gofmt` and the repository-pinned `golangci-lint`. TypeScript, JavaScript, and React use Oxfmt and Oxlint. Node tooling is development-only; the production `tokeninsights` binary does not require a JavaScript runtime.

## Web Dashboard

React source lives in `packages/web`. Vite stages output in ignored `packages/web/dist`; the build tooling copies it to `packages/cli/internal/server/static` for `go:embed`. That directory is ignored on `main`; CI commits it only on `edge` and stable module tags ([ADR 0017](adr/0017-generated-assets-on-installable-refs.md)).

Go tests cover SQL aggregation, API behavior, raw ingestion/replay, processing generations and collector outbox/acknowledgements, assets, listener lifecycle, and date boundaries. React tests cover URL state, filtering, cancelled requests, account forms, capability/loading states, excluded evidence, retries and preference retention.

Seven Chromium cases cover real-server dashboard filtering/history/pagination,
secure hosted cookies and cross-tab account isolation, bundled chart recovery,
pointer/keyboard resize, mobile keyboard navigation and excluded-evidence focus.
Calendar/view/percentage/error matrices belong in React or Go tests. Full theme,
font and screenshot matrices are intentionally omitted. Success screenshots are
not captured; failure screenshots/traces and bounded server logs remain in unique
per-run output directories. Browser setup binds ephemeral ports, owns private
fixture data and account credentials, and removes runtime metadata after joining
its children. Concurrent runs must not share `TOKENINSIGHTS_E2E_RUN` overrides.

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

Native CI Linux arm64 and macOS amd64/arm64 jobs and release Linux/macOS amd64/arm64 jobs build with `CGO_ENABLED=0` and test on matching runners. See [design](design.md) for current storage and acceptance contracts.

Live PostgreSQL verification: `mise run test` and `mise run test:race` start a pinned
PostgreSQL 18 Docker container and remove it afterward. Alternatively set
`TOKENINSIGHTS_TEST_POSTGRES_DSN` to a test server with CREATEDB privileges; tests
create random databases and delete only those. Never use production credentials.
Direct `go test` skips PostgreSQL cases without that variable; root verification
always supplies a live database and fails if it cannot start one.
