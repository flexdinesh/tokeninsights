# tokeninsights — Agent Guide

Track local token usage for OpenCode, Pi, Codex, and Claude Code.

TokenInsights is a Go CLI composed as single-process or distributed. Bare invocation prints help. Local `tui` shows saved usage while background capture directly ingests/processes, then refreshes direct queries; `web` adds a foreground read-only HTTP dashboard with the same background startup. Both support `--sync=false`. Reload only queries. One viewer owns storage; concurrent sync requests enter a durable local queue and execute within that owner.

Distributed `sync` starts a finite detached authenticated HTTP submission; `--print` also submits, `--wait` waits for acceptance, and `--debug` shows receipt processing. `sync status` reports durable jobs. Plugins pass `--wait --harness`. Remote servers run as one authenticated Docker/native process and never collect. No remote analytics TUI. In-process mode uses SQLite. Hosted mode selects SQLite or PostgreSQL for both token and account storage, through separate contracts. Wire/storage kinds are personal/hosted.

Everyday commands are `sync`, `tui`, `web`, and `config`; finite local maintenance uses `data reprocess|wait`. Legacy `service`, `server`, and `collector` commands are removed. Remote admin uses the private owner socket. Default paths: collector.sqlite, server.sqlite and paired app.sqlite; operational requests use `<canonical-collector-path>.jobs.sqlite`.

Full architecture, schema contract, pipelines, and invariants are in [`docs/design.md`](docs/design.md). Read it before any non-trivial change.

## Agent Rules

- **Minimal, surgical changes**.
- **pnpm monorepo**. Go packages contain production code. JavaScript ecosystem packages use TypeScript: browser code uses Vite and React; Node scripts use native, erasable TypeScript supported by Node 26+.
- **Never use `any`** or type assertions (`!`, `as Type`) in TypeScript.
- **CLI, schema, docs, and tests move together**. When changing storage, schema, events, SQL, aggregation, metric names, table columns, token semantics, or grouping, update the affected CLI code, tests, README, and `docs/design.md` in the same task.
- **Separate DB roles**: defaults are `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/collector.sqlite` and `server.sqlite`; overrides use `--collector-db-path` / `TOKENINSIGHTS_COLLECTOR_DB_PATH` and `--server-db-path` / `TOKENINSIGHTS_SERVER_DB_PATH`. No imports or migrations; unrelated files remain untouched. Reject role mismatches and aliased paths. `TOKENINSIGHTS_RETENTION_DAYS` is not current behavior.
- **Schemas are the contract**. `schema/schema.sql` defines collector storage; `schema/data.sql` defines SQLite token storage; `schema/data.postgres.sql` and `schema/app.postgres.sql` define PostgreSQL token/account namespaces; `schema/app.sql` and `schema/jobs.sql` define application/operational SQLite. Go embeds checked copies and validates application role, schema version, and compatible data/protocol versions. Server history never uses collector reset/resync recovery.
- **Current role schemas only**: collector 20, token SQLite 1, PostgreSQL token/account 1, application SQLite 2, sync-jobs SQLite 1. Incompatible versions or changed contracts reject without mutation. No schema migrations or legacy wire adapters. Processor upgrades inside current storage use durable replacement generations; never delete accepted evidence or receipts.
- **Server capabilities and isolation**: `/api/v2/instance` advertises enabled capabilities and caller permissions. Hosted data reads/writes always use the authenticated dataset; client-supplied IDs cannot select another user's data. Raw protocol 3 requires dataset identity; older raw protocols and v1 read routes are absent. Local loading progress is command-owned; distributed servers expose no collector progress. Remote server startup never collects.
- **Schema changes require explicit user approval**. Before modifying any authoritative or embedded schema SQL, table structures, column definitions, or any cross-language schema contract, clearly explain the reasons to the user and ask for explicit approval. Never make silent or implicit schema changes — even for non-breaking additions.
- **Canonical token usage is session-centric**. Every canonical token row must resolve to a stable `session_id`; raw facts may preserve missing source session IDs as null and normalization must skip unresolved facts with diagnostics.
- **Prefer durable token data** over estimated stream deltas. `message.part.delta` is live UI only if realtime support returns later.
- **Missing provider/model handling**: Raw facts preserve source absence as null. Canonical/view model absence becomes `unknown`; provider absence becomes `unknown` except Claude Code artifact-derived rows, which use provider `maybe-anthropic` with `provider_source='inferred'`.
- **Server ingestion accepts sanitized raw evidence**. Source paths/cursors stay local. Evidence/receipts/scopes commit together; asynchronous facts/provenance/outcomes commit together. Status is in SQLite/PostgreSQL. Replays after collector deletion must preserve stable IDs and totals.
- **Raw storage is metadata-only**. Do not store prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, or full source paths.
- **Production is Go-only**. Node, npm, pnpm, `node_modules`, and repository JavaScript tooling are build-, test-, and development-only. Direct Go builds and installs from committed source must keep working. The production `tokeninsights` binary must run without a host JavaScript runtime: browser JavaScript is prebuilt, committed, embedded with `go:embed`, served as bytes by Go, and executed only in the browser. Go runtime code must never invoke Node, npm, or pnpm.
- **Format and lint before verification**. Use `gofmt` and the repository-pinned `golangci-lint` for Go. Use Oxfmt and Oxlint for TypeScript, JavaScript, and React. Run them through the root pnpm scripts.
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
- **Compose modes at the boundary**: local and hosted roots resolve mode policy,
  select adapters, and own startup/shutdown. Core modules receive explicit policy
  and dependencies; they do not discover mode, read deployment configuration, or
  construct another layer's adapters. Keep capability, permission and dataset
  authorization distinct. A remote failure must never select a local fallback.
- **Resolve deployment inputs once**: executable boundaries parse env/flags and
  secret files into typed settings; domains never read them. Proxy trust belongs
  at the hosted HTTP boundary and cannot override origin or authorization. Keep
  native/image behavior aligned; deployment changes require `pnpm run test:container`.
- **Keep policy and mechanism separate**: mode policy determines permitted behavior;
  adapters implement storage/transport. Changing the storage engine must not change
  accounting, tenant isolation, or the meaning of a successful operation.
- **Use semantic contracts**: define interfaces beside consumers around operations
  and observable outcomes. Specify atomicity, identity, isolation, ordering, error,
  retry, cancellation and ownership guarantees where relevant. Keep SQL, driver
  types and transaction handles inside persistence adapters.
- **Apply DRY to shared knowledge**: share domain rules and genuinely identical
  behavior. Do not merge unrelated lifecycles or create a generic framework merely
  because SQL or control flow looks similar. Do not duplicate business policy
  across adapters or modes.
- **Prove boundaries**: extend shared contract suites for every affected adapter;
  use real storage for transactions, restart, concurrency and failure recovery.
  Keep pure accounting/identity/calendar tests. Update dependency guards with new
  boundaries; do not weaken assertions to accommodate an implementation.
- **Review the complete change**: trace each affected mode and backend through
  success, rejection, cancellation, partial startup and reopen. Check dependency
  direction, resource cleanup and durable state. Update code, contracts, docs and
  tests together. Surface design conflicts and record superseding decisions before
  treating a different design as current.

### Domain and schema

- Schema changed? Get explicit approval first. Update the affected collector source/embed/constants or affected embedded schema and role-version constants; run `pnpm run check-schema` and affected live adapter contracts.
- Token semantics changed? Update pipeline normalization, CLI query structs, SQL, aggregation, rendering, tests, README, and `docs/design.md`.
- CLI query columns changed? Update scan order, aggregation, rendering, tests, README, and `docs/design.md`.
- Grouping changed? Update sorting and table alignment tests.
- Event source or adapter behavior changed? Update pipeline expectations, conformance fixtures, tests, and `docs/design.md`.
- Identity/publication changed? Preserve collector-rebuild fixtures and real ingestion failure tests; verify stable IDs, all token components, receipts, cursor progress, and REST totals. Do not weaken semantic oracles to match bugs.

## Commands

`mise.toml` pins development tools. `mise run setup` installs dependencies and Husky's pre-push hook; `mise run setup:browser` installs Chromium. Root pnpm scripts own tasks; mise delegates to them. Heavy verification runs locally before push. CI runs formatting, schema consistency, native builds, focused data-store tests and live PostgreSQL contracts. Root test/race scripts require Docker or `TOKENINSIGHTS_TEST_POSTGRES_DSN` with CREATEDB privileges.

```sh
mise run check:push
mise run check:ci
pnpm run format
pnpm run format:check
pnpm run lint

pnpm run check-schema
pnpm run bench:smoke
pnpm run test
pnpm run build
```

## Verification

- After changing code, run `pnpm run format`, `pnpm run lint`, the relevant focused tests, and `pnpm run test`.
- Run `pnpm run build` after tests pass.
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

Storage engine decision and lifecycle: [`docs/adr/0012-sqlite-and-postgres-persistence.md`](docs/adr/0012-sqlite-and-postgres-persistence.md). Preserve separate token/account interfaces. Select adapters only in composition. PostgreSQL writes must remain on the reserved ownership connection; losing it stops the server. No reconnecting writer, cross-domain transaction assumption, migration or fallback.
