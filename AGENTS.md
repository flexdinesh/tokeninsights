# tokeninsights — Agent Guide

Track local token usage for OpenCode, Pi, Codex, and Claude Code.

TokenInsights is a Go CLI with a host collector and canonical-only SQLite server. Bare invocation ensures the local server and prints status; startup never collects. `sync` collects all harnesses by default, normalizes locally, journals changes, and publishes normalized batches. `collector normalize` processes collector raw facts only. `tui` and web Reload query the server over REST; `tui --sync` explicitly collects first. Local and foreground remote compositions share ingestion/query code. Completion plugins invoke the same Go `sync`; manual sync remains primary.

Everyday commands are `service`, `sync`, and `tui`. Advanced maintenance uses `collector normalize|reset-canonical|reset-all`. Previous command names, `--db-path`, and `--no-sync` are removed; use role-specific commands and flags.

Full architecture, schema contract, pipelines, and invariants are in [`docs/design.md`](docs/design.md). Read it before any non-trivial change.

## Agent Rules

- **Minimal, surgical changes**.
- **pnpm monorepo**. Go packages contain production code. JavaScript ecosystem packages use TypeScript: browser code uses Vite and React; Node scripts use native, erasable TypeScript supported by Node 26+.
- **Never use `any`** or type assertions (`!`, `as Type`) in TypeScript.
- **CLI, schema, docs, and tests move together**. When changing storage, schema, events, SQL, aggregation, metric names, table columns, token semantics, or grouping, update the affected CLI code, tests, README, and `docs/design.md` in the same task.
- **Separate DB roles**: defaults are `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/collector.sqlite` and `server.sqlite`; overrides use `--collector-db-path` / `TOKENINSIGHTS_COLLECTOR_DB_PATH` and `--server-db-path` / `TOKENINSIGHTS_SERVER_DB_PATH`. The old `tokeninsights.sqlite` remains untouched; no automatic legacy import. Reject role mismatches and aliased paths. `TOKENINSIGHTS_RETENTION_DAYS` is not current behavior.
- **Schemas are the contract**. `schema/schema.sql` defines collector storage; `schema/server.sql` defines server storage. Go embeds checked copies and validates application role, schema version, and compatible data/protocol versions. Server history never uses collector reset/resync recovery.
- **Current role schemas only**: collector schema 16 and server schema 2. Reject previous schemas without mutation; no metadata migrations or schema-reset fallback. Older data generations within current collector schema can rebuild locally; current-generation pending rebuilds resume with the same source scope. Newer generations reject.
- **Schema changes require explicit user approval**. Before modifying `schema/schema.sql` or `schema/server.sql`, table structures, column definitions, or any cross-language schema contract, clearly explain the reasons to the user and ask for explicit approval. Never make silent or implicit schema changes — even for non-breaking additions.
- **Canonical token usage is session-centric**. Every canonical token row must resolve to a stable `session_id`; raw facts may preserve missing source session IDs as null and normalization must skip unresolved facts with diagnostics.
- **Prefer durable token data** over estimated stream deltas. `message.part.delta` is live UI only if realtime support returns later.
- **Missing provider/model handling**: Raw facts preserve source absence as null. Canonical/view model absence becomes `unknown`; provider absence becomes `unknown` except Claude Code artifact-derived rows, which use provider `maybe-anthropic` with `provider_source='inferred'`.
- **Server ingestion is normalized-only**. Raw facts, harness parsers, source paths, cursors, and source diagnostics stay in collector storage. Canonical mutations and publication journal entries commit together; server facts and receipts commit together. Replays after collector deletion must preserve stable IDs and totals.
- **Raw storage is metadata-only**. Do not store prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, or full source paths.
- **TPS is first-class**. Keep the TPS tab and `tps avg`, `tps mean`, and `tps median` viewer concepts even when timing data is sparse or unavailable.
- **Production is Go-only**. Node, npm, pnpm, `node_modules`, and repository JavaScript tooling are build-, test-, and development-only. Direct Go builds and installs from committed source must keep working. The production `tokeninsights` binary must run without a host JavaScript runtime: browser JavaScript is prebuilt, committed, embedded with `go:embed`, served as bytes by Go, and executed only in the browser. Go runtime code must never invoke Node, npm, or pnpm.
- **Format and lint before verification**. Use `gofmt` and the repository-pinned `golangci-lint` for Go. Use Oxfmt and Oxlint for TypeScript, JavaScript, and React. Run them through the root pnpm scripts.
- **Write for maintainability**. Do not use magic numbers in calculations for quick fixes that violate code discipline.
- **Propose refactoring**. When you see an opportunity to refactor to strongly adhere to guidelines and quality, suggest it to the user.

## Change Checklist

- Schema changed? Get explicit approval first. Update the affected collector source/embed/constants or server `schema/server.sql` / `packages/cli/internal/serverstore/schema/server.sql` / role-version constants; run `pnpm run check-schema`.
- Token semantics changed? Update pipeline normalization, CLI query structs, SQL, aggregation, rendering, tests, README, and `docs/design.md`.
- CLI query columns changed? Update scan order, aggregation, rendering, tests, README, and `docs/design.md`.
- Grouping changed? Update sorting and table alignment tests.
- Event source or adapter behavior changed? Update pipeline expectations, conformance fixtures, tests, and `docs/design.md`.
- Identity/publication changed? Preserve collector-rebuild fixtures and real ingestion failure tests; verify stable IDs, all token components, receipts, cursor progress, and REST totals. Do not weaken semantic oracles to match bugs.

## Commands

`mise.toml` pins development tools. `mise run setup` installs dependencies and Husky's pre-push hook; `mise run setup:browser` installs Chromium. Root pnpm scripts own tasks; mise delegates to them. Heavy verification runs locally before push. CI runs only formatting, schema consistency, and native build.

```sh
mise run check:push
mise run check:ci
pnpm run format
pnpm run format:check
pnpm run lint

pnpm run check-schema
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

Issues and PRDs are tracked as local markdown files under `.scratch/`. See `docs/agents/issue-tracker.md`.

### Triage labels

This repo uses the default five-label triage vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs

This is a single-context repo with root `CONTEXT.md`, root `docs/adr/`, and `docs/design.md` as the main design contract. See `docs/agents/domain.md`.
