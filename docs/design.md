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
       +----------+-----------+
       |                      |
       v                      v
 tokeninsights view   tokeninsights serve
 direct SQLite read   versioned REST API
       |                      |
       v                      v
 terminal TUI          embedded React UI
                       same-origin API
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
- `view` opens the interactive terminal UI, runs implicit all-harness sync by default, and reads canonical SQLite data directly without an HTTP server. Compatible saved usage remains available during sync; first sync and recovery show progress.
- `serve` runs an HTTP server over the same canonical state, hosts the embedded React dashboard, performs startup all-harness sync, and exposes explicit web Sync through the REST API.
- `reset-canonical` clears rebuildable canonical facts and diagnostics, then requeues raw token facts.
- `reset-all` transactionally recreates application tables inside the existing SQLite file.

Realtime plugins and checkpoint plugins are future-compatible concepts, not active product code in this repository. The old direct-write OpenCode and Pi plugin packages are removed from the active workspace.

## Code And Process Boundaries

The repository is a polyglot monorepo with one Go module and a pnpm workspace. The Go module is the native product and release unit; pnpm coordinates build-, test-, and development-only TypeScript tooling and the browser application.

Go dependency direction is:

```text
commands and transports -> application semantics -> pipeline and database
```

Protocol handlers must not become the reusable application boundary. Future transports such as MCP should share protocol-independent analytics and sync behavior with the REST server rather than call REST internally or duplicate database queries. Multiple commands that share one release lifecycle should remain in the existing Go module. A second Go module and root `go.work` become appropriate only when a component needs an independent dependency or release lifecycle.

SQLite is the durable state boundary. Durable job status and canonical revisions coordinate HTTP, CLI, and TUI processes through the database writer lock. Transport-local startup/recovery feedback covers the interval before compatible operational metadata exists.

The OpenAPI document and SQLite schema remain language-neutral contracts. Deployable browser applications may share narrowly scoped workspace packages such as UI primitives or API clients after a second consumer exists; generic shared packages and application-feature coupling should be avoided.

## Current Implementation Status

The sync-first canonical path is the active product path. Schema V14, automatic schema/data compatibility recovery, `sync`, OpenCode/Pi/Codex/Claude Code Recent Source Refresh, same-version unchanged-source reuse, Pi JSONL byte cursors, pending-work `normalize`, reset commands, canonical token aggregation, optional fact-level location attribution, and fixture-style pipeline conformance tests are implemented.

Known gaps are part of the current design contract:

- Recent Source Refresh is implemented for OpenCode V1/V2 SQLite plus Pi, Codex, and Claude Code JSONL Durable Sources; byte cursors cover eligible Pi files, while changed OpenCode, Codex, and Claude Code sources still fully parse;
- canonical token upserts are deterministic by semantic key, but there is not yet an explicit conflict/precedence model for competing raw facts;
- diagnostics exist for parser warnings, missing canonical session identity, and some source-level suppressions such as duplicate or stale snapshots, but the full rejected/conflicting/suppressed diagnostic taxonomy is still future work;
- the viewer is aligned to token aggregation tabs; future metric domains should stay hidden until durable canonical facts exist;
- realtime and checkpoint plugin parity remains future-compatible only.

## Schema Contract

`schema/schema.sql` is the single source of truth for SQLite table and column definitions. The Go CLI embeds a checked copy at `packages/cli/internal/db/schema/schema.sql`.

Compatibility is gated by `PRAGMA user_version` plus `database_lifecycle.data_generation`. The current schema version is `14` and data generation is `5`. Release version numbers are not compatibility markers. Bump schema version for structural changes and data generation for breaking token semantics or raw/canonical identity changes requiring reingestion.

Schema V4 adds the persisted `claude-code` harness value. Existing V3 databases reject that value physically through SQLite `CHECK` constraints.

Schema V5 adds canonical provider provenance through `canonical_token_usage.provider_source`.

Schema V6 adds `normalization_work_queue` for pending canonical-domain work.

Schema V7 adds `source_refresh_state` for Local-only Continuity Metadata used by Recent Source Refresh.

Schema V8 adds `database_lifecycle` for local compatibility and resumable rebuild state. Recognized older schema/data now recover automatically through a transactional in-place application-table reset and all-harness normalized reingestion. This supersedes the former manual `reset-all` upgrade requirement; it is not a row-preserving schema migration.

Schema V10 retains optional `usage_locations` metadata and `location_id` on raw and canonical token facts, with only repository and directory identities. Data generation 3 introduced location attribution; generation 4 refreshed display paths; generation 5 removes worktree and branch attribution. Each upgrade triggers a full reset and resync. The database is reconstructable from retained source artifacts; usage whose artifacts were deleted may disappear after recovery. Sync checks recorded source directories against current Git metadata when those directories exist. A path reused by a different repository can therefore attribute older facts to the checkout present at sync time; provenance records the origin of repository values.

Schema V11 adds `source_cursor_state` for verified Pi JSONL byte offsets and same-version source fingerprints. Upgrading from V10 uses the existing full reset and resync recovery, so usage whose original source artifacts are gone may disappear.

Schema V12 adds per-harness `normalization_rule_state`. V11 databases upgrade transactionally without deleting raw or canonical usage; normalization then refreshes identifier rules once. Older incompatible schemas retain reset and resync recovery.

Cross-language/schema validation is handled by:

- `pnpm run check-schema`
- `tools/build/src/check-schema.ts`
- `packages/cli/internal/db/schema_test.go`

Any modification to `schema/schema.sql`, table structures, column definitions, or cross-language schema constants requires explicit user approval before implementation.

## Data Model

### `database_lifecycle`

Singleton Local-only Continuity Metadata, excluded from analytics and future export:

- `id`: constrained to `1`.
- `data_generation`: current semantic/identity compatibility generation. Generation `1` corrected Codex cumulative/replay accounting; generation `2` makes input/cache and output/reasoning components additive across harnesses, hardens source counters, and aligns OpenCode V1/V2 message identity; generation `3` rebuilds raw and canonical facts with location attribution; generation `4` rebuilds display paths; generation `5` rebuilds repository and directory identities without worktree or branch fields.
- `rebuild_pending`: recovery remains incomplete until all configured harnesses sync and normalize successfully.
- `rebuild_source_key`: hash of the normalized recovery source configuration, NULL when ready and nonempty while pending; stores no source paths.
- `updated_at_ms`: lifecycle update time.

Fresh databases start at the current generation with no pending rebuild and a NULL source key. Reset commits the current schema/generation, pending state, and source-scope fingerprint atomically. Failed recovery preserves that state and any committed partial imports for a same-scope retry; successful completion clears pending state and the source key together.

Schema V14 adds nullable `ingest_runs.hostname` to record the machine performing source ingestion. V11–V13 upgrade additively without deleting usage or changing data generation. Existing runs keep NULL; hostname is captured on future syncs, including unchanged-source checks. Missing hostname lookup remains NULL.

### Durable sync status (V13)

`sync_state` is a local singleton with a durable viewer publication revision and the last successful normalized all-harness sync time. It advances in the canonical transaction, including standalone normalization and canonical resets, and when terminal harness/job coverage commits. Existing per-source ingest completion is no longer presented as overall sync success.

`sync_jobs` records metadata-only scope fingerprints, running/completed/failed/cancelled/interrupted outcome, phase, actual timestamps, normalization policy, and all-harness scope. `sync_harnesses` records discovery, source counts, status, and successful checked time for each job/harness. `sync_sources` records hashed source identities, reading/ingested/ready/unchanged/failed/deferred state, safe error codes, and conservative UTC usage-time bounds. No full paths or transcript content are added. These tables are Local-only Continuity Metadata, excluded from analytics and exports.

V11–V13 upgrade transactionally to V14 without deleting source facts or canonical usage. V13 sync history is preserved. V11/V12 prior successful overall check time is unknown until the first new all-harness sync; source ingest history cannot prove that coverage.

One database writer lock owns a job. Status reads observe that lock without creating it; orphaned running jobs read as interrupted and the next owner persists their interrupted outcome. Server POST joins an observed active job; CLI/TUI writers wait with a named waiting phase. Scope-changing contenders remain serialized. Recovery creates its job after transactional reset and retains it on retry; analytics remain unavailable until all normalized recovery work completes.

### `ingest_runs`

One row per source sync attempt. Runs start as `running` and complete as `completed` or `failed`.

Important fields:

- `run_id`: unique sync-run identity.
- `hostname`: nullable hostname captured once per sync from the ingesting machine; older rows and unavailable lookups remain NULL.
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
- avoids raw JSONL lines and full source paths unless a specific adapter cannot maintain continuity without them;
- is cleared by `reset-all --confirm` and preserved by `reset-canonical --confirm`.

`source_cursor_state` is separate Local-only Continuity Metadata. Eligible Pi files persist a byte offset, file size, mtime, parser/collector identity, hashes of the processed prefix and prior boundary, and a hashed location fingerprint. Ordinary Codex and Claude Code sources persist a full content fingerprint and current location fingerprint. OpenCode hashes parser-relevant V1/V2 message rows and table definitions in a consistent SQLite read snapshot; its location fingerprint binds each session ID to current directory/repository attribution. Unrelated SQLite writes and WAL checkpoints do not invalidate the OpenCode marker. No path or transcript content is stored. Markers advance only in the source ingest transaction after parsing and raw writes succeed. A changed prefix, changed checkout attribution, same-size rewrite, truncation, nonterminated line, changed parser/collector, or uncertain Pi session header causes a full parse. Codex fork replay and changed Claude Code streaming copies or mutable OpenCode rows remain on full parsing.

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
- `provider` and `model`: persisted query identifiers derived from the source values in `raw_token_usage`, which remain unchanged and accessible through `primary_raw_fact_id`. Code-defined rules map Pi `openai-codex` to `openai`, map `fireworks-ai` to `fireworks` across harnesses, and remove `accounts/fireworks/models/` from Fireworks models when a nonempty model name remains. Other identifiers pass through. Missing models normalize to `unknown`. Missing providers normalize to `unknown` except Claude Code artifact-derived rows, which canonicalize to `maybe-anthropic`.
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
10. Normalizes and publishes after each harness unless `--no-normalize` or `--dry-run` is set; discovers all harnesses first so source-work totals are known.

Each source ingest is transactional. If a raw fact, observation, or diagnostic write fails after the run is created, raw writes for that source are rolled back and the ingest run is committed as failed with no partial raw fact or observation rows.

`sync --dry-run` discovers sources, uses existing source refresh state to preview old unchanged source skips when available, parses sources that would be refreshed, reports counts, and writes nothing. When recovery is needed, it previews reset/resume and rebuild parsing without stale refresh-state suppression or database mutation.

`sync --full-refresh` ignores source refresh state for the requested harness scope and full-parses discovered sources using the existing parser behavior. Successful source ingest updates source refresh state after commit. Full refresh does not requeue all existing raw facts for canonical rebuild by default; only newly inserted raw facts enqueue pending normalization work.

Identifier rules apply while writing canonical facts. Normal sync and `normalize` also refresh previously written canonical identifiers from linked raw facts, including when no new raw work is pending. Raw source identifiers remain unchanged.

Source refresh optimization preserves the same correctness behavior while reducing repeated work in phases:

- Phase 1, Recent Source Refresh: skip file-based Durable Sources whose local modification metadata is older than a conservative freshness window before the last successful source refresh, and fully parse sources inside that window;
- Phase 2, incremental normalization: process pending work and refresh canonical identifiers once per rule signature;
- Phase 3, source continuity: Pi JSONL byte-offset cursors are active for eligible files; other adapters skip verified unchanged sources but need source-specific continuity proofs before incremental changed-source parsing.

Phase 1 rules:

- freshness checks use monotonic source metadata such as file modification time, not source event timestamps or local calendar dates;
- the cutoff should be conservative, initially `last_successful_source_refresh_at_ms - 48h`;
- sources older than the cutoff may be skipped as up to date;
- sources at or after the cutoff are parsed from the beginning unless an eligible Pi cursor proves an unchanged prefix or a non-Pi fingerprint proves unchanged content and location attribution;
- raw fact dedupe remains the correctness guard for repeated parsing inside the freshness window.
- OpenCode SQLite plus Pi, Codex, and Claude Code JSONL sources participate in Recent Source Refresh. After a successful source refresh, TokenInsights records metadata-safe source state. Later syncs skip unchanged Pi files older than the 48-hour cutoff. Pi cursor-eligible recent files skip unchanged bytes or parse appended bytes. OpenCode, ordinary Codex, and Claude Code sources skip when their content and current location attribution match their successful marker, regardless of the 48-hour window. Changed files fully parse.
- Codex fork/subagent sources always reparse, because linked parent history affects replay resolution. Ancestor parses are cached within each sync; ordinary Codex sources retain Recent Source Refresh.
- A skipped up-to-date source still creates a completed lightweight ingest run with zero raw facts and zero observations.

Phase 3 rules:

- adapters decide whether a source can be parsed incrementally;
- the pipeline commits cursor advancement atomically with successful source ingest;
- JSONL adapters should use byte offsets plus file size, mtime, and boundary hashes, falling back to full parse when a file shrinks, rewrites, or cannot be verified;
- OpenCode SQLite would need source-native row ordering plus a way to detect updates to prior rows before a cursor could replace full parsing;
- an incremental run writes observations only for facts actually parsed in that run;
- an up-to-date source creates a completed lightweight ingest run with zero raw facts and zero observations;
- cursor invalidation and full-parse fallback should be local-only operational information, not viewer analytics;
- `sync --dry-run` should use cursor state to preview real sync behavior without writing cursor updates;
- `sync --full-refresh` ignores cursor or source refresh state for the requested harness scope without requeueing all existing raw facts for canonical rebuild.

`sync --all` attempts all requested harnesses and continues later sources after recoverable read/parse failures. Successful source scopes still normalize when one source or harness fails; the command exits non-zero if any requested scope fails. Database write failures and cancellation stop the job. Per-source transaction rollback cannot leak in-memory dedupe state. Source ingest uses an injected wall clock for actual start/completion times; source observations retain the command observation time.

JSONL readers use Reader with a captured file extent, cancellation checks between chunks, and no fixed Scanner record limit. Large irrelevant Codex tool/output records do not block later usage, and large usage-bearing records remain countable. An unfinished trailing JSON record is deferred, not diagnosed as corrupt; its source coverage stays pending and no reusable fingerprint/cursor advances past that tail. Complete valid JSON without a trailing newline remains readable, without establishing a byte cursor.

With `sync --all --source-dir <root>`, harness discovery is bounded to `<root>/<harness>`. Harnesses whose subdirectory is absent are skipped. Single-harness sync with `--source-dir` still scans the provided directory directly for ad hoc fixtures.

Before normal data work, public sync/normalize coordinate compatibility recovery under a database-scoped writer lock. Targeted default-source commands recover all default harnesses first; all-harness `--source-dir` recovery stays within the specified root. Single-harness custom-source recovery defers without changing the database. Recovery always normalizes before satisfying the requested scope, including `--no-normalize`; missing installations remain normal skips. Failed recovery preserves partial imports but keeps analytics unavailable until the all-harness rebuild succeeds.

Pending recovery must resume with the same normalized source configuration. Default-source keys fingerprint the effective OpenCode, Pi, Codex, and Claude Code roots, including environment overrides. Custom all-harness keys fingerprint the normalized canonical absolute root. Only a hash is persisted; full paths cannot be reconstructed from it. Mismatched retries, including dry-run attempts, are rejected before data writes. Repeat the original `--source-dir` and `--db-path`, preserving source environment settings. Default-source `normalize`/TUI/web startup cannot finish a custom-root rebuild.

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

OpenCode sync parses durable V1 and V2 SQLite databases named `opencode.db` or `opencode-<channel>.db` from `${XDG_DATA_HOME:-~/.local/share}/opencode`. Sessions marked archived remain in these databases and are included. It reads V1 assistant rows from `message.data` and V2 assistant rows from `session_message.data`, maps their message-scoped token, model, provider, and timing metadata, and uses stable row/session IDs. The SQLite row ID is authoritative in both layouts; embedded V1 JSON IDs cannot split one migrated message into two canonical identities. OpenCode's stored input excludes cache tokens and its stored output excludes reasoning, so the five components are directly additive. Component sums that exceed SQLite's signed integer range are rejected with a diagnostic. In mixed migrated databases, usable V2 rows take precedence for the same session/message; V1 remains the fallback when the V2 row has no usable token data. Copied fork or channel rows are suppressed with deterministic non-private fingerprints. OpenCode-specific parser provenance forces one automatic reparse when V2 support is introduced; canonical semantic keys prevent analytics duplication. A successful logical fingerprint of message rows and session attribution skips later unchanged parses, including when unrelated tables or WAL files change. Missing or changed markers fall back to the full table parse. True OpenCode row cursors are not implemented.

Pi sync parses durable JSONL session files under `~/.pi/agent/sessions`, including one nested project directory level. Pi has no harness-owned archive; sessions moved to OS trash are outside the durable-source boundary and are not parsed. It uses assistant message usage as exact message-scoped token facts. Current Pi input excludes cache tokens, while reasoning is an optional subset of output; the adapter subtracts reasoning from output. For legacy rows whose total proves input still includes cache, it subtracts cache read/write from input. A source `totalTokens` is retained only when it equals the normalized component sum; otherwise canonical total falls back to the components with a diagnostic. Session identity comes from the session header when available and may fall back to the filename session suffix. Pi JSONL sources participate in Recent Source Refresh using metadata-safe source keys, parser/collector provenance, file modification time, and file size; if state is missing, stale, or changed, Pi falls back to the existing full parse.

Codex sync parses rollout JSONL session files under `${CODEX_HOME:-~/.codex}/sessions` and `${CODEX_HOME:-~/.codex}/archived_sessions`. It uses `event_msg` records with `payload.type == "token_count"` as exact message-scoped token facts. Codex parsing is stateful: the first usable `session_meta` establishes owning session identity, with the existing filename fallback; later copied metadata cannot replace ownership. `turn_context` or `task_started` provides turn/model state, `last_token_usage` provides countable token components, and `total_token_usage` provides snapshot identity and duplicate/stale guards rather than additive usage. Codex input includes cached input and output includes reasoning; the adapter subtracts those subsets into mutually exclusive canonical components.

The per-sync Codex adapter indexes explicit `forked_from_id`, `source.subagent.thread_spawn.parent_thread_id`, and top-level `parent_thread_id` within discovered source boundaries. Consistent parent references are accepted. A lazy ancestor resolver caches metadata and parsed facts so linked parents full-parse at most once per sync. Fork/subagent sources always reparse; ordinary sources still use Recent Source Refresh with metadata-safe source keys, parser/collector provenance, modification time, and size. This trades extra fork parsing for correct cross-source resolution.

Verified parent-history copies require explicit ancestry plus matching turn, provider/model, and complete last/cumulative token metadata. Replay timestamps may be rewritten and are not match keys; ordinals and `history_mode` do not prove boundaries. Verified copies emit the original source/session/message identity, occurrence time, provider/model, and tokens with current observation/provenance. Raw and canonical dedupe count them once regardless of source order or skipped parent ingestion. Deterministic snapshot fingerprints distinguish different facts sharing a timestamp; line identity is retained when cumulative identity is absent. Pending model/turn backfill preserves snapshot and line information.

Missing/unreadable parents, conflicting ancestry, ambiguous source copies, cycles, or unproven replay retain uncertain child usage with metadata-only diagnostics. Optional parent failures do not discard child usage; normal source failures retain normal failure behavior. Replay resolution precedes local duplicate/stale handling so inherited counters cannot suppress genuine child requests. Unresolved fork history conservatively resets suppression baseline at explicit new turns, with diagnostics for ambiguous boundary snapshots; ordinary-session and same-turn suppression remain. Automatic reconciliation of previously imported ambiguous history after parent availability changes is out of scope; an explicit `reset-all` and sync may be needed. Only retained local sources are reconstructable.

Claude Code sync parses retained local JSONL transcript files under `${CLAUDE_CONFIG_DIR:-~/.claude}/projects` regardless of UI/server archive state; cloud-only archived sessions are outside the local durable-source boundary and are not parsed. Single-harness `--source-dir` scans the provided directory directly, and `sync --all --source-dir <root>` scans `<root>/claude-code`. It uses assistant message `usage` metadata as derived message-scoped token facts, with the file stem as the fallback session identity and top-level `sessionId` as the parent session identity when present. This attributes sidechain/subagent usage to the user-visible parent session when Claude Code records that parent. Message identity uses `message.id` when present and falls back to the row `uuid`. Streaming duplicate assistant rows are merged within a source by `message.id + requestId`, or by `message.id` when no request ID is present, keeping the maximum source-native token values before normalization. Copied transcript facts are suppressed by logical dedupe keys that do not include full source paths. `cache_read_input_tokens` maps to cache read tokens, `cache_creation_input_tokens` maps to cache write tokens, and `output_tokens_details.thinking_tokens` is subtracted from inclusive output into reasoning when valid. A source total is retained only when it equals the normalized component sum. Claude Code explicit provider values are preserved with provider source `explicit`. When Claude Code artifacts omit provider metadata, canonical rows use provider `maybe-anthropic` with provider source `inferred`. Model values come from explicit model fields, and missing models canonicalize to `unknown`. Claude Code JSONL sources participate in Recent Source Refresh using metadata-safe source keys, parser/collector provenance, file modification time, and file size; if state is missing, stale, or changed, Claude Code falls back to the existing full parse.

Harness-specific source parsing stays behind the adapter interface and feeds the same raw-to-canonical pipeline. Missing provider/model remains null in raw facts. Canonical provider provenance records whether a provider was explicit, inferred, or unknown. JSON counters are decoded as exact signed integers; fractional, overflowing, and component-sum-overflow values are rejected. Canonical input/cache and output/reasoning columns are mutually exclusive, so `total = input + output + reasoning + cache read + cache write` whenever a valid authoritative source total is unavailable.

Location is optional, fact-level metadata. OpenCode uses session directory and Git project metadata, Pi uses session-header `cwd`, Codex uses turn/session `cwd` plus recorded session Git remote, and Claude Code uses assistant-row `cwd`. A shared resolver examines an available directory at sync time for Git repository details. Remote identity combines clones; if unavailable, repository identity falls back to the Git common directory and then a confirmed OpenCode Git project. Missing or conflicting location fields remain unknown with diagnostics. Codex replay copies preserve the original fact location.

`usage_locations` stores a stable tuple key, nullable directory/repository keys and display names, and repository provenance. Raw and canonical token rows reference the location by nullable `location_id`. Directory labels use `~/` when the current home or a standard home-directory pattern can be identified; otherwise the full directory path is stored and returned by the API. Full remote URLs and source artifact paths stay out of analytics storage. Stable key hashes never appear in display names. When repository identity is unknown and every included fact has the same known directory, the row and facet show `unknown · <directory>`; otherwise they show `unknown`, and Directory grouping exposes known paths separately. One session can contribute facts to multiple locations; row session counts are distinct within each group and may repeat across groups. Unknown values participate in totals.

Parser provenance is part of raw identity. Any parser revision that can change emitted facts, token semantics, or identity must bump `data_generation`; automatic recovery then resets and reingests every harness rather than mixing generations. Parser-only reparsing is reserved for output-compatible changes.

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

Incremental normalization processes pending raw-fact work. A per-harness signature of current provider/model rules triggers one refresh of existing canonical identifiers when missing or changed; matching signatures with no pending work return before a write transaction. Rule markers commit with normalization, and `reset-canonical` clears them. Explicit rebuild paths mark raw facts dirty and use the same work mechanism. Deterministic updates remain allowed for dirty raw facts.

Explicit conflict precedence between competing raw facts is not implemented yet. Until that model exists, canonical identity is governed by semantic keys and deterministic upsert behavior.

`normalize --dry-run` computes candidate canonical and diagnostic counts without writing.

## Viewer

`tokeninsights view` is interactive-only. By default it opens the TUI into an Implicit View Sync progress state: the same all-harness refresh behavior as `sync --all`, including default normalization and create-if-missing DB lifecycle. The TUI opens the database read-only and queries committed canonical tables during ordinary sync and after completion.

`view --no-sync` skips raw ingest and normalization. It preserves read-only viewer behavior and rejects a missing, incompatible, or rebuild-pending database instead of creating or modifying it.

Implicit view sync normalizes pre-existing pending work even when sources are up to date. With no pending work and current rule markers, normalization performs no writes. `view --no-sync` remains read-only and must not process pending work.

Viewer Dimension Filters remain display constraints. For example, `view --harness pi` refreshes all supported Durable Sources first, then filters the displayed canonical facts to Pi.

The Implicit View Sync progress state shows all supported harnesses in sequential sync order with high-level statuses: `waiting`, `pending`, `resetting`, `rebuilding`, `discovering`, `syncing`, `skipped`, `synced`, `failed`, `normalizing`, and `loading dashboard`. Resetting/rebuilding are active spinner states explaining compatibility recovery. Explicit sync/normalize print concise recovery notices to stderr. It must not show source paths, source IDs, project names, or file-level details.

Successful Implicit View Sync does not print a sync summary before rendering the dashboard. Ordinary failures retain committed usage and allow `u` to retry sync; quitting reports the unresolved sync error. Shared metadata is polled read-only, including with `--no-sync`, and committed revisions refresh analytics during sync. Daily calendar markers show checked, empty, pending, updating, incomplete, or not checked. Unknown metrics render `—`; confirmed empty days render zero. Markers never contribute to fact-row counts or totals. The header shows days checked and source work percentage after discovery completes. Failed compatibility recovery instead recommends retrying `tokeninsights sync --all` with the original database, source override, and environment settings; pending data cannot be inspected with `--no-sync`.

The TUI uses the **Instrument desk** visual system, implemented in `internal/cli/theme.go` and `internal/cli/desk.go` and documented in [`packages/cli/DESIGN.md`](../packages/cli/DESIGN.md). Full-screen terminals (120×35 and larger) get a spaced header, compact seven-view navigation, scope controls, a token readout strip, and a full-width table. The canvas, readouts, and drawers preserve the terminal background; only selection highlights use fills. Foregrounds adapt to light/dark terminals. Terminal fonts remain user-owned. Bracketed active navigation, a row cursor, explicit sync status words, and checkbox markers retain meaning without color.

The header renders typed statusline state as `TokenInsights · host: <host> · synced: <time>`. Hostname is resolved once and falls back to `unknown`; absent sync history renders as `never`. Hostname shortens before sync time when space is constrained. Date range, bucket, and sort remain typed viewer state but render as keyboard controls above the readouts, alongside a filter action. Active Dimension Filters, including startup session-ID filters, appear immediately below the controls. Presets render as `today`, `yesterday`, `week`, `month`, `year`, or `all time`; custom bounds render as `from..to`, `from..`, or `..to`. Choosing a preset explicitly clears custom bounds, including when choosing the already configured preset.

The readout strip has one blank row above and below its two content rows, replacing the previous external top spacer without consuming additional table space. It sums the exact integer total, input, output, reasoning, cache read, and cache write fields across all filtered rows. It never parses abbreviated table strings or sums only visible rows. These components come from the same canonical queries as the table, without additional queries or schema changes. Loading and failed reads never display stale readout values. Context replaces the strip with an explanation of Session Peak Context Load, not a sum or an average of grouped averages. Terminals below 72 content columns or 24 rows omit the strip; all table metrics remain reachable through scrolling. Available table height derives from the rendered chrome. Header and footer compact before sacrificing data, and rendered output is bounded to the terminal dimensions.

Date, bucket, sort, filter, and help drawers open on the right, retaining visible dashboard context. Lists scroll to keep the cursor in view and actions remain pinned. Filter changes remain drafts until Enter applies them; Escape discards drafts and preserves table position. Space toggles values; clearing and applying the selection removes that Dimension Filter. Drawers appear immediately without decorative animation; actual sync progress retains its activity animation. Read failures retain navigation and expose a read-only retry action. Empty results distinguish an empty database from a filtered-out scope and give an appropriate next action.

The table summary is an explicit, pinned, full-width band below the table viewport, separated from data by one blank spacer row. It remains visible and unchanged during vertical and horizontal scrolling because it summarizes the full filtered result set rather than only visible rows. All tabs lead with `sessions <shown> shown / <synced> synced`, followed by `rows <count>`. Token-bearing tabs also show `total <tokens>`; the `context` tab omits this additive token total. Session coverage comes first so it remains visible ahead of row and token totals on narrow terminals; the line is clipped to the available width. Loading reserves a blank summary row so the table layout does not jump. An empty filtered result can show `sessions 0 shown / 214 synced · rows 0 · total 0`; an empty database shows zero synced sessions.

Session coverage is queried from canonical data on every dashboard reload, independently of the active aggregation tab and time bucket. `shown` counts distinct canonical session IDs with countable token facts matching all active Date Range and Dimension Filters, including startup session filters and custom day bounds. `synced` counts distinct canonical session IDs with countable token facts across the entire database, ignoring every viewer filter. Both counts exclude empty canonical sessions and sessions with only non-countable facts. The counts use canonical database IDs so the same source session ID in different harnesses remains distinct, and sessions spanning multiple dates/models/providers are counted once. This is coverage of imported, normalized usage, not the number of source files discovered or provider-account lifetime sessions. The query is read-only and needs no schema change. Date and dimension changes reload coverage; scrolling and sorting do not change it.

The single-row footer owns essential shortcuts, loading state, and scroll position. Detailed keys live in the help drawer. Active filters live above the readouts. Session coverage and row count remain pinned in the table summary; the full-result token total also appears in the readout strip. Last sync time belongs only to the statusline.

The TUI queries canonical usage and operational sync metadata read-only. Every reader uses `db.BeginAnalyticsRead` to validate lifecycle inside its read-only transaction, after the preliminary `db.Open` guard. Dashboard rows, session counts, and last-sync time share one validated snapshot; filters and standalone reads also use guarded snapshots. A concurrent reset cannot expose partial recovery through a validation-to-query race.

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

Interactive shortcuts use `d` for Date Range Filter, `g` for Time Bucket or Repo grouping, `s` for sorting, and `p`, `m`, and `h` for provider, model, and harness filters. `f` opens the filter menu and `?` opens the keyboard guide. Tab/Shift-Tab and 1–7 select views; up/down or j/k move rows, PageUp/PageDown move a page, and left/right plus home/end scroll columns. Repo adds repository and directory facets to the filter menu. `h` remains reserved for the harness filter. `r` reloads canonical data without syncing; successful retry clears the failure state. Ctrl+C exits even while a drawer is open. During implicit sync, dashboard controls are inactive while quit remains available.

The `context` tab sorts by `avg ctx` descending by default and supports sorting by `avg ctx`, `median ctx`, `max ctx`, `sessions`, `harness`, `provider`, and `model`.

Viewer tables are viewport-aware. They use consistent column width rules across Aggregation Tabs, stack multiple model, provider, or harness summary values vertically within a row, truncate long display values, and fall back to horizontal scrolling only when minimum readable widths cannot fit.

TPS, request, and tool domains are future-compatible canonical domains. They should remain absent from the active tab bar until durable canonical facts exist for them.

Cost tracking is not part of TokenInsights and must not appear in viewer columns, totals, sort options, or docs.

## Web Viewer

`tokeninsights serve` serves the React dashboard and versioned JSON REST API from one Go binary. The REST boundary reads the server's canonical SQLite state; the TUI continues to read that state directly. `serve` accepts the viewer's Date Range Filters, Time Bucket, Dimension Filters, session-ID filters, DB-path override, and `--no-sync`, plus `--host` (explicit IPv4 bind address; omitted binds to localhost) and `--port` (default `8765`, `0` requests an available port). Defaults match `view`: this month and daily buckets. Viewer flag registration and date/filter semantics are shared; browser clients can independently change their initial CLI selections.

The default server binds `127.0.0.1` and prints a colored, indented startup summary containing the TokenInsights version, machine hostname, and `http://localhost:<port>` URL. Explicit `--host` binding uses and prints only that IPv4 address; `0.0.0.0` remains available when deliberately requested. IPv6 and invalid bind addresses are rejected. When the omitted default port is busy, interactive startup identifies listeners with `lsof`, asks before sending `SIGTERM`, waits up to five seconds for release, and then starts the server. An explicitly passed busy port fails without prompting. Interrupt/termination cancels sync and request work, shuts down HTTP, and closes the listener.

Once HTTP serving starts, `serve` attempts to open the default browser using macOS `open`, Windows `rundll32`, or Linux `xdg-open`. Any nonempty `SSH_CONNECTION`, `SSH_CLIENT`, or `SSH_TTY` suppresses opening, even with display forwarding. Linux also requires `DISPLAY` or `WAYLAND_DISPLAY`; unsupported platforms skip opening. The browser receives the actual assigned port and uses localhost for a wildcard bind. Launcher startup failures print a warning and manual URL without stopping the server. Launcher processes are reaped asynchronously so serving does not wait for a browser to close. Cancelled or failed startup does not open a browser.

Startup serves the UI immediately and runs the existing all-harness sync/normalization pipeline, including automatic compatibility recovery. Compatible committed usage remains queryable during ordinary sync, with a visible progress state; first sync shows loading until usage exists. The web UI shows metadata-only per-harness progress and explains resetting/rebuilding phases. `serve --no-sync` validates an existing compatible, fully recovered DB and skips startup writes; **Sync now** remains enabled. Sync requests share one process-wide job, independent of client filters. **Reload data** rereads canonical data without ingest. An ordinary sync failure keeps saved usage visible with retry. Source coverage has expandable day details with server-local dates/check times, unknown values `—`, and confirmed empty values zero. Source-count progress measures checked work, independently of ready publication; it never claims a percentage of lifetime usage. Recovery-required or rebuild-pending failures use status phase `rebuild_failed`, offer retry, and hide inspection. Dashboard/filter reads reject incompatible/pending data with HTTP 503. Detailed errors stay in terminal logs, while HTTP errors omit source paths.

During recovery, progress leaves the global `rebuilding` phase intact until completion, including normalization. Dashboard and filter reads validate lifecycle inside the transaction that reads analytics using `db.BeginAnalyticsRead`, closing the gap between preliminary open validation and the read snapshot. Recovery retry must preserve source configuration; a custom-root pending rebuild must be resumed from the CLI with the original `--source-dir` and `--db-path`.

The web viewer includes all seven active Aggregation Tabs and their TUI metrics/sort concepts. TanStack Router maps them to `/tokens`, `/models`, `/providers`, `/harnesses`, `/sessions`, `/context`, and `/repo`; `/` redirects to `/tokens`, and legacy `/?tab=<view>` URLs redirect to the matching path. Direct loads of those paths serve the embedded React application, while unknown `/api/*` paths remain JSON 404s. Tables support ascending/descending sorting, column visibility, adjustable column widths, and pagination (default 50, maximum 200 rows per API page). Comma-separated harness, provider, and model summaries display one value per line in table rows, matching the TUI; their API values and sort semantics stay unchanged. Column widths are local viewer state; dragging a header edge or using its keyboard separator changes layout without changing query or analytics data. Canonical grouping happens in existing Go/SQLite viewer queries, then the server sorts and slices grouped rows. Each dashboard response reads its rows, chart data, full-result summary, and last completed sync in one read-only database transaction. Summary session counts use the TUI's shared `ViewerSessionCounts` query: API `summary.sessions` is the distinct shown count and `summary.syncedSessions` is the all-dates/all-harnesses count ignoring every viewer filter. Empty canonical sessions and non-countable-only sessions are excluded. The Sessions card labels its filtered count as **Sessions shown** with the synced denominator underneath. Every table summary leads with `Sessions <shown> shown / <synced> synced`, including Context and empty filtered results, followed by row count and applicable token total. Table summaries remain independent of pagination; Context has no additive token-total summary. Loading placeholders omit stale coverage until the new filtered response arrives.

Token/session charts show chronological Time Buckets. Model/provider/harness charts show the top 12 groups by canonical total tokens, with keyboard-accessible labels that apply the corresponding Dimension Filter. Each bar, filter label, and tooltip shows its share of the full filtered canonical token total from the response summary, including groups outside the top 12 and independently of table pagination. Shares use at most one decimal place; nonzero shares below 0.1% render as `<0.1%`, and a zero total yields 0%. Context charts show the top 12 groups by average Session Peak Context Load, including average, median, and maximum. Charts use canonical totals directly; components do not redefine total-token semantics. Date bounds are inclusive local dates, custom bounds replace the preset, and Monday-start weeks use server-local calendar arithmetic even when the browser is in another timezone.

Repo defaults to repository grouping. Users switch between repository and directory grouping. Rows list all distinct providers, harnesses, and models in both the TUI and web table. Its chart ranks the top 12 selected groups. Repository and directory facets accept stable keys and show names or sanitized paths; **unknown** is selectable. In either grouping, an unknown row starts with its directories hidden. When recorded directories contribute, the web table offers **Show directories** and reveals every directory on request, along with any missing-directory note. An unknown directory group has no recorded directory to expand, so it shows **No recorded directory** without a disclosure control. These details describe the facts in that row after filters, without inferring a repository; the TUI unknown-repository label is unchanged. These location filters apply only to Repo, while existing date and usage filters continue to apply there. Repo rows count sessions distinctly within each row, so their session counts are not additive across locations. Other views do not consume location filters.

The V1 REST API consists of exactly these endpoints; unversioned `/api/*` routes are not supported:

| Endpoint | Purpose |
|----------|---------|
| `GET /api/v1/instance` | API/server versions, saved data hostname, timezone, capabilities, and initial viewer defaults |
| `GET /api/v1/sync` | Shared sync phase, per-harness progress, error, and completion revision |
| `POST /api/v1/sync` | Start or join the server's all-harness sync job; returns HTTP 202 |
| `GET /api/v1/usage` | Summary, chronological/ranked chart, sorted/paginated rows, and last sync |
| `GET /api/v1/usage/facets` | Provider/model/harness facets and bounded session-ID search |

Usage/facet query parameters are `period`, `bucket`, `from`, `to`, repeated `provider`, `model`, `harness`, and `session`, plus `tab`, `sort`, `direction`, `page`, and `pageSize`. Repo adds `locationGroup` and repeated `repository` and `directory` stable-key filters. The latter parameters require `tab=repo`; other views ignore location selections. Repo API rows include sorted distinct `directoryNames` and `hasUnknownDirectory` for their contributing facts; other tabs return an empty list and false. Session lookup adds literal substring `search`, returning at most 100 values. Facet queries apply all other filters while omitting their own Dimension Filter. React retains selected values even when other facets exclude them. API inputs are validated, sort fields allowlisted, database reads have request deadlines, and failures use the standard `{ code, message }` JSON body. Unknown API routes also return JSON errors.

[`docs/openapi.yaml`](openapi.yaml) is the authoritative, repository-only API contract; the server does not expose it at runtime. `pnpm run generate:api` generates committed Go transport models and TypeScript types/Zod schemas. `pnpm run check-api` verifies generated output has not drifted from the contract. Handwritten handlers map canonical query results into generated response models, while browser query hooks validate responses with the generated schemas. Direct Go builds consume committed generated files and do not require Node or code-generation tools.

V1 API routes do not advertise cross-origin browser access or provide CORS preflight handling. The embedded dashboard uses relative, same-origin API URLs. The server has no authentication; network bindings remain intended for trusted networks.

`packages/web` uses React, strict TypeScript, Vite, Tailwind CSS, local shadcn primitives backed by Radix UI, TanStack Router/Query/Table, and Recharts. Feature components compose through `components/ui`; bespoke CSS is limited to dashboard layout, responsive behavior, and data-visualization geometry. The route path owns the active Aggregation Tab, and validated route search owns dashboard filters, sorting, and pagination. Theme, visible columns, chart metric, and transient popover/search drafts remain local UI state.

Every browser request, including **Sync now** and session search, targets the server serving the page. Users can open that server directly by IP or DNS name, including with `serve --host 0.0.0.0`. The header displays the saved data hostname as plain text; the footer includes the page origin. `/api/v1/instance.hostname` reads the latest completed ingest run, including unchanged-source checks, and returns `unknown` when no recorded hostname is available. Running or failed runs do not replace that display. It never substitutes the serving machine or browser address for the data hostname. Sync revision changes refresh instance metadata without a page reload. There is no add/remove/select-host UI, configurable browser API destination, or persisted host list. Legacy `tokeninsights.sources.v1` browser storage is ignored. Query keys retain filter scope and canonical revision; superseded requests are cancelled and route transitions preserve summary/result semantics. Connection failures offer retry.

Development Vite proxies `/api` to the Go server; its browser requests also remain same-origin.

The route path owns the tab. Route search parameters own period, bucket, custom dates, repeated provider/model/harness/session filters, sort, direction, page, and page size, including browser back/forward and explicitly cleared startup filters. Tab navigation preserves filters, resets pagination, and applies the destination tab's valid default sort when needed. The Graphite & Lime dashboard uses a compact hostname/status/action header, route tabs and quick periods on a shared row, always-visible wrapping horizontal filters, static readouts, a 10rem chart, and dense tables. Summary readouts are compact static data displays on every tab; only the chart toolbar selects the chart metric. Route controls, filters, sync/error feedback, and summaries remain mounted across route changes; only the chart/table region loads, and it never displays rows from the previous route. Theme preference persists locally. CSS typography, color, spacing, and radius tokens use browser-scalable rem/em sizing, with responsive layouts, focus styles, accessible controls, and reduced-motion support. The active route uses lime text and an underline with aria-current. Readouts have no click, hover, tooltip, or selection state. Dark mode uses neutral graphite and bright lime; light mode uses warm white, bright lime action fills with dark text, and deep lime selections and chart ink. The secondary chart series uses a neutral sage gray.

The React visual contract is [`DESIGN.md`](../DESIGN.md), implemented by `packages/web/src/tokens.css`, Tailwind theme utilities, local shadcn primitives under `components/ui`, and feature rules in `styles.css`. All Aggregation Tabs share semantic light/dark colors, a 4px-based spacing scale, three radius roles, aligned page/panel insets, and standard/compact controls with larger touch targets. Narrow layouts retain all seven navigation choices in a horizontally scrollable rail and keep accessible names for icon-only actions. Visual changes must follow that contract without changing canonical analytics semantics.

The pnpm monorepo contains Go production packages and TypeScript development/browser packages. Browser code uses Vite and React. Node scripts use native, erasable TypeScript supported by Node 26+.

Vite output is checked into `packages/cli/internal/server/static` and embedded using `go:embed`, preserving direct Go installs and offline runtime use. The workspace builds React before Go; CI rebuilds and checks generated assets for drift. Node, npm, pnpm, `node_modules`, and repository TypeScript tooling are build-, test-, and development-only. Production is one native Go binary: Go serves embedded browser JavaScript as bytes, the browser executes it, and Go runtime code never invokes a host JavaScript runtime. Web analytics use the canonical token and optional location contracts; lifecycle state is local-only and not an analytics dimension.

## Source coverage and progress

Both viewers show retained-source freshness separately from token totals. REST `/api/v1/sync` returns optional durable progress (job identity, actual timestamps, source counts, discovery completeness, last successful time); revision changes after canonical publication, not merely after overall job completion. `/api/v1/usage` returns day coverage in the same validated snapshot as its analytics, independent of pagination. Days have unverified/pending/partial/checked/empty status, pending/failed source counts, successful checked time, and a nullable filtered canonical total. Unknown/incomplete days never imply zero. Empty means no matching usage found in checked retained sources, not account-wide absence.

Coverage uses serving-machine calendar dates and date/harness filters. Provider/model/session/location filters affect the displayed canonical day total, but never claim that unchecked sources cannot contain matching usage. A source can span days; UTC occurrence bounds are conservative, and unknown or unfinished source bounds affect every selected day. Pending canonical work prevents a checked-empty claim. Failed/unverified coverage never advances its check time. Bounded coverage includes at most 366 days through today; all-time defaults to the recent seven days, while a complete custom bounded range replaces the preset. Future days are omitted. Operational placeholders belong only to presentation; they do not create facts, change session counts or summaries, or manufacture chart activity.

Progress is indeterminate during discovery. Afterwards source counts/percentages describe work checked and sources ready; failures stay explicit. Normalization is a named phase. A processed-source percentage never means completeness of lifetime usage or time remaining. Compatible committed usage can refresh during ordinary sync; recovery data stays hidden until ready. Orphaned jobs show interrupted with retry rather than remaining running forever.

## DB Lifecycle

- `db.Open` opens existing compatible, fully recovered DBs read-only. `db.BeginAnalyticsRead` revalidates lifecycle inside the returned read-only transaction; analytics use that same snapshot. Incompatible or rebuild-pending data is unavailable to analytics; `db.OpenWritable` permits current pending data for internal recovery normalization.
- `db.CreateIfMissing` creates missing DBs from the embedded schema with current generation and no pending rebuild. Help/version never access the database.
- Public sync/normalize and implicit TUI/web sync inspect compatibility, acquire a context-aware database-scoped interprocess writer lock with a named timeout, and recheck under lock. Internal prepared helpers avoid nested lock acquisition. Sync, normalization, and resets share this lock; process exit releases it.
- Recognized older schema or data generations trigger one transactional reset to current, even across skipped generations. Compatible databases are preserved; unknown, corrupt, or newer schema/generation files are rejected without automatic deletion.
- Reset recreates application tables inside the existing SQLite file. Connection PRAGMAs run outside the reset transaction; current schema/generation and pending state commit together. A pre-commit crash preserves old data; a post-commit crash leaves durable pending recovery. SQLite transactions preserve consistent readers; live DB/WAL/SHM files are not unlinked.
- Recovery syncs all configured harnesses and normalizes before clearing pending state and source key only on success. Missing harnesses are normal skips. Failure retains committed partial imports, pending state, and a source-scope hash; same-scope retry resumes using dedupe/refresh without another reset. Mismatched normalized source configurations reject before data writes; source paths are never persisted or automatically reconstructed. Only retained local sources can reconstruct history.
- Targeted default-source recovery expands to all defaults. All-harness custom roots stay bounded to `<root>/<harness>`. Single-harness custom-source recovery defers without mutation until an eligible command runs.
- `--dry-run` previews reset/resume without writes or stale refresh-state suppression. `--no-sync` stays read-only and never repairs data.
- `reset-canonical --confirm` acquires the writer lock and requires compatible, fully recovered data. It deletes canonical token usage, messages, sessions, and normalization diagnostics while keeping raw facts, observations, and source refresh state, then requeues existing raw facts. It cannot repair incompatible raw usage or identity.
- `reset-all --confirm` owns the writer lock and uses the common transactional reset primitive, leaving a fresh current database. It clears source refresh state and pending normalization work. Explicit reset remains available, including when changed source availability requires rebuilding previously ambiguous Codex history.

## Invariants

Must not change silently:

- schema changes require explicit user approval;
- `schema/schema.sql` remains the table source of truth;
- canonical token usage must be session-centric;
- missing model and unavailable provider must render with canonical fallback values, not cause row loss;
- raw storage must remain metadata-only and avoid private content;
- Syncable Analytics Data must exclude prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, and full source paths;
- Local-only Continuity Metadata must never be used for viewer analytics or future cloud export;
- default token analytics use only countable canonical token rows;
- unavailable metric domains must not appear as empty active viewer tabs;
- cost tracking must stay out of the active product;
- the TUI queries canonical data read-only during and after optional Implicit View Sync, and `view --no-sync` remains a read-only command path.

Can evolve with care:

- new harness adapters;
- new canonical fact domains for TPS, requests, or tools;
- explicit conflict precedence and richer diagnostic categories;
- generated schema constants;
- future checkpoint plugins that write equivalent raw/canonical concepts.

## File Organization

| Path | Role |
|------|------|
| `schema/schema.sql` | SQLite schema source of truth |
| `packages/cli/internal/db/schema/schema.sql` | embedded checked schema copy |
| `tools/build/src/check-schema.ts` | schema contract validator |
| `docs/openapi.yaml` | authoritative, repository-only REST API contract |
| `tools/build/` | private Node 26+ native TypeScript build/test/development tooling package |
| `packages/cli/cmd/tokeninsights/main.go` | CLI executable entry point |
| `packages/cli/internal/cli/commands.go` | command dispatch and thin orchestration |
| `packages/cli/internal/cli/flags.go` | view flag parsing |
| `packages/cli/internal/cli/serve.go` | web command flags and orchestration |
| `packages/cli/internal/viewer/filters.go` | shared calendar and filter semantics |
| `packages/cli/internal/server/` | HTTP lifecycle, sync coordination, generated API models, handlers, embedded assets |
| `packages/web/` | typed same-origin React dashboard, generated API schemas, and design tokens |
| `packages/cli/internal/cli/table.go` | interactive TUI model |
| `packages/cli/internal/cli/desk.go` | Instrument desk layout, readouts, and drawers |
| `packages/cli/internal/cli/theme.go` | semantic light/dark terminal colors and styles |
| `packages/cli/internal/cli/statusline.go` | typed dashboard statusline state and width-aware rendering |
| `packages/cli/internal/cli/table_summary.go` | pinned table summary state and width-aware rendering |
| `packages/cli/internal/cli/render.go` | table rendering |
| `packages/cli/internal/db/open.go` | DB open/create/reset/schema lifecycle |
| `packages/cli/internal/db/schema.go` | Go schema constants |
| `packages/cli/internal/db/aggregate.go` | canonical aggregation queries |
| `packages/cli/internal/db/events.go` | canonical event rows for UI model |
| `packages/cli/internal/db/filter_values.go` | canonical filter value discovery |
| `packages/cli/internal/db/viewer_summary.go` | full-filter token/session summary and session-ID lookup |
| `packages/cli/internal/db/session_counts.go` | shared shown vs all-synced canonical session counts |
| `packages/cli/internal/pipeline/adapters.go` | harness adapter interface and registry |
| `packages/cli/internal/pipeline/opencode_sqlite.go` | OpenCode durable SQLite adapter |
| `packages/cli/internal/pipeline/codex_jsonl.go` | Codex JSONL session adapter |
| `packages/cli/internal/pipeline/pi_jsonl.go` | Pi JSONL session adapter |
| `packages/cli/internal/pipeline/claude_code_jsonl.go` | Claude Code JSONL transcript adapter |
| `packages/cli/internal/pipeline/sync.go` | raw ingest and observation pipeline |
| `packages/cli/internal/pipeline/normalize.go` | canonical normalization and diagnostics |
| `packages/cli/internal/pipeline/pipeline_test.go` | fixture-style sync/normalize conformance tests |
| `packages/cli/testdata/conformance/sync-first-basic/` | shared CLI-owned development and conformance fixture |
| `packages/cli/internal/pipeline/testdata/conformance/` | pipeline-only conformance fixtures |

## Testing And Verification

Run before schema or pipeline changes are considered done:

```sh
pnpm run format:check
pnpm run lint
pnpm run check-schema
pnpm run test
pnpm run build
```

Use `pnpm run format` to apply `gofmt` and Oxfmt. Verification uses `gofmt` checks plus the pinned `golangci-lint` for Go, and Oxfmt plus Oxlint for TypeScript, JavaScript, and React.

Use focused Go tests during iteration:

```sh
cd packages/cli
go test ./internal/pipeline
go test ./internal/db
go test ./...
```

The shared `sync-first-basic` fixture lives under `packages/cli/testdata/conformance/`; pipeline-only conformance fixtures remain under `packages/cli/internal/pipeline/testdata/conformance/`.
Fixture sources may include harness-native durable stores, such as synthetic OpenCode SQLite setup SQL, and expected raw, observation, canonical, and diagnostic outputs are JSON so future non-Go writers can reuse the shared contract.

`sync-first-basic/source/` is also the shared development source fixture. It contains compact representative OpenCode, Pi, Codex, and Claude Code data, roughly two sessions and two canonical facts per harness. Source structures reflect durable harness formats, but every retained value is synthetic. Fixtures must exclude conversation content, tool arguments/output, request headers, secrets, real user or repository paths, signatures, and other identifying data. Raw local harness databases and transcripts must never be copied into the repository.

`pnpm run dev:data` builds the Go CLI without rebuilding web assets, recreates the ignored `.tokeninsights-dev/` directory, copies the sanitized JSONL sources, materializes OpenCode SQLite from its reviewable `source.sql`, and syncs all harnesses into `.tokeninsights-dev/tokeninsights.sqlite`. `dev:cli` builds and prepares that data before opening the all-time, no-sync TUI. `dev:server` builds and prepares the same data before running the Go REST server on loopback. `dev:web` runs Vite directly, binds to all IPv4 interfaces, and proxies `/api` to the Go server at `127.0.0.1:8765`. `dev:web:mock` runs Vite independently with contract-validated Mock Service Worker responses and requires no Go process or local harness data. `dev` runs both real server commands in parallel. `start:web` remains unchanged and uses normal local sources. Web browser tests retain their separate generated 80-session synthetic dataset because pagination requires more rows than the compact shared fixture.
