# mise owns task orchestration

Status: Accepted. Date: 24 October 2026.

## Context

Root pnpm scripts owned application commands and mise tasks delegated to them,
so orchestration was defined twice. Scripts chained other scripts, repeating
`build:web` inside one `check:push` run, and `check:push` ran every check
serially. `go = "1"` floated in `mise.toml` and drifted from the Dockerfile Go
image; Node and pnpm pins had no drift check.

## Decision

`mise.toml` owns all task orchestration. This supersedes the documented decision
in `docs/development.md` and `AGENTS.md` that root pnpm scripts own commands and
mise delegates to them. Root `package.json` keeps only `prepare` (Husky);
`packages/cli/package.json` has no scripts. Per-package leaf scripts remain in
`packages/web`, `tools/build`, plugins and `docs-website`; tasks call them. Every
former root script is a task with the same name. New tasks: `test:go` (Go tests
without race) and `check:toolchain`.

- **Pins**: exact Go 1.27.3, Node 26.12.1 and pnpm 12.15.0. Committed `mise.lock`
  records checksums, and pnpm GitHub attestations, for Linux/macOS x64/arm64.
  Project installs are locked; `min_version` is 2026.10.18.
- **Drift**: `check:toolchain`, in `check:push`, fails when `mise.toml` differs
  from the Dockerfile `golang`/`node` image tags and pnpm install, root
  `packageManager`, or the `packages/cli/go.mod` `toolchain go1.27.3` line. The
  `go` directive stays 1.27.0.
- **Environment**: `GOTOOLCHAIN=local` and `node_modules/.bin` on `PATH`.
  Personal overrides live in ignored `mise.local.toml` / `mise.*.local.toml`.
- **Dependencies**: experimental `[deps.pnpm]` with `auto = true` runs
  `pnpm install --frozen-lockfile` before `mise run`/`mise exec` when
  `package.json` or `pnpm-lock.yaml` changed or `node_modules` is missing;
  `--no-deps` skips. `mise run setup` is `mise deps install`.
- **Graph**: `check:push` runs format check (including `mise fmt --check`), lint,
  schema, API, toolchain and TypeScript checks in parallel, then race tests,
  benchmark smoke, CLI build, distribution, browser E2E and container contracts
  serially, with `keep-order` output. Go tasks depend on `build:web`, so it runs
  once per graph and keeps its own digest skip. `build:cli` and `build:plugins`
  skip on unchanged content-hashed sources and outputs. Verification tasks are
  never cached.
- **Outside mise**: the Dockerfile web stage runs `node tools/build/src/build-web.ts`.
  CI assets and release use `mise run build:web`. Release calls
  `node tools/build/src/generate-homebrew-formula.ts` directly so retried older
  releases still work. The pre-push hook still runs `mise run check:push`.

## Consequences

One definition per task, parallel independent checks, and one asset build per
graph. Tool versions cannot drift silently across development, image and module.

- All contributors run experimental mise features; mise changes may break
  `[deps]` behavior.
- `toolchain go1.27.3` makes `go install` with Go 1.27.0-1.27.2 and default
  `GOTOOLCHAIN=auto` download 1.27.3. Direct Go builds still require Go 1.27+.
- Incomplete `sources` globs can skip a needed `build:cli` or `build:plugins`
  rebuild and leave stale outputs.
- Every tool bump requires `mise lock` plus Dockerfile, `packageManager` and
  `go.mod` updates.
- `keep-order` buffers parallel task output, hiding live progress until each
  task finishes.
- The Docker image cannot use mise, so it calls leaf scripts directly and must
  track their paths.

Unresolved questions: none.
