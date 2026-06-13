# tokeninsights Design

## North Star

Track local token usage across supported coding harnesses over time, without relying on vendor dashboards.

The durable data model is a session-centric token time series. Every canonical token row must resolve to a stable `session_id` through `canonical_sessions`. Raw facts may preserve missing source values as null, but token facts without stable session identity must not enter canonical analytics.

TPS remains a first-class UI/domain concept. The sync-first V1 schema does not require every harness to expose durable timing data, but `tps avg`, `tps mean`, and `tps median` stay part of the viewer surface and must not be removed when timing support is added back through canonical facts.

## System Architecture

```text
Local harness data
  OpenCode        Pi        Codex
     |            |          |
     +------------+----------+
                  |
                  v
        tokeninsights-cli sync
        - discover sources
        - parse metadata-only facts
        - write ingest runs
        - dedupe raw facts
        - record observations
                  |
                  v
          SQLite raw tables
                  |
                  v
      tokeninsights-cli normalize
        - resolve sessions/messages
        - choose countable token facts
        - write diagnostics
                  |
                  v
        SQLite canonical tables
                  |
                  v
        tokeninsights-cli view
        interactive read-only TUI
```

Default storage is:

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

`TOKENINSIGHTS_DB_PATH` and `--db-path` override the database path. `TOKENINSIGHTS_RETENTION_DAYS` is not part of sync-first V1 behavior.

## Product Boundary

TokenInsights V1 is a local Go CLI:

- `sync` ingests durable local harness data into raw tables and normalizes by default.
- `normalize` rebuilds canonical facts from existing raw facts.
- `view` opens the interactive terminal UI over canonical data only.
- `reset-canonical` clears rebuildable canonical facts and diagnostics.
- `reset-all` recreates the local database and SQLite sidecars.

Realtime plugins and checkpoint plugins are future-compatible concepts, not active product code in this repository. The old direct-write OpenCode and Pi plugin packages are removed from the active workspace.

## Schema Contract

`packages/schema/schema.sql` is the single source of truth for SQLite table and column definitions. The Go CLI embeds a checked copy at `packages/cli/internal/db/schema/schema.sql`.

Compatibility is gated by `PRAGMA user_version`. The current sync-first schema version is `3`.

Cross-language/schema validation is handled by:

- `pnpm run check-schema`
- `packages/check-schema/check-schema.ts`
- `packages/cli/internal/db/schema_test.go`

Any modification to `packages/schema/schema.sql`, table structures, column definitions, or cross-language schema constants requires explicit user approval before implementation.

## Data Model

### `ingest_runs`

One row per source sync attempt. Runs start as `running` and complete as `completed` or `failed`.

Important fields:

- `run_id`: unique sync-run identity.
- `harness`: `opencode`, `pi`, or `codex`.
- `collector` and `parser`: implementation/version provenance.
- `source_id` and `source_kind`: stable logical source identity without storing full paths.
- `status`, `started_at_ms`, `completed_at_ms`, `error_message`.
- count columns for raw facts, observations, canonical facts, and diagnostics.

Raw fact and observation counts are written when source ingest completes. Auto-normalization may later increment `canonical_count` and `diagnostic_count` for newly inserted canonical facts or diagnostics associated with that run's observations; repeat normalization does not increment counters for existing rows.

Completed ingest metadata is audit history and should not be rewritten except for active-run completion fields and post-ingest normalization count increments.

### `raw_token_usage`

Deduplicated metadata-only token facts parsed from harness sources.

Raw facts preserve source absence as null. Missing provider/model stays null here and is normalized to `unknown` only in canonical facts.

Important fields:

- `raw_fact_key`: stable deterministic dedupe key.
- `harness`, `source_id`, `source_kind`, `collector`, `parser`.
- `observed_at_ms` and optional `occurred_at_ms`.
- optional `session_id` and `message_id`.
- optional `provider` and `model`.
- `usage_scope`, `quality`.
- token count columns.
- optional `metadata_json` for constrained metadata only.

Raw facts must not store prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, or full source paths.

### `raw_observations`

One row per ingest-run sighting of a raw fact.

Repeated syncs of the same source should not duplicate `raw_token_usage`, but they may append new `raw_observations`.

### `canonical_sessions`

Stable canonical session identities.

Every canonical token row references this table. Session semantic keys must be independent of raw row IDs and import order.

### `canonical_messages`

Optional canonical message or turn identities within a canonical session.

Token facts can be canonicalized without a message ID, but when a source provides one it should be represented here.

### `canonical_token_usage`

The primary viewer-facing fact table for sync-first V1.

Rows include:

- `semantic_key`: stable fact identity.
- `recorded_at_ms`, `harness`, canonical `session_id`, optional canonical `message_id`.
- `provider` and `model`, with missing values normalized to `unknown`.
- `usage_scope` and `quality`.
- `is_countable`: default token analytics use only countable rows.
- token count columns.
- `primary_raw_fact_id` and optional `ingest_run_id` provenance.

Canonical rows are not pre-aggregated rollups. Aggregation happens in the query layer.

### `normalization_diagnostics`

Structured metadata-only diagnostics for skipped, rejected, conflicting, or suppressed facts.

Examples:

- missing stable session identity.
- source parse warnings.
- unsupported or unavailable metric domains.

Diagnostics must not contain private source content or full paths.

## Sync Pipeline

`tokeninsights-cli sync`:

1. Selects harnesses through `--harness` or `--all`.
2. Discovers local sources with the selected adapters.
3. Creates one `ingest_runs` row per source attempt.
4. Parses candidate raw token facts and parser diagnostics.
5. Inserts deduplicated `raw_token_usage` rows.
6. Inserts `raw_observations` for this run.
7. Records diagnostics.
8. Marks ingest runs as `completed` or `failed`.
9. Runs normalization unless `--no-normalize` or `--dry-run` is set.

`sync --dry-run` discovers and parses sources, reports counts, and writes nothing.

`sync --all` attempts all requested harnesses. Successful harness scopes should still normalize when one harness fails, and the command exits non-zero if any requested harness fails.

With `sync --all --source-dir <root>`, harness discovery is bounded to `<root>/<harness>`. Harnesses whose subdirectory is absent are skipped. Single-harness sync with `--source-dir` still scans the provided directory directly for ad hoc fixtures.

## Adapter Contract

All harness adapters implement the same interface:

- return their harness ID;
- discover durable local sources;
- parse a source into raw token facts and diagnostics.

OpenCode sync parses modern durable SQLite databases named `opencode.db` or `opencode-<channel>.db` from the OpenCode data directory. Pi sync parses durable JSONL session files under `~/.pi/agent/sessions`, using assistant message usage as exact message-scoped token facts. Codex sync parses rollout JSONL session files under `${CODEX_HOME:-~/.codex}/sessions`, using `event_msg` records with `payload.type == "token_count"` as exact message-scoped token facts. Harness-specific source parsing stays behind the adapter interface and feeds the same raw-to-canonical pipeline.

Codex token parsing is stateful per file. It uses the first `session_meta` record for the logical session and provider, `turn_context` or `task_started` records for turn/model state, `last_token_usage` for countable token components, and `total_token_usage` only to suppress duplicate or stale cumulative snapshots. Cached input is normalized into `cache_read_tokens` and subtracted from raw input tokens before canonical aggregation. Missing provider/model remains null in raw facts and normalizes to `unknown`.

Uneven metric coverage is valid. An adapter should produce diagnostics for unavailable or rejected data instead of failing unrelated token usage sync.

## Normalization Pipeline

`tokeninsights-cli normalize`:

1. Loads raw token facts, optionally filtered by harness.
2. Rejects facts without stable session identity and writes a diagnostic.
3. Upserts canonical sessions.
4. Upserts canonical messages when source message identity exists.
5. Upserts canonical token usage by semantic key.
6. Normalizes missing provider/model to `unknown`.
7. Marks fallback-like scopes as non-countable to avoid default double counting.

Normalization must be idempotent: repeated runs should converge on the same canonical identities and must not duplicate canonical facts or diagnostics.

`normalize --dry-run` computes candidate canonical and diagnostic counts without writing.

## Viewer

`tokeninsights-cli view` is interactive-only and opens the database read-only.

The TUI queries canonical tables only:

- token totals come from countable `canonical_token_usage` rows;
- provider/model/harness filters derive from available canonical rows;
- missing provider/model renders as `unknown`;
- empty canonical tables produce a clean empty state.

Current grouping modes:

| Mode | Group key |
|------|-----------|
| `day` | local day, harness, provider, model |
| `hour` | local day, local hour, harness, provider, model |
| `session` | local day, session ID, harness, provider, model |

Current tabs:

| Tab | V1 source |
|-----|-----------|
| tokens | `canonical_token_usage` |
| tps | empty until canonical timing facts exist |
| requests | empty until canonical request facts exist |
| tool calls | empty until canonical tool facts exist |
| tool breakdown | empty until canonical tool facts exist |

Sparse or unavailable domains must not break token viewing.

## DB Lifecycle

- `db.Open` opens existing compatible DBs read-only for view.
- `db.CreateIfMissing` creates missing DBs from the embedded schema for sync/normalize workflows.
- incompatible schema versions are rejected with a `reset-all --confirm` instruction.
- `reset-canonical --confirm` deletes canonical token usage, messages, sessions, and normalization diagnostics while keeping raw facts and observations.
- `reset-all --confirm` removes the DB plus `-wal` and `-shm`, then recreates from schema.

## Invariants

Must not change silently:

- schema changes require explicit user approval;
- `packages/schema/schema.sql` remains the table source of truth;
- canonical token usage must be session-centric;
- missing provider/model must render as `unknown`, not cause row loss;
- raw storage must remain metadata-only and avoid private content;
- default token analytics use only countable canonical token rows;
- TPS labels and UI concepts remain first-class for future timing support;
- `view` opens read-only and remains interactive-only.

Can evolve with care:

- new harness adapters;
- new canonical fact domains for TPS, requests, tools, or costs;
- richer conflict precedence;
- generated schema constants;
- future checkpoint plugins that write equivalent raw/canonical concepts.

## File Organization

| Path | Role |
|------|------|
| `packages/schema/schema.sql` | SQLite schema source of truth |
| `packages/cli/internal/db/schema/schema.sql` | embedded checked schema copy |
| `packages/check-schema/check-schema.ts` | schema contract validator |
| `packages/cli/cmd/tokeninsights-cli/main.go` | CLI executable entry point |
| `packages/cli/internal/cli/commands.go` | command dispatch and thin orchestration |
| `packages/cli/internal/cli/flags.go` | view flag parsing |
| `packages/cli/internal/cli/table.go` | interactive TUI model |
| `packages/cli/internal/cli/render.go` | table rendering |
| `packages/cli/internal/db/open.go` | DB open/create/reset/schema lifecycle |
| `packages/cli/internal/db/schema.go` | Go schema constants |
| `packages/cli/internal/db/aggregate.go` | canonical aggregation queries |
| `packages/cli/internal/db/events.go` | canonical event rows for UI model |
| `packages/cli/internal/db/filter_values.go` | canonical filter value discovery |
| `packages/cli/internal/pipeline/adapters.go` | harness adapter interface and registry |
| `packages/cli/internal/pipeline/codex_jsonl.go` | Codex JSONL session adapter |
| `packages/cli/internal/pipeline/pi_jsonl.go` | Pi JSONL session adapter |
| `packages/cli/internal/pipeline/sync.go` | raw ingest and observation pipeline |
| `packages/cli/internal/pipeline/normalize.go` | canonical normalization and diagnostics |
| `packages/cli/internal/pipeline/pipeline_test.go` | fixture-style sync/normalize conformance tests |
| `packages/cli/internal/pipeline/testdata/conformance/` | language-neutral source and expected-output fixture contract |

## Testing And Verification

Run before schema or pipeline changes are considered done:

```sh
pnpm run check-schema
pnpm run test
pnpm run build
```

Use focused Go tests during iteration:

```sh
cd packages/cli
go test ./internal/pipeline
go test ./internal/db
go test ./...
```

Pipeline conformance fixtures live under `packages/cli/internal/pipeline/testdata/conformance/`.
Fixture sources may include harness-native durable stores, such as synthetic OpenCode SQLite setup SQL, and expected raw, observation, canonical, and diagnostic outputs are JSON so future non-Go writers can reuse the same contract.
