# TokenInsights design

Implements [ADR 0007](adr/0007-raw-ingestion-and-server-processing.md) and
[ADR 0008](adr/0008-personal-hosted-composition-and-capabilities.md).
See the [original raw-ingestion plan](raw-ingestion-plan.md), [whitelist](raw-ingestion-whitelist.md)
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
globally shared consumption. Personal uses dataset `default`; hosted assigns one
dataset per user in the same DuckDB file. Native identity hashes remain stable
inside a dataset; isolation keys include dataset throughout acceptance, dependency
processing, generations, receipts and analytics.

## Storage contracts

| Role | Default | Schema | Version |
| --- | --- | --- | --- |
| Collector | collector.sqlite | schema/schema.sql | 19 |
| Server | server.duckdb | schema/data.sql | 2 |
| Legacy import | server.sqlite | schema/server.sql | 2 |

Defaults use XDG_DATA_HOME or ~/.local/share/tokeninsights. Role-specific flags/
environment/config override them. Reject aliased paths, wrong roles, incompatible
versions and corrupt contracts. Former tokeninsights.sqlite stays untouched.
Hosted users, token digests and browser session digests live in the `accounts`
logical schema of the same DuckDB file. One shared write coordinator owns all
writes; no application/per-user database or cross-file commit.

Verified collector schemas 16/17/18 upgrade additively to 19, retaining legacy facts,
journals, bindings, exact protocol-1/2 requests, hashes, receipts and cursors.
Schema 18 adds dataset/protocol bindings to delivery state; existing saved requests
retain `default` and their original protocol without byte rewriting. Schema 19 adds local-only durable capture quarantine without rewriting saved requests. Raw extraction has its own
version/stream; older canonical generations need no rebuild for capture, newer
generations reject. Maintenance cannot delete unaccepted raw outbox.

Fresh default server.duckdb imports verified sibling server.sqlite read-only.
Stage initialization/import/checkpoint before atomic publication. Preserve database
identity, components/revisions/history/receipt bytes; verify copied counts/totals.
Custom import uses service import --server-db-path NEW --legacy-server-db-path OLD,
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
- accounts.users/tokens/sessions: user/dataset ownership, enabled status, token
  permissions/digests and session expiry/revocation.
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
still explicitly filters the authorized dataset. One file gives each
acceptance/projection and account/dataset creation a transaction boundary. No broker,
cross-file commit or generic backend framework.

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
Manual sync/publish-only retries; no autonomous collector retry process.

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

Sync finishes at acceptance, not query visibility. Personal TUI then reads available confirmed data and refreshes pending processing
state; Reload queries only. TUI submission progress compares acknowledged evidence entries with pending entries, rather than comparing batches with entries. Load failures expose fixed reason codes and distinguish server storage failures from authentication failures. Browser reports lag/separate estimates. No viewer
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

Managed personal public dashboard/query: 127.0.0.1:8765 or optional 0.0.0.0.
Writes/reprocess/lifecycle/progress publication use a private mode-0600 Unix socket
plus instance verification; public write routes return 404. Foreground personal
requires server-db-path and exposes unauthenticated HTTP ingestion/query. Hosted
requires its kind, shared data path and canonical HTTPS public URL; users share
storage with isolated datasets. Startup never collects in any composition.

`serverfeatures` owns typed kind/capabilities and validates dependent features.
`GET /api/v2/instance` returns serverKind, datasetId, bounded capabilities and caller
permissions separately. `usage`, `facets`, `web-dashboard`, `raw-ingestion`,
`terminal-dashboard`, `collector-progress`, `reprocess` determine mounted routes
and command/browser preflight. Unknown feature names are ignored; absent required
features/unknown kinds reject. Hosted cannot enable terminal-dashboard or
collector-progress. Managed personal advertises progress only with its read/write
components. Server config may disable features but cannot disable hosted isolation.

Read API v2 shares analytics with personal v1 adapters. Instance/status/usage/facets
bind dataset snapshot identity. `/api/v2/status` contains processing readiness and
user-scoped revisions/pending work, without collector status. Hosted exposes no
v1 read fallback. Feature support never substitutes for read/ingest permission.

CLI configuration: `server-kind` personal by default, `server-url`, `server-token`,
local `host`/`port` and role paths. Flags > environment > file > defaults. Kind and
token use file or `TOKENINSIGHTS_SERVER_KIND`/`TOKENINSIGHTS_ACCESS_TOKEN`; no token
flag. Hosted needs URL/token. Token without an explicit remote destination rejects.
`config set server-token` reads secure prompt/stdin and rejects a command-line
value; get masks it. The private config file remains atomic/mode-0600. Clients
compare configured/advertised kind before collecting or opening viewers; remote
failure never falls back to local.

`tui` composes personal startup/sync/query and rejects hosted even query-only.
`web` syncs by default with `--sync=false` for saved data. Managed personal opens
its dashboard before sync to show bounded sanitized progress; hosted syncs in the
terminal and opens browser login without collector progress. `web --host` binds
managed local only; mismatching an already running bind needs explicit restart.
Plugins/manual sync share the same capture/delivery behavior. Reload remains query-only.

Managed personal clients publish leased attempts through
`/control/v1/collector-progress`; public GET `/api/v2/collector-progress` exposes
bounded fixed-stage/status/counters only. No paths, native payloads, secrets or
arbitrary error text. Registry is in memory and checks owner instance; leases expire
on missed heartbeats/cancellation. Concurrent attempts remain separate. Progress
reporting failures cannot invalidate durable delivery. Hosted mounts neither route.

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

| Package | Behavior boundary |
| --- | --- |
| `evidence`, `publication` | Sanitized wire records and stable contribution contracts |
| `pipeline` | Capture/native readers, continuity and safe enrichment; retained legacy normalization |
| `rawcollectorstore` | Collector capture/checkpoint/outbox transactions and immutable dataset-bound requests |
| `collector` | Delivery orchestration with injected destination transport; no local-service discovery |
| `clientworkflow` | CLI endpoint/service resolution, descriptor preflight and progress fan-out |
| `processor` | Pure evidence interpretation; no host reads, SQL, network or wall clock |
| `dataengine` | Transport-independent work/processing orchestration and retry scheduling |
| `datastore` | DuckDB adapter, upgrades/import and dataset-scoped atomic persistence operations |
| `analytics` | Typed query contracts/results and dataset-scoped SQL |
| `server`, `ingestionhttp` | Authorized REST/asset and ingestion adapters |
| `serverfeatures` | Typed kind/capability policy |
| `accounts`, `collectorprogress` | Hosted principals/sessions/admission; managed personal progress registry |
| `serverruntime` | Shared storage/worker/listener lifecycle |
| `service`, `remoteserver` | Managed personal and foreground personal/hosted composition |

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
