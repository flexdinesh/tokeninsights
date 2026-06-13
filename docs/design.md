# tokeninsights Design

## North Star

Track local token usage across supported coding harnesses over time, without relying on vendor dashboards.

The durable data model is a session-centric token time series. Every canonical token row must resolve to a stable `session_id` through `canonical_sessions`. Raw facts may preserve missing source values as null, but token facts without stable session identity must not enter canonical analytics.

Token usage is the active V1 viewer domain. TPS remains a future-compatible data domain when durable timing facts exist, but unavailable metric domains should not appear as empty active viewer tabs.

## System Architecture

```text
Local harness data
  OpenCode        Pi        Codex
     |            |          |
     +------------+----------+
                  |
                  v
        tokeninsights sync
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
      tokeninsights normalize
        - resolve sessions/messages
        - choose countable token facts
        - write diagnostics
                  |
                  v
        SQLite canonical tables
                  |
                  v
        tokeninsights view
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

## Current Implementation Status

The sync-first canonical path is the active product path. Schema V3, DB lifecycle checks, `sync`, `normalize`, reset commands, canonical token aggregation, and fixture-style pipeline conformance tests are implemented.

Known gaps are part of the current design contract:

- normalization is idempotent, but it currently scans and upserts all matching raw token facts rather than tracking an incremental work queue;
- canonical token upserts are deterministic by semantic key, but there is not yet an explicit conflict/precedence model for competing raw facts;
- diagnostics exist for parser warnings, missing canonical session identity, and some source-level suppressions such as duplicate or stale snapshots, but the full rejected/conflicting/suppressed diagnostic taxonomy is still future work;
- the viewer is aligned to token aggregation tabs; future metric domains should stay hidden until durable canonical facts exist;
- realtime and checkpoint plugin parity remains future-compatible only.

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

`tokeninsights sync`:

1. Selects harnesses through `--harness` or `--all`.
2. Discovers local sources with the selected adapters.
3. Creates one `ingest_runs` row per source attempt.
4. Parses candidate raw token facts and parser diagnostics.
5. Inserts deduplicated `raw_token_usage` rows.
6. Inserts `raw_observations` for this run.
7. Records diagnostics.
8. Marks ingest runs as `completed` or `failed`.
9. Runs normalization unless `--no-normalize` or `--dry-run` is set.

Each source ingest is transactional. If a raw fact, observation, or diagnostic write fails after the run is created, raw writes for that source are rolled back and the ingest run is committed as failed with no partial raw fact or observation rows.

`sync --dry-run` discovers and parses sources, reports counts, and writes nothing.

`sync --all` attempts all requested harnesses. Successful harness scopes should still normalize when one harness fails, and the command exits non-zero if any requested harness fails.

With `sync --all --source-dir <root>`, harness discovery is bounded to `<root>/<harness>`. Harnesses whose subdirectory is absent are skipped. Single-harness sync with `--source-dir` still scans the provided directory directly for ad hoc fixtures.

## Adapter Contract

All harness adapters implement the same interface:

- return their harness ID;
- discover durable local sources;
- parse a source into raw token facts and diagnostics.

OpenCode sync parses modern durable SQLite databases named `opencode.db` or `opencode-<channel>.db` from `${XDG_DATA_HOME:-~/.local/share}/opencode`. It reads assistant rows from the `message` table, parses metadata-only token fields from `message.data`, uses message/session IDs when available, and suppresses copied fork or channel rows with deterministic non-private fingerprints.

Pi sync parses durable JSONL session files under `~/.pi/agent/sessions`, including one nested project directory level. It uses assistant message usage as exact message-scoped token facts. Session identity comes from the session header when available and may fall back to the filename session suffix.

Codex sync parses rollout JSONL session files under `${CODEX_HOME:-~/.codex}/sessions`. It uses `event_msg` records with `payload.type == "token_count"` as exact message-scoped token facts. Codex token parsing is stateful per file: `session_meta` provides session/provider state, `turn_context` or `task_started` provides turn/model state, `last_token_usage` provides countable token components, and `total_token_usage` is used only to suppress duplicate or stale cumulative snapshots.

Harness-specific source parsing stays behind the adapter interface and feeds the same raw-to-canonical pipeline. Missing provider/model remains null in raw facts and normalizes to `unknown`. Cached input is represented as `cache_read_tokens` where the source exposes it; Codex cached input is subtracted from raw input tokens before canonical aggregation.

Uneven metric coverage is valid. An adapter should produce diagnostics for unavailable or rejected data instead of failing unrelated token usage sync.

## Normalization Pipeline

`tokeninsights normalize`:

1. Loads raw token facts, optionally filtered by harness.
2. Rejects facts without stable session identity and writes a diagnostic.
3. Upserts canonical sessions.
4. Upserts canonical messages when source message identity exists.
5. Upserts canonical token usage by semantic key.
6. Normalizes missing provider/model to `unknown`.
7. Marks fallback-like scopes as non-countable to avoid default double counting.

Normalization must be idempotent: repeated runs should converge on the same canonical identities and must not duplicate canonical facts or diagnostics.

Current normalization is rebuild-capable but not work-queue incremental. It loads all matching raw token facts for the selected harness filter, upserts canonical rows by semantic key, and increments ingest-run canonical/diagnostic counters only for newly inserted canonical facts or diagnostics. Existing canonical rows may be updated deterministically when the same semantic key is seen again.

Explicit conflict precedence between competing raw facts is not implemented yet. Until that model exists, canonical identity is governed by semantic keys and deterministic upsert behavior.

`normalize --dry-run` computes candidate canonical and diagnostic counts without writing.

## Viewer

`tokeninsights view` is interactive-only and opens the database read-only.

The TUI queries canonical tables only:

- token totals come from countable `canonical_token_usage` rows;
- provider/model/harness filters derive from available canonical rows;
- missing provider/model renders as `unknown`;
- empty canonical tables produce a clean empty state.

The active viewer surface uses token aggregation tabs:

| Tab | Primary aggregation |
|-----|---------------------|
| `tokens` | local calendar Time Bucket |
| `models` | model |
| `providers` | provider |
| `harnesses` | harness |
| `sessions` | canonical session |

Date Range Filters choose which canonical facts are included. Supported presets are today, yesterday, this week, this month, this year, and all time; the default is this month. The `tokens` tab additionally uses a Time Bucket of day, week, month, or year, with day as the default and Monday-start local weeks.

Dimension Filters choose included provider, model, and harness values. Session filtering may be provided as a startup filter, but interactive session search/filtering is not part of the active viewer surface.

Interactive shortcuts use `d` for Date Range Filter, `g` for Time Bucket, `s` for sorting, and `p`, `m`, and `h` for provider, model, and harness filters. Horizontal scrolling uses left/right arrows and home/end; `h` is reserved for the harness filter.

TPS, request, and tool domains are future-compatible canonical domains. They should remain absent from the active tab bar until durable canonical facts exist for them.

Cost tracking is not part of TokenInsights and must not appear in viewer columns, totals, sort options, or docs.

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
- unavailable metric domains must not appear as empty active viewer tabs;
- cost tracking must stay out of the active product;
- `view` opens read-only and remains interactive-only.

Can evolve with care:

- new harness adapters;
- new canonical fact domains for TPS, requests, or tools;
- explicit conflict precedence and richer diagnostic categories;
- generated schema constants;
- future checkpoint plugins that write equivalent raw/canonical concepts.

## File Organization

| Path | Role |
|------|------|
| `packages/schema/schema.sql` | SQLite schema source of truth |
| `packages/cli/internal/db/schema/schema.sql` | embedded checked schema copy |
| `packages/check-schema/check-schema.ts` | schema contract validator |
| `packages/cli/cmd/tokeninsights/main.go` | CLI executable entry point |
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
| `packages/cli/internal/pipeline/opencode_sqlite.go` | OpenCode durable SQLite adapter |
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
