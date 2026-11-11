# Development

Run from repo root. `mise tasks ls` lists tasks. User commands: [CLI](cli.md).

## Install any ref

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-dev.sh | sh -s -- --ref=my-branch
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-dev-native.sh | sh -s -- --ref=my-branch
```

Builds locally from a branch, tag or full commit SHA of the canonical repository (default `main`); no CI publishes non-stable builds. Requires `git` and mise; mise installs the locked Go, Node and pnpm. `install-dev.sh` installs desktop with CLI/backend and puts the CLI on PATH; `--cli-only` builds the CLI product. `install-dev-native.sh` builds TokenInsights Native only. Any macOS or Linux host on amd64/arm64 works, including macOS Intel.
The checkout lives in `${XDG_CACHE_HOME:-~/.cache}/tokeninsights/source`, which keeps `node_modules` between builds and is locked against concurrent installs. Delete it to reclaim space.
Use `tokeninsights --version` (`src.<sha12>`) or the product's `build-info.json` (`ref`, `commit`) to inspect the installed build.
Stable and dev installs replace the same product. Installation never terminates viewers. `TOKENINSIGHTS_SOURCE_URL` overrides the repository for local fixtures.

## Setup and fixtures

```sh
git clone git@github.com:flexdinesh/tokeninsights.git
cd tokeninsights
mise trust
mise install
mise run setup
mise run setup:browser
mise run dev:data
mise run dev
mise run dev:cli
mise run dev:web:mock
mise run dev:docs
```

Development data is synthetic and isolated in `.tokeninsights-dev/`. Fixture
capture uses the same runtime/collector and ownership locks. The guarded internal
entrypoint is not a public headless capture command.

## Build and run

```sh
mise run build:cli
./packages/cli/bin/tokeninsights tui
./packages/cli/bin/tokeninsights tui --capture=false --all-time
./packages/cli/bin/tokeninsights web --port 0 --open=false
./packages/cli/bin/tokeninsights
mise run build:desktop
mise run build:docs
```

The desktop requires the installed app. No browser fallback.
`build:cli` produces CLI/backend in packages/cli/bin; browser assets are generated
before Go compilation. After tooling/web-asset changes also build directly from
packages/cli and run the binary with Node/npm/pnpm absent from PATH.
For manual CLI/TUI checks use the built project binary.

## SSH and network validation

```sh
./packages/cli/bin/tokeninsights connect ssh://you@devbox
./packages/cli/bin/tokeninsights tui --connection ssh://you@devbox
./packages/cli/bin/tokeninsights web --connection ssh://you@devbox
./packages/cli/bin/tokeninsights data wait --connection ssh://you@devbox
./packages/cli/bin/tokeninsights config set remote.command /home/you/.local/bin/tokeninsights
./packages/cli/bin/tokeninsights remote
```

`remote` prints a shell-quoted connect command with a temporary TLS pin/token URI on stdout; guidance goes to stderr.
Use the exact string, including on the serving host. Stop invalidates it.
See [remote](remote.md) for `--listen`/`--advertise`.
Keep real OpenSSH and isolated TCP/network checks in local container validation;
unit stubs alone cannot prove transport or installed-product behavior.

## Format and validation

Format/lint before verification. Public validation entrypoints use isolated groups:

```sh
mise run format
mise run check:pr -- --group go
mise run check:pr -- --group deployment
```

Groups: format, static, web, tooling, go, benchmarks, distribution, browser,
deployment. Internal leaf tasks are implementation details.

Default `check:pr` runs every correctness group, including Go race detection,
real SSH/TCP, browser workflows and installed-product checks. Performance
benchmarks are opt-in through `bench:smoke`, `bench:storage` or explicit
`--group benchmarks`; default success does not claim benchmark coverage.
Formatting runs on the host first; its failure stops the run. Container groups
then need one of two admission slots shared machine-wide with other repositories
(`~/.cache/local-validation/slots`); cached successes and format-only runs need
no slot. A run executes up to three groups at once, longest first, each in its
own Compose project with 2 CPUs and 6g memory. A failed group lets the others
finish; only passed groups get receipts and every failure is reported.
Cancellation stops queued groups and tears down running projects.

Before PR creation/update, commit intended changes and require clean tracked HEAD:

```sh
mise run check:pr
mise run build:docs
```

Full successful evidence must match inputs. No PR or push CI runs; manual stable
release CI builds and publishes products only.
Packaging/runtime/SSH/network checks remain isolated. Never weaken accounting,
privacy, receipt replay or restart assertions to accommodate a failure.

Container checks need Docker 28+ with Compose and BuildKit; there is no engine
to set up. `tools/validation/Dockerfile` and `compose.yaml` define the images
and projects (see [ADR 0029](adr/0029-compose-check-runner.md)). Projects are
named `tokeninsights-check-s<slot>-<group>` and images
`tokeninsights-check:s<slot>-<target>`. Each project has an internal network with
no egress or host route, no published ports and no Docker socket. Success,
failure and cancellation remove the project and untag its images; acquiring a
slot removes this repository's stale resources for that slot. Never prune
Docker or BuildKit state broadly or touch another repository's resources.
`mise run check:isolation` proves project and host isolation against the local
engine.

Artifacts: `<canonical-git-common-dir>/tokeninsights-validation/v2/runs/<run-id>/`.
Per-group `<group>-build.log`, `<group>-run.log`, service and teardown logs sit
beside `<group>/check.log`, `status.json` and copied artifacts. Inspect failed
retained artifacts; rerun the failed group. `--force` bypasses successful check
caches. Never poll long checks or delete another invocation's artifacts. Status
is exposed by `check:status`.

`run.json` records start/completion times, slot, selected groups, cache reuse and
per-group `QueueMS`, `PreparationMS`, `ExecutionMS` and `TotalMS`. Queue time
includes admission; preparation covers context materialization, image builds
and service startup; execution measures the check container. Total time also
includes evidence export and teardown. A reused group records `Reused=true`
without claiming a new execution duration.

Fingerprints hash every tracked file of every build context a group declares
in `groups.json`; the runner materializes those same files as the named build
contexts, so container inputs and receipt keys cannot diverge. Shared stages
(base, toolchain from `mise.lock`, dependencies, web assets, CLI/backend
binaries) are reused across groups and worktrees through BuildKit. Named volumes
`tokeninsights-check-go-build` and `tokeninsights-check-golangci-lint` retain
compiler and lint output; explicit cache paths survive test changes to `HOME`.
Test commands still run
fresh on a cache miss or `--force`; writable databases, credentials and fixture
processes are never cached. Backend-only edits do not invalidate web unit tests;
frontend/contracts edits do. Installed-product builds retain their release flags.

To measure changes, use equivalent explicit correctness groups and `--force`
before/after, then repeat unchanged inputs to observe receipt reuse. Test backend
and frontend edits separately. Never clear shared build caches for a cold-run
measurement or confuse cache hits with fresh test execution. Review
[test coverage](agents/test-coverage.md) before deleting assertions.

## Benchmarks and toolchain

```sh
mise run bench:smoke
mise run bench:storage
mise run check:toolchain
```

Use synthetic real SQLite fixtures. Benchmark timing is not a production SLO.
Tool versions live in mise.toml/mise.lock; keep package manager, Go module and
validation fixture images aligned when bumping. See [performance](ingestion-performance.md).

Scratch issues/reports stay ignored in `.scratch/`. Durable docs:
[design](design.md), [release](release.md), [coverage](collector-ingestion-tests.md).
