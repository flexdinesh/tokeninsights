# Collector/server storage contract proposal

Status: **approval required; prose only, no approved schema or wire change**.
Scope: implementation in PR #53. References: G01–G11 in
`docs/collector-server-architecture.md`; F01–F14 in
`docs/collector-ingestion-tests.md`.

## Decisions

- One server database binds one owner. Multiple machines publish to that owner;
  copied native histories dedupe. Multi-tenant authorization and provisioning are
  deferred. The request cannot select or impersonate an owner.
- Hostname is display metadata, never identity. Do not introduce a generated
  machine UUID into session/fact IDs. Stable configured source namespaces are
  only needed for harness profiles known to reuse native IDs; default is one
  native namespace per harness. Such configuration must survive collector DB
  deletion and travel with copied histories.
- Separate collector/server SQLite files. Server tables contain canonical
  metadata, receipt state, and its durable identity; no raw facts, parser state,
  source paths, harness cursors, or normalization work.
- Retain existing canonical query columns and indexes at the server. Change
  opening/lifecycle and status/coverage boundaries instead of rewriting correct
  aggregation SQL. Server migration must never invoke collector reset/rebuild.
- Journal canonical fact snapshots in the same transaction that commits local
  canonical changes. Referenced session/message/location metadata accompanies
  each fact, making every bounded batch self-contained.
- Store exact immutable request bytes before send. One pending batch per
  destination; ack advances only that destination's contiguous cursor.
- Server transaction commits fact mutations, reference merges, analytics
  revision, and matching receipt together. Invalid/conflicting batches roll back
  completely. No success receipt or collector progress for a rejected batch.
- Same stable ID and same canonical payload succeeds as a no-op. Changed values
  require an approved adapter-specific source revision rule; otherwise 409 and
  no mutation. Collector journal sequence is never source revision evidence.
- Session occurrence ranges merge minimum/maximum source occurrence times;
  collection/observation wall clocks do not cross ingestion.
- Keep journal and receipts indefinitely initially. This trades storage growth
  for simple, demonstrable replay/rebuild behavior; retention needs a separate
  approved recovery design.
- Normal delivery has one pending batch per destination, so batches do not
  normally reorder. Server acceptance does not require arrival in journal order;
  only source evidence determines fact revision. Collector cursor progression
  still requires the matching contiguous acknowledged range (F06/G07).

## Collector additions requiring approval

Current `schema/schema.sql` and its embedded copy remain collector contracts.
Add these tables; preserve existing raw/canonical tables and their values through
an additive versioned migration rather than destructive generation recovery.

Proposed collector schema version 15/data generation 6; server schema version 1.
The collector generation bump records the changed OpenCode/Claude identity rules.
Use a dedicated row-preserving split migration, not today's generation-reset
path: snapshot legacy history and prove/block identity mappings before marking
the new generation compatible. Distinct SQLite application IDs identify roles;
both opening paths reject the other role before any recovery or mutation.

| Table | Columns / constraints |
| --- | --- |
| `publication_state` | singleton `id`; random `stream_id` created with collector DB; `identity_version`, `semantics_version`; `source_namespace`; creation time. Stream identifies transport history, never facts. |
| `publication_entities` | `fact_id` primary key; latest canonical `payload_hash`; latest journal `sequence`; typed source revision fields. Enables no-op normalization to avoid duplicate journal events. |
| `publication_journal` | increasing `sequence` primary key; `fact_id`; `payload_hash`; immutable allowlisted `payload_json`; identity/semantics versions; source revision rule/value; local creation time. No deletion initially. |
| `publication_destinations` | stable configured `destination_id` primary key; endpoint; bound server `database_id`; acknowledged `sequence` default zero; last receipt/ack time. Endpoint changes or replacement database create a new delivery binding with cursor zero. |
| `publication_batches` | `batch_id` primary key; destination FK; stream; server database ID; first/last sequence; request hash; exact request bytes; nullable validated receipt bytes/ack time; unique destination+range; at most one pending batch per destination. |

FKs/local IDs never leave the collector. Counters, range bounds, JSON validity,
nonempty IDs, and supported versions receive schema checks and application
validation. Journal sequence contiguity is defined by retained journal entries;
batch range and entries must match exactly. No coalescing different sequences
before acknowledgment in the first implementation.

Normalization identifier refresh must journal changed entities within its
existing transaction, including alias/model normalization updates. A later
scan of canonical tables is not an adequate substitute for G04. Reset-canonical
may preserve publication history and regenerate no-op entities; reset-all or
collector deletion creates a new stream and republishes rebuilt facts.

## Separate server schema requiring approval

Authoritative `schema/server.sql`; checked embedded copy in `internal/serverstore`;
own supported schema version/lifecycle. Extend `check-schema` to check both
contracts independently. Do not masquerade the server file as collector version
14 or create unused raw tables to satisfy collector opening logic.

| Table | Columns / constraints |
| --- | --- |
| `server_metadata` | singleton `id`; durable random `database_id`; bound `owner_id`; schema/identity/semantics versions; analytics `revision`; last ingestion time; creation time. No rebuild-pending/source key. |
| `canonical_sessions` | Existing query shape: integer `id`, stable `semantic_key` unique, harness, native session ID, first/last occurrence times. Remove `primary_raw_fact_id`. |
| `canonical_messages` | Existing query shape: integer `id`, stable `semantic_key` unique, session FK, harness, native message ID, occurrence time. Remove raw FK. |
| `usage_locations` | Existing canonical location fields; fingerprint unique; no full source paths. Location absent is valid. Preserve existing privacy rules. |
| `canonical_token_usage` | Existing canonical analytics columns including integer IDs and references. `semantic_key` is stable published fact ID. Remove `primary_raw_fact_id` and `ingest_run_id`; add payload hash and typed source revision rule/value. No token component or session may be null. |
| `ingestion_receipts` | `stream_id`, `batch_id` composite key; exact request hash; first/last sequence; committed receipt JSON; committed time; inserted/updated/no-op counts; resulting analytics revision. Durable across restart. |
| `ingestion_producers` | stream ID primary key; bounded optional hostname label; last committed ingestion time. Used only for availability labels, not identity or completeness. |
| `legacy_identity_aliases` | Optional migration-only legacy canonical key -> stable fact ID alias, only where retained metadata proves the mapping. No arrival-based matching. |

Existing canonical token/session/location indexes and constraints carry over.
References must resolve to the same harness/session; an attacker cannot supply a
valid message ID that belongs to another session. Batch reference dedupe is by
stable identity. Incoming contradictory metadata is an explicit conflict, except
documented source range merges. Query snapshots read server revision in the same
transaction as analytics. A duplicate batch returns its original receipt;
duplicate facts in a new batch do not increase analytics revision.

Current raw coupling needing extraction: `ViewerDayCoverage` reads raw queue and
source job state; `LatestIngestHostname` reads ingest runs; server/controller
startup can run parsing; collector `Open` validates rebuilding generations.
Replace these with saved-data ingestion status. Server must not claim checked
or empty source coverage from ingestion alone. Shared canonical queries use
`db.Reader`; server supplies its own validated read transaction.

## Wire concepts requiring approval

Typed Go protocol package shared by collector and server, with no dependency on
pipeline/adapters. Authoritative OpenAPI describes HTTP fields; generated browser
types remain strict. Unknown fields, duplicate JSON object keys, trailing JSON,
unsupported versions, and excessive sizes fail before mutation.

- `GET /api/v1/ingestion/capabilities`: supported protocol, identity/semantics
  versions, limits, durable database ID. Database ID is persisted, unchanged on
  process restart. No producer paths, secrets, or owner selector.
- `POST /api/v1/ingestion/batches`: protocol/identity/semantics versions, expected
  server database ID, stream ID, batch ID, exact first/last journal range,
  ordered entries, optional bounded hostname label. Each entry contains its
  sequence, stable fact ID, canonical payload, optional typed source revision,
  and required normalized session/message/location references. No arbitrary
  metadata JSON, parser/source IDs, local row IDs, or paths.
- Success receipt echoes database ID, stream ID, batch ID, range, request hash,
  inserted/updated/no-op counts, committed time and analytics revision. Validate
  all binding fields before cursor mutation. A receipt for another server,
  batch, payload, or range cannot advance progress.
- 400 invalid canonical data; 409 replay/value/database identity conflict; 413
  request limits; 422 incompatible contract; 503 bounded admission/database busy.
  Error envelope contains stage/code and safe correlation IDs, never raw body.
- Network error/timeout/cancellation means unknown outcome. Keep saved request,
  retry it byte-for-byte on the next manual sync. No automatic durable background
  worker is required. A bounded synchronous writer queue is admission, not an
  asynchronous successful ingestion acknowledgment.

Initial concrete bounds: 1 MiB decoded body, 256 fact entries, 256-byte IDs/
labels, 4 concurrent accepted ingestion requests, 5-second SQLite busy timeout.
All integers must be nonnegative exact JSON integers within the existing browser
safe-integer domain (9,007,199,254,740,991); check arithmetic overflow and additive
components explicitly. Reject a mutation that would move server aggregate token
components outside that domain; never silently round or wrap. Bound total body
bytes before allocation and decoded list/string lengths before transaction.

Use deterministic typed encoding for stable IDs and payload hashes. Request hash
binds exact stored bytes; canonical payload hash excludes transport fields and
JSON ordering. Identity version selects a documented algorithm per harness,
rather than pretending every harness supplies equivalent native identity. Native
session/message/request identities and usage kind form fact identity where
available; mutable counters, location labels, capture clock, path, stream, and
batch do not. Weak native identity produces an explicit publication diagnostic
rather than guessed equality or an arbitrary duplicate contribution.

Codex is an explicit immutable-event exception: retain the existing deterministic
typed last/cumulative usage snapshot witness inside its derived event/message
identity, together with owning session/turn/source time. Distinct snapshots at
one millisecond must remain distinct. Presence versus absence of counter fields
is preserved in that witness. Proven ancestry replay keeps the original owning
event identity. Counter changes are changed source events under this algorithm,
not a supported revision of that snapshot identity. Line-based fallbacks retain
their existing unchanged-input limitation; do not claim inserted-line invariance.
Replace this algorithm only after native event identity evidence and migration
fixtures prove equivalent accounting. The per-harness algorithm is identified
by the existing identity contract version, not a new arbitrary wire field.

Claude streaming revisions can use a typed adapter rule backed by same native
session/message/request plus source event timestamp. Newer source timestamp
updates that single contribution; older becomes a stale no-op; equal timestamp
with different canonical values is conflict. This applies only after fixtures
establish that the source timestamp orders revisions. Pi/Codex immutable facts
and other unsupported changed values conflict. OpenCode completion/update
revision requires its own native-evidence fixtures before being enabled.

## Compatibility and migration

- Storage versions are independent per role. Recognized compatible versions
  migrate transactionally; unknown/newer versions reject without mutation.
- Server never resets historical canonical rows because producer data may have
  disappeared. Incompatible accounting/identity needs an explicit migration or
  rejection, not a source reconstruction requirement at the server.
- Split migration reads the old collector file using a stable SQLite snapshot;
  creates server references and canonical rows in one transaction; writes a
  durable migration identity marker. Server copy is canonical-only. Preserve
  the original file and every historical contribution.
- Existing raw native metadata can prove new stable identity mappings without
  reopening harness artifacts. Use it while migrating the collector, then send
  only normalized records. An old Claude row without retained request ID may be
  intrinsically ambiguous: preserve historical row, record migration diagnostic,
  and refuse overlapping ambiguous republishing until an approved native-data
  repair can prove equivalence. Do not silently add its rebuilt equivalent or
  invent a retraction.
- Identity aliases are safe only when the canonical/native fields prove the
  mapping. Neither matching counters nor same timestamp proves equality.
- Server replacement at the same URL changes database ID. Never send a saved
  batch bound to the old identity or continue its acknowledged cursor. Resolve
  the new binding, retain old state, and start journal delivery at zero. Local
  reinitialization can create that binding automatically; remote identity
  changes fail clearly until destination configuration is deliberately updated.

### Focused legacy identity mapping algorithm

This expands migration review, not the requested wire or schema scope. Use the
existing optional migration alias concept; do not add producer-controlled aliases
to ingestion.

1. Under collector writer ownership, stop legacy service writes and open a
   consistent read snapshot. Capture canonical rows with their session/message/
   location references and primary raw native identity metadata. Validate harness,
   native session, message relationship, source occurrence time and token
   components. Record a migration fingerprint before any collector reset. Preserve
   original SQLite/WAL state or use SQLite backup; copying only the live main
   file loses WAL commits.
2. Build a candidate new identity from the same per-harness algorithm used by
   publication, using retained metadata only. OpenCode/Pi require nonempty native
   session and message identities. Codex requires its preserved derived message
   witness; keep its immutable-event algorithm unchanged. Claude additionally
   requires explicit request identity metadata, or an independently documented
   native-message-only identity rule; absence must not silently become an empty
   request field that later conflicts with a recovered real request.
3. Group old rows by candidate new identity. Exactly one validated old row is a
   direct mapping. Equal canonical payloads under the same proven native identity
   are duplicate representations; annotate them for reviewed migration handling.
   Different payloads are revisions only if adapter evidence proves ordering.
   Equal-value rows with different native IDs remain separate. Never infer a
   match from counters, timestamps, observed time, ingest run, source path, or
   processing order.
4. Partition results: **proven one-to-one**, **proven revision/duplicate group**,
   or **ambiguous**. Initial implementation can map the first category. Any
   reduction of historical contributions in the second is an accounting repair,
   must be independently tested, and must not be mislabeled as preserving old
   totals. The ambiguous category retains historical contribution and native
   evidence in normalized migration metadata with a safe diagnostic.
5. Within one server transaction, copy proven rows under new stable IDs and
   preserve ambiguous rows under immutable legacy IDs; persist proven aliases
   and migration identity. Retry checks that marker and exact snapshot fingerprint,
   returning no-op for an already committed migration. Interrupted copy exposes
   no partial imported history. If collector reset follows, do it only after
   durable migrated history and native matching state are established.
6. Republished proven facts resolve their existing stable ID and dedupe normally.
   Incoming facts overlapping an ambiguous legacy native identity group fail
   explicitly before inserts. They cannot bypass the block by generating a new
   batch/stream or changed fact ID. A missing legacy message identity conservatively
   blocks that harness/session, because a narrower native match is impossible.
   A requestless Claude legacy message blocks that message's request variants;
   distinct messages remain ingestible. This protects totals at the cost of
   explicit blocked publication rather than silently counting twice.

Concrete ambiguity diagnostics: `legacy_missing_session`,
`legacy_missing_message`, `legacy_missing_request`, `legacy_native_group_conflict`,
`legacy_occurrence_unavailable`, `legacy_reference_mismatch`, and
`legacy_identity_algorithm_unsupported`. Correlate legacy key and harness/native
identity only; omit source paths and raw metadata bodies.

Known limitations need review: old OpenCode suppression may have removed genuine
facts before they reached canonical storage; migration cannot manufacture those
without retained raw/source evidence. Old Claude streaming rows may include both
partial and final contributions while request IDs were not retained. Preserving
that snapshot preserves its historical overcount; correcting it requires evidence
that both rows represent one native request. The absence of that evidence is a
real blocker, not a reason to merge on equal counters or delete history.

Migration tests must cover native one-to-one reupload, ambiguous requestless Claude,
two independent equal-valued native requests, old partial/final contribution
groups, missing raw FK targets, legacy duplicate groups, WAL-only committed rows,
pre/post commit interruption, and repeated migration. Check exact facts/components
and explicit diagnostics, not merely total equality.

## File ownership

| Owner | Files |
| --- | --- |
| Storage | Collector authoritative/embedded schemas and migration; new server authoritative/embedded schema, serverstore open/read/transaction/migration; schema checker. |
| Protocol/ingestion | Pure normalized protocol models, ID/hash validation, transactional ingestion, strict HTTP handler and durable receipts. |
| Collector | Publication transaction hooks, journal, batch preparation, destination binding, client/receipt validation, manual delivery. |
| Composition | Remove pipeline/source configuration from server runtime; local manager/remote foreground composition; CLI destinations; API TUI; web reload/status. |
| Identity | Harness adapter identity/revision metadata and existing CFI-007/008 regressions. |
| Failure verification | F01–F14 production HTTP/storage/crash tests, fixture permutation oracle, subprocess commit seams. |

## Unresolved questions

- Approve tables/wire and strict bounds?
- Approve preserving ambiguous legacy rows with explicit blocked republishing?
- Which native OpenCode timestamp proves revisions?
