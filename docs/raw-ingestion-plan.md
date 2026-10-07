# Raw ingestion implementation plan

Historical implementation contract for ADR 0007. Current personal/hosted,
dataset and version contracts are in [design](design.md) and
[ADR 0008](adr/0008-personal-hosted-composition-and-capabilities.md).

Status: Approved; implemented. Verification recorded in PR.
Implements [ADR 0007](adr/0007-raw-ingestion-and-server-processing.md) and its
[whitelist](raw-ingestion-whitelist.md). Scope: data capture, delivery, durable
acceptance, server processing, analytics, history preservation and native builds.
Authentication/accounts and the separate client configuration proposal are not
part of this implementation.

## Approved contracts

### Collector SQLite: schema 17

Keep the existing schema 16 tables as retained legacy state. Add:

| Table | Columns / constraints |
| --- | --- |
| `evidence_state` | Singleton; random stream ID, extractor version, creation time |
| `evidence_sources` | Local source key PK; source-instance/lineage ID, format, extractor version, complete byte offset, prefix/boundary hashes, context bytes; source paths stay local |
| `evidence_outbox` | Monotonic sequence PK; observation key UNIQUE, harness/format, opaque source identity, ordinal, immutable allowlisted record bytes and context references/enrichment, creation time |
| `evidence_destinations` | Destination PK; endpoint, pinned database ID, acknowledged sequence/time |
| `evidence_batches` | Batch PK; destination/stream/database binding, sequence range, request hash/bytes, immutable acceptance bytes/time; at most one unacknowledged batch per destination |

Capture outbox and continuity/context in one transaction. Extraction versions
invalidate cursor reuse. JSONL resumes only after checking saved prefix/record
boundary and required context; source replacement starts a new lineage and
retains prior observations. OpenCode reads a consistent source SQLite snapshot,
compares sanitized row snapshots and captures changed rows, including older rows.
Incomplete JSONL tails wait. Full rereads remain safe through observation dedupe.
Retain acknowledged outbox initially; no compaction implementation in this PR.

Allow an explicit, verified additive schema-16-to-17 upgrade without resetting
legacy facts, journals or pending normalized requests. Other versions/roles fail
without mutation. Old protocol-1 batches remain identifiable legacy delivery
state; never reinterpret them as raw protocol requests. Maintenance must not
silently delete unacknowledged raw evidence.

### Server DuckDB: data schema 1

New default data path: `server.duckdb`. `--server-db-path` and its environment
override address the data store. Existing server SQLite is an import source,
never overwritten or relabeled as DuckDB. Future application SQLite is separate
and is not created until it has an actual application-data use.

| Schema / table | Columns / constraints |
| --- | --- |
| ingestion.metadata | Singleton; role/schema version, database/dataset IDs, active/target generation, accepted input/published revisions and timestamps |
| raw.evidence | Evidence ID PK; dependency scope, harness, immutable allowlisted record JSON, first acceptance time |
| ingestion.batches | (stream_id,batch_id) PK; exact request bytes/hash and immutable acceptance receipt JSON |
| ingestion.items | (stream_id,sequence) PK; immutable evidence binding across overlapping batches |
| ingestion.batch_items | (stream_id,batch_id,sequence) PK; every submitted entry's evidence mapping |
| processing.scopes | Scope PK; requested/processed revision, completed generation, attempts/retry time and fixed error code |
| processing.dependencies | (child,parent) PK; native ancestry dependencies |
| processing.outcomes | (generation,evidence_id) PK; input revision, disposition, diagnostic and fact reference |
| analytics.generations | Generation PK; processor version, state and creation/activation times |
| analytics.facts | (generation,fact_id) PK; scope, native references, occurrence time, typed dimensions, five token components and sum, original fact payload JSON, input revision |
| analytics.provenance | (generation,fact_id,evidence_id) PK; contribution/evidence edges |
| analytics.estimates | Unique (generation,fact_id); usable typed counters/time/dimensions, evidence reference and ambiguity reason |
| analytics.legacy | Stable fact ID unique; original imported dimensions/components/native references/revision in payload |
| analytics.legacy_coverage | (generation,fact_id) PK; exact replacement payload hash |
| ingestion.legacy_receipts | (stream_id,batch_id) PK; protocol-1 request hash and original receipt JSON |
| analytics.confirmed / analytics.estimated | Active-generation views; confirmed reconciles individually proven legacy replacement; estimated remains separate |

All ingestion/processing authoritative state stays in one DuckDB file. One server
owns it; a shared write coordinator provides short transactions. Read connections
reuse the same server-owned engine, avoiding per-request database opens. Primary
keys protect deliveries/evidence/contributions; status lookups use registry
indexes. No foreign-database transaction, broker or shared network file.

Use dependency scopes derived from native sessions and Codex parent references.
Conflicting/missing session context uses an explicit unresolved scope. Parent
arrivals invalidate dependents. Worker input snapshots include the relevant
revision; changed component membership/revisions cannot commit. A single worker initially limits
complexity while keeping processing off the ingestion request path.

### Raw ingestion protocol 2

Keep protocol-1 completion semantics distinct. New endpoints:

- `GET /api/v2/ingestion/capabilities`: versions, durable database/dataset identity,
  limits and acceptance-only completion mode.
- `POST /api/v2/ingestion/batches`: database/stream/batch identity, contiguous
  sequence range, entries containing sanitized evidence and context. Initially
  reuse limits of 1 MiB/256 entries; metadata strings at most 256 UTF-8 bytes.
- `GET /api/v2/ingestion/batches/{stream}/{batch}`: immutable acceptance receipt
  plus current per-item processing outcome and generation/input revision.

Receipt binds exact request bytes and every submitted entry. Return `202` after
durable acceptance while any applicable work remains, `200` if all outcomes are
terminal, `409` for changed bytes under the same batch identity, `400`/`413`/`422`
for invalid inputs/contracts, and `503` if durable acceptance is unavailable.
Atomic rejection of structurally invalid/unknown/private fields. Safe semantic
problems are stored and processed asynchronously. New clients accept both
`200`/`202` only after validating matching acceptance. They never await processing
to acknowledge delivery. No hostname becomes source or ownership identity.

Update OpenAPI plus generated Go/TypeScript together. Queries retain their
existing routes while gaining generation, processing-lag and separate estimated
usage metadata. A confirmed/estimated query selector never combines totals.
Pagination/facets guard instance/database/generation/revision snapshots.

## Implementation sequence

1. **Worktree and baseline.** Fetch origin, create `codex/raw-ingestion` from
   current `origin/main` (including PR #55) using Worktrunk. Copy approved uncommitted architecture
   documents into it, preserving the original worktree. Pin official
   `github.com/duckdb/duckdb-go/v2 v2.10506.0` (DuckDB 1.5.6); embedded native
   linking has passed a standalone Go probe on Linux amd64. Build storage around
   that pinned driver, with native release jobs for the other supported targets.
2. **Evidence contract and capture.** Add a storage-independent evidence package
   with strict per-harness leaf whitelists, type/presence-preserving extraction,
   safe diagnostics, record/order/context identities and explicit location
   enrichment. Collector discovery reuses source discovery, never normalizing
   parsers. Add transactional raw outbox/checkpoints and immutable batch/ack
   handling. Switch `sync`/TUI capture to this path. Keep legacy normalization maintenance distinct; deprecated sync normalization
   flags do not affect raw capture.
3. **DuckDB acceptance.** Add checked SQL/embed/version contract and safe private
   initialization. Acceptance inserts raw evidence, every batch membership,
   receipt and needed jobs atomically. Handle concurrent exact retries,
   intra-batch duplicates and overlapping batches with database constraints.
   Expose indexed live status separately from immutable receipts.
4. **Server interpretation.** Extract current counter normalization, provider
   aliases, Claude snapshot precedence, OpenCode V1/V2 precedence and Codex
   ancestry/cumulative copy proofs into reusable Go processing functions over
   sanitized records. Retain existing semantic fixtures; avoid reconstructing
   private transcripts or making source access part of server processing.
   Missing/conflicting proof becomes durable ambiguity, with usable counters
   projected separately as estimated. Known duplicates/superseded snapshots
   appear in neither confirmed nor estimated totals.
5. **Durable worker and replay.** Select dependency components with revision fencing, read a stable evidence
   snapshot, compute outside write transactions, and atomically publish complete
   contribution/provenance/outcome replacements. Resume unfinished scopes after restart;
   retain safe diagnostics and retry transient failures. Add deliberate server
   reprocessing that builds a new generation and activates it only after
   coverage validation; never mix generations.
6. **History import.** Import supported server schema 2 read-only into a private
   staged DuckDB file. Verify native keys, all token components, receipts and
   database identity before publishing. Preserve imported baseline contributions
   until identity-specific coverage proves replacement. Default-path upgrade
   detects existing `server.sqlite` rather than silently starting empty history;
   custom paths get explicit import guidance. Unsupported files fail untouched.
7. **Queries and viewers.** Introduce server-owned analytics operations using
   typed DuckDB columns. Date/filter/aggregate/order/pagination run in SQL, with
   IANA timezone buckets and exact integer totals. Preserve session counts,
   context peaks/medians, repository grouping, unknown attribution and TPS
   concepts. Show estimated usage separately in the web dashboard, with ambiguity counts
   and processing freshness; Reload remains a query only.
8. **Composition.** Local public query/dashboard routes have no ingestion writes;
   reuse the private Unix control transport for local Collector acceptance.
   Remote foreground composition shares the data core; auth remains deferred.
   Reuse the separate remote entry point and client configuration already merged
   in PR #55. No auth/account implementation in this scope.
9. **Build/docs/PR.** Pin DuckDB native dependency, enable CGO and replace
   CGO-disabled cross-builds with native release jobs for the supported targets.
   Package both entry points; preserve standalone Go builds and Node-free
   execution. Update README, design, glossary, schema checks and developer data
   setup. Format/lint, focused tests, full tests/race/build/API/schema/asset checks
   and browser verification before committing, pushing and opening a PR to main.

## Verification oracles

- Every supported harness fixture produces the same confirmed component totals
  under server processing; copied histories and Collector rebuilds remain stable.
- Privacy fixtures prove content, paths, secrets and unknown nested payloads never
  reach the outbox/server; null/zero/aliases/native numeric precision survive.
- Append/rewrite/truncation, incomplete tails and mutable older SQLite rows do
  not skip evidence; concurrent captures do not allocate duplicate observations.
- Concurrent/overlapping/duplicate batches preserve all item mappings and only
  one confirmed contribution; changed request bytes conflict without mutation.
- Crashes before acceptance leave no acknowledgement; crashes after commit/lost
  replies replay the same receipt. Collector checkpoint/outbox and ack/cursor
  failure tests assert atomicity rather than weakening expected counts.
- Worker restart, stale revisions, late parent/model evidence and new input arriving
  during processing retain work and cannot publish stale outcomes.
- Conflicts remain debuggable estimates outside confirmed totals; resolution
  removes the estimate atomically. Invalid counters/time create no invented usage.
- Imported history with vanished sources survives; partial replacement never
  adds both old/new copies or deletes uncovered facts. Import failures preserve
  the original SQLite file and destination identity.
- Filtered rows, summaries, chart and facets share consistent generation/revision;
  SQL pagination remains bounded and timezone/DST tests preserve calendar results.
- Both binaries start/query with no Node/npm/pnpm in PATH; local wildcard query
  access cannot POST ingestion, while local private delivery works.

## Implemented refinements

Typed dimensions live directly in facts. Scope revision and complete component
membership fence the single worker; no claim tokens. OpenCode scans consistent
snapshots because older mutable rows lack a trustworthy incremental cursor.
SQL DDL and design document the final columns. Retention, component memory,
remote auth/rebinding and measured scaling remain follow-up work.

## Unresolved questions

None.
