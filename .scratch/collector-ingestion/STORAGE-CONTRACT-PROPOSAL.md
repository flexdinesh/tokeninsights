# Collector/server storage and publication contract

Status: **Accepted; implemented in PR #53.** Approved on 5 October 2026.
The filename retains its review history; this document now records the approved
contract rather than a request for approval. References: G01–G11 in
[architecture](../../docs/collector-server-architecture.md), F01–F14 in
[failure contract](../../docs/collector-ingestion-tests.md), and actual verification
in [VALIDATION.md](VALIDATION.md).

## Roles and compatibility

Create fresh `collector.sqlite` and `server.sqlite` under the product data
directory. Leave the former `tokeninsights.sqlite` untouched. No legacy import,
row-preserving split, identity aliases or overlapping legacy/rebuilt accounting
is implemented. Selected collector/server paths must be distinct after canonical
symlink resolution and existing hard-link identity checks, including remote sync.

| Role | Authoritative schema | Embedded checked copy | Current version |
| --- | --- | --- | --- |
| Collector | `schema/schema.sql` | `packages/cli/internal/db/schema/schema.sql` | SQLite 15; data generation 6; application ID 1414091587 |
| Server | `schema/server.sql` | `packages/cli/internal/serverstore/schema/server.sql` | SQLite 1; application ID 1414091606 |

`check-schema` verifies both contracts. Role checks precede writes or producer
recovery. Existing wrong-role, unknown, unsupported or corrupt databases are
rejected without resetting their contents. Server opening verifies its durable
metadata and current identity/semantics versions; it never invokes collector
recovery or attempts to reconstruct server history from source files.

Protocol, identity and accounting semantics are separate versions, currently all
1. Current compatibility requires an exact supported match; no mixed-version
acceptance or automatic identity migration is advertised. An incompatible change
requires an explicit migration/upgrade design or rejection without data loss.
The server preserves historical facts even if their producer artifacts disappear.

One server database binds one owner, initially `default`; requests cannot select
or impersonate an owner. Multiple machines publish to that owner. The fixed
namespace assumes native IDs distinguish histories within each harness. Hostname,
stream and installation identity do not affect accounting identity. Configurable
profile namespaces and multi-tenant authentication remain future contracts.

## Collector persistence

Existing metadata-only raw tables, observations, canonical tables, diagnostics,
normalization work and source continuity remain collector-local. Prompt/assistant
text, tool arguments/output, secrets, provider payloads and full source paths are
not retained as raw facts or transmitted through publication.

| Table | Durable purpose and constraints |
| --- | --- |
| `publication_state` | Singleton stream ID, identity/semantics version 1, fixed source namespace and creation time. Stream is regenerated with a new collector database; it never identifies facts. |
| `publication_journal` | Positive increasing safe-integer sequence, fact ID, payload hash, immutable allowlisted fact JSON, versions, optional typed source revision and local creation time. Triggers reject updates/deletes. |
| `publication_entities` | Fact ID primary key, current payload hash, latest journal sequence FK and optional source revision. Suppresses unchanged fact envelopes. |
| `publication_destinations` | Destination ID primary key, endpoint, bound server database ID, contiguous acknowledged sequence, latest receipt and acknowledgement time. |
| `publication_batches` | Batch ID primary key, destination FK, stream/server identities, first/last sequence, exact request hash and BLOB, nullable receipt BLOB and acknowledgement time. Unique destination/range; partial unique index permits one unacknowledged batch per destination. Request fields are immutable. |

Normalization and journal recording share the canonical transaction. Initially
publication scans the canonical snapshot inside that writer transaction;
`Record` suppresses an unchanged payload hash **and** unchanged full envelope.
A changed session range/message occurrence creates a journal entry even though it
does not change the contribution hash. This lets server references merge without
adding another usage fact. A later independent scan after canonical commit would
not satisfy G04.

Missing raw occurrence, missing stable identity, conflicting candidate identities
or invalid normalized values yield fixed local diagnostic codes and are withheld
from publication. Local canonical availability is not proof of publishability.

`PrepareBatch` first returns any persisted pending request unchanged. Otherwise
it reads journal entries after the cursor in sequence order, requires a contiguous
range and saves the largest prefix fitting entry/body limits. It never advances
progress before delivery. Entries are complete fact snapshots with their
session, optional message and optional location references. No journal coalescing
or deletion is performed.

`Acknowledge` strictly decodes and binds the receipt to database, stream, batch,
range and exact request hash. Counts must match the entry count. The destination
cursor must immediately precede the range. Receipt and cursor commit together.
An already acknowledged batch accepts an identical receipt as a no-op and rejects
a changed receipt.

Bindings never silently change endpoint or durable server identity. The collector
derives a remote binding from endpoint, pinning its first server database ID.
Replacement at that endpoint rejects. Local binding includes endpoint plus server
database ID: a replaced local server creates a new cursor at zero and replays the
retained journal. Changing endpoint also creates a new binding; old state remains.

Journal and receipt retention are indefinite initially. Deleting undelivered
collector data is recoverable only when sufficient source evidence remains.
Source disappearance does not retract committed server history.

## Server persistence

Server SQLite contains canonical query tables and ingestion metadata only.
Integer primary keys are private query/storage references; stable semantic keys
come from publication. There are no raw facts, ingest runs, parser state, source
paths, continuity cursors, normalization jobs or legacy identity aliases.

| Table | Contract |
| --- | --- |
| `server_metadata` | Singleton durable random database ID, owner, identity/semantics versions, analytics revision, last committed ingestion time and creation time. |
| `canonical_sessions` | Unique published session semantic key, harness, native session ID, earliest/latest source occurrence. No raw FK. |
| `canonical_messages` | Unique published message semantic key, session FK, harness, native message ID and earliest source occurrence. No raw FK. |
| `usage_locations` | Unique published location semantic key, normalized directory/repository keys, bounded labels and repository provenance. No full directory path. |
| `canonical_token_usage` | Unique published fact semantic key, occurrence, session/message/location FKs, provider/model attribution, scope, quality, countability and five additive components/total. Adds payload hash and typed source revision; no raw or ingest-run FK. |
| `ingestion_receipts` | Composite stream/batch primary key, exact request hash, submitted range, original committed receipt JSON, counts, commit time and resulting revision. |
| `ingestion_producers` | Stream primary key, bounded optional hostname label and last committed ingestion time. Label is not identity or source completeness. |

References must match their declared harness/native session relationships.
Existing reference identity contradictions are 409. Session first/last values
merge minimum/maximum source times; message occurrence merges minimum. Location
keys identify an exact bounded metadata tuple; contradictory labels/provenance
are conflicts. An older Claude fact does not introduce its obsolete location.

A single transaction validates and applies the full batch, checks aggregate
numeric safety, records its receipt, updates producer metadata and commits.
Receipt uniqueness and stable fact uniqueness are durable SQLite constraints.
A crash before commit exposes neither partial facts nor a success receipt.
A crash after commit permits exact replay across server restart.

Revision increases once per batch that changes contributions or merged
references. A new all-no-op batch records its receipt and last ingestion time
without advancing revision. Exact replay returns the original receipt without
updating any state. Queries read revision and durable database ID in the same
transaction as their analytics snapshot. Process instance ID changes on restart;
database epoch remains stable.

## Stable identity and value hashes

IDs use SHA-256 hex of UTF-8 JSON string arrays with this exact ordered encoding:

| Entity | Array elements |
| --- | --- |
| Session | `session-v1`, `default`, harness, native session ID |
| Message | `message-v1`, `default`, harness, native session ID, native message ID |
| Fact | `fact-v1`, `default`, harness, native session ID, native message ID or empty string, native request ID or empty string, usage scope |
| Location | `location-v1`, directory key, repository key |

Every fact has a native session and at least one usable native message/request ID.
Usage scope is currently `message`. Server validation recomputes IDs rather than
trusting producer-supplied hashes. Tokens, stream/batch, sequence, hostname,
filesystem path and observation time are excluded from the fact tuple.

The typed normalized payload hash includes attribution, source occurrence,
components, countability, optional location and source revision. It zeros session
first/last and message occurrence envelopes before deterministic Go struct JSON
encoding. Transport metadata is outside the fact. The request hash instead binds
the exact persisted HTTP request bytes, including JSON formatting; changed bytes
under an existing stream/batch identity conflict even if decoded values match.

Codex is an immutable-event witness exception, not a native request revision:
its adapter-derived message ID retains source session/turn/time and typed
last/cumulative token snapshot evidence, preserving absent-versus-present fields.
Distinct same-time snapshots remain distinct. Proven ancestry replay keeps the
original owning event identity. Counter changes alter that witness. Existing
line-based fallbacks guarantee unchanged-input reproducibility, not stability
after arbitrary inserted lines.

OpenCode/Pi stable native message identities distinguish equal-valued requests.
Pi records lacking usable native identity are withheld. Claude request identity
is preserved in metadata locally and explicitly published. The only supported
source revision rule is `claude-source-timestamp-v1`: native message and request
must both exist; revision value equals source occurrence time. With changed
payload, newer source timestamp replaces one contribution, older is a stale
no-op, and equal source timestamp conflicts. Other changed payloads conflict.
Arrival order, collector sequence, largest counters and installation age never
provide universal revision precedence.

## HTTP publication

The Go `publication` contract is independent of pipeline/adapters.
[OpenAPI](../../docs/openapi.yaml) describes the HTTP boundary. Decoder rejects unknown fields, duplicate object
keys, trailing JSON, missing required fields and non-integer JSON numbers.

- `GET /api/v1/ingestion/capabilities` returns exact supported protocol/identity/
  semantics versions, durable database ID and configured contract limits.
- `POST /api/v1/ingestion/batches` accepts versions, expected database ID,
  stream/batch IDs, first/last sequence, optional hostname and ordered entries.
  Each entry has its sequence and a complete normalized fact.
- Success returns database/stream/batch/range/request hash, inserted/updated/no-op
  counts, committed time and analytics revision. Each entry contributes exactly
  one result count; a rejected batch has no receipt.
- Errors expose fixed `code` and `stage`, never raw body, source paths or tokens.
  Ingestion authentication errors use the same JSON envelope.

| Status | Meaning |
| --- | --- |
| 400 | Invalid JSON, identity, references, integer/token invariants or aggregate limits |
| 401 | Missing/invalid configured authentication |
| 409 | Database mismatch, changed batch replay, unsupported changed fact or reference conflict |
| 413 | Excess body or explicitly classified size violation |
| 422 | Unsupported protocol, identity or accounting semantics version |
| 503 | Full bounded admission or busy SQLite |
| 500 | Transaction/database failure without a success receipt |

Current concrete bounds: 1 MiB body, 256 facts, 256 UTF-8 bytes per ID/label,
four accepted concurrent ingestion requests, one SQLite connection, five-second
busy timeout. Empty or inconsistent ranges fail validation. All protocol integers
must be exact and within 0–9,007,199,254,740,991; sequences/ranges are positive.
Five token components must add exactly to total without overflow. Countable
server component/total aggregates must remain in that domain. No rounding,
wrapping or best-effort partial acceptance is permitted.

The durable queue lives in the collector journal; server admission is synchronous
and bounded. Requests exceeding capacity fail, not return accepted-job success.
Network error, timeout or cancellation has unknown commit outcome. Keep the
pending bytes and resume manually. No asynchronous normalization inbox,
autonomous retry agent, retraction or producer backflow is implemented.

## Composition and follow-ups

Local startup creates empty server projection and never collects. TUI and web
read query APIs. `tokeninsights tui` is the read-only REST viewer;
`tui --sync` explicitly runs collection first. Root commands are `service`,
`sync`, `tui` and `server`. Advanced producer maintenance uses
`collector normalize`, `collector reset-canonical` and `collector reset-all`;
it preserves server history. Former `view` and top-level maintenance names remain
deprecated aliases. `GET /api/v1/sync` is read-only readiness/revision compatibility
status; POST is removed. Server saved configuration contains database/bind/auth
settings, not source roots. Producer labels never imply source completeness.

Every exposed non-loopback binding requires a token regardless of local/remote
command. Loopback is default. TLS deployment, credential provisioning,
multi-tenant ownership, identity migration, retention and optional asynchronous
hook workers are deferred. Thin plugins invoke the same bounded collector CLI;
they contain no independent accounting or delivery persistence.

No unresolved questions for the accepted implementation scope.
