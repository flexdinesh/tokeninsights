# Development

Run from repo root. `mise tasks ls` lists tasks. User commands: [CLI](cli.md).

## Install main

```sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-mygo.sh | sh -s -- --channel=main
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-main.sh | sh
curl -fsSL https://raw.githubusercontent.com/flexdinesh/tokeninsights/main/install-electron.sh | sh -s -- --channel=main
fish_add_path ~/.local/bin
```

MyGo includes CLI/backend. CLI-only supports TUI/web; Electron remains separate. Main has no macOS Intel build.
Use `tokeninsights --version` to inspect the installed version.
Stable/main replace the selected product. Installation never terminates viewers.

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
./packages/cli/bin/tokeninsights --open=mygo
./packages/cli/bin/tokeninsights --open=electron
mise run build:desktop
mise run build:docs
```

Desktop selection requires the installed selected app. No browser fallback.
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

`remote` prints a shell-quoted connect command with a temporary TLS pin/token URI.
Use the exact string, including on the serving host. Stop invalidates it.
See [remote](remote.md) for `--listen`/`--advertise`.
Keep real OpenSSH and isolated TCP/network checks in local Dagger validation;
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

Before PR creation/update, commit intended changes and require clean tracked HEAD:

```sh
mise run check:pr
mise run check:desktop
mise run build:docs
```

Full successful evidence must match inputs. No PR CI runs; main/release CI builds
and publishes products only. Electron GUI validation runs on the host with Chromium
sandboxing enabled because managed Dagger denies unprivileged user namespaces.
Packaging/runtime/SSH/network checks remain isolated. Never weaken accounting,
privacy, receipt replay or restart assertions to accommodate a failure.

Use the explicitly managed repository Dagger engine and dedicated named cache.
Set `DAGGER_LEAVE_OLD_ENGINE=true` and the direct runner endpoint, including setup.
Never stop/restart/prune other tasks' engines or volumes. Unknown ownership means
leave untouched. Cancellation stops only the invocation.

Artifacts: `<canonical-git-common-dir>/tokeninsights-validation/v2/runs/<run-id>/`.
Inspect failed retained artifacts; rerun the failed group. `--force` bypasses
successful check caches. Never poll long checks or delete another invocation's
artifacts. Status/setup are exposed by `check:status` and
`check:pr -- --setup-engine`; engine upgrades/cleanup require coordination.

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
