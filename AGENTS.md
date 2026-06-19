# tokeninsights — Agent Guide

Track local token usage for OpenCode, Pi, Codex, and Claude Code.

TokenInsights is a sync-first Go CLI: `sync` ingests durable local harness data into raw SQLite tables, `normalize` writes canonical facts, and `view` runs implicit all-harness sync by default before reading canonical tables. `view --no-sync` skips ingest and normalization. Realtime and checkpoint plugins are future-compatible concepts, not active product code.

Full architecture, schema contract, pipelines, and invariants are in [`docs/design.md`](docs/design.md). Read it before any non-trivial change.

## Agent Rules

- **Minimal, surgical changes**.
- **Never use `any`** or type assertions (`!`, `as Type`) in TypeScript.
- **CLI, schema, docs, and tests move together**. When changing storage, schema, events, SQL, aggregation, metric names, table columns, token semantics, or grouping, update the affected CLI code, tests, README, and `docs/design.md` in the same task.
- **Default DB path** is `~/.local/share/tokeninsights/tokeninsights.sqlite`; overrides use `--db-path` or `TOKENINSIGHTS_DB_PATH`. `TOKENINSIGHTS_RETENTION_DAYS` is not sync-first V1 behavior.
- **Schema is the contract**. `packages/schema/schema.sql` is the SQLite source of truth. The Go CLI embeds a checked copy and validates `PRAGMA user_version`.
- **Schema changes require explicit user approval**. Before modifying `packages/schema/schema.sql`, table structures, column definitions, or any cross-language schema contract, clearly explain the reasons to the user and ask for explicit approval. Never make silent or implicit schema changes — even for non-breaking additions.
- **Canonical token usage is session-centric**. Every canonical token row must resolve to a stable `session_id`; raw facts may preserve missing source session IDs as null and normalization must skip unresolved facts with diagnostics.
- **Prefer durable token data** over estimated stream deltas. `message.part.delta` is live UI only if realtime support returns later.
- **Missing provider/model handling**: Raw facts preserve source absence as null. Canonical/view model absence becomes `unknown`; provider absence becomes `unknown` except Claude Code artifact-derived rows, which use provider `maybe-anthropic` with `provider_source='inferred'`.
- **Raw storage is metadata-only**. Do not store prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, or full source paths.
- **TPS is first-class**. Keep the TPS tab and `tps avg`, `tps mean`, and `tps median` viewer concepts even when timing data is sparse or unavailable.
- **Write for maintainability**. Do not use magic numbers in calculations for quick fixes that violate code discipline.
- **Propose refactoring**. When you see an opportunity to refactor to strongly adhere to guidelines and quality, suggest it to the user.

## Change Checklist

- Schema changed? Get explicit approval first, then update `packages/schema/schema.sql`, `packages/cli/internal/db/schema/schema.sql`, and `packages/cli/internal/db/schema.go`; run `pnpm run check-schema`.
- Token semantics changed? Update pipeline normalization, CLI query structs, SQL, aggregation, rendering, tests, README, and `docs/design.md`.
- CLI query columns changed? Update scan order, aggregation, rendering, tests, README, and `docs/design.md`.
- Grouping changed? Update sorting and table alignment tests.
- Event source or adapter behavior changed? Update pipeline expectations, conformance fixtures, tests, and `docs/design.md`.

## Commands

```sh
pnpm run check-schema
pnpm run test
pnpm run build
```

## Agent skills

### Issue tracker

Issues and PRDs are tracked as local markdown files under `.scratch/`. See `docs/agents/issue-tracker.md`.

### Triage labels

This repo uses the default five-label triage vocabulary. See `docs/agents/triage-labels.md`.

### Domain docs

This is a single-context repo with root `CONTEXT.md`, root `docs/adr/`, and `docs/design.md` as the main design contract. See `docs/agents/domain.md`.
