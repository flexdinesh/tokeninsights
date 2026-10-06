# TokenInsights design

Implemented [ADR 0007](adr/0007-raw-ingestion-and-server-processing.md).
See [approved plan](raw-ingestion-plan.md), [whitelist](raw-ingestion-whitelist.md)
and [OpenAPI](openapi.yaml). Supersedes collector-owned normalization,
normalized-only ingestion, SQLite analytics and synchronous completion.

## Ownership

Collector discovers OpenCode/Pi/Codex/Claude Code, captures whitelisted native
metadata in a SQLite outbox, and submits raw batches. It owns continuity and
local location enrichment; never normalizes new counters or resolves ancestry.
Server durably accepts evidence, asynchronously processes typed confirmed facts/
separate estimates, then serves SQL analytics and embedded browser assets.
Startup resumes processing; never discovers client source files or collects.

    Sources -> whitelist extraction -> Collector SQLite -> acceptance
      -> DuckDB raw/receipts/scopes -> async processor -> facts/estimates -> views

Delivery IDs are operational stream/sequence/batch identities. Evidence IDs name
qualified native records plus preserved snapshots/context; missing native record
IDs use source lineage/order only as evidence witnesses. Contributions use stable
native harness/session/message/request rules, independent of collector installation,
row IDs, capture times or deliveries. Equal counters/hash alone never identify
globally shared consumption. Initial dataset shared; auth/accounts deferred.

## Storage contracts

| Role | Default | Schema | Version |
| --- | --- | --- | --- |
| Collector | collector.sqlite | schema/schema.sql | 17 |
| Server | server.duckdb | schema/data.sql | 1 |
| Legacy import | server.sqlite | schema/server.sql | 2 |

Defaults use XDG_DATA_HOME or ~/.local/share/tokeninsights. Role-specific flags/
environment/config override them. Reject aliased paths, wrong roles, incompatible
versions and corrupt contracts. Former tokeninsights.sqlite stays untouched.
Future transactional users/auth/tokens use separate application SQLite when needed.

Verified collector schema 16 upgrades additively, retaining legacy facts,
journals, bindings and exact protocol-1 requests. Raw extraction has its own
version/stream; older canonical generations need no rebuild for capture, newer
generations reject. Maintenance cannot delete unaccepted raw outbox.

Fresh default server.duckdb imports verified sibling server.sqlite read-only.
Stage initialization/import/checkpoint before atomic publication. Preserve database
identity, components/revisions/history/receipt bytes; verify copied counts/totals.
Custom import uses service import --server-db-path NEW --legacy-server-db-path OLD,
or remote startup with the same flag. Explicit import requires new target and
stopped local service. Source never overwritten. Newer processors reject; older
ones schedule a replacement generation.

DuckDB schemas:
- raw.evidence: immutable sanitized JSON and qualified scope.
- ingestion.metadata: role/version, database/dataset, active/target generations,
  acceptance/published revisions and times.
- ingestion.batches/items/batch_items: exact request/receipt bytes, immutable
  stream/sequence bindings, every submitted mapping including duplicates.
- processing.scopes/dependencies/outcomes: durable queue/revisions/generation,
  attempts/retry time/fixed errors, native ancestry and per-item dispositions.
- analytics.generations/facts/estimates/provenance: versioned typed usage,
  flattened native/session/message/location dimensions and evidence edges.
- analytics.legacy/legacy_coverage, ingestion.legacy_receipts: imported history,
  exact contribution replacement proof and protocol-1 receipt replay.

One file gives each acceptance/projection a transaction boundary. No broker,
cross-file commit or generic backend framework.

## Capture and reliable submission

SQLite evidence_state/sources/outbox/destinations/batches retain lineage/context,
immutable observations, monotonic sequences, bindings, exact saved requests and
acceptance receipts. Capture/checkpoint commit together.

JSONL validates saved byte prefix, parses appended records, then verifies captured
bytes and inode before commit. Rewrite/truncation rotates lineage without deleting
evidence. Partial JSON tail waits; complete JSON without newline is captured but
keeps checkpoint before the last line. Full refresh preserves verified lineage
and deduplicates observations. Prefix verification reads old bytes for continuity.

OpenCode reads consistent SQLite snapshots. Existing rows can revise without
native update cursor, so snapshot scans compare sanitized observations; unchanged
rows never requeue. Native session/turn/task context accompanies usage.
Types, null/absence and numeric precision survive extraction. Invalid safe counters
become server diagnostics rather than client repairs.

Unknown/private fields never cross ingestion. Source paths/cursors/Git URLs stay
local; enrichment contains stable hashes, basenames, repository names/provenance.
Collector writer lock serializes capture/delivery state. Persist request before
sending; matching acceptance advances destination cursor transactionally.
Manual sync/publish-only retries; no autonomous collector retry process.

Raw/outbox/old generations retain indefinitely initially; no compaction. Replay
can repair only captured fields; whitelist changes may require source rereads.
Remote URL pins database identity; replacement rejects existing binding.
Local identity includes database ID for deliberate retained-history replay.
One destination per invocation; no relay/fan-out.

## Acceptance protocol 2

GET capabilities, POST batches and GET batches/{stream}/{batch} under
/api/v2/ingestion/. Limits: 1 MiB, 256 entries, 256 UTF-8 bytes per metadata string.
Strict decoding rejects unknown/private fields, duplicate keys, invalid UTF-8,
trailing values and invalid envelopes.

Transaction commits exact request/hash, every mapping, deduplicated evidence,
receipt and scopes. Concurrent/overlapping/intrabatch duplicates succeed.
Changed bytes under stream/batch or changed evidence under stream/sequence
reject atomically. Safe semantic problems are accepted for async diagnosis.

202 pending, 200 terminal, 409 identity/conflict, 400 malformed/private, 413 size,
422 version, 503 busy/unavailable. Receipt binds database/dataset/stream/batch,
request hash, contiguous range/count, acceptance revision/time.
Collector validates acceptance independently of mutable processing status.

Sync finishes at acceptance, not query visibility. TUI then reads available
confirmed data; Reload queries only. Browser reports lag/separate estimates.
Service wait explicitly waits up to 30 seconds for maintenance/fixtures.
Deprecated protocol-1 bridge preserves synchronous legacy bytes/receipts.
Flush retained old requests before raw delivery; new sync never populates legacy
canonical tables. Legacy maintenance handles old tables only.

## Processing

One server owns file/lifetime lock. Connections share its engine; HTTP never
reopens paths. Four admission slots, shared short write transactions, one worker.
Scopes are native sessions or unresolved source lineages; Codex ancestry connects
dependencies. Late parent arrivals invalidate terminal dependent outcomes.

Read consistent connected component; interpret outside write lock; fence generation,
complete component membership and each scope revision before publishing.
Stale work retries; unrelated ingestion cannot starve processing.
Facts/estimates/provenance/outcomes/completed revisions commit atomically.
Failures stay pending with fixed error/attempts/exponential retry capped 64 seconds;
independent scopes proceed. Join worker before closing storage.

Pure processor preserves five token components and their sum. Require usable
native session/time and nonnegative bounded counters. Missing provider/model
become unknown; Claude absence uses inferred maybe-anthropic. Aliases canonicalize
server-side. OpenCode v2 supersedes v1 native assistant message; Pi native IDs;
Claude request/message plus source timestamp revision, equal-revision conflicts
ambiguous. Contradictory native aliases never confirmed.

Codex copy proof requires explicit ancestry, native turn/provider/model, complete
last/cumulative snapshots and field presence. Missing/cyclic/conflicting ancestry/
uncertain boundaries stay ambiguous. Known copies, unchanged cumulative and stale
snapshots never inflate confirmed/estimated. Equal independent sessions remain
independent. Later same-lineage turn context can resolve missing model metadata.

Usable ambiguity gets stable candidate identity/reason and separate estimates.
Unusable session/time/counters remain unresolved receipt diagnostics.
Confirmed/estimated never combine. Conflict withdraws confirmed projection while
retaining evidence. Confirmed reflects current evidence/rules, not infallible truth.

## Generations and preserved history

POST /api/v2/processing/reprocess or service reprocess queues new generation.
Old published data remains queryable; restart resumes. Immutable receipt status
reflects target/latest scope revision. Activate only after every scope covers
latest accepted inputs, atomically advancing published generation/revision.
Raw arriving during build must be covered.

Imported baselines persist until matching contribution ID and equal components
or valid newer native revision prove coverage. Proof binds exact new payload hash,
preventing changed unproven projection from hiding history. Replace once;
unmatched history remains. Rebuilding 300 of 500 keeps 500. Ambiguity cannot erase
imported confirmed baseline.

## Querying and deployment

Typed columnar SQL filters/groups/sorts/pages active facts and reconciled baselines;
dashboard queries never scan raw JSON. Raw tables' mere presence does not slow
analytics scans. Read transaction covers metadata/summary/rows/facets. Responses
identify database/generation/published revision/input revision; browser guards
snapshot identities. Values are bound; identifiers are fixed selections.
Clamp page before offset arithmetic; reject unsafe JavaScript integer aggregates.
IANA/fixed local offsets apply to calendars. Pages 50 default/200 max, dimensions
12 chart groups, time charts 1000 buckets, session facets 100.
Repo-only location filters; unknown visible. Context: per-session prompt-side
peak input+cache read+cache write, then average/median/max.
TPS terminology remains; this change adds no timing inference.

One connected session/ancestry component is processed in memory. Huge components,
partitioning/preaggregations and retention need measured follow-up, not benchmark
claims in this change.

Local public dashboard/query: 127.0.0.1:8765 or optional 0.0.0.0. Writes/reprocess/
lifecycle: private mode-0600 Unix socket plus instance verification; public write
routes return 404. Remote foreground server requires server-db-path and exposes
shared HTTP ingestion/query; auth deferred.

Go embeds committed web assets. CGO/C/C++ needed to build DuckDB; native CI/release
Linux/macOS amd64/arm64 runners. Production needs no JavaScript runtime.
Verification: format/lint/schema/API, native semantic fixtures, full/race tests,
native build/JS-absent smoke and browser E2E.
