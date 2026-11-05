# tokeninsights Agent Guide

Track local token usage for OpenCode, Pi, Codex, and Claude Code.

TokenInsights has CLI/TUI, web, MyGo and Electron over one personal Go backend per storage identity. Bare invocation opens MyGo; unavailable selected desktop fails with actionable guidance. `connect STRING` selects exactly local/SSH/temporary TCP; `tui|web --connection STRING` explicitly selects another backend. No implicit remote defaults or fallback.

The backend alone owns SQLite capture, direct acceptance, durable processing and queries. Local sessions use private Unix socket protocol 3. Capture runs on launch and explicit Refresh, coalescing overlap; no periodic scans. `--capture=false` skips launch capture while processing resumes. Filters/query reload only read saved data. Each session holds a lease; final disconnect plus grace cancels/joins work and closes stores. SSH can discover/start the backend. Foreground `remote` shares it through ephemeral pinned TLS, default `0.0.0.0:0`, with `--listen`/`--advertise` and a shell-quoted connect command. Stop invalidates its strings/connections; other leases survive. TCP remains TCP on the serving host.

Everyday commands: bare desktop, `connect`, `remote`, `tui`, `web`, `config`. Finite `data reprocess|wait` supports local/SSH only. No hosted accounts, PostgreSQL, external ingestion/sync/jobs, completion plugins or server/Docker products. Default collector.sqlite/server.sqlite and retained SQL contracts stay unchanged. Existing removed-role user files remain untouched; no migrations or deletion. See ADR 0027.

Full architecture, schema contract, pipelines, and invariants are in [`docs/design.md`](docs/design.md). Read it before any non-trivial change.

## Agent Rules

- **Minimal, surgical changes**.
- **pnpm monorepo**. Go packages contain production code. JavaScript ecosystem packages use TypeScript: browser code uses Vite and React; Node scripts use native, erasable TypeScript supported by Node 26+.
- **Never use `any`** or type assertions (`!`, `as Type`) in TypeScript.
- **CLI, schema, docs, and tests move together**. When changing storage, schema, events, SQL, aggregation, metric names, table columns, token semantics, or grouping, update the affected CLI code, tests, README, and `docs/design.md` in the same task.
- **Separate DB roles**: defaults are `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/collector.sqlite` and `server.sqlite`; overrides use `--collector-db-path` / `TOKENINSIGHTS_COLLECTOR_DB_PATH` and `--server-db-path` / `TOKENINSIGHTS_SERVER_DB_PATH`. No imports or migrations; unrelated files remain untouched. Reject role mismatches and aliased paths. `TOKENINSIGHTS_RETENTION_DAYS` is not current behavior.
- **Schemas are the contract**. `schema/schema.sql` defines collector storage; `schema/data.sql` defines SQLite token history. Go embeds checked copies and validates role, version and physical contracts. There are no account/application/jobs stores. Retained dataset/kind/destination SQL fields are historical schema contracts, not hosted product support.
- **Current role schemas only**: collector 20, token SQLite 1. Incompatible/changed contracts reject without mutation. No schema migrations or legacy wire adapters. Explicit older-local-store recovery requires terminal confirmation and moves files aside; never deletes them. Processor upgrades use durable replacement generations; never delete accepted evidence or receipts.
- **Session boundaries**: Unix socket protocol 3 requires exact equality. Remote sessions never compare local storage paths. Temporary TCP credentials authenticate every request after pin verification and permit reads, progress, leases and default capture only. Reject ingestion, maintenance and storage overrides. Bridges bind loopback; Refresh requires same origin and JSON. Native handoff keeps secrets off OS URLs and waits for desktop acknowledgement.
- **Schema changes require explicit user approval**. Before modifying any authoritative or embedded schema SQL, table structures, column definitions, or any cross-language schema contract, clearly explain the reasons to the user and ask for explicit approval. Never make silent or implicit schema changes — even for non-breaking additions.
- **Canonical token usage is session-centric**. Every canonical token row must resolve to a stable `session_id`; raw facts may preserve missing source session IDs as null and normalization must skip unresolved facts with diagnostics.
- **Prefer durable token data** over estimated stream deltas. `message.part.delta` is live UI only if realtime support returns later.
- **Missing provider/model handling**: Raw facts preserve source absence as null. Canonical/view model absence becomes `unknown`; provider absence becomes `unknown` except Claude Code artifact-derived rows, which use provider `maybe-anthropic` with `provider_source='inferred'`.
- **Direct acceptance uses sanitized raw evidence**. Source paths/cursors stay local. Evidence/receipts/scopes commit together; asynchronous facts/provenance/outcomes commit together in SQLite. Replay after collector deletion preserves stable IDs and totals.
- **Raw storage is metadata-only**. Do not store prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, or full source paths.
- **Production core is Go; Electron bundles its runtime**. One root Go module contains production Go packages. CLI/backend/MyGo need no host JavaScript runtime; Go never invokes Node/npm/pnpm. Electron is an explicit packaged runtime exception. React presentation lives in packages/dashboard, browser composition in packages/web, TypeScript contracts in packages/contracts. Backend embeds ignored generated assets from packages/core/server/static. No compiled binaries, bundles, archives or generated web assets are committed on any branch/tag. Publish prebuilt products only as stable releases or the rolling main-build prerelease; source tags point to source commits. Shell installers replace Homebrew/Go-install/edge publication. See [ADR 0027](docs/adr/0027-personal-backend-and-explicit-connections.md).
- **Format and lint before verification**. Use `gofmt` and the repository-pinned `golangci-lint` for Go. Use Oxfmt and Oxlint for TypeScript, JavaScript, and React. Run them through mise tasks.
- **Write for maintainability**. Do not use magic numbers in calculations for quick fixes that violate code discipline.
- **Propose refactoring**. When you see an opportunity to refactor to strongly adhere to guidelines and quality, suggest it to the user.

## Change Checklist

### Architecture

Read `docs/design.md` and the relevant ADRs before non-trivial changes, especially
[`ADR 0010`](docs/adr/0010-current-contracts-and-boundaries.md) and
[`ADR 0011`](docs/adr/0011-storage-adapter-contracts.md).

- **Identify ownership first**: name the module owning the behavior, its consumers,
  its state/resources, and the contract affected. Encapsulate behavior and lifecycle;
  avoid packages that merely expose mutable state or collect unrelated helpers.
- **Compose connections at the boundary**: executable roots resolve target/transport,
  select adapters and own startup/shutdown. Core modules receive explicit policy
  and dependencies; they never discover deployment mode or construct another layer's
  adapters. Keep capabilities and permissions distinct. Failure never selects a
  different transport or local fallback.
- **Resolve executable inputs once**: boundaries parse env/flags into typed settings;
  domains never read them. Retired config reports safe repair guidance without values,
  rewrites or file deletion. Connection strings cannot supply executable overrides.
  Preserve local Dagger, real SSH and isolated network validation.
- **Keep policy and mechanism separate**: mode policy determines permitted behavior;
  adapters implement storage/transport. Changing the storage engine must not change
  accounting, retained identity boundaries, or the meaning of a successful operation.
- **Use semantic contracts**: define interfaces beside consumers around operations
  and observable outcomes. Specify atomicity, identity, isolation, ordering, error,
  retry, cancellation and ownership guarantees where relevant. Keep SQL, driver
  types and transaction handles inside persistence adapters.
- **Apply DRY to shared knowledge**: share domain rules and genuinely identical
  behavior. Do not merge unrelated lifecycles or create a generic framework merely
  because SQL or control flow looks similar. Do not duplicate business policy
  across adapters or transports.
- **Prove boundaries**: extend shared contract suites for every affected adapter;
  use real storage for transactions, restart, concurrency and failure recovery.
  Keep pure accounting/identity/calendar tests. Update dependency guards with new
  boundaries; do not weaken assertions to accommodate an implementation.
- **Review the complete change**: trace each affected connection and presentation through
  success, rejection, cancellation, partial startup and reopen. Check dependency
  direction, resource cleanup and durable state. Update code, contracts, docs and
  tests together. Surface design conflicts and record superseding decisions before
  treating a different design as current.

### Domain and schema

- Schema changed? Get explicit approval first. Update the affected collector source/embed/constants or affected embedded schema and role-version constants; run `mise run check-schema` and affected live adapter contracts.
- Token semantics changed? Update pipeline normalization, CLI query structs, SQL, aggregation, rendering, tests, README, and `docs/design.md`.
- CLI query columns changed? Update scan order, aggregation, rendering, tests, README, and `docs/design.md`.
- Grouping changed? Update sorting and table alignment tests.
- Event source or adapter behavior changed? Update pipeline expectations, conformance fixtures, tests, and `docs/design.md`.
- Identity/publication changed? Preserve collector-rebuild fixtures and real ingestion failure tests; verify stable IDs, all token components, receipts, cursor progress, and REST totals. Do not weaken semantic oracles to match bugs.

## Commands

`mise.toml` pins exact development tools, locked by `mise.lock`, and owns all tasks; root `package.json` keeps only `prepare`, and tasks call per-package leaf scripts. `mise tasks ls` lists them. `mise run setup` installs dependencies and Husky hooks; pre-push runs no checks; `mise run setup:browser` installs Chromium. Bump tools per `docs/development.md`; `check:toolchain` rejects drift. Public docs live in `docs-website/`, a Vocs pnpm workspace package run with `mise run dev:docs` and `mise run build:docs`. Locally reproducible checks run in `check:pr`, including real SQLite, OpenSSH and isolated network contracts; docs build and approved host Electron GUI `check:desktop` remain separate. CI runs only on main pushes and manual releases, and only builds and publishes archives for Linux and macOS on amd64 and arm64, except macOS Intel on main; it runs no checks. Pull requests run no CI. Other platforms are tested by hand from the main channel; macOS Intel builds only for stable releases. Validation requires the explicitly managed Dagger engine. Tests receive private container services, never host database URLs.

```sh
mise run format
mise run check-schema
mise run check-api
mise run build:cli
mise run check:pr    # isolated suite before PR creation/update
mise run check:desktop # sandboxed Electron GUI on host, ADR 0024
```

## Verification

- **Validate before PR creation and updates**. Run full `mise run check:pr`, `mise run check:desktop` and `mise run build:docs` on committed HEAD with clean tracked files. The user approved host Electron GUI validation because the managed Dagger container denies unprivileged user namespaces; retain Chromium sandboxing. Pushes do not trigger the full suite. Reuse only complete successful evidence with matching inputs.
- **Use isolated groups for targeted validation**. Run `mise run check:pr -- --group <name>`; groups are format, static, web, tooling, go, benchmarks, distribution, browser and deployment. Internal leaf tasks are implementation details, not host validation entrypoints.
- **Format before verification** with `mise run format`. Commit intended changes before HEAD validation. Do not claim uncommitted development checks validate HEAD.
- **Preserve local execution**. Preserve existing validation ownership; approved product/connection changes are specified in ADR 0027. New remote coverage requires a concrete reason unavailable locally.
- **Protect shared engines**. Use the explicitly managed repository engine and dedicated named cache. Set `DAGGER_LEAVE_OLD_ENGINE=true` and the direct runner endpoint, including initialization. Never stop, restart, delete or prune another task's engine/volumes. Unknown ownership means leave untouched. Cancellation stops only the invocation. Upgrades/cleanup require coordination.
- **Inspect retained artifacts** in `<canonical Git common dir>/tokeninsights-validation/v2/runs/<run-id>/`. Rerun a failed group; use `--force` to bypass successful check caches. Never delete another invocation's artifacts.
- **Local infrastructure**: retain explicitly managed Dagger and real SSH/network tests. Retired hosted/Docker product coverage is superseded by ADR 0027; ADR 0022 retains engine ownership and isolation rules.
- **Never poll long checks**. Run long commands in the background and wait for the completion notification. Never poll logs or `sleep` in loops.
- Run `mise run build:cli` only when manual CLI verification needs a fresh binary.
- After build-tooling or web-asset changes, build directly from `packages/cli` and verify the resulting binary runs with Node, npm, and pnpm absent from `PATH`.
- For manual CLI or TUI verification, print the following project-local command on screen and ask the user to run it and verify the result:

  ```sh
  ./packages/cli/bin/tokeninsights
  ```

- Alternatively, run `./packages/cli/bin/tokeninsights` directly in the current shell or PTY and verify it there.
- Do not request permission to launch or use Ghostty, tmux, iTerm, Terminal, screenshot utilities, or other tools unrelated to this project for verification. Use the built TokenInsights binary only.

## Agent skills

### Issue tracker

Issues and PRDs live only in local, ignored `.scratch/` files. Never stage or commit scratch files, even with `git add --force`. Keep durable documentation in `docs/`. See `docs/agents/issue-tracker.md`.

### Triage labels

This repo uses the default five-label triage vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs

This is a single-context repo with root `CONTEXT.md`, root `docs/adr/`, and `docs/design.md` as the main design contract. See `docs/agents/domain.md`.

Architecture rules and contract-test strategy: [`docs/adr/0010-current-contracts-and-boundaries.md`](docs/adr/0010-current-contracts-and-boundaries.md). Preserve pure accounting tests alongside boundary tests. DRY shares behavior, not unrelated lifecycles.

Storage and connection decision: [ADR 0027](docs/adr/0027-personal-backend-and-explicit-connections.md). SQLite only; backend owns collector/token storage and lifecycle. Preserve semantic boundaries and real storage/replay contracts from ADRs 0010/0011. Accounts, application pairing, jobs and PostgreSQL are retired; historical ADRs record their former decisions.
