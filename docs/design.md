# TokenInsights design

Implements [ADR 0007](adr/0007-raw-ingestion-and-server-processing.md) and
[ADR 0009](adr/0009-single-process-and-distributed-compositions.md).
See the [original raw-ingestion plan](raw-ingestion-plan.md), [whitelist](raw-ingestion-whitelist.md)
and [OpenAPI](openapi.yaml). Supersedes collector-owned normalization,
normalized-only ingestion, SQLite analytics and synchronous completion.

## Ownership

Collector discovers OpenCode/Pi/Codex/Claude Code, captures whitelisted native
metadata in a SQLite outbox, and submits raw batches. It owns continuity and
local location enrichment; never normalizes new counters or resolves ancestry.
Server durably accepts evidence, asynchronously processes typed confirmed facts/
separate estimates, then serves SQL analytics and embedded browser assets.
The remote server resumes processing without reading host sources. Local commands
compose capture, acceptance, processing and queries within their own process.

    Sources -> whitelist extraction -> Collector SQLite -> acceptance
      -> DuckDB raw/receipts/scopes -> async processor -> facts/estimates -> views

Delivery IDs are operational stream/sequence/batch identities. Evidence IDs name
qualified native records plus preserved snapshots/context; missing native record
IDs use source lineage/order only as evidence witnesses. Contributions use stable
native harness/session/message/request rules, independent of collector installation,
row IDs, capture times or deliveries. Equal counters/hash alone never identify
globally shared consumption. Personal uses dataset `default`; hosted assigns one
dataset per user in the same DuckDB file. Native identity hashes remain stable
inside a dataset; isolation keys include dataset throughout acceptance, dependency
processing, generations, receipts and analytics.

## Storage contracts

| Role | Default | Schema | Version |
| --- | --- | --- | --- |
| Collector | collector.sqlite | schema/schema.sql | 19 |
| Token data | server.duckdb | schema/data.sql | 2 |
| Application | app.sqlite | schema/app.sql | 1 |
| Sync jobs | collector.sqlite.jobs.sqlite | schema/jobs.sql | 1 |
| Legacy import | server.sqlite | schema/server.sql | 2 |

Defaults use XDG_DATA_HOME or ~/.local/share/tokeninsights. Role-specific flags/
environment/config override them. Reject aliased paths, wrong roles, incompatible
versions and corrupt contracts. Former tokeninsights.sqlite stays untouched.
Application SQLite holds users, token/session digests and provisioning state. It is
paired to the token database identity and kind. Local setup creates one default user.
A one-time transaction copies legacy DuckDB accounts after validating dataset links;
the completed marker prevents stale source credentials overwriting later revocations.
DuckDB account tables remain read-only migration sources. App initialization publishes
a fully initialized file atomically; wrong roles/pairs reject before app mutation.

Provisioning persists an inactive user and fixed dataset ID, idempotently creates the
dataset through a contract, then activates the user. Restart resumes pending users;
disabled ready users stay disabled. There is no cross-engine atomic transaction.

Verified collector schemas 16/17/18 upgrade additively to 19, retaining legacy facts,
journals, bindings, exact protocol-1/2 requests, hashes, receipts and cursors.
Schema 18 adds dataset/protocol bindings to delivery state; existing saved requests
retain `default` and their original protocol without byte rewriting. Schema 19 adds local-only durable capture quarantine without rewriting saved requests. Raw extraction has its own
version/stream; older canonical generations need no rebuild for capture, newer
generations reject. Maintenance cannot delete unaccepted raw outbox.

Fresh default server.duckdb imports verified sibling server.sqlite read-only.
Stage initialization/import/checkpoint before atomic publication. Preserve database
identity, components/revisions/history/receipt bytes; verify copied counts/totals.
Custom import uses data import --server-db-path NEW --legacy-server-db-path OLD,
or remote startup with the same flag. Explicit import requires new target and
stopped local service. Source never overwritten. Newer processors reject; older
ones schedule a replacement generation.

A verified DuckDB-1 database upgrades to personal DuckDB 2 through a staged,
read-only copy: assign dataset `default`, verify identities/receipts/counts/token
components, checkpoint and atomically publish with a recoverable previous copy.
Database identity and interrupted generation state survive. Hosted starts fresh;
personal/hosted kind mismatches and newer contracts reject without mutation.
Recovery never overwrites the sole verified source. Legacy import format stays 2.

DuckDB schemas:
- raw.evidence: immutable sanitized JSON and qualified scope.
- ingestion.instance: global role/version/database/kind identity.
- ingestion.metadata: dataset identity, active/target generations,
  acceptance/published revisions and times.
- accounts.users/tokens/sessions: retained read-only legacy account migration source.
- ingestion.batches/items/batch_items: exact request/receipt bytes, immutable
  stream/sequence bindings, every submitted mapping including duplicates.
- processing.scopes/dependencies/outcomes: durable queue/revisions/generation,
  attempts/retry time/fixed errors, native ancestry and per-item dispositions.
- analytics.generations/facts/estimates/provenance: versioned typed usage,
  flattened native/session/message/location dimensions and evidence edges.
- analytics.legacy/legacy_coverage, ingestion.legacy_receipts: imported history,
  exact contribution replacement proof and protocol-1 receipt replay.

All raw, receipt, scope, dependency, generation, fact, estimate, provenance and
legacy keys include dataset. Active views join generation by dataset; analytics
still explicitly filters the authorized dataset. DuckDB gives acceptance and projection their own atomic transaction boundaries.
Application transactions remain in SQLite; no broker or cross-file commit is assumed.

## Capture and reliable submission

SQLite evidence_state/sources/outbox/destinations/batches retain lineage/context,
immutable observations, monotonic sequences, bindings, exact saved requests and
acceptance receipts. Capture/checkpoint commit together.

Schema-19 evidence_quarantine retains only source key/format, an opaque fingerprint of local device/inode/size/mtime, parser policy version, fixed error code, byte offset and capture time. It stores no source path or record contents. Deterministic record-limit failures preserve the previous evidence/checkpoint and remain incomplete on later syncs while their fingerprint/policy matches. File changes, parser policy changes and `sync --full-refresh` retry capture; successful capture clears quarantine. Cancellation, transient I/O failures and source changes during capture remain retryable without quarantine.

JSONL validates saved byte prefix, parses appended records, then verifies captured
bytes and inode before commit. Rewrite/truncation rotates lineage without deleting
evidence. Partial JSON tail waits; complete JSON without newline is captured but
keeps checkpoint before the last line. Full refresh preserves verified lineage
and deduplicates observations. Prefix verification reads old bytes for continuity and reuses that pass for the captured-prefix hash, retaining final byte/inode verification.

Raw Codex discovery lists files without the legacy ancestry-header scan; ancestry remains sanitized evidence interpreted by the server.

At most four readers prepare sources concurrently with one SQLite writer. JSONL records retain at most 16 MiB per reader; native decoding overhead varies with record shape, so worker admission is a nominal allowance rather than a hard heap cap. SQLite scans retain their existing snapshot/row behavior; sanitized metadata-only temporary spools keep prepared results off the heap. Spools use private permissions and are removed on completion/failure/cancellation. Disk use follows active source sizes; no total disk cap is claimed. Sources commit atomically after continuity verification; one source failure does not discard another source. Location resolution caches are shared and concurrency-safe.

The source-atomic writer encodes each sanitized observation once for its stable
hash and stored bytes, reusing transaction-local lookup/insert statements. It
checks duplicates before insertion so replay does not consume AUTOINCREMENT
sequences or leave delivery gaps. Preparation order, reader/processor limits,
prefix verification and OpenCode snapshot revision checks remain unchanged.

Records exceeding the 16 MiB JSONL bound are skipped only when a streaming JSON discriminator proves they are irrelevant. Oversized usage/context or unknown records fail the source rather than silently lose counters or continuity. Partial trailing records wait. Source extraction and skipped-record hashing preserve byte offsets and ordinals. Whitelist extraction caches decoded nested objects per record while preserving numeric precision, null/absence and source duplicate-key semantics.

OpenCode reads consistent SQLite snapshots. Existing rows can revise without
native update cursor, so snapshot scans compare sanitized observations; unchanged
rows never requeue. Native session/turn/task context accompanies usage.
Types, null/absence and numeric precision survive extraction. Invalid safe counters
become server diagnostics rather than client repairs.

Unknown/private fields never cross ingestion. Source paths/cursors/Git URLs stay
local; enrichment contains stable hashes, basenames, repository names/provenance.
Collector writer lock serializes capture/delivery state. Persist request before
sending; matching acceptance advances destination cursor transactionally.
Finite background sync retries submission at most three times within ten minutes,
respecting Retry-After. Manual sync/publish-only also replays retained work.

Raw/outbox/old generations retain indefinitely initially; no compaction. Replay
can repair only captured fields; whitelist changes may require source rereads.
Canonical endpoint plus server-authenticated dataset binds delivery progress.
Token rotation retains the binding; switching users creates independent progress.
Remote URL pins database identity; replacement rejects existing binding.
Local identity includes database ID for deliberate retained-history replay. Private owner-socket routing never replaces the canonical endpoint identity. Verified existing local aliases for the same database/dataset reuse pending requests first, then acknowledged progress; remote bindings remain strict. Control requests use three seconds; ingestion uses thirty seconds and caller cancellation.
Batch construction encodes each entry once, accounts for the exact envelope/byte limit, then validates and persists one final request. Retried bytes remain immutable.
One destination per invocation; no relay/fan-out.

## Acceptance protocol 3

GET capabilities, POST batches and GET batches/{stream}/{batch} under
/api/v3/ingestion/. Envelopes bind databaseId and datasetId; authenticated hosted
principal must match before mutation. Receipts and lookups are dataset-scoped.
Hosted mounts only protocol 3. Personal retains protocol-2 and legacy protocol-1
adapters for exact saved-byte replay, flushing older requests before new protocol-3
delivery. Limits: 1 MiB, 256 entries, 256 UTF-8 bytes per metadata string.
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

Sync finishes at acceptance, not query visibility. Local TUI startup additionally
waits for query visibility; local Web reads published history while processing
continues. Reload queries only. TUI submission progress compares acknowledged evidence entries with pending entries, rather than comparing batches with entries. Load failures expose fixed reason codes and distinguish server storage failures from authentication failures. Browser reports lag/separate estimates. No viewer
waits for global hosted queue emptiness.
Service wait explicitly waits up to 30 seconds for maintenance/fixtures.
Deprecated protocol-1 bridge preserves synchronous legacy bytes/receipts.
Flush retained old requests through their original personal adapters before new delivery; new sync never populates legacy
canonical tables. Legacy maintenance handles old tables only.

## Processing

One server owns file/lifetime lock. Connections share its engine; HTTP never
reopens paths. Four admission slots, shared short write transactions, two processing workers. The shared DuckDB engine uses two threads and a 1 GB memory budget for acceptance, processing and analytics. One dispatcher owns fair dataset selection and claims whole dataset-qualified components; components connected to an in-flight claim wait. A 32 MiB estimated raw-JSON admission budget bounds concurrent loading; a component over budget runs alone.
Scopes are dataset-qualified native sessions or unresolved source lineages; Codex
ancestry connects dependencies only within that dataset. Pending counts, revision
fences, generation activation and receipt outcomes are dataset-local. Worker
selection proceeds fairly across datasets; failures cannot connect or block
another user's dependency graph. Late parent arrivals invalidate terminal dependent outcomes.

Read consistent connected component; interpret outside write lock; fence generation,
complete component membership and each scope revision before publishing.
Stale work retries; unrelated ingestion cannot starve processing.
Facts/estimates/provenance/outcomes/completed revisions commit atomically.
Failures stay pending with fixed error/attempts/exponential retry capped 64 seconds;
independent scopes proceed. Backoff applies to every unchanged scope in a failed component; revised input remains eligible. Join dispatcher and workers before closing storage. Acceptance/projection inserts use bounded bulk statements under the shared writer, preserving conflict checks, receipt bytes, provenance and transaction fences.

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

Private administration or service reprocess queues a new generation per dataset;
reprocessing never follows implicitly from ingestion permission.
Old published data remains queryable; restart resumes. Immutable receipt status
reflects target/latest scope revision. Activate only after every scope covers
latest accepted inputs, atomically advancing published generation/revision.
Raw arriving during build must be covered. On a processor upgrade, an interrupted
older build is retained and a fresh generation uses the current rules; the old
published generation stays active until that replacement completes.

Imported baselines persist until matching contribution ID and equal components
or valid newer native revision prove coverage. Proof binds exact new payload hash,
preventing changed unproven projection from hiding history. Replace once;
unmatched history remains. Rebuilding 300 of 500 keeps 500. Ambiguity cannot erase
imported confirmed baseline.

## Querying and deployment

Typed columnar SQL filters/groups/sorts/pages active facts and reconciled baselines;
dashboard queries never scan raw JSON. Raw tables' mere presence does not slow
analytics scans. Read transaction covers metadata/summary/rows/facets. Responses
identify instance/database/dataset/generation/published revision/input revision; browser guards
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

## Composition, capabilities and access

Single-process `tui` owns collector, direct ingestion, processing and direct query
adapters. `web` adds a foreground read-only HTTP listener, bound to 127.0.0.1:8765
by default or the requested IPv4 host/port. Reserve the listener before capture;
initialize owned storage, register startup progress, then serve/open the browser
before asynchronous capture. Saved published history remains readable during
capture and processing; published revisions refresh the dashboard. Capture failure
does not close Web. Command cancellation or HTTP failure cancels and joins capture
and progress observers before closing storage/listeners. A database lifetime lock
excludes a second viewer. TUI retains its fresh-data startup visibility wait.

Distributed collectors submit to an authenticated remote hosted server in one
container. Bearer auth selects the dataset; the server starts no collector. A
canonical HTTPS public origin governs browser login/session security. The existing
DuckDB processing queue remains behind dataengine's backend contract.

`serverfeatures` owns typed kind/capabilities and validates dependent features.
`GET /api/v2/instance` returns serverKind, datasetId, bounded capabilities and caller
permissions separately. `usage`, `facets`, `web-dashboard`, `raw-ingestion`,
`terminal-dashboard`, `collector-progress`, `dashboard-reload`, `reprocess` determine mounted routes
and command/browser preflight. Unknown feature names are ignored; absent required
features/unknown kinds reject. Hosted cannot enable terminal-dashboard,
collector-progress or dashboard-reload. Local composition explicitly grants Reload,
including saved-only viewers; it is independent of collection progress. Progress
requires an installed command-owned registry. The shared observer publishes directly
to that registry for startup and queued sync jobs, retaining bounded leases and
heartbeats. Internal raw-ingestion support does not mount public HTTP ingestion:
the local Web listener remains read-only. Server config may disable features but
cannot disable hosted isolation. Modules consume resolved policy and injected
dependencies; mode selection stays at composition boundaries.

Read API v2 shares analytics with personal v1 adapters. Instance/status/usage/facets
bind dataset snapshot identity. `/api/v2/status` contains processing readiness and
user-scoped revisions/pending work, without collector status. Optional failed scope
counts and earliest failure retry time distinguish retry backoff from ordinary
processing; they come from the same dataset/generation snapshot. Accepted collector
evidence is not yet query-ready: pending work or differing active/target generations
still means processing. Hosted exposes no
v1 read fallback. Feature support never substitutes for read/ingest permission.

Local direct/HTTP instance descriptors share runtime identity and the machine
hostname resolved at ownership initialization. The hostname labels the local
viewer machine, not historical producer attribution; imported history can span
machines. Hosted never substitutes its operating-system hostname for producer
metadata. Connection/storage, usage and facet errors remain distinct in Web.

CLI configuration uses `mode=single-process|distributed`, URL/token, bind preferences
and three database paths. Defaults are single-process; legacy hosted configuration
maps to distributed. Flags > environment > file > defaults. The config file is
private/atomic; token entry uses prompt/stdin. Distributed requires bearer token and
URL. Remote failures never select local fallback. Bare invocation prints help.

Local TUI startup captures, directly accepts, then waits for pending work to drain
and active/target generations to agree in a consistent status snapshot. Retry/View
saved/Quit remain available. Saved-data viewing skips capture and visibility waiting.
Reload is query-only. Remote TUI is unavailable; remote web syncs before browser login.

Local visibility reads count pending scopes and failed pending scopes in the same
dataset/generation snapshot, using existing durable scope error codes. A recorded
failure in retry backoff returns `processing_failed` promptly, including after
owner restart. Due retries get a chance to finish before reporting an old failure,
so finite commands can recover transient errors without rebuilding a generation.
The bounded visibility deadline returns `processing_timeout`; caller cancellation
and storage query errors retain their own classification. Native query interruption
uses the caller's cancellation/deadline when present. Neither failure exposes
raw database errors or source metadata in the TUI. Retry repeats visibility waiting
without capture; View saved bypasses the wait and queries published history.
Reprocessing remains explicit and keeps the old generation until replacement is
complete. `data wait` resumes an interrupted rebuild without creating a new one.
No automatic reset, index repair, or generation rebuild follows a processing error.

Native publication verification covers bulk insert constraint failures and
cancellation after projection writes, rollback, owner reopen and retry. Failed
transactions must preserve all fact IDs/components, provenance, outcomes, scope
progress, metadata and immutable receipts; retry/replay must publish exactly once.

`sync` defaults to a finite detached worker in distributed mode. Parent commits a
job to separate operational SQLite, passes credentials through inherited private
pipes, and waits only for startup ACK. `--print` also submits and reserves stdout for
the canonical URL. `--wait` waits for acceptance. `--debug` requires read+ingest and
observes each accepted receipt, never the unrelated global queue; non-TTY output is
plain progress. Requests/receipts and fixed errors survive in jobs; tokens do not.
An OS lock serializes workers without holding a jobs transaction during networking.
Later workers drain earlier queued requests with the same endpoint/credential.
Abandoned running claims become interrupted; pending evidence stays in the outbox.

Local sync runs directly when unowned; otherwise it queues a local request. The
foreground owner's job loop captures/accepts it in-process. Enqueue does not need the
collector writer lock. Each completion during another scan remains a follow-up job.
If the viewer exits during handoff, the waiting sync may acquire ownership and finish
it. Pending requests survive shutdown and are consumed by the next local owner.
Plugin subprocesses use `--wait --harness`; TypeScript adapters coalesce overlaps
while retaining a follow-up pass, and do not forward event payloads.

Legacy `service stop/status` only migrate old owners; new lifecycle startup is retired.
Finite `data import/reprocess/wait` replaces local service maintenance. Public local
web has no ingestion/admin/progress-write routes; remote admin remains private.

Hosted administrator creates/disables users and creates/revokes scoped tokens
through the private owner socket. Random tokens have 256-bit entropy; store digests.
Dataset stays stable across rotation. `read`/`ingest` permissions enforce routes.
`POST /api/v2/auth/session` exchanges a read token for an opaque, 24-hour,
HttpOnly/Secure/SameSite=Lax host-only cookie; DELETE logs out. Browser sessions
cannot ingest/administer. User disable/source-token revocation blocks sessions.
Validate canonical public origin on state changes; do not trust proxy identity
headers. Fixed auth errors expose no credentials. Static login shell/health are
public; authenticated failures render login and clear account-specific/in-flight
query caches, including placeholder data.

Bound admission before body decoding: four global acceptance slots, one active
acceptance per hosted user, per-user 10 batches/second with burst 20, login five
attempts/minute per client address. Rate limits return 429/Retry-After; global busy
returns 503. Saved requests remain unacknowledged for manual retry.

## Package and deployment boundaries

[ADR 0009](adr/0009-single-process-and-distributed-compositions.md) preserves
processing semantics while changing composition. `collector.Delivery` has direct and
HTTP adapters; `analytics.Repository` shares direct/HTTP query semantics. `accounts.Repository`
and its SQLite adapter isolate application persistence. Dataengine owns processing
contracts; DuckDB is an adapter, not a required future backend.

| Package | Behavior boundary |
| --- | --- |
| `evidence`, `publication` | Sanitized wire records and stable contribution contracts |
| `pipeline` | Capture/native readers, continuity and safe enrichment; retained legacy normalization |
| `rawcollectorstore` | Collector capture/checkpoint/outbox transactions and immutable dataset-bound requests |
| `collector` | Delivery orchestration with injected destination transport; no local-service discovery |
| `clientworkflow` | Remote endpoint resolution and authenticated descriptor preflight |
| `processor` | Pure evidence interpretation; no host reads, SQL, network or wall clock |
| `dataengine` | Transport-independent work/processing orchestration and retry scheduling |
| `datastore` | DuckDB adapter, upgrades/import and dataset-scoped atomic persistence operations |
| `analytics` | Typed query contracts/results and dataset-scoped SQL |
| `server`, `ingestionhttp` | Authorized REST/asset and ingestion adapters |
| `serverfeatures` | Typed kind/capability policy |
| `accounts`, `appstore` | Credential contract/SQLite adapter, provisioning and application pairing |
| `syncjob` | Durable finite jobs, native detachment and delivery retries |
| `localruntime` | Command ownership, direct ingestion/query and local request consumption |
| `serverruntime` | Shared storage/worker/listener lifecycle |
| `service`, `remoteserver` | Legacy migration/fixture support and authenticated remote composition |

Retain package names where they already express the boundary. Retained
normalization/import/wire adapters are explicit compatibility paths; they are not
relocated solely for directory symmetry. Small consumer interfaces describe atomic
operations rather than generic backend CRUD.

One process owns the shared database; two bounded compute workers. Docker packages
the foreground executable with committed assets, non-root glibc runtime, CA/timezone
native dependencies, persistent `/data` and private `/run/tokeninsights` admin
socket. `/healthz` is liveness; `/readyz` checks initialized storage/auth/routes,
not absence of pending work. SIGTERM closes listeners, joins worker and closes
storage. See [deployment](deployment.md) for TLS, provisioning and backup.

Collector JSONL replacement, per-user DBs, organizations, external queue/backends,
replicas and public signup/OIDC are out of scope.

Go embeds committed web assets. CGO/C/C++ needed to build DuckDB; native CI/release
Linux/macOS amd64/arm64 runners. Production needs no JavaScript runtime.
Verification: format/lint/schema/API, native semantic fixtures, full/race tests,
native build/JS-absent smoke and browser E2E.

Application pairing also persists `<canonical-token-path>.application.json`, containing
only the application instance ID. Keep this guard with both databases in stopped
backups. A missing/replaced app database fails closed instead of re-importing stale
legacy credentials. Restore the matched set; do not delete the guard to bypass recovery.
