# tokeninsights Design

Current collector/server architecture and storage contract. See the
[architecture overview](collector-server-architecture.md) and
[failure-test matrix](collector-ingestion-tests.md) for traceable decisions,
production coverage, and remaining verification gaps.

## North Star

Track local token usage across supported coding harnesses over time, without relying on vendor dashboards.

The durable data model is a session-centric token time series. Every canonical token row must resolve to a stable `session_id` through `canonical_sessions`. Raw facts may preserve missing source values as null, but token facts without stable session identity must not enter canonical analytics.

Token usage is the active V1 viewer domain. TPS remains a future-compatible data domain when durable timing facts exist, but unavailable metric domains should not appear as empty active viewer tabs.

## System Architecture

```text
Durable harness sources: OpenCode / Pi / Codex / Claude Code
                            |
                   host collector: sync
            discover -> parse -> normalize -> journal
                            |
                      collector.sqlite
               raw facts + canonical facts + continuity
                 immutable journal + saved upload batches
                            |
              normalized-only versioned ingestion API
                            |
             shared local / foreground remote server
                 validate -> dedupe -> atomic commit
                            |
                        server.sqlite
             canonical history + durable receipts + revision
                            |
                 REST queries and embedded assets
                     /                   \
                  TUI                   browser
```

The collector owns harness parsing and normalization. The server cannot discover
sources, interpret raw harness facts, or launch collection. Local is the default
deployment; explicit remote transport uses the same normalized contract and
skips local startup. Provisioning and TLS deployment remain later work.

Defaults under `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/` are
`collector.sqlite` and `server.sqlite`. Override them with `--collector-db-path` /
`TOKENINSIGHTS_COLLECTOR_DB_PATH` and `--server-db-path` /
`TOKENINSIGHTS_SERVER_DB_PATH`. `TOKENINSIGHTS_SERVER_URL` selects an explicit
endpoint and `TOKENINSIGHTS_SERVER_TOKEN` supplies its token. Flags override
environment defaults. Legacy `TOKENINSIGHTS_DB_PATH` does not select either new
role. The old `tokeninsights.sqlite` remains untouched; retained sources rebuild
the fresh files. Aliased paths and wrong database roles are rejected.

## Product Boundary

- `sync` defaults to all harnesses: capture changed durable data, normalize,
  journal canonical changes, then publish pending batches. `--harness` narrows
  collection. Collection and delivery report independently.
- `sync --publish-only` sends retained journal work without discovering sources.
  Retry is manual; offline publication stays durable until another invocation.
- `normalize` processes retained collector raw facts and journals canonical
  changes. It does not publish; a subsequent sync performs delivery.
- Bare invocation ensures the local canonical server and prints its URL.
  Startup initializes only missing server storage and never collects.
- `view` queries saved server data through REST. Without a server URL it ensures
  the local server; an explicit URL bypasses local storage/discovery.
  `--no-sync` aliases this read-only default. `--sync` explicitly runs caller-side
  collection/publication before opening. TUI `r` and browser Reload query only.
- `service start|stop|restart|status|run` manages the local server.
  `server run` provides the shared foreground remote composition; non-loopback
  binding requires a token. `serve` remains a deprecated foreground alias.
- `reset-canonical` and `reset-all` affect collector storage only. Rebuilding a
  collector does not remove server facts or publish implicit retractions.
- Completion plugins are thin triggers for the same Go `sync`, never alternate
  parsers or transports. Manual collection remains primary; host hooks do not
  prove that every durable write was available. See [plugins](plugins.md) for
  native artifacts, supported surfaces, and remaining real-host verification.

## Code And Process Boundaries

One Go module owns the native product and release lifecycle; pnpm owns browser
and plugin development/build tooling. Browser JavaScript is prebuilt, committed,
embedded with `go:embed`, and executed only in the browser. The Go binary never
invokes Node, npm, or pnpm. Stable Go tags and releases remain manually published
from `main`; see [release details](release.md).

```text
collector CLI -> internal/collector -> pipeline + collectorstore
                                       |          |
                                       +-> publication contract <-+
                                                                 |
local/remote runtime -> ingestion -> serverstore -> canonical SQL |
       |                                                         |
       +-> REST queries -> Go queryclient / embedded React         |
```

`internal/publication` owns the normalized domain/codec/identity contract without
harness or storage dependencies. `internal/collectorstore` owns journal snapshots,
destinations, saved request bytes, and acknowledgement progress.
`internal/ingestion` owns validation, stable fact uniqueness, transactional
receipts, and bounded admission; `internal/serverstore` owns canonical-only
storage compatibility. Existing canonical query implementations are reused
through `db.Reader`, without opening collector databases from viewer paths.

## Current Implementation Status

Collector schema V15/data generation 6 and server schema V1 are separate roles.
The normalized journal, saved batches, manual HTTP publication, transactional
server ingestion/receipts, REST TUI, and read-only browser are implemented.
Verified continuity and exact additive token accounting remain adapter-owned.

Limits remain explicit: single configured owner, one destination selected per
invocation, no automatic retry, no retractions or legacy import, no remote
provisioning/TLS/account administration, and no active timing-only tabs.
Missing native identity or unsupported competing normalized facts are withheld
with local publication diagnostics. Verified byte cursors cover eligible Pi
files; changed OpenCode/Codex/Claude sources still full-parse where required.
General harness revision ordering is not guessed from arrival time.

## Persistent service ownership

`internal/service` composes the shared ingestion core, canonical queries, public
HTTP, and private lifecycle control. The detached local server re-executes the
Go binary with configuration/readiness inherited through descriptors. Foreground
`service run` / `server run` reuse ownership and server runtime. Server startup
has no captured harness roots or source refresh queue.

Canonical database paths share SHA-256 lifecycle identity across symlink aliases;
existing hard-linked server aliases are rejected. Persistent operation/lifetime
lock inodes serialize lifecycle and ownership; collector writes use their own
database writer lock. Never unlink live lock inodes. Held but unreachable server
ownership blocks takeover. Busy ports fail without killing unrelated processes.

Saved configuration is private versioned JSON under
`$XDG_CONFIG_HOME/tokeninsights/services/`. Logs live under
`$XDG_STATE_HOME/tokeninsights`, bounded to 4 MiB plus three backups. Discovery and
Unix sockets use `$XDG_RUNTIME_DIR/tokeninsights` or private state/runtime, with
a fallback discovery record for SSH/local clients. Config stores bind/database
settings, never source roots or the full environment. Runtime instance identity
is transient; the server database ID persists across restarts and binds delivery.

Startup creates private service directories with mode `0700` and tightens owned
legacy directories through descriptors without following a final symlink.
Symlinks, non-directories, and other-user ownership are rejected; shared XDG
parents remain unchanged and status is read-only. Public data reads expose
runtime/database identity and canonical revision. Neither public query endpoints
nor private control can start parsing or normalize server data. No periodic
collection, watcher, or login/reboot autostart is implemented.

The [earlier service decision](adr/0005-persistent-service-and-explicit-refresh.md)
is historical; its server-owned refresh workflow is superseded by the
[collector/server architecture](collector-server-architecture.md).

## Schema Contract

`schema/schema.sql` defines collector tables and is embedded at
`packages/cli/internal/db/schema/schema.sql`; `schema/server.sql` defines server
tables and is embedded at `packages/cli/internal/serverstore/schema/server.sql`.
Compatibility validates `PRAGMA application_id` before mutation and
`PRAGMA user_version` plus role-specific metadata. Collector schema is `15`,
data generation `6`; server schema is `1`, with identity/semantics version `1`.
Release versions are not compatibility markers.

Fresh collector/server databases replace the default use of the legacy mixed
file. Existing legacy files are rejected by role checks rather than silently
upgraded/imported. Collector compatibility/rebuild machinery stays local; server
facts and receipts cannot use destructive reset/resync recovery. Unsupported
server compatibility fails without deleting historical facts. Future preserving
migrations must be explicit, independently tested changes.

Structural or cross-language contract changes require explicit approval. Update
source SQL, embedded copies, role/version constants, tests, and affected docs
in one task; `pnpm run check-schema` checks both contracts.

### Historical collector schema evolution

The following records earlier mixed/local schema decisions. They do not promise
legacy-file migration into the new collector/server roles.

Schema V4 adds the persisted `claude-code` harness value. Existing V3 databases reject that value physically through SQLite `CHECK` constraints.

Schema V5 adds canonical provider provenance through `canonical_token_usage.provider_source`.

Schema V6 adds `normalization_work_queue` for pending canonical-domain work.

Schema V7 adds `source_refresh_state` for Local-only Continuity Metadata used by Recent Source Refresh.

Schema V8 adds `database_lifecycle` for local compatibility and resumable rebuild state. Recognized older schema/data now recover automatically through a transactional in-place application-table reset and all-harness normalized reingestion. This supersedes the former manual `reset-all` upgrade requirement; it is not a row-preserving schema migration.

Schema V10 retains optional `usage_locations` metadata and `location_id` on raw and canonical token facts, with only repository and directory identities. Data generation 3 introduced location attribution; generation 4 refreshed display paths; generation 5 removes worktree and branch attribution. Each upgrade triggers a full reset and resync. The database is reconstructable from retained source artifacts; usage whose artifacts were deleted may disappear after recovery. Sync checks recorded source directories against current Git metadata when those directories exist. A path reused by a different repository can therefore attribute older facts to the checkout present at sync time; provenance records the origin of repository values.

Schema V11 adds `source_cursor_state` for verified Pi JSONL byte offsets and same-version source fingerprints. Upgrading from V10 uses the existing full reset and resync recovery, so usage whose original source artifacts are gone may disappear.

Schema V12 adds per-harness `normalization_rule_state`. V11 databases upgrade transactionally without deleting raw or canonical usage; normalization then refreshes identifier rules once. Older incompatible schemas retain reset and resync recovery.

V15 adds immutable normalized publication journal/entities, per-destination
acknowledgements, saved batches/receipts, and an explicit collector application
role. Data generation 6 includes native OpenCode distinction and Claude
source-timed native-request identity/revision rules. Server V1 contains only
canonical sessions/messages/locations/usage, server metadata, producer labels,
and durable ingestion receipts.

## Data Model

### `database_lifecycle`

Singleton Local-only Continuity Metadata, excluded from server ingestion and analytics:

- `id`: constrained to `1`.
- `data_generation`: current semantic/identity compatibility generation. Generation `1` corrected Codex cumulative/replay accounting; generation `2` makes input/cache and output/reasoning components additive across harnesses, hardens source counters, and aligns OpenCode V1/V2 message identity; generation `3` rebuilds raw and canonical facts with location attribution; generation `4` rebuilds display paths; generation `5` rebuilds repository/directory identities without worktree/branch fields; generation `6` establishes native identity/revision compatibility and normalized publication.
- `rebuild_pending`: recovery remains incomplete until all configured harnesses sync and normalize successfully.
- `rebuild_source_key`: hash of the normalized recovery source configuration, NULL when ready and nonempty while pending; stores no source paths.
- `updated_at_ms`: lifecycle update time.

Fresh databases start at the current generation with no pending rebuild and a NULL source key. Reset commits the current schema/generation, pending state, and source-scope fingerprint atomically. Failed recovery preserves that state and any committed partial imports for a same-scope retry; successful completion clears pending state and the source key together.

Collector `ingest_runs.hostname`, introduced in V14, records the collecting machine on source attempts, including unchanged-source checks; missing lookups remain NULL. Server producer labels are separately recorded from committed normalized delivery metadata.

### Collector-local sync status (introduced in V13)

`sync_state` is a local singleton with a durable collector normalization revision and the last successful normalized all-harness sync time. It advances in the canonical transaction, including standalone normalization and canonical resets, and when terminal harness/job coverage commits. Existing per-source ingest completion is no longer presented as overall sync success.

`sync_jobs` records metadata-only scope fingerprints, running/completed/failed/cancelled/interrupted outcome, phase, actual timestamps, normalization policy, and all-harness scope. `sync_harnesses` records discovery, source counts, status, and successful checked time for each job/harness. `sync_sources` records hashed source identities, reading/ingested/ready/unchanged/failed/deferred state, safe error codes, and conservative UTC usage-time bounds. No full paths or transcript content are added. These tables are Local-only Continuity Metadata, excluded from server ingestion and analytics.

These local job tables do not imply current producer completeness to the server, and their prior successful checks are not uploaded as server freshness.

One database writer lock owns a job. Status reads observe that lock without creating it; orphaned running jobs read as interrupted and the next owner persists their interrupted outcome. Collector CLI writers wait with a named waiting phase; server queries do not join these jobs. Scope-changing contenders remain serialized. Recovery creates its job after transactional reset and retains it on retry; analytics remain unavailable until all normalized recovery work completes.

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

Raw facts remain metadata-only and collector-local. Server ingestion accepts only the normalized publication allowlist; it never includes raw facts or parser diagnostics.

### `raw_observations`

One row per ingest-run sighting of a raw fact.

Repeated syncs of the same source should not duplicate `raw_token_usage`, but they may append new `raw_observations`.

### Source Refresh State

`source_refresh_state` is best-effort Local-only Continuity Metadata. It exists only to reduce repeated local Durable Source parsing and must not be sent to server ingestion or used for server analytics.

Current source state properties:

- keyed by `harness`, `source_kind`, and an adapter-provided metadata-safe source state key;
- records parser/collector provenance used to decide whether a cursor can be trusted;
- stores last successful source refresh time, observed source file modification time, and observed source file size;
- avoids raw JSONL lines and full source paths unless a specific adapter cannot maintain continuity without them;
- is cleared by `reset-all --confirm` and preserved by `reset-canonical --confirm`.

`source_cursor_state` is separate Local-only Continuity Metadata. Eligible Pi files persist a byte offset, file size, mtime, parser/collector identity, hashes of the processed prefix and prior boundary, and a hashed location fingerprint. Ordinary Codex and Claude Code sources persist a full content fingerprint and current location fingerprint. Codex forks use `cursor_kind='codex-ancestry-v1'`: `prefix_hash` binds child content, `location_fingerprint` binds child attribution, and `boundary_hash` binds the complete ancestry's content, locations, source/session identities, parent links, and parser/collector provenance. Existing fork sources establish these markers once, without resetting usage or upgrading schema. Missing, conflicting, cyclic, unreadable, or unstable ancestry cannot establish reusable markers.

OpenCode hashes parser-relevant V1/V2 message rows and table definitions in a consistent SQLite read snapshot shared with parsing; its location fingerprint binds each session ID to current directory/repository attribution. Unrelated SQLite writes and WAL checkpoints do not invalidate the OpenCode marker. No path or transcript content is stored. Markers advance only in the source ingest transaction after parsing and raw writes succeed. A changed prefix, changed checkout attribution, same-size rewrite, truncation, nonterminated line, changed parser/collector, or uncertain Pi session header causes a full parse. Changed Codex, Claude Code streaming copies, and mutable OpenCode rows retain full parsing.

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

### Collector publication state

`publication_state` stores the delivery stream and supported identity/semantics
versions. Stream IDs identify delivery histories, not usage facts.
`publication_journal` stores immutable normalized snapshots ordered by sequence;
`publication_entities` points to each fact's current snapshot.
`publication_destinations` binds an endpoint/database identity to its contiguous
acknowledged sequence. `publication_batches` stores immutable request bytes/hash,
sequence range, and the exact validated receipt. One pending batch per destination
survives retries. No-op canonical values create no journal work; changed reference
occurrence envelopes can require publication without adding a usage contribution.

### Canonical server state

`server_metadata` stores a persistent database ID, fixed owner `default`,
identity/semantics versions, canonical revision, and last committed ingestion.
Server canonical sessions/messages/locations/usage share query columns with the
collector projection but omit raw/run references. Usage has a stable fact key,
payload hash, and supported source revision evidence. Server session occurrence
envelopes merge ranges; message occurrence envelopes preserve the earliest source
time without identifying another contribution.

`ingestion_receipts` persists each stream/batch request hash, sequence range,
insert/update/no-op counts, commit time, and revision. `ingestion_producers`
records optional uploaded hostnames; multiple labels display `multiple machines`.
These labels do not determine fact identity or ownership.

## Sync Pipeline

`tokeninsights sync`:

1. Defaults to all harnesses; `--harness` narrows collection and `--all` makes the default explicit.
2. Discovers selected harnesses concurrently, before source processing, so source-work totals are known.
3. Preloads a harness's continuity markers in one query.
4. Prepares verified reuse decisions, candidate facts, diagnostics, bounds, and markers outside write transactions using bounded workers.
5. Commits prepared sources in discovery order through one writer: creates `ingest_runs`, deduplicates raw facts, enqueues newly inserted facts, writes observations/diagnostics/markers, and completes source status and counters atomically.
6. Normalizes and journals canonical changes after each harness unless `--no-normalize` or `--dry-run` is set.
7. The collector sends saved pending batches after local work, including retained batches when collection failed; dry runs skip delivery. It reports local and delivery outcomes separately.

Each changed source ingest is transactional. If raw writes, markers, or completion bookkeeping fail after the run is created, a savepoint rolls back that source's writes and commits its failed audit when possible. If the failure audit cannot commit, the entire transaction rolls back. In-memory dedupe and summary state advance only after commit. An unchanged source still receives a completed lightweight ingest run with zero facts and observations; unchanged completion and reading status are batched at 64 sources or 250ms, flushing before publication and job completion.

Workers derive their budget from `GOMAXPROCS`, with at most twice that many sources outstanding, including queued and out-of-order results. Only the coordinator mutates summaries and publishes progress. A slow earlier source bounds further preparation; cancellation and writer failures cancel and join workers. Codex parse/verification and Git inspection use synchronized per-job futures. Ancestors resolve inline rather than waiting for an unscheduled task in the worker pool; ancestry cycles are rejected before recursion.

`sync --dry-run` uses the same concurrent discovery, source preparation, and continuity checks to report facts that would be parsed, without writing jobs, runs, markers, or usage. When recovery is needed, it previews reset/resume and rebuild parsing without stale refresh-state suppression or database mutation.

`sync --full-refresh` ignores source refresh state for the requested harness scope and full-parses discovered sources using the existing parser behavior. Successful source ingest updates source refresh state after commit. Full refresh does not requeue all existing raw facts for canonical rebuild by default; only newly inserted raw facts enqueue pending normalization work.

Identifier rules apply while writing canonical facts. Normal sync and `normalize` also refresh previously written canonical identifiers from linked raw facts, including when no new raw work is pending. Raw source identifiers remain unchanged.

Source continuity rules:

- Source age, size, or mtime alone never proves reuse; the previous 48-hour metadata-only skip is removed. Content checks detect rewrites even when size and mtime are preserved.
- JSONL verification combines content hashing and location extraction in one scan, then verifies content stability. New or known changed sources capture fingerprints from the exact parsed bytes instead of scanning again for location metadata. Necessary rewrite verification reads remain.
- Eligible Pi files hash the processed prefix and verify its boundary/header/location. Append parsing carries that hasher into the suffix, avoiding rereading merely to construct the advanced cursor. A final content verification protects against rewrites during parsing. Unchanged Pi files verify their prefix without parsing token records.
- OpenCode fingerprinting and parsing share one SQLite read snapshot; there is no post-parse table fingerprint scan. A changed database snapshot is handled on the next sync. Native row ordering alone cannot replace full parsing because prior rows are mutable.
- Codex fork markers require complete ancestry proof. Parent parses and verification are shared within the job; uncertain histories retain existing diagnostics and full parsing behavior.
- Prepared JSONL sources and dependencies are checked again for metadata/file replacement before writing. Detected changes defer coverage and clear markers; no marker advances beyond incomplete tails.
- An incremental run writes observations only for facts actually parsed. Raw dedupe remains the correctness guard when parsing repeats.
- Normalization processes pending work and refreshes canonical identifiers once per rule signature. Pending-work checks use existence queries.
- Cursor invalidation remains collector-local operational metadata, independent of published fact identity and destination acknowledgement progress.
- `sync --full-refresh` ignores continuity markers without requeueing all existing raw facts for canonical rebuild.

`sync --all` attempts all requested harnesses and continues later sources after recoverable read/parse failures. Successful source scopes still normalize when one source or harness fails; the command exits non-zero if any requested scope fails. Database write failures and cancellation stop the job. Per-source transaction rollback cannot leak in-memory dedupe state. Source ingest uses an injected wall clock for actual start/completion times; source observations retain the command observation time.

JSONL readers use Reader with a captured file extent, cancellation checks between chunks, and no fixed Scanner record limit. Large irrelevant Codex tool/output records do not block later usage, and large usage-bearing records remain countable. An unfinished trailing JSON record is deferred, not diagnosed as corrupt; its source coverage stays pending and no reusable fingerprint/cursor advances past that tail. Complete valid JSON without a trailing newline remains readable, without establishing a byte cursor.

With `sync --all --source-dir <root>`, harness discovery is bounded to `<root>/<harness>`. Harnesses whose subdirectory is absent are skipped. Single-harness sync with `--source-dir` still scans the provided directory directly for ad hoc fixtures.

Before normal data work, public sync/normalize coordinate compatibility recovery under a database-scoped writer lock. Targeted default-source commands recover all default harnesses first; all-harness `--source-dir` recovery stays within the specified root. Single-harness custom-source recovery defers without changing the database. Recovery always normalizes before satisfying the requested scope, including `--no-normalize`; missing installations remain normal skips. Failed recovery preserves partial imports but keeps analytics unavailable until the all-harness rebuild succeeds.

Pending recovery must resume with the same normalized source configuration. Default-source keys fingerprint the effective OpenCode, Pi, Codex, and Claude Code roots, including environment overrides. Custom all-harness keys fingerprint the normalized canonical absolute root. Only a hash is persisted; full paths cannot be reconstructed from it. Mismatched retries, including dry-run attempts, are rejected before data writes. Repeat the original `--source-dir` and `--collector-db-path`, preserving source environment settings. Default-source normalization cannot finish a custom-root rebuild; server/TUI/browser startup never performs collector recovery.

## Normalized Publication And Ingestion

Stable IDs are SHA-256 hashes of unambiguously JSON-encoded versioned native
identity tuples. Session identity uses owner/harness/native session. Message
identity adds the native message. Fact identity adds native request evidence
where available and usage kind. Fact identity does not directly use mutable normalized counters/labels,
installation, local row IDs, batch/stream IDs, capture clock, or destination.
Codex retains its adapter-derived event witness, including a source snapshot
fingerprint; arbitrary changes to such snapshots are not a supported revision
rule. Verified ancestry preserves original fact ownership. Identical counters are not dedupe
proof. Reprocessing retained bytes in fresh collector storage must reproduce
published IDs.

Normalization journals supported facts in its canonical transaction. Missing
source occurrence, missing message/request identity, invalid normalized values,
or competing unsupported native revisions are withheld with metadata-only
`publication_*` diagnostics. The server receives no raw facts, source roots,
parser provenance, diagnostics, cursors, transcript text, or collector-local
full directory paths. Location references publish stable keys and basename labels.

Delivery negotiates protocol/identity/semantics V1 and the destination database
ID. Batches are self-contained fact/reference snapshots with contiguous journal
ranges, at most 256 entries and 1 MiB encoded body. Strings are at most 256 bytes;
integers and aggregate token columns stay within `0..9007199254740991`.
Explicit counter presence, integer type, additive totals, and reference identity
are validated. Unknown/private fields, duplicate JSON keys, incompatible versions,
invalid references, and excessive bodies fail before success acknowledgement.

Collector saves exact batch bytes before transport. The server transaction
applies references, inserts/no-ops supported stable facts, records the batch
receipt and producer label, and advances metadata. `200 OK` means committed and
queryable. Replaying an identical stream/batch returns its saved receipt; changing
its request bytes conflicts. A new batch containing identical facts is also a
successful no-op. Rebuilding the collector creates a new delivery stream while
stable fact IDs still dedupe history.

Changed payloads require supported source revision evidence. Claude's
`claude-source-timestamp-v1` uses the native request snapshot timestamp: newer
replaces, older contributes no update, equal-time differing payloads conflict.
Other immutable fact payload conflicts reject the entire batch. No arrival-time,
collector sequence, upload clock, or universal largest-counter precedence exists.
Session range merging can advance server revision without adding usage.

Admission allows four concurrent ingestions, with SQLite serializing writes and
`busy` failures remaining retryable. A rejected batch commits neither a visible
prefix nor success receipt. Earlier acknowledged batches remain committed when a
later batch fails. Collector acknowledgement checks database/stream/batch/range,
request hash, counts, and receipt against its saved bytes before atomically saving
receipt and advancing progress. Unknown network/commit outcomes keep pending work;
a later manual invocation retries it. There is no asynchronous server processing
queue or background delivery agent. Pending counts can be unknown before a
successful destination binding; CLI output distinguishes this from zero.

Local delivery binds endpoint plus persistent database ID; a replacement local
server starts a new binding and replays retained journal work. An explicit remote
endpoint stays pinned to its negotiated database identity and refuses silent
rebinding. No automatic backflow, pruning, retractions, or legacy import is
implemented. Collector deletion/source disappearance never means server deletion.
Detailed failure fixtures and executable guarantees live in
[the failure contract](collector-ingestion-tests.md).

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

OpenCode sync parses durable V1 and V2 SQLite databases named `opencode.db` or `opencode-<channel>.db` from `${XDG_DATA_HOME:-~/.local/share}/opencode`. Sessions marked archived remain in these databases and are included. It reads V1 assistant rows from `message.data` and V2 assistant rows from `session_message.data`, maps their message-scoped token, model, provider, and timing metadata, and uses stable row/session IDs. The SQLite row ID is authoritative in both layouts; embedded V1 JSON IDs cannot split one migrated message into two canonical identities. OpenCode's stored input excludes cache tokens and its stored output excludes reasoning, so the five components are directly additive. Component sums that exceed SQLite's signed integer range are rejected with a diagnostic. In mixed migrated databases, usable V2 rows take precedence for the same session/message; V1 remains the fallback when the V2 row has no usable token data. Copied channel rows are suppressed only when native session/message IDs and source usage match; equal counters or timestamps across distinct native requests do not prove copies. OpenCode-specific parser provenance forces one automatic reparse when V2 support is introduced; canonical semantic keys prevent analytics duplication. A successful logical fingerprint of message rows and session attribution skips later unchanged parses, including when unrelated tables or WAL files change. Missing or changed markers fall back to the full table parse. True OpenCode row cursors are not implemented.

Pi sync parses durable JSONL session files under `~/.pi/agent/sessions`, including one nested project directory level. Pi has no harness-owned archive; sessions moved to OS trash are outside the durable-source boundary and are not parsed. It uses assistant message usage as exact message-scoped token facts. Current Pi input excludes cache tokens, while reasoning is an optional subset of output; the adapter subtracts reasoning from output. For legacy rows whose total proves input still includes cache, it subtracts cache read/write from input. A source `totalTokens` is retained only when it equals the normalized component sum; otherwise canonical total falls back to the components with a diagnostic. Session identity comes from the session header when available and may fall back to the filename session suffix. Pi continuity uses metadata-safe source keys, parser/collector provenance, verified prefix/boundary hashes, and current location attribution. A valid byte cursor parses only appended records; uncertain continuity falls back to full parsing.

Codex sync parses rollout JSONL session files under `${CODEX_HOME:-~/.codex}/sessions` and `${CODEX_HOME:-~/.codex}/archived_sessions`. It uses `event_msg` records with `payload.type == "token_count"` as exact message-scoped token facts. Codex parsing is stateful: the first usable `session_meta` establishes owning session identity, with the existing filename fallback; later copied metadata cannot replace ownership. `turn_context` or `task_started` provides turn/model state, `last_token_usage` provides countable token components, and `total_token_usage` provides snapshot identity and duplicate/stale guards rather than additive usage. Codex input includes cached input and output includes reasoning; the adapter subtracts those subsets into mutually exclusive canonical components.

The per-sync Codex adapter indexes explicit `forked_from_id`, `source.subagent.thread_spawn.parent_thread_id`, and top-level `parent_thread_id` within discovered source boundaries. Consistent parent references are accepted. A concurrent ancestor resolver caches metadata, verification, and parsed facts so shared parents full-parse at most once per sync. Ordinary sources reuse verified content/location markers; fork/subagent sources additionally require complete ancestry markers. Missing or uncertain ancestry falls back to full parsing, preserving cross-source resolution.

Verified parent-history copies require explicit ancestry plus matching turn, provider/model, and complete last/cumulative token metadata. Replay timestamps may be rewritten and are not match keys; ordinals and `history_mode` do not prove boundaries. Verified copies emit the original source/session/message identity, occurrence time, provider/model, and tokens with current observation/provenance. Raw and canonical dedupe count them once regardless of source order or skipped parent ingestion. Deterministic snapshot fingerprints distinguish different facts sharing a timestamp; line identity is retained when cumulative identity is absent. Pending model/turn backfill preserves snapshot and line information.

Missing/unreadable parents, conflicting ancestry, ambiguous source copies, cycles, or unproven replay retain uncertain child usage with metadata-only diagnostics. Optional parent failures do not discard child usage; normal source failures retain normal failure behavior. Replay resolution precedes local duplicate/stale handling so inherited counters cannot suppress genuine child requests. Unresolved fork history conservatively resets suppression baseline at explicit new turns, with diagnostics for ambiguous boundary snapshots; ordinary-session and same-turn suppression remain. Automatic reconciliation of previously imported ambiguous history after parent availability changes is out of scope. Collector reset can reparse retained sources, but does not retract prior server facts; missing ancestry cannot be retroactively repaired on the server without an explicit future reconciliation policy.

Claude Code sync parses retained local JSONL transcript files under `${CLAUDE_CONFIG_DIR:-~/.claude}/projects` regardless of UI/server archive state; cloud-only archived sessions are outside the local durable-source boundary and are not parsed. Single-harness `--source-dir` scans the provided directory directly, and `sync --all --source-dir <root>` scans `<root>/claude-code`. It uses assistant message `usage` metadata as derived message-scoped token facts, with the file stem as the fallback session identity and top-level `sessionId` as the parent session identity when present. This attributes sidechain/subagent usage to the user-visible parent session when Claude Code records that parent. Message identity uses `message.id` when present and falls back to the row `uuid`. Allowlisted `request_id` metadata retains native request identity locally. Streaming assistant rows are scoped by native session/message/request identity using unambiguous tuple encoding. The latest source-timed snapshot replaces the entire earlier snapshot before normalization; independent component maxima are never synthesized. Canonical fact keys exclude mutable occurrence time for stable Claude message/request identity. Older copies cannot overwrite newer canonical snapshots; equal-time conflicting usage aborts source preparation or normalization atomically. Copied transcript facts are suppressed by logical dedupe keys that do not include full source paths. `cache_read_input_tokens` maps to cache read tokens, `cache_creation_input_tokens` maps to cache write tokens, and `output_tokens_details.thinking_tokens` is subtracted from inclusive output into reasoning when valid. A source total is retained only when it equals the normalized component sum. Claude Code explicit provider values are preserved with provider source `explicit`. When Claude Code artifacts omit provider metadata, canonical rows use provider `maybe-anthropic` with provider source `inferred`. Model values come from explicit model fields, and missing models canonicalize to `unknown`. Claude Code reuse verifies full content, current location attribution, metadata-safe source identity, and parser/collector provenance; missing or changed markers fall back to full parsing.

Harness-specific source parsing stays behind the adapter interface and feeds the same raw-to-canonical pipeline. Missing provider/model remains null in raw facts. Canonical provider provenance records whether a provider was explicit, inferred, or unknown. JSON counters are decoded as exact signed integers; fractional, overflowing, and component-sum-overflow values are rejected. Canonical input/cache and output/reasoning columns are mutually exclusive, so `total = input + output + reasoning + cache read + cache write` whenever a valid authoritative source total is unavailable.

Location is optional, fact-level metadata. OpenCode uses session directory and Git project metadata, Pi uses session-header `cwd`, Codex uses turn/session `cwd` plus recorded session Git remote, and Claude Code uses assistant-row `cwd`. A shared resolver examines an available directory at sync time for Git repository details. Remote identity combines clones; if unavailable, repository identity falls back to the Git common directory and then a confirmed OpenCode Git project. Missing or conflicting location fields remain unknown with diagnostics. Codex replay copies preserve the original fact location.

`usage_locations` stores a stable tuple key, nullable directory/repository keys and display names, and repository provenance. Raw and canonical token rows reference the location by nullable `location_id`. Collector directory labels use `~/` when the current home or a standard home-directory pattern can be identified; otherwise the full directory path remains local. Publication includes the directory basename and stable keys; server analytics/facets never expose collector-local full directory paths. Full Git remote URLs and source artifact paths stay out of analytics storage. Stable key hashes never appear in display names. When repository identity is unknown and every included fact has the same known directory, the row and facet show `unknown · <directory>`; otherwise they show `unknown`, and Directory grouping exposes known paths separately. One session can contribute facts to multiple locations; row session counts are distinct within each group and may repeat across groups. Unknown values participate in totals.

Parser provenance is part of local raw identity. Changes to emitted facts/token semantics/native identity must update collector compatibility and the publication identity/semantics contract as appropriate. Collector reprocessing cannot silently mix incompatible server interpretations; unsupported published versions are rejected. Output-compatible parser changes may safely reparse without changing published stable IDs.

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
8. Records supported canonical publication snapshots in the same transaction, with diagnostics for withheld facts.

Normalization must be idempotent: repeated runs should converge on the same canonical identities and must not duplicate canonical facts or diagnostics.

Current normalization is work-queue incremental. It loads pending `token_usage` work for the selected harness filter, upserts canonical rows by semantic key, removes completed work in the same transaction, and increments ingest-run canonical/diagnostic counters only for newly inserted canonical facts or diagnostics. Existing canonical rows may be updated deterministically when the same semantic key is requeued by an explicit rebuild path.

Incremental normalization processes pending raw-fact work. A per-harness signature of current provider/model rules triggers one refresh of existing canonical identifiers when missing or changed; matching signatures with no pending work return before a write transaction. Rule markers commit with normalization, and `reset-canonical` clears them. Explicit rebuild paths mark raw facts dirty and use the same work mechanism. Deterministic updates remain allowed for dirty raw facts.

Claude native session/message/request identity has explicit source-timestamp precedence: the latest entire snapshot replaces older usage; equal-time conflicting snapshots fail. Other competing normalized payloads without an approved source revision are withheld or rejected at publication/ingestion rather than ordered by arrival time.

`normalize --dry-run` computes candidate canonical and diagnostic counts without writing.

## Viewer

`tokeninsights view` is interactive-only and reads the server API. Without an
explicit server URL it ensures the local query server; `--server-url` bypasses
local bootstrap/storage. `--no-sync` aliases the read-only default.
`view --sync` invokes caller-side all-harness collection/publication before
opening and stops on an unresolved failure. Viewer filters constrain queries,
never collection scope.

Loading, facet search, and Reload are bounded GET requests. Queries use generated
DTOs and one typed Go client for both deployments. The client loads bounded pages
from one runtime/database identity and revision, restarting at most three times
on a changing snapshot. It rejects mixed revisions and caps rows at 100,000;
200-row pages keep server request work bounded. Selection changes and quitting
cancel superseded HTTP requests. Failed reads retain saved rows and selected
filters; `r` retries. The obsolete `u` collection shortcut is inert.

Source coverage and collector job progress are absent from server viewers.
Missing uploaded usage does not establish empty/checked source days. Status
observes canonical revision/readiness only, and last ingestion records server
commit time. The header hostname comes from published producer labels, never
from the machine running the viewer.

The TUI uses the **Instrument desk** visual system, implemented in `internal/cli/theme.go` and `internal/cli/desk.go` and documented in [`packages/cli/DESIGN.md`](../packages/cli/DESIGN.md). Full-screen terminals (120×35 and larger) get a spaced header, compact seven-view navigation, scope controls, a token readout strip, and a full-width table. The canvas, readouts, and drawers preserve the terminal background; only selection highlights use fills. Foregrounds adapt to light/dark terminals. Terminal fonts remain user-owned. Bracketed active navigation, a row cursor, explicit loading/error status words, and checkbox markers retain meaning without color.

The header renders typed statusline state as `TokenInsights · host: <host> · ingested: <time>`. Hostname is server-reported producer metadata and falls back to `unknown`; absent ingestion history renders as `never`. Multiple producer labels render as `multiple machines`. Hostname shortens before ingestion time when space is constrained. Date range, bucket, and sort remain typed viewer state but render as keyboard controls above the readouts, alongside a filter action. Active Dimension Filters, including startup session-ID filters, appear immediately below the controls. Presets render as `today`, `yesterday`, `week`, `month`, `year`, or `all time`; custom bounds render as `from..to`, `from..`, or `..to`. Choosing a preset explicitly clears custom bounds, including when choosing the already configured preset.

The readout strip has one blank row above and below its two content rows, replacing the previous external top spacer without consuming additional table space. It sums the exact integer total, input, output, reasoning, cache read, and cache write fields across all filtered rows. It never parses abbreviated table strings or sums only visible rows. These components come from the same canonical queries as the table, without additional queries or schema changes. Loading and failed reads never display stale readout values. Context replaces the strip with an explanation of Session Peak Context Load, not a sum or an average of grouped averages. Terminals below 72 content columns or 24 rows omit the strip; all table metrics remain reachable through scrolling. Available table height derives from the rendered chrome. Header and footer compact before sacrificing data, and rendered output is bounded to the terminal dimensions.

Date, bucket, sort, filter, and help drawers open on the right, retaining visible dashboard context. Lists scroll to keep the cursor in view and actions remain pinned. Filter changes remain drafts until Enter applies them; Escape discards drafts and preserves table position. Space toggles values; clearing and applying the selection removes that Dimension Filter. Drawers appear immediately without decorative animation; actual sync progress retains its activity animation. Read failures retain navigation and expose a read-only retry action. Empty results distinguish an empty database from a filtered-out scope and give an appropriate next action.

The table summary is an explicit, pinned, full-width band below the table viewport, separated from data by one blank spacer row. It remains visible and unchanged during vertical and horizontal scrolling because it summarizes the full filtered result set rather than only visible rows. All tabs lead with `sessions <shown> shown / <synced> synced`, followed by `rows <count>`. Token-bearing tabs also show `total <tokens>`; the `context` tab omits this additive token total. Session coverage comes first so it remains visible ahead of row and token totals on narrow terminals; the line is clipped to the available width. Loading reserves a blank summary row so the table layout does not jump. An empty filtered result can show `sessions 0 shown / 214 synced · rows 0 · total 0`; an empty database shows zero synced sessions.

Session coverage is queried from canonical data on every dashboard reload, independently of the active aggregation tab and time bucket. `shown` counts distinct canonical session IDs with countable token facts matching all active Date Range and Dimension Filters, including startup session filters and custom day bounds. `synced` counts distinct canonical session IDs with countable token facts across the entire database, ignoring every viewer filter. Both counts exclude empty canonical sessions and sessions with only non-countable facts. The counts use canonical database IDs so the same source session ID in different harnesses remains distinct, and sessions spanning multiple dates/models/providers are counted once. This is coverage of imported, normalized usage, not the number of source files discovered or provider-account lifetime sessions. The query is read-only and needs no schema change. Date and dimension changes reload coverage; scrolling and sorting do not change it.

The single-row footer owns essential shortcuts, loading state, and scroll position. Detailed keys live in the help drawer. Active filters live above the readouts. Session coverage and row count remain pinned in the table summary; the full-result token total also appears in the readout strip. Last sync time belongs only to the statusline.

The TUI queries REST usage/facets/status only. The server reads rows, session counts, revision, and last ingestion from a canonical read snapshot. Paginated TUI reads require matching runtime/database identity and revision across every page, so concurrent ingestion/restart cannot combine incompatible results. No viewer fallback opens SQLite or collector operational metadata.

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

Interactive shortcuts use `d` for Date Range Filter, `g` for Time Bucket or Repo grouping, `s` for sorting, and `p`, `m`, and `h` for provider, model, and harness filters. `f` opens the filter menu and `?` opens the keyboard guide. Tab/Shift-Tab and 1–7 select views; up/down or j/k move rows, PageUp/PageDown move a page, and left/right plus home/end scroll columns. Repo adds repository and directory facets to the filter menu. `h` remains reserved for the harness filter. `r` reloads committed server data; successful retry clears the failure state. Ctrl+C exits even while a drawer is open. Saved data and dashboard controls remain usable during ordinary ingestion; server storage/query errors offer retry without initiating collector recovery.

The `context` tab sorts by `avg ctx` descending by default and supports sorting by `avg ctx`, `median ctx`, `max ctx`, `sessions`, `harness`, `provider`, and `model`.

Viewer tables are viewport-aware. They use consistent column width rules across Aggregation Tabs, stack multiple model, provider, or harness summary values vertically within a row, truncate long display values, and fall back to horizontal scrolling only when minimum readable widths cannot fit.

TPS, request, and tool domains are future-compatible canonical domains. Preserve `tps avg`, `tps mean`, and `tps median` concepts and future TPS capability, but keep unavailable domains absent from the active tab bar until durable timing facts exist; never manufacture timing from token counts.

Cost tracking is not part of TokenInsights and must not appear in viewer columns, totals, sort options, or docs.

## Web Viewer

`tokeninsights service start` serves the embedded React dashboard, canonical
queries, and normalized ingestion from one native Go binary. Default binding is
`127.0.0.1:8765`; `--port 0` reports the assigned port. `--open` launches the local
URL when possible. `server run` provides the foreground remote composition.
Non-loopback serving requires an authentication token; clients use bearer auth,
and browser Basic auth accepts the same token as its password. Remote setup,
TLS termination/deployment, and accounts remain later scope.

Opening, reconnecting, filtering, status polling, and **Reload** query saved
canonical data only. Empty state points to `tokeninsights sync` on the producer.
There is no server-owned refresh queue or POST sync endpoint. Startup never reads
Durable Sources or runs recovery on producer data. Queries read canonical facts
and server metadata in one SQLite read snapshot; collector absence or pending
uploads does not block saved analytics. Exposed HTTP errors use safe fixed messages.

The web viewer includes all seven active Aggregation Tabs and their TUI metrics/sort concepts. TanStack Router maps them to `/tokens`, `/models`, `/providers`, `/harnesses`, `/sessions`, `/context`, and `/repo`; `/` redirects to `/tokens`, and legacy `/?tab=<view>` URLs redirect to the matching path. Direct loads of those paths serve the embedded React application, while unknown `/api/*` paths remain JSON 404s. Tables support ascending/descending sorting, column visibility, adjustable column widths, and pagination (default 50, maximum 200 rows per API page). Comma-separated harness, provider, and model summaries display one value per line in table rows, matching the TUI; their API values and sort semantics stay unchanged. Column widths are local viewer state; dragging a header edge or using its keyboard separator changes layout without changing query or analytics data. Canonical grouping happens in existing Go/SQLite viewer queries, then the server sorts and slices grouped rows. Each dashboard response reads its rows, chart data, full-result summary, and last committed ingestion in one read-only database transaction. Summary session counts use the TUI's shared `ViewerSessionCounts` query: API `summary.sessions` is the distinct shown count and `summary.syncedSessions` is the all-dates/all-harnesses count ignoring every viewer filter. Empty canonical sessions and non-countable-only sessions are excluded. The Sessions card labels its filtered count as **Sessions shown** with the synced denominator underneath. Every table summary leads with `Sessions <shown> shown / <synced> synced`, including Context and empty filtered results, followed by row count and applicable token total. Table summaries remain independent of pagination; Context has no additive token-total summary. Loading placeholders omit stale results until the new filtered response arrives.

Token/session charts show chronological Time Buckets. Model/provider/harness charts show the top 12 groups by canonical total tokens, with keyboard-accessible labels that apply the corresponding Dimension Filter. Each bar, filter label, and tooltip shows its share of the full filtered canonical token total from the response summary, including groups outside the top 12 and independently of table pagination. Shares use at most one decimal place; nonzero shares below 0.1% render as `<0.1%`, and a zero total yields 0%. Context charts show the top 12 groups by average Session Peak Context Load, including average, median, and maximum. Charts use canonical totals directly; components do not redefine total-token semantics. Date bounds are inclusive local dates, custom bounds replace the preset, and Monday-start weeks use server-local calendar arithmetic even when the browser is in another timezone.

Repo defaults to repository grouping. Users switch between repository and directory grouping. Rows list all distinct providers, harnesses, and models in both the TUI and web table. Its chart ranks the top 12 selected groups. In both groupings, bars and tooltips show shares of the full filtered canonical token total, using the same denominator and formatting as dimension charts. Repository and directory facets accept stable keys and show names or sanitized paths; **unknown** is selectable. In either grouping, an unknown row starts with its directories hidden. When recorded directories contribute, the web table offers **Show directories** and reveals every directory on request, along with any missing-directory note. An unknown directory group has no recorded directory to expand, so it shows **No recorded directory** without a disclosure control. These details describe the facts in that row after filters, without inferring a repository; the TUI unknown-repository label is unchanged. These location filters apply only to Repo, while existing date and usage filters continue to apply there. Repo rows count sessions distinctly within each row, so their session counts are not additive across locations. Other views do not consume location filters.

The V1 REST API has these public endpoints; unversioned routes are unsupported:

| Endpoint | Purpose |
| --- | --- |
| `GET /api/v1/instance` | Runtime/database identity, versions, producer labels, reporting timezone, capabilities, defaults |
| `GET /api/v1/sync` | Compatibility-named read-only server readiness and canonical revision |
| `GET /api/v1/usage` | Filtered summary, chart, rows, revision, and last committed ingestion |
| `GET /api/v1/usage/facets` | Filter facets and bounded session-ID search |
| `GET /api/v1/ingestion/capabilities` | Supported normalized versions/limits and durable database identity |
| `POST /api/v1/ingestion/batches` | Atomic canonical ingestion; committed receipt on success |

`POST /api/v1/sync` is rejected. Legacy query field names such as `lastSynced` and
`summary.syncedSessions` remain transport compatibility names; they describe
server ingestion and saved canonical session counts, not source completeness.

Usage/facet query parameters are `period`, `bucket`, `from`, `to`, repeated `provider`, `model`, `harness`, and `session`, plus `tab`, `sort`, `direction`, `page`, and `pageSize`. Repo adds `locationGroup` and repeated `repository` and `directory` stable-key filters. The latter parameters require `tab=repo`; other views ignore location selections. Repo API rows include sorted distinct `directoryNames` and `hasUnknownDirectory` for their contributing facts; other tabs return an empty list and false. Session lookup adds literal substring `search`, returning at most 100 values. Facet queries apply all other filters while omitting their own Dimension Filter. React retains selected values even when other facets exclude them. API inputs are validated, sort fields allowlisted, database reads have request deadlines, and failures use the standard `{ code, message }` JSON body. Unknown API routes also return JSON errors.

[`docs/openapi.yaml`](openapi.yaml) is the authoritative, repository-only API contract; the server does not expose it at runtime. `pnpm run generate:api` generates committed Go transport models and TypeScript types/Zod schemas. `pnpm run check-api` verifies generated output has not drifted from the contract. Handwritten handlers map canonical query results into generated response models, while browser query hooks validate responses with the generated schemas. Direct Go builds consume committed generated files and do not require Node or code-generation tools.

V1 API routes do not advertise cross-origin browser access or provide CORS preflight handling. Unsafe browser query requests use Go CrossOriginProtection; loopback bindings reject non-local Host headers. The embedded dashboard uses relative, same-origin API URLs. Authentication is composed by the runtime before handlers; non-loopback serving requires an explicit token. These origin/Host checks remain separate from authentication.

`packages/web` uses React, strict TypeScript, Vite, Tailwind CSS, local shadcn primitives backed by Radix UI, TanStack Router/Query/Table, and Recharts. Feature components compose through `components/ui`; bespoke CSS is limited to dashboard layout, responsive behavior, and data-visualization geometry. The route path owns the active Aggregation Tab, and validated route search owns dashboard filters, sorting, and pagination. Theme, visible columns, chart metric, and transient popover/search drafts remain local UI state.

The dashboard composes separate header and results components. A route-query provider exposes named navigation actions backed by the pure `reduceQuery` transitions; a separate display-preference provider stays mounted across route and query loading so chart metrics and column visibility survive those transitions. TanStack Query remains the server-data owner. `useDashboardSync` observes server status/revision and invalidates read queries on
Reload or committed ingestion. It never requests collection; stale/superseded
status reads are cancelled, and database/runtime identity changes discard old
query state. Session search owns its draft, debounce, and query failure/retry within its filter control. Selected filters and drafts survive lookup failures. Stable table cell components preserve Repo directory disclosure and focus through ingestion updates and sorting.

The chart has its own error boundary inside the persistent dashboard, preserving controls, summaries, and tables after chart rendering or chunk-loading failure. Its explicit **Reload page** action retries loading in a fresh document, retaining the URL and saved theme; transient display preferences follow normal page-reload behavior. A root route fallback also offers page reload for other render failures. Suspense handles chart loading only; ordinary API failures retain explicit query-owned retry paths.

Every browser request, including **Reload** and session search, targets the server
serving the page. The header displays published producer hostname metadata; the
footer shows the page origin. `/api/v1/instance.hostname` uses server producer
labels, returning `unknown` without labels and `multiple machines` when labels
differ. It never substitutes the serving machine or browser address. Committed
ingestion revision changes refresh instance metadata without page reload. There is no add/remove/select-host UI, configurable browser API destination, or persisted host list. Legacy `tokeninsights.sources.v1` browser storage is ignored. Query keys retain filter scope, random runtime instance/data epoch, and canonical revision; database identity changes cancel/remove stale analytics/facets, and delayed responses cannot populate another epoch. Focus/reconnect refresh service status before analytics; superseded requests are cancelled and route transitions preserve summary/result semantics. Connection failures offer retry.

Development Vite proxies `/api` to the Go server; its browser requests also remain same-origin.

The route path owns the tab. Route search parameters own period, bucket, custom dates, repeated provider/model/harness/session filters, sort, direction, page, and page size, including browser back/forward and explicitly cleared startup filters. Tab navigation preserves filters, resets pagination, and applies the destination tab's valid default sort when needed. The Graphite & Lime dashboard uses a compact hostname/status/action header, route tabs and quick periods on a shared row, always-visible wrapping horizontal filters, static readouts, a 10rem chart, and dense tables. Summary readouts are compact static data displays on every tab; only the chart toolbar selects the chart metric. Route controls, filters, ingestion/query error feedback, and summaries remain mounted across route changes; only the chart/table region loads, and it never displays rows from the previous route. Theme preference persists locally. CSS typography, color, spacing, and radius tokens use browser-scalable rem/em sizing, with responsive layouts, focus styles, accessible controls, and reduced-motion support. The active route uses lime text and an underline with aria-current. Readouts have no click, hover, tooltip, or selection state. Dark mode uses neutral graphite and bright lime; light mode uses pure white (`#ffffff`) for the workspace, header, fields, and overlays, Electric Lime (`#b8f500`) action fills with dark text, pale lime (`#efffcc`) selections, deep lime (`#426300`) selection/focus ink, and `#577b00` chart ink. The secondary chart series uses a neutral sage gray.

The React visual contract is [`DESIGN.md`](../DESIGN.md), implemented by `packages/web/src/tokens.css`, Tailwind theme utilities, local shadcn primitives under `components/ui`, and feature rules in `styles.css`. All Aggregation Tabs share semantic light/dark colors, a 4px-based spacing scale, three radius roles, aligned page/panel insets, and standard/compact controls with larger touch targets. Narrow layouts retain all seven navigation choices in a horizontally scrollable rail and keep accessible names for icon-only actions. Visual changes must follow that contract without changing canonical analytics semantics.

The pnpm monorepo contains Go production packages and TypeScript development/browser packages. Browser code uses Vite and React. Node scripts use native, erasable TypeScript supported by Node 26+.

Vite output is checked into `packages/cli/internal/server/static` and embedded using `go:embed`, preserving direct Go installs and offline runtime use. The workspace builds React before Go; local pre-push verification rebuilds and checks generated assets for drift. Node, npm, pnpm, `node_modules`, and repository TypeScript tooling are build-, test-, and development-only. Production is one native Go binary: Go serves embedded browser JavaScript as bytes, the browser executes it, and Go runtime code never invokes a host JavaScript runtime. Web analytics use the canonical token and optional location contracts; lifecycle state is local-only and not an analytics dimension.

`mise.toml` pins development tool versions and delegates tasks to root pnpm scripts. Husky registers `pre-push` through dependency installation, clears Git-local environment variables to isolate fixture repositories, and runs `check:push`: format/lint, schema/API contracts, unit/conformance and Go race tests, embedded asset comparison, native build, and browser E2E. GitHub CI/release run `check:ci`: formatting, schema-copy consistency, and a native build, with no test suites or browser/frontend build. Publication/packaging remain workflow-owned. Checks preserve tracked files; generated API/assets must be updated deliberately.

## Collector Progress And Server Freshness

Collector `sync_jobs`, `sync_harnesses`, and `sync_sources` retain metadata-only
source progress, interrupted outcomes, conservative occurrence bounds, and local
diagnostics. Discovery is indeterminate; checked-source percentages describe
local work rather than lifetime completeness. Captured JSONL extents and
incomplete-tail diagnostics preserve later retry. These records do not cross
publication.

Server status exposes readiness, durable database identity, canonical revision,
and last committed ingestion. Web/TUI display available usage and **Reload**;
they do not infer checked/empty source days from missing uploads. A receipt proves
batch commit, not harness coverage. Last ingestion is server commit time, never a
claim that all producers are current. Reporting calendar filters use the server
local timezone consistently; occurrence timestamps and stable identity remain
independent of collection/viewer clocks.

## DB Lifecycle

- Collector `db.Open` / `OpenWritable` validate collector role before compatibility
  or mutation. Collector analytics reads revalidate compatible local state inside
  a transaction. CLI collect/normalize/reset share a context-aware writer lock.
- Fresh defaults leave the legacy mixed file untouched. Existing wrong-role,
  unknown, corrupt, or newer contracts fail explicitly. Legacy role-zero files
  are not silently migrated into either new role.
- Collector compatibility/reset machinery applies only to collector storage.
  Explicit resets recreate application tables transactionally without unlinking
  DB/WAL/SHM/lock inodes. `reset-canonical` preserves raw facts and publication
  history while requeuing raw normalization work; `reset-all` clears collector
  journal/cursors and establishes a fresh delivery stream. Neither retracts
  committed server usage.
- Supported local rebuilds retain a pending marker/source-scope fingerprint for
  retry. Missing harnesses are normal skips; changed scope cannot silently finish
  a pending rebuild. Only retained sources can reconstruct missing collector raw
  facts. `--dry-run` performs no data or publication writes.
- Server `CreateIfMissing` initializes only missing canonical-only storage.
  `Open` inspects server application role/schema/metadata read-only before writer
  access. Server storage uses preserving compatibility checks; unavailable
  producer artifacts are never a reason to reset server history or receipts.
- Server batches commit normalized facts, references, durable receipt, and
  metadata together. Collector delivery progress commits only with its validated
  acknowledgement. Reopen/retry reads persisted request/receipt identities;
  failed/unknown outcomes do not advance cursors.
- TUI/browser never open collector SQLite, normalize facts, or repair storage.
  Explicit remote URLs skip local database/bootstrap. Query failures preserve
  saved data/filters and offer read retry. Help/version never create databases.

## Invariants

Must not change silently:

- schema changes require explicit user approval;
- collector and server source SQL remain the table sources of truth;
- canonical token usage must be session-centric;
- missing model and unavailable provider must render with canonical fallback values, not cause row loss;
- raw storage must remain metadata-only and avoid private content;
- normalized server publication must exclude raw facts, source diagnostics/continuity, prompt text, assistant text, tool arguments/output, headers, secrets, provider payloads, source paths, and collector-local directory paths;
- Local-only Continuity Metadata must never be ingested into the server or used for server analytics;
- default token analytics use only countable canonical token rows;
- unavailable metric domains must not appear as empty active viewer tabs;
- cost tracking must stay out of the active product;
- TUI/browser query committed server data through REST; Reload never collects, and explicit caller-side `view --sync` is the only view-start collection path.

Can evolve with care:

- new harness adapters;
- new canonical fact domains for TPS, requests, or tools;
- explicit conflict precedence and richer diagnostic categories;
- generated schema constants;
- future checkpoint plugins that write equivalent raw/canonical concepts.

## File Organization

| Path | Role |
|------|------|
| `schema/schema.sql` | Collector SQLite schema source |
| `schema/server.sql` | Canonical-only server SQLite schema source |
| `packages/cli/internal/db/schema/schema.sql` | embedded collector schema |
| `packages/cli/internal/serverstore/schema/server.sql` | embedded server schema |
| `tools/build/src/check-schema.ts` | schema contract validator |
| `docs/openapi.yaml` | authoritative, repository-only REST API contract |
| `tools/build/` | private Node 26+ native TypeScript build/test/development tooling package |
| `packages/cli/cmd/tokeninsights/main.go` | CLI executable entry point |
| `packages/cli/internal/cli/commands.go` | command dispatch and thin orchestration |
| `packages/cli/internal/cli/flags.go` | view flag parsing |
| `packages/cli/internal/cli/serve.go` | web command flags and orchestration |
| `packages/cli/internal/viewer/filters.go` | shared calendar and filter semantics |
| `packages/cli/internal/server/` | Public HTTP handlers, snapshots, generated API models, embedded assets |
| `packages/web/` | typed same-origin React dashboard, generated API schemas, and design tokens |
| `packages/cli/internal/cli/table.go` | interactive TUI model |
| `packages/cli/internal/cli/desk.go` | Instrument desk layout, readouts, and drawers |
| `packages/cli/internal/cli/theme.go` | semantic light/dark terminal colors and styles |
| `packages/cli/internal/cli/statusline.go` | typed dashboard statusline state and width-aware rendering |
| `packages/cli/internal/cli/table_summary.go` | pinned table summary state and width-aware rendering |
| `packages/cli/internal/cli/render.go` | table rendering |
| `packages/cli/internal/db/open.go` | Collector DB open/create/reset/schema lifecycle |
| `packages/cli/internal/collector/` | Host collection and manual durable delivery |
| `packages/cli/internal/collectorstore/` | Canonical journal, saved batches, destination acknowledgements |
| `packages/cli/internal/publication/` | Normalized identity, payload, codec, limits, receipts |
| `packages/cli/internal/ingestion/` | Shared transactional ingestion core and HTTP routes |
| `packages/cli/internal/serverstore/` | Canonical-only storage role/compatibility |
| `packages/cli/internal/queryclient/` | Bounded REST queries and consistent pagination |
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

The `collector-rebuild` fixture exercises real adapters against synthetic harness-native sources with explicit expected usage, repeated collection, and reconstruction from fresh databases. `collector-ingestion-protocol` contains candidate canonical publication/failure traces for the proposed server; these traces are acceptance specifications, not proof that server ingestion exists. See the [failure-test matrix](collector-ingestion-tests.md) for coverage and reproduction commands. Expected usage must be derived independently from source semantics, never regenerated from the implementation to make a failing test pass.

`sync-first-basic/source/` is also the shared development source fixture. It contains compact representative OpenCode, Pi, Codex, and Claude Code data, roughly two sessions and two canonical facts per harness. Source structures reflect durable harness formats, but every retained value is synthetic. Fixtures must exclude conversation content, tool arguments/output, request headers, secrets, real user or repository paths, signatures, and other identifying data. Raw local harness databases and transcripts must never be copied into the repository.

`pnpm run dev:data` builds the Go CLI without rebuilding browser assets, resets
only the controlled `.tokeninsights-dev/collector.sqlite` and `server.sqlite`
application tables, recreates synthetic source/home subdirectories, materializes
OpenCode SQLite from reviewable SQL, and publishes fixtures through production
collector/HTTP ingestion. It starts a loopback server on a temporary port,
collects/sends, then stops it. DB/lock inodes, unrelated files, and the legacy
`tokeninsights.sqlite` remain intact; live/unreachable ownership blocks reset.
`dev:cli` opens read-only all-time REST usage from the server fixture, while
`dev:server` serves canonical `server.sqlite` on loopback without source settings.
Vite `dev:web` proxies `/api` to `127.0.0.1:8765`; `dev:web:mock` uses validated
synthetic API responses without Go/harness data. `dev` runs server and web in
parallel. `start:web` ensures the ordinary local server without collection.
Browser E2E retains its larger synthetic dataset for pagination, populating the
canonical server rather than pointing viewers at collector storage.
