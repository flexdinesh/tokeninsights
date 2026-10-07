# Sanitized raw ingestion and server-owned processing

Status: **Accepted; implemented**. Date: 6 October 2026.
Deployment/authentication and separate application-store choices superseded by
[ADR 0008](0008-personal-hosted-composition-and-capabilities.md). Versions below
record this decision's original implementation; [design.md](../design.md) is current.
Supersedes collector-owned normalization, normalized-only ingestion, server
SQLite analytics and synchronous publication completion in
[ADR 0006](0006-collector-server-ingestion.md) and the corresponding parts of
[system design](../system.md). Deployment/configuration decisions remain unless
explicitly revised below. [design.md](../design.md) describes the implementation
and its concrete storage contracts.

Architecture and concrete schema/protocol/import contracts separately approved.
Implemented with collector schema 17, DuckDB schema 1 and raw protocol 2; see the
[implementation plan](../raw-ingestion-plan.md).

## Context

Before this decision, the Collector interpreted Durable Sources, normalized
token usage and published canonical facts. Server history survives Collector deletion, but the
server cannot reprocess original evidence. Corrections depend on surviving
client sources and compatible Collector processing. The aim is a simple,
accurate, durable system with one processing implementation shared by local and
remote deployments.

Source snapshots are observations, not necessarily distinct token consumption.
Streaming updates, copied histories, cumulative counters and ancestry require
interpretation. An append-only table of every received counter would inflate
usage. Preserve evidence immutably; maintain a replaceable canonical projection
with one active contribution per proven stable identity.

## Ownership and storage

| Component | Store | Responsibility |
| --- | --- | --- |
| Collector | SQLite | Discovery, allowlisted extraction, source continuity/context, durable outbox, immutable requests, per-destination acceptance acknowledgements |
| Server application | SQLite, when needed | Future users, auth, tokens and application settings; outside this decision's ingestion transactions |
| Server data | One DuckDB file | Raw evidence, acceptance receipts, processing queue/status, provenance and queryable projections |

The Collector preserves native IDs and counters. It does not split inclusive
counters, infer providers, merge usage snapshots, resolve copied ancestry or
compute canonical contribution IDs. Operational sequences, byte hashes and
explicitly marked safe location enrichment are permitted; they are not facts
about consumption. The Collector still needs versioned extraction and continuity
state. Central processing reduces its responsibilities; it does not eliminate
every Collector bug or SQLite compatibility requirement.

Within the server DuckDB file, use logical schemas:

| Schema | Logical records |
| --- | --- |
| `raw` | Immutable sanitized observations and context records |
| `ingestion` | Server/dataset identity, immutable acceptance receipts, submitted-item-to-evidence mappings |
| `processing` | Durable scopes, input/dependency revisions, generation-specific outcomes and diagnostics |
| `analytics` | Active-generation session-centric contributions, dimensions, provenance and separate estimates |

Schema names organize tables. Analytics queries explicitly select narrow
analytics tables; they do not scan raw payloads. Schemas do not isolate storage
I/O, memory pressure or write contention. Keeping related state in one file
allows the required transactions without cross-database commits. See DuckDB's
[schemas](https://duckdb.org/docs/current/sql/statements/create_schema) and
[transactions](https://duckdb.org/docs/current/sql/statements/transactions).

One Go server process owns the data file. An internal write coordinator serializes
short transactions from ingestion and processing; workers compute outside those
transactions. Use bounded admission and batches, not one transaction per token
component. Queries use separate connections and consistent snapshots. This
chooses predictable embedded ownership rather than a multi-process worker pool;
see [DuckDB concurrency](https://duckdb.org/docs/current/connect/concurrency).

The durable queue is processing records in DuckDB; no external broker. Acceptance
commits evidence, item mappings, receipt and needed work together. Processing
commits complete replacement contributions, provenance, outcomes and progress
together. Application SQLite never participates in either transaction.

Both deployments use this model and serve HTTP queries and the dashboard. Local
ingestion is confined to a private socket or a local-only listener. A local
dashboard/query listener may bind to `0.0.0.0` without exposing ingestion or
administration. Remote ingestion accepts multiple Collectors and requires a
future authentication/identity boundary; its implementation is out of scope.
This decision does not authorize public unauthenticated remote writes. Begin
with one shared dataset; future authenticated ownership supplies its namespace.

## Extraction and reliable delivery

The [raw ingestion whitelist](../raw-ingestion-whitelist.md) defines the fields
needed by the current OpenCode, Pi, Codex and Claude Code adapters. Preserve
source types, missing versus null versus zero, native timestamp representation,
both supported aliases and complete usage snapshots. Send session/turn/ancestry
context as evidence alongside counters so the server can reproduce attribution.
Never upload complete source records or unknown nested objects.
Store received sanitized record bytes without counter or timestamp rewriting;
keep a separate deterministic, type-preserving fingerprint for evidence lookup.

Source paths, filesystem refresh markers and absolute working directories remain
local. Content, tool payloads, credentials and full Git URLs are excluded. A
Collector bug that omits or changes an allowed source field cannot be repaired
from server evidence; reprocessing is bounded by the approved whitelist.

Commit captured sanitized evidence, required extraction context and the captured
source checkpoint atomically in Collector SQLite. Delivery progress is separate:
acknowledgements are per destination and advance only after a matching durable
receipt validates server/dataset binding, stream/batch, request hash and complete
submitted-item coverage. Save immutable request bytes before sending. Lost responses
or crashes retry those bytes and the same batch identity. Only acknowledged
outbox data may be compacted; retained history remains available on the server.
A new destination receives retained outbox history and available re-extracted
sources, not a promise to recover already compacted, vanished sources. Historical
transfer between servers requires an explicit export/import, not automatic routing.

Incremental JSONL reads require verified continuity, a complete record boundary
and the context needed to interpret the suffix. Defer unfinished tails. Mutable
SQLite sources require checking existing rows for changes unless the source
provides a reliable native change log. Truncation, replacement or uncertain
continuity triggers rereading; cursors optimize reads, never prove correctness.
Changing an extraction version requires reevaluating its checkpoint/context.

## Four separate identities

| Identity | Meaning and scope |
| --- | --- |
| Local source continuity | Which local source can safely resume; never proves remote usage identity |
| Delivery | Dataset/server binding, producer stream and batch ID, plus exact request hash; retries only |
| Evidence | A qualified native record identity and type-preserving sanitized snapshot fingerprint, with required context/order evidence; identifies an observation/version |
| Contribution | Dataset, harness and proven native session/message/request identity or documented harness-specific witness; dedupes actual usage |

Retain every submitted ordinal's evidence mapping and eventual disposition,
including repeated items within one batch. Fingerprints alone do not prove two
ID-less events are the same request. When no suitable native record ID exists,
retain opaque source-instance/order metadata to distinguish observations,
without claiming it supplies a globally stable contribution identity. Contribution
IDs must not depend on hostname, Collector installation, arrival time or filesystem path.
Equal counters alone never prove copies. Dataset scope is shared across
Collectors; inventing a per-Collector namespace would break copied-history dedupe.

OpenCode uses native session/message IDs, preserving V1/V2 origins for server
precedence. Pi uses its native session header and message ID when present.
Claude Code preserves session/message/request aliases, UUID fallback and source
timestamps without claiming UUID alone proves a request identity. Codex requires
native session/turn context, ancestry and immutable snapshot witnesses; it does
not provide a universal native token-event ID. Missing/conflicting evidence is
retained with ambiguity diagnostics rather than assigned a confident identity.

## Acceptance, deduplication and status

Validate bounded, strict allowlisted payloads before storing. Structural/privacy
failures reject the whole batch without receipt or cursor advancement. A safe
record with missing identity, contradictory counters or missing context is
accepted as evidence for asynchronous diagnosis.

Concurrent duplicate batches, overlapping batches and duplicate records within
a batch succeed. Database uniqueness and the write coordinator arbitrate races;
avoid check-then-insert outside the transaction. Reuse evidence/work when identity
is proven, and record duplicate suppression during processing. Contribution
uniqueness protects totals even when early evidence dedupe is impossible.
Changed bytes under an existing stream/batch ID conflict; ordinary duplicates do
not. Updated snapshots of a native record are new evidence, not exact duplicates.

| Result | HTTP | Collector meaning |
| --- | --- | --- |
| Durable acceptance; any item needs queued/running/retry/dependency work | `202` | Accepted; may advance delivery acknowledgement |
| Durable acceptance; every item already has a terminal outcome for the applicable generation/input revision | `200` | Accepted; inspect dispositions to learn whether counted, suppressed or ambiguous |
| Same delivery identity, different request bytes | `409` | Do not advance; resolve identity conflict |
| Invalid structure, size or supported contract | `400`, `413` or `422`, as applicable | Rejected; do not advance |
| Temporarily unable to accept durably | `503` | Retain request and retry |

`sync` waits for acceptance only. `202` is returned after commit, not after an
in-memory enqueue. A replay returns the same immutable acceptance receipt;
current processing information is a separate, mutable response section. Its
HTTP status may change from `202` to `200` as work completes. A successful status
never means every record contributed to confirmed usage.

Provide a read-only status resource identified by receipt, with item dispositions,
counts, diagnostics, processing generation and input/dependency revision.
Lookups use indexed registry records, not raw scans. Empty batches are invalid.
Context records can complete without contributing usage. Mixed batches are
`202` while any member remains nonterminal; clients need not poll to release
accepted outbox data. Query endpoints independently report processing lag.
This is a new protocol; do not silently change protocol 1's committed/queryable
receipt semantics. Exact endpoint and wire definitions belong to implementation.

## Processing and ambiguous evidence

Process affected sessions and dependency scopes, not isolated counters alone.
Late parent evidence or turn context invalidates dependent outcomes and enqueues
work. A processed boolean or a whole-file marker cannot express this: outcomes
are keyed by evidence, processing generation and relevant input/dependency
revision. `200` describes those stated revisions, not permanent immunity to
reprocessing.

Durable scope revisions and generation track pending/completed work; retry time,
attempts and fixed error codes track failures. One worker snapshots a complete
dependency component, computes outside the write coordinator, then fences every
scope revision and component membership before committing. Restart resumes
pending scopes. New input and reprocess requests clear obsolete retry backoff.
Missing dependencies are classified ambiguous and invalidated when context arrives.
Permanently unusable evidence is retained with a terminal diagnostic.


Canonical facts are a projection, not irrevocable conclusions. A contribution
row holds all token components together, its stable session identity, source
occurrence time, dimensions and provenance. Preserve missing source metadata;
canonical model/provider fall back to `unknown`, with Claude Code's existing
`maybe-anthropic`/`inferred` convention. Server processing owns inclusive-counter
adjustments, snapshot precedence, provider inference and copy proofs. It selects
one complete valid snapshot; component-wise maxima may invent a nonexistent
snapshot and are prohibited.

Replace or remove obsolete derived contributions atomically when evidence
proves their interpretation changed. Source disappearance alone never retracts
usage. Store many-to-one provenance and reasons for suppression or supersession.
Build new processing generations alongside the active one, catch up to a known
input revision, validate coverage, then switch atomically; do not mix generations.

Ambiguous evidence is marked `ambiguous` in processing storage with reason codes,
candidate identities, conflicts and supporting evidence references. It stays
outside confirmed totals. Where usable counters and attribution exist, expose
it separately as **estimated usage**, explicitly indicating possible overlap or
unresolved identity. Estimates are uncertain observations, not proven unique
consumption, and must never be added automatically to confirmed usage. Known
duplicates and superseded snapshots are excluded from estimates too. Evidence
without usable counters/time/session remains visible as an unresolved count;
invent no numeric usage or timestamps. Resolving ambiguity moves the projection
atomically, removing its old estimate as appropriate.

## Querying and retention

Query typed analytics columns only, using source occurrence time for date ranges
and a distinct server acceptance time for ingestion health. Keep native IDs and
provenance available for debugging without joining raw JSON on each dashboard
request. Apply date, harness, provider, model, session and location filters in
SQL before aggregation, ordering and pagination. Calendar buckets use the
reporting IANA timezone. Return active generation/revision consistently across
rows, totals and facets; pin or reject stale pagination snapshots. Include pending
processing information: accepted evidence is not a promise of immediately fresh
totals. A snapshot is consistent with its published processing revision.

Order/compact analytical storage around common time scans; use constraints and
targeted indexes for identity/status lookups. Raw table volume does not add rows
to fact scans, although shared resources still matter. DuckDB's
[indexing guidance](https://duckdb.org/docs/current/guides/performance/indexing)
describes ordering/zonemap benefits and selective index use. Add rebuildable
summaries only when needed. Distinct sessions, peak context and medians cannot
be obtained by summing every bucket's summary. Use exact integer accounting and
explicit overflow/transport limits. TPS concepts remain; evidence retention
does not by itself establish a reliable duration.

Initially retain unique sanitized evidence, confirmed history, ambiguous evidence
and delivery/dedupe registries indefinitely. Prune transient logs, redundant
operational sightings and obsolete projection generations after safe cutover.
Retention enables replay but is not a backup; preserve recoverable server
storage. No additional infrastructure or capacity target is required now.

## Existing history and compatibility

Existing history means canonical facts already committed to today's server
SQLite. Some original sources may no longer exist; those facts cannot be
reconstructed as raw evidence. Preserve them as a marked legacy baseline with
their identities, token components and provenance, keeping the original database
untouched during verified import. Do not fabricate raw payloads from legacy facts.

An active confirmed projection includes each legacy contribution until a new
projection proves its replacement. Coverage requires identity reconciliation
with the specific legacy contributions, supported processing semantics and
atomic activation. A successful job, a matching total or a date-range scan alone
does not prove coverage. A replacement may legitimately correct counters; it
must explain which old contribution it replaces. Unmatched legacy contributions
remain queryable. Potentially overlapping new evidence without a proof stays
ambiguous/estimated; proven distinct new contributions count normally.

Example: existing usage is 500 tokens. Reprocessing accounts for 300 tokens of
that history. Keep the other 200; confirmed usage stays 500, not 800 or 300.
Never union legacy and rebuilt copies without reconciliation, and never delete
all server history because a Collector was reset or a new generation appeared.

Preserve database identity, receipts and destination bindings through a deliberate
upgrade; do not make storage replacement look like a fresh empty server. The approved import contract verifies supported legacy server SQLite read-only
and preserves database identity and receipts. Legacy protocol-1 completion remains unchanged.

## Trade-offs and implementation boundaries

Centralized interpretation and retained evidence support replay after client
loss and keep normalization consistent. Costs are additional server storage,
asynchronous freshness, retained privacy-sensitive metadata, and processing
state/projection maintenance. DuckDB suits columnar analytics and keeps commits
in one file; small status mutations and embedded process ownership are constraints
to manage. Keeping status in separate SQLite would instead require a recoverable
cross-store protocol. We choose the single data-file transaction boundary now.

Implement operation boundaries for acceptance, processing and snapshot queries;
HTTP handlers should not open database paths themselves. Avoid a generic storage
plugin framework. Keep production Go-only, with committed embedded browser
assets; verify DuckDB native-driver packaging separately before release.

Required behavioral verification covers duplicate/concurrent/overlapping batches,
changed batch bytes, lost responses, every commit crash boundary, stale revisions,
late ancestry, rewritten sources, snapshot conflicts, ambiguous-to-confirmed
transitions, generation cutover and legacy replacement without double-counting.
Use existing semantic fixtures for all token components and copied histories.
Performance tuning follows working, accurate queries; no benchmark gate now.

Unresolved questions: none. Concrete contracts separately approved in the
[implementation plan](../raw-ingestion-plan.md).
