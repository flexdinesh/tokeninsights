# tokeninsights Design

## North Star

Track local token usage across supported coding harnesses over time, without relying on vendor dashboards.

The durable data model is a session-centric token time series. Every canonical token row must resolve to a stable `session_id` through `canonical_sessions`. Raw facts may preserve missing source values as null, but token facts without stable session identity must not enter canonical analytics.

Token usage is the active V1 viewer domain. TPS remains a future-compatible data domain when durable timing facts exist, but unavailable metric domains should not appear as empty active viewer tabs.

## System Architecture

```text
Local harness data
  OpenCode        Pi        Codex      Claude Code
     |            |          |             |
     +------------+----------+-------------+
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
- `normalize` processes pending canonical work from existing raw facts.
- `view` opens the interactive terminal UI and runs implicit all-harness sync by default before showing canonical data.
- `reset-canonical` clears rebuildable canonical facts and diagnostics, then requeues raw token facts.
- `reset-all` recreates the local database and SQLite sidecars.

Realtime plugins and checkpoint plugins are future-compatible concepts, not active product code in this repository. The old direct-write OpenCode and Pi plugin packages are removed from the active workspace.

## Current Implementation Status

The sync-first canonical path is the active product path. Schema V7, DB lifecycle checks, `sync`, OpenCode/Pi/Codex/Claude Code Recent Source Refresh, pending-work `normalize`, reset commands, canonical token aggregation, and fixture-style pipeline conformance tests are implemented.

Known gaps are part of the current design contract:

- Recent Source Refresh is implemented for OpenCode SQLite plus Pi, Codex, and Claude Code JSONL Durable Sources; true intra-source cursors are not implemented;
- canonical token upserts are deterministic by semantic key, but there is not yet an explicit conflict/precedence model for competing raw facts;
- diagnostics exist for parser warnings, missing canonical session identity, and some source-level suppressions such as duplicate or stale snapshots, but the full rejected/conflicting/suppressed diagnostic taxonomy is still future work;
- the viewer is aligned to token aggregation tabs; future metric domains should stay hidden until durable canonical facts exist;
- realtime and checkpoint plugin parity remains future-compatible only.

## Schema Contract

`packages/schema/schema.sql` is the single source of truth for SQLite table and column definitions. The Go CLI embeds a checked copy at `packages/cli/internal/db/schema/schema.sql`.

Compatibility is gated by `PRAGMA user_version`. The current sync-first schema version is `7`.

Schema V4 adds the persisted `claude-code` harness value. Existing V3 databases reject that value physically through SQLite `CHECK` constraints, so users must run `reset-all --confirm` rather than relying on an in-place migration.

Schema V5 adds canonical provider provenance through `canonical_token_usage.provider_source`. Existing V4 databases need `reset-all --confirm` because TokenInsights does not perform in-place SQLite migrations.

Schema V6 adds `normalization_work_queue` for pending canonical-domain work. Existing V5 databases need `reset-all --confirm` because TokenInsights does not perform in-place SQLite migrations.

Schema V7 adds `source_refresh_state` for Local-only Continuity Metadata used by Recent Source Refresh. Existing V6 databases need `reset-all --confirm` because TokenInsights does not perform in-place SQLite migrations.

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
- `harness`: `opencode`, `pi`, `codex`, or `claude-code`.
- `collector` and `parser`: implementation/version provenance.
- `source_id` and `source_kind`: stable logical source identity without storing full paths.
- `status`, `started_at_ms`, `completed_at_ms`, `error_message`.
- count columns for raw facts, observations, canonical facts, and diagnostics.

Raw fact and observation counts are written when source ingest completes. Auto-normalization may later increment `canonical_count` and `diagnostic_count` for newly inserted canonical facts or diagnostics associated with that run's observations; repeat normalization does not increment counters for existing rows.

Completed ingest metadata is audit history and should not be rewritten except for active-run completion fields and post-ingest normalization count increments.

### `raw_token_usage`

Deduplicated metadata-only token facts parsed from harness sources.

Raw facts preserve source absence as null. Missing provider/model stays null here and is resolved only in canonical facts.

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

Raw facts are Syncable Analytics Data, so they must remain metadata-only. Future cloud export should be canonical-first by default rather than exporting raw ingestion facts.

### `raw_observations`

One row per ingest-run sighting of a raw fact.

Repeated syncs of the same source should not duplicate `raw_token_usage`, but they may append new `raw_observations`.

### Source Refresh State

`source_refresh_state` is best-effort Local-only Continuity Metadata. It exists only to reduce repeated local Durable Source parsing and must not be used for viewer analytics or future cloud export.

Current source state properties:

- keyed by `harness`, `source_kind`, and an adapter-provided metadata-safe source state key;
- records parser/collector provenance used to decide whether a cursor can be trusted;
- stores last successful source refresh time, observed source file modification time, and observed source file size;
- may later store cursor kind and cursor values such as JSONL byte offset, file size, mtime, boundary hashes, or OpenCode SQLite `(time_created, id)`;
- avoids raw JSONL lines and full source paths unless a specific adapter cannot maintain continuity without them;
- is cleared by `reset-all --confirm` and preserved by `reset-canonical --confirm`.

If source continuity cannot be trusted, the pipeline must fall back to a full parse. Cursor availability must never be required for correctness.

### Normalization Work Queue

`normalization_work_queue` is Local-only Continuity Metadata for incremental normalization. Work is queued by `raw_fact_id` and canonical domain, initially `token_usage`. A work item means "attempt to normalize this raw fact for this domain." The result may be a canonical row or a normalization diagnostic.

Work queue rules:

- `sync --no-normalize` still enqueues work for newly inserted raw facts;
- ordinary `normalize` processes pending work rather than scanning all raw facts;
- `normalize --dry-run` reports pending work by default;
- work is removed only in the same transaction that writes the canonical fact or diagnostic;
- missing-session diagnostics complete their work item;
- `reset-canonical --confirm` marks all existing raw facts dirty so canonical data and diagnostics can be rebuilt from raw facts;
- `reset-all --confirm` clears the queue with the rest of the database.

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
- `provider` and `model`. Missing models normalize to `unknown`. Missing providers normalize to `unknown` except Claude Code artifact-derived rows, which canonicalize to `maybe-anthropic`.
- `provider_source`: `explicit`, `inferred`, or `unknown`.
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
6. Enqueues `token_usage` normalization work for newly inserted raw facts.
7. Inserts `raw_observations` for this run.
8. Records diagnostics.
9. Marks ingest runs as `completed` or `failed`.
10. Runs normalization unless `--no-normalize` or `--dry-run` is set.

Each source ingest is transactional. If a raw fact, observation, or diagnostic write fails after the run is created, raw writes for that source are rolled back and the ingest run is committed as failed with no partial raw fact or observation rows.

`sync --dry-run` discovers sources, uses existing source refresh state to preview old unchanged source skips when available, parses sources that would be refreshed, reports counts, and writes nothing.

`sync --full-refresh` ignores source refresh state for the requested harness scope and full-parses discovered sources using the existing parser behavior. Successful source ingest updates source refresh state after commit. Full refresh does not requeue all existing raw facts for canonical rebuild by default; only newly inserted raw facts enqueue pending normalization work.

Source refresh optimization preserves the same correctness behavior while reducing repeated work in phases:

- Phase 1, Recent Source Refresh: skip file-based Durable Sources whose local modification metadata is older than a conservative freshness window before the last successful source refresh, and fully parse sources inside that window;
- Phase 2, incremental normalization: process only pending normalization work instead of scanning all raw facts;
- Phase 3, true intra-source cursors: add JSONL byte-offset cursors and OpenCode SQLite row cursors only if Phase 1 and Phase 2 do not make implicit view sync cheap enough.

Phase 1 rules:

- freshness checks use monotonic source metadata such as file modification time, not source event timestamps or local calendar dates;
- the cutoff should be conservative, initially `last_successful_source_refresh_at_ms - 48h`;
- sources older than the cutoff may be skipped as up to date;
- sources at or after the cutoff are parsed from the beginning using existing parser behavior;
- raw fact dedupe remains the correctness guard for repeated parsing inside the freshness window.
- OpenCode SQLite plus Pi, Codex, and Claude Code JSONL sources participate in Recent Source Refresh. After a successful source refresh, TokenInsights records metadata-safe source state. Later syncs skip unchanged files older than the 48-hour cutoff, fully parse recent files, and fully parse files whose modification metadata changed.
- A skipped up-to-date source still creates a completed lightweight ingest run with zero raw facts and zero observations.

Phase 3 rules, if needed:

- adapters decide whether a source can be parsed incrementally;
- the pipeline persists adapter-returned cursor advancement metadata only after source ingest commits;
- JSONL adapters should use byte offsets plus file size, mtime, and boundary hashes, falling back to full parse when a file shrinks, rewrites, or cannot be verified;
- OpenCode SQLite should use source-native row ordering, initially `(time_created, id)` from the `message` table, rather than SQLite file offsets;
- an incremental run writes observations only for facts actually parsed in that run;
- an up-to-date source creates a completed lightweight ingest run with zero raw facts and zero observations;
- cursor invalidation and full-parse fallback should be local-only operational information, not viewer analytics;
- `sync --dry-run` should use cursor state to preview real sync behavior without writing cursor updates;
- `sync --full-refresh` ignores cursor or source refresh state for the requested harness scope without requeueing all existing raw facts for canonical rebuild.

`sync --all` attempts all requested harnesses. Successful harness scopes should still normalize when one harness fails, and the command exits non-zero if any requested harness fails.

With `sync --all --source-dir <root>`, harness discovery is bounded to `<root>/<harness>`. Harnesses whose subdirectory is absent are skipped. Single-harness sync with `--source-dir` still scans the provided directory directly for ad hoc fixtures.

## Adapter Contract

All harness adapters implement the same interface:

- return their harness ID;
- discover durable local sources;
- parse a source into raw token facts and diagnostics.

The future source-refresh adapter contract should additionally let adapters:

- receive prior Local-only Continuity Metadata when present;
- choose skip, full parse, incremental parse, or up-to-date result;
- return source refresh state advancement metadata that is safe to persist only after successful source ingest;
- invalidate stale source refresh state when parser provenance or source continuity checks fail.

OpenCode sync parses modern durable SQLite databases named `opencode.db` or `opencode-<channel>.db` from `${XDG_DATA_HOME:-~/.local/share}/opencode`. It reads assistant rows from the `message` table, parses metadata-only token fields from `message.data`, uses message/session IDs when available, and suppresses copied fork or channel rows with deterministic non-private fingerprints. OpenCode SQLite sources participate in Recent Source Refresh using metadata-safe source keys, parser/collector provenance, database file modification time, and database file size; if state is missing, stale, or changed, OpenCode falls back to the existing full table parse. True OpenCode row cursors are not implemented.

Pi sync parses durable JSONL session files under `~/.pi/agent/sessions`, including one nested project directory level. It uses assistant message usage as exact message-scoped token facts. Session identity comes from the session header when available and may fall back to the filename session suffix. Pi JSONL sources participate in Recent Source Refresh using metadata-safe source keys, parser/collector provenance, file modification time, and file size; if state is missing, stale, or changed, Pi falls back to the existing full parse.

Codex sync parses rollout JSONL session files under `${CODEX_HOME:-~/.codex}/sessions`. It uses `event_msg` records with `payload.type == "token_count"` as exact message-scoped token facts. Codex token parsing is stateful per file: `session_meta` provides session/provider state, `turn_context` or `task_started` provides turn/model state, `last_token_usage` provides countable token components, and `total_token_usage` is used only to suppress duplicate or stale cumulative snapshots. Codex JSONL sources participate in Recent Source Refresh using metadata-safe source keys, parser/collector provenance, file modification time, and file size; if state is missing, stale, or changed, Codex falls back to the existing full parse.

Claude Code sync parses JSONL transcript files under `${CLAUDE_CONFIG_DIR:-~/.claude}/projects`. Single-harness `--source-dir` scans the provided directory directly, and `sync --all --source-dir <root>` scans `<root>/claude-code`. It uses assistant message `usage` metadata as derived message-scoped token facts, with the file stem as the fallback session identity and top-level `sessionId` as the parent session identity when present. This attributes sidechain/subagent usage to the user-visible parent session when Claude Code records that parent. Message identity uses `message.id` when present and falls back to the row `uuid`. Streaming duplicate assistant rows are merged within a source by `message.id + requestId`, or by `message.id` when no request ID is present, keeping the maximum token component values. Copied transcript facts are suppressed by logical dedupe keys that do not include full source paths. `cache_read_input_tokens` maps to cache read tokens, `cache_creation_input_tokens` maps to cache write tokens, and raw total tokens remain null unless the source provides a total. Claude Code explicit provider values are preserved with provider source `explicit`. When Claude Code artifacts omit provider metadata, canonical rows use provider `maybe-anthropic` with provider source `inferred`. Model values come from explicit model fields, and missing models canonicalize to `unknown`. Claude Code JSONL sources participate in Recent Source Refresh using metadata-safe source keys, parser/collector provenance, file modification time, and file size; if state is missing, stale, or changed, Claude Code falls back to the existing full parse.

Harness-specific source parsing stays behind the adapter interface and feeds the same raw-to-canonical pipeline. Missing provider/model remains null in raw facts. Canonical provider provenance records whether a provider was explicit, inferred, or unknown. Cached input is represented as `cache_read_tokens` where the source exposes it; Codex cached input is subtracted from raw input tokens before canonical aggregation.

Uneven metric coverage is valid. An adapter should produce diagnostics for unavailable or rejected data instead of failing unrelated token usage sync.

## Normalization Pipeline

`tokeninsights normalize`:

1. Loads raw token facts, optionally filtered by harness.
2. Rejects facts without stable session identity and writes a diagnostic.
3. Upserts canonical sessions.
4. Upserts canonical messages when source message identity exists.
5. Upserts canonical token usage by semantic key.
6. Resolves missing provider/model according to canonical provider/model rules.
7. Marks fallback-like scopes as non-countable to avoid default double counting.

Normalization must be idempotent: repeated runs should converge on the same canonical identities and must not duplicate canonical facts or diagnostics.

Current normalization is work-queue incremental. It loads pending `token_usage` work for the selected harness filter, upserts canonical rows by semantic key, removes completed work in the same transaction, and increments ingest-run canonical/diagnostic counters only for newly inserted canonical facts or diagnostics. Existing canonical rows may be updated deterministically when the same semantic key is requeued by an explicit rebuild path.

Incremental normalization changes the default work selection, not the canonical identity rules. Ordinary normalization processes only pending normalization work, while explicit rebuild paths mark raw facts dirty and then use the same work mechanism. Deterministic updates remain allowed for dirty raw facts.

Explicit conflict precedence between competing raw facts is not implemented yet. Until that model exists, canonical identity is governed by semantic keys and deterministic upsert behavior.

`normalize --dry-run` computes candidate canonical and diagnostic counts without writing.

## Viewer

`tokeninsights view` is interactive-only. By default it opens the TUI into an Implicit View Sync progress state: the same all-harness refresh behavior as `sync --all`, including default normalization and create-if-missing DB lifecycle. After that optional write-before-read step, the TUI opens the database read-only and queries canonical tables only.

`view --no-sync` skips raw ingest and normalization. It preserves read-only viewer behavior and rejects a missing or incompatible database instead of creating or modifying it.

Implicit view sync normalizes pre-existing pending work even when sources are up to date, but skips normalization when no work is pending. `view --no-sync` remains read-only and must not process pending work.

Viewer Dimension Filters remain display constraints. For example, `view --harness pi` refreshes all supported Durable Sources first, then filters the displayed canonical facts to Pi.

The Implicit View Sync progress state shows all supported harnesses in sequential sync order with high-level statuses: `pending`, `discovering`, `syncing`, `skipped`, `synced`, `failed`, `normalizing`, and `loading dashboard`. It must not show source paths, source IDs, project names, or file-level details.

Successful Implicit View Sync does not print a sync summary before rendering the dashboard. If Implicit View Sync fails, `view` exits the TUI, prints the sync summary, returns the sync error, recommends targeted manual refresh with `tokeninsights sync --harness <harness>` followed by `tokeninsights view --no-sync`, and does not render the dashboard table.

The TUI queries canonical tables only:

- token totals come from countable `canonical_token_usage` rows;
- provider/model/harness filters derive from available canonical rows;
- missing model renders as `unknown`; missing provider renders as `unknown` except inferred Claude Code provider, which renders as `maybe-anthropic`;
- empty canonical tables produce a clean empty state.

The active viewer surface uses token aggregation tabs:

| Tab | Primary aggregation |
|-----|---------------------|
| `tokens` | local calendar Time Bucket |
| `models` | model |
| `providers` | provider |
| `harnesses` | harness |
| `sessions` | canonical session |
| `context` | harness, provider, and model |

The token tabs use short cache labels, `cache R` and `cache W`. The sessions tab also exposes a derived `ctx used` column: the maximum prompt-side context load in the group, computed per token fact as input plus cache read plus cache write tokens. It excludes output and reasoning tokens so future context-window percentages can divide by a separate denominator.

The `context` tab compares Session Peak Context Load across harness/provider/model combinations. For each row, the viewer first computes one in-range session peak per canonical session and harness/provider/model combination, then summarizes those peaks as `sessions`, `avg ctx`, `median ctx`, and `max ctx`. Even-count medians average the two middle session peaks and render as integer token counts. The tab uses countable canonical token rows only, includes canonical `unknown` and `maybe-anthropic` values, applies Date Range Filters and Dimension Filters before aggregation, and requires no schema changes.

Date Range Filters choose which canonical facts are included. Supported presets are today, yesterday, this week, this month, this year, and all time; the default is this month. The `tokens` tab additionally uses a Time Bucket of day, week, month, or year, with day as the default and Monday-start local weeks.

Dimension Filters choose included provider, model, and harness values. Session filtering may be provided as a startup filter, but interactive session search/filtering is not part of the active viewer surface.

Interactive shortcuts use `d` for Date Range Filter, `g` for Time Bucket, `s` for sorting, and `p`, `m`, and `h` for provider, model, and harness filters. Horizontal scrolling uses left/right arrows and home/end; `h` is reserved for the harness filter.

The `context` tab sorts by `avg ctx` descending by default and supports sorting by `avg ctx`, `median ctx`, `max ctx`, `sessions`, `harness`, `provider`, and `model`.

Viewer tables are viewport-aware. They use consistent column width rules across Aggregation Tabs, stack multiple model, provider, or harness summary values vertically within a row, truncate long display values, and fall back to horizontal scrolling only when minimum readable widths cannot fit.

TPS, request, and tool domains are future-compatible canonical domains. They should remain absent from the active tab bar until durable canonical facts exist for them.

Cost tracking is not part of TokenInsights and must not appear in viewer columns, totals, sort options, or docs.

## DB Lifecycle

- `db.Open` opens existing compatible DBs read-only for view.
- `db.CreateIfMissing` creates missing DBs from the embedded schema for sync/normalize workflows and implicit view sync.
- incompatible schema versions are rejected with a `reset-all --confirm` instruction.
- `reset-canonical --confirm` deletes canonical token usage, messages, sessions, and normalization diagnostics while keeping raw facts, observations, and source refresh state, then requeues existing raw token facts for canonical rebuild.
- `reset-all --confirm` removes the DB plus `-wal` and `-shm`, then recreates from schema, clearing source refresh state and pending normalization work.

## Invariants

Must not change silently:

- schema changes require explicit user approval;
- `packages/schema/schema.sql` remains the table source of truth;
- canonical token usage must be session-centric;
- missing model and unavailable provider must render with canonical fallback values, not cause row loss;
- raw storage must remain metadata-only and avoid private content;
- Syncable Analytics Data must exclude prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, and full source paths;
- Local-only Continuity Metadata must never be used for viewer analytics or future cloud export;
- default token analytics use only countable canonical token rows;
- unavailable metric domains must not appear as empty active viewer tabs;
- cost tracking must stay out of the active product;
- the TUI queries canonical data read-only after any optional Implicit View Sync, and `view --no-sync` remains a read-only command path.

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
| `packages/cli/internal/pipeline/claude_code_jsonl.go` | Claude Code JSONL transcript adapter |
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
