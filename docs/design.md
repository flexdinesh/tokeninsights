# TokenInsights design

Implements [ADR 0007](adr/0007-raw-ingestion-and-server-processing.md) and
[ADR 0009](adr/0009-single-process-and-distributed-compositions.md).
See the [whitelist](raw-ingestion-whitelist.md)
and [OpenAPI](openapi.yaml). Supersedes collector-owned normalization,
normalized-only ingestion and synchronous completion. [ADR 0012](adr/0012-sqlite-and-postgres-persistence.md) defines current persistence engines.

## Ownership

Collector discovers OpenCode/Pi/Codex/Claude Code, captures whitelisted native
metadata in a SQLite outbox, and submits raw batches. It owns continuity and
local location enrichment; never normalizes new counters or resolves ancestry.
Server durably accepts evidence, asynchronously processes typed confirmed facts/
separate estimates, then serves SQL analytics and embedded browser assets.
The remote server resumes processing without reading host sources. Local commands
compose capture, acceptance, processing and queries within their own process.

    Sources -> whitelist extraction -> Collector SQLite -> acceptance
      -> SQLite/PostgreSQL raw/receipts/scopes -> async processor -> facts/estimates -> views

Delivery IDs are operational stream/sequence/batch identities. Evidence IDs name
qualified native records plus preserved snapshots/context; missing native record
IDs use source lineage/order only as evidence witnesses. Contributions use stable
native harness/session/message/request rules, independent of collector installation,
row IDs, capture times or deliveries. Equal counters/hash alone never identify
globally shared consumption. Personal uses dataset `default`; hosted assigns one
dataset per user in shared SQLite or PostgreSQL storage. Native identity hashes remain stable
inside a dataset; isolation keys include dataset throughout acceptance, dependency
processing, generations, receipts and analytics.

## Storage contracts

| Role | Default | Schema | Version |
| --- | --- | --- | --- |
| Collector | collector.sqlite | schema/schema.sql | 20 |
| Token data (SQLite) | server.sqlite | schema/data.sql | 1 |
| Token data (PostgreSQL 18) | tokeninsights_data namespace | schema/data.postgres.sql | 1 |
| Accounts (PostgreSQL 18) | tokeninsights_accounts namespace | schema/app.postgres.sql | 2 |
| Application (personal SQLite) | app.sqlite | schema/app.sql | 2 |
| Application (hosted SQLite) | app.sqlite | schema/app.hosted.sql | 3 |
| Sync jobs | collector.sqlite.jobs.sqlite | schema/jobs.sql | 1 |

Defaults use XDG_DATA_HOME or ~/.local/share/tokeninsights. Role-specific flags/
environment/config override them. Reject aliased paths, wrong roles, incompatible
versions and corrupt contracts. Former tokeninsights.sqlite stays untouched.
Local CLI application role/version errors include resolved config/application paths
and manual deletion/recreation guidance, including account/credential loss. They
preserve the error identity and do not delete files. Pairing errors retain backup
recovery requirements; this guidance does not bypass the application pair guard.
SQLite startup validates role/version and actual schema objects read-only before writable opening. Expected objects are derived from embedded SQL and cached; actual storage is inspected on every open. PostgreSQL validates both fixed namespaces against the committed catalog contract, paired identity and role versions. Missing halves and incompatible contracts reject without repair. New PostgreSQL namespaces initialize together in one transaction.

Local composition always uses SQLite. Remote `--storage-backend=sqlite|postgres` (or `TOKENINSIGHTS_STORAGE_BACKEND`) selects both token/account adapters. PostgreSQL requires `TOKENINSIGHTS_POSTGRES_DSN` and an absolute `--admin-socket`; SQLite paths cannot be mixed with PostgreSQL settings. Credentials stay out of argv, client config and descriptors. PostgreSQL 18 is supported; no extensions are required.

SQLite uses one immediate writer, WAL, FULL synchronous durability and read snapshots. PostgreSQL serializes all token/account writes on one dedicated advisory-locked connection with synchronous commit. That connection never reconnects; loss cancels the server. Pooled multi-statement reads use REPEATABLE READ. Writes use READ COMMITTED under the owner mutex; failures roll back and existing durable replay/processing retries resolve work. No multi-server scheduling is implied.

Frequent dataset metadata reads retrieve metadata, active/target generation states,
the active-generation count and newest processor version in one SQL statement
within the caller's snapshot. All generation checks remain dataset-scoped and run
on every read, including newer processor versions in retained generations.
The account adapter holds users, credential/session digests and provisioning state. Hosted
accounts additionally hold unique usernames, Argon2id password hashes and named API keys.
Personal SQLite application schema 2 remains unchanged; hosted SQLite uses application
schema 3 and PostgreSQL account schema 2. Older hosted account schemas reject without
mutation or migration. SQLite uses a separate application file. It is
paired to the token database identity and kind. Local setup creates one default user.
App initialization publishes a fully initialized file atomically; wrong roles/pairs
reject before app mutation. PostgreSQL stores the same account contract in its separate namespace, paired by database and application instance IDs.

Provisioning persists an inactive user and fixed dataset ID, idempotently creates the
dataset through a contract, then activates the user. Restart resumes pending users;
disabled ready users stay disabled. There is no cross-engine atomic transaction.

Only current schemas are supported. Older/newer schemas and changed contracts
reject without mutation. There are no schema migrations, automatic sibling imports
or normalized-history baselines. New databases initialize atomically; source replay
creates current evidence. Collector storage contains only capture state, checkpoints,
immutable outbox, dataset-bound delivery state and quarantine. Maintenance cannot
discard unaccepted evidence. Processor-version changes inside a current SQLite/PostgreSQL
schema still use durable replacement generations; this is projection recomputation,
not storage compatibility.

Token tables (same logical names in each engine):
- raw_evidence: immutable sanitized JSON and qualified scope.
- ingestion_instance: global role/version/database/kind identity.
- ingestion_metadata: dataset identity, active/target generations,
  acceptance/published revisions and times.
- ingestion_batches / ingestion_items / ingestion_batch_items: exact request/receipt bytes, immutable
  stream/sequence bindings, every submitted mapping including duplicates.
- processing_scopes / processing_dependencies / processing_outcomes: durable queue/revisions/generation,
  attempts/retry time/fixed errors, native ancestry and per-item dispositions.
- analytics_generations / analytics_facts / analytics_estimates / analytics_provenance: versioned typed usage,
  flattened native/session/message/location dimensions and evidence edges.

All raw, receipt, scope, dependency, generation, fact, estimate and provenance keys include dataset. Active views join generation by dataset; analytics
still explicitly filters the authorized dataset. SQLite/PostgreSQL gives acceptance and projection their own atomic transaction boundaries.
Account transactions remain separate; no broker or cross-domain commit is assumed.

## Capture and reliable submission

SQLite evidence_state/sources/outbox/destinations/batches retain lineage/context,
immutable observations, monotonic sequences, bindings, exact saved requests and
acceptance receipts. Capture/checkpoint commit together.

Collector evidence_quarantine retains only source key/format, an opaque fingerprint of local device/inode/size/mtime, parser policy version, fixed error code, byte offset and capture time. It stores no source path or record contents. Deterministic record-limit failures preserve the previous evidence/checkpoint and remain incomplete on later syncs while their fingerprint/policy matches. File changes, parser policy changes and startup/sync `--full-refresh` retry capture; successful capture clears quarantine. Cancellation, transient I/O failures and source changes during capture remain retryable without quarantine.

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
Collector writer lock serializes capture and journal mutations. Per-destination
locks serialize delivery; transport never holds the capture lock, so independent
local/remote delivery and capture can proceed during a stalled request. Persist request before
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
Both modes accept only protocol 3. Limits: 1 MiB, 256 entries, 256 UTF-8 bytes per metadata string.
Strict decoding rejects unknown/private fields, duplicate keys, invalid UTF-8,
trailing values and invalid envelopes.
The authorized receiver owns strict decoding once per submission and validates
the requested protocol before mutation. Direct/HTTP adapters pass exact bytes,
bound admission and body size, and map typed validation failures to the same
public stage/code (HTTP 400 or 422). HTTP also owns content-type and body-read
handling. Collector request preparation validates its independent boundary;
it never supplies an unchecked decoded batch to the receiver.

Transaction commits exact request/hash, every mapping, deduplicated evidence,
receipt and scopes. Concurrent/overlapping/intrabatch duplicates succeed.
Changed bytes under stream/batch or changed evidence under stream/sequence
reject atomically. Safe semantic problems are accepted for async diagnosis.

202 pending, 200 terminal, 409 identity/conflict, 400 malformed/private, 413 size,
422 version, 503 busy/unavailable. Receipt binds database/dataset/stream/batch,
request hash, contiguous range/count, acceptance revision/time.
Collector validates acceptance independently of mutable processing status.

Sync finishes at acceptance, not query visibility. Local TUI and Web read published
history while command-owned capture, submission and processing continue. TUI refresh
completion requires processing visibility and a successful read of the current
published revision. TUI reload queries only. Collection progress retains acknowledged evidence counts; TUI
phase text separates submission from processing. Load failures distinguish storage,
collection and processing problems without exposing private error causes. Browser reports lag/separate estimates. No viewer
waits for global hosted queue emptiness.
`data wait` explicitly waits up to 30 seconds for maintenance/fixtures.

## Processing

One server owns file/lifetime lock. Connections share its engine; HTTP never
reopens paths. Four admission slots, shared short write transactions, two processing workers. One dispatcher owns fair dataset selection and claims whole dataset-qualified components; components connected to an in-flight claim wait. A 32 MiB estimated raw-JSON admission budget bounds concurrent loading; a component over budget runs alone.
Scopes are dataset-qualified native sessions or unresolved source lineages; Codex
ancestry connects dependencies only within that dataset. Pending counts, revision
fences, generation activation and receipt outcomes are dataset-local. Worker
selection proceeds fairly across datasets; failures cannot connect or block
another user's dependency graph. Late parent arrivals invalidate terminal dependent outcomes.

Read consistent connected component; interpret outside write lock; fence generation,
complete component membership and each scope revision before publishing.
After a stale publication, the concurrent dispatcher delays that component's next
attempt by 250 ms. The deadline does not move with acceptance wakeups; a dedicated
timer restores eligibility even during continuous arrivals. First attempts and
successful publications remain eager. Delayed membership is excluded before raw
JSON loading, using the current dataset-qualified dependency graph, so late
ancestors and alternate roots cannot bypass it. Independent components remain
eligible; delayed components occupy neither worker slots nor active byte admission.
The dispatcher retains at most 128 delayed components and 4,096 scope bindings,
without evidence records. Excess hints fall back to immediate eligibility. Hints
expire and disappear on restart; durable pending scopes remain authoritative.
Serial maintenance remains eager. Staleness never increments failure attempts or
changes durable retry status. This bounds intentional scheduling delay, not total
visibility: admission, processing and failure backoff still apply, and continuous
changes to the same component can invalidate every snapshot. Revision/membership
fences stay strict; unrelated ingestion cannot starve processing.
Facts/estimates/provenance/outcomes/completed revisions commit atomically.
Failures stay pending with fixed error/attempts/exponential retry capped 64 seconds;
independent scopes proceed. Backoff applies to every unchanged scope in a failed component; revised input remains eligible. Join dispatcher and workers before closing storage. Acceptance/projection inserts use bounded bulk statements under the shared writer, preserving conflict checks, receipt bytes, provenance and transaction fences.
Projection preparation deduplicates evidence edges per fact before acquiring the
writer. Publication deletes the component's previous provenance before inserting
the prepared edges, so these inserts need no conflict-ignore clause. Provenance
remains a set; facts, edges and completed revisions still commit or roll back together.

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

Browser usage totals show confirmed data by default. A secondary **Review excluded
usage** action appears below the summary when estimates match the active date range
and dimension filters. The review labels its totals, chart, table and session coverage
as excluded usage; **Back to usage** restores the confirmed view. Both views retain
filters and URL/browser-history state, with clear-filter recovery for empty results.
An estimate-summary request uses the existing read API, dataset snapshot identity
and revision; failure offers a separate retry without hiding normal usage. Excluded
usage and unusable-evidence diagnostics remain distinct. No totals combine the two
datasets, and no evidence classification or storage contract changes.

The browser presentation uses the White First design system in root `DESIGN.md`:
white light surfaces, neutral charcoal dark surfaces, blue actions and chart lines,
and locally bundled DM Sans. Branding uses a text wordmark. A desktop sidebar owns view navigation; below 55rem,
the same links wrap above the workspace. The header keeps machine identity,
a fixed-size status indicator beside the theme switcher, the current view heading
(`/ Tokens`, for example), and theme selection. Wrapping filters and period navigation
share a compact toolbar directly below the header. One date filter offers Daily,
Weekly, Monthly, Yearly, All time and Custom. Only Custom reveals inclusive,
optionally open date bounds and Apply. Previous/next controls step whole calendar
periods in the server timezone, with Monday-start weeks; Next is disabled in the
current period. All time and Custom have no step controls. Browser-only
`dateMode=calendar` distinguishes stepped explicit bounds from custom ranges in
URLs/history; API requests retain existing period/from/to semantics. Selecting a
period returns to its current calendar range. Date changes reset pagination while
preserving dimension filters and the selected view; returning to the current period
clears explicit bounds so refresh continues to follow the current period.
The static summary strip, chart, and padded result table follow. Controls preserve keyboard access, mobile touch targets, and 200%
text reflow. Presentation components use existing query state and callbacks;
URL/history, dataset snapshots, authentication, and request lifecycle remain owned
by their existing browser modules. Fonts and browser assets are committed and
embedded by Go, with no remote font or host JavaScript runtime dependency.

Compact header/navigation padding, section gaps and gutters, a 12rem plot and
reduced row padding keep more results visible
without reducing type or control targets. The redundant dashboard introduction is
omitted. Hover, keyboard focus, or tap on the loading indicator reveals query and
processing status plus capability-gated local collection details. Its compact
popup includes TokenInsights process CPU and resident RAM. CPU measures user plus
system time over the sampling interval (100% is one core); unavailable/initial
measurements stay unknown. The Web command constructs a synchronized, cached
process sampler and injects it into its HTTP adapter. Local `/api/v2/status` adds
optional `processUsage`; hosted handlers neither sample nor expose it, even if
a sampler is supplied. No measurements are stored. Both local and hosted status
popups show the last ingestion time in the server timezone; hosted popups omit
local process resources and collection details.
The icon keeps the same width for ready, loading, and attention states; accessible status labels
announce changes, and rotation respects reduced motion. The popup shows active
runs and the latest completed collection. Failures retain visible recovery beside
saved usage. Harness labels describe the collection run, independently of saved
analytics.
The wire state `skipped` displays as **No new usage**: unchanged sources or no
sources found, without implying that previously saved usage is excluded. Both local
and hosted browsers poll saved-data status every five seconds and refresh published
revisions automatically; there is no manual dashboard Reload button. Request failures
retain Retry controls. Browser reads never trigger collection.
In-process collector progress uses the same five-second polling interval,
including after a run finishes. Hosted browsers do not poll collector progress.

## Generations and preserved history

Private administration or `data reprocess` queues a new generation per dataset;
reprocessing never follows implicitly from ingestion permission.
Old published data remains queryable; restart resumes. Immutable receipt status
reflects target/latest scope revision. Activate only after every scope covers
latest accepted inputs, atomically advancing published generation/revision.
Raw arriving during build must be covered. On a processor upgrade, an interrupted
older build is retained and a fresh generation uses the current rules; the old
published generation stays active until that replacement completes.

## Querying and deployment

Typed columnar SQL filters/groups/sorts/pages active facts;
dashboard queries never scan raw JSON. Raw tables' mere presence does not slow
analytics scans. Read transaction covers metadata/summary/rows/facets. Responses
identify instance/database/dataset/generation/published revision/input revision; browser guards
snapshot identities. Values are bound; identifiers are fixed selections.
Clamp page before offset arithmetic; reject unsafe JavaScript integer aggregates.
Row counts group only visible identities, using the same grouping definition as
row aggregates. Fact counts share the filtered summary scan; synced-session counts
remain dataset-wide. Counts, page clamping, summaries and rows stay in the same
read transaction on both engines.
IANA/fixed local offsets apply to calendars. Pages 50 default/200 max, dimensions
12 chart groups, time charts 1000 buckets, session facets 100.
The direct query adapter offers bounded complete results for TUI through the same
analytics contract and SQL as paginated HTTP reads. It loads metadata, summaries,
counts, chart and all requested rows within one read transaction, avoiding repeated
dashboard aggregation per page. The TUI retains its 100,000-row limit; HTTP keeps
its existing page limits. Adapter selection stays in command composition and existing
capability policy still gates terminal access. Date range filters constrain analytics,
never collector discovery or capture.
Repo-only location filters; unknown visible. Context: per-session prompt-side
peak input+cache read+cache write, then average/median/max.

One connected session/ancestry component is processed in memory. Huge components,
partitioning/preaggregations and retention need measured follow-up, not benchmark
claims in this change.

## Composition, capabilities and access

Single-process `tui` owns collector, direct ingestion, processing and direct query
adapters. `web` adds a foreground read-only HTTP listener, bound to 127.0.0.1:8765
by default or the requested IPv4 host/port. Reserve the Web listener before capture.
Once ready, the Web command prints a compact app heading, highlighted dashboard URL
and Ctrl+C hint. Styling follows the output stream's terminal capabilities and
`NO_COLOR`/`CLICOLOR`/`CLICOLOR_FORCE`; redirected output is plain by default.
Both initialize owned storage, register startup progress and open saved usage
before asynchronous capture. Shared local startup orchestration owns the background
capture and progress observers; command cancellation or listener failure cancels and
joins them before storage/listeners close. Saved published history remains readable
during capture and processing; committed published revisions refresh the dashboard.
Capture failures, including quarantine, preserve the dashboard and show incomplete
refresh. A database lifetime lock excludes a second viewer.

Distributed collectors submit to an authenticated remote hosted server in one
container. Bearer auth selects the dataset; the server starts no collector. A
configured HTTP listen address does not require a domain or TLS. Browser login
uses the request Host for same-origin matching; an optional public origin can pin it. The
existing SQLite/PostgreSQL processing queue remains behind dataengine's backend contract.

`serverfeatures` owns typed kind/capabilities and validates dependent features.
`GET /api/v2/instance` returns serverKind, datasetId, bounded capabilities and caller
permissions separately. `usage`, `facets`, `web-dashboard`, `raw-ingestion`,
`terminal-dashboard`, `collector-progress`, `dashboard-reload`, `reprocess` determine mounted routes
and command/browser preflight. Unknown feature names are ignored; absent required
features/unknown kinds reject. Hosted cannot enable terminal-dashboard,
collector-progress or dashboard-reload. Local composition still advertises the
dashboard-reload capability, including saved-only viewers; the browser uses
automatic refresh rather than a manual Reload control. Progress
requires an installed command-owned registry. The localruntime observer publishes directly
to that registry for startup capture, retaining bounded leases and
heartbeats. Single-process TUI composition additionally enables opt-in capture
measurements before background startup. Pipeline emits typed per-harness source
snapshots; localruntime translates them into collectorprogress values. The registry
remains independent of capture and owns validation, attempt isolation, snapshot
copies and expiry. Measurements contain only approved harnesses, phases and bounded
counters, never source identities, paths or native text. Detail snapshots are a
Go-only contract separate from existing HTTP messages/responses. Web and distributed
compositions leave the detail callback unset; public API and hosted capability
policy are unchanged. Reporting does no I/O or blocking channel sends, and rejected
observations cannot invalidate capture or delivery.
Internal raw-ingestion support does not mount public HTTP ingestion:
the local Web listener remains read-only. Server config may disable features but
cannot disable hosted isolation. Modules consume resolved policy and injected
dependencies; mode selection stays at composition boundaries.

Read API v2 shares analytics with direct queries. Instance/status/usage/facets
bind dataset snapshot identity. `/api/v2/status` contains processing readiness and
user-scoped revisions/pending work, without collector status. Optional failed scope
counts and earliest failure retry time distinguish retry backoff from ordinary
processing; they come from the same dataset/generation snapshot. Accepted collector
evidence is not yet query-ready: pending work or differing active/target generations
still means processing. No read API fallback is supported. Feature support never substitutes for read/ingest permission.

Local direct/HTTP instance descriptors share runtime identity and the machine
hostname resolved at ownership initialization. The hostname labels the local
viewer machine; captured history can span machines. Hosted never substitutes its operating-system hostname for producer
metadata. Connection/storage, usage and facet errors remain distinct in Web.

Commands select composition: `tui` and `web` always own local capture/direct
ingestion/processing/queries; `sync` always submits over authenticated hosted HTTP;
`browse` opens the configured hosted URL without capture, authentication preflight
or storage. Bare invocation prints help. No global mode, mode flag or environment
selector exists. `data reprocess|wait` remains finite local maintenance.

Client config has `collector.db-path`, `in-process` bind/token/account paths and
`distributed` URL/token groups. Flags > environment > file > defaults. Read strict
JSON structure, then resolve/validate only consumed settings into distinct local,
sync and browse values at the executable boundary. Missing, invalid or unreachable
distributed settings cannot redirect or disable local viewers. Browse needs only a
valid URL; browser login owns authentication. Config writes remain private/atomic,
tokens enter through prompt/stdin and are masked on reads. Flat config rejects
without mutation; users regroup settings explicitly. Hosted server deployment
continues resolving its independent flags/env/secret files.

Local Web defaults to port 8765; hosted native/image/Compose defaults to 8766.
Explicit binds remain authoritative and occupied ports fail. Personal and hosted
token/account storage must be distinct, including on the same machine. One shared
collector outbox may serve both compositions: existing capture/delivery locks
serialize writers and each endpoint/database/dataset binding owns progress.
Each invocation submits to one destination; no fan-out or relay is implied.

Local TUI queries saved committed usage while displaying a persistent refresh
strip below the machine header and above navigation. Collection, submission,
processing and display-update states describe actual work; no estimated percentage
or ETA is shown. Four persistent harness rows extend the strip, retaining layout
through completion and failure. Compact terminals use two paired rows when width
allows; labels shorten before table space is reduced. Drawers start below the
complete progress header so measurements remain visible.

Source totals remain unknown during discovery. Enumerated sources wait until an
actual reader starts; writing is reported while finalizing capture. Checked counts
only finalized outcomes: captured sources with new committed evidence, sources
with no new evidence, ordinary failures and quarantined sources. Outcomes are
mutually exclusive; failed/quarantined counts do not imply successful capture.
Remaining is total minus checked, including unissued sources after cancellation.
Counts measure sources, not sessions or evidence records. Source preparation alone
does not advance checked counts. Startup capture uses the command observer,
with detail keyed to the selected attempt.
Local retained-evidence delivery is labeled Processing and shows entries
acknowledged after receipt validation and pending entries when known, so local
work does not imply upstream submission. Subsequent processing shows current
dataset scopes pending, including earlier
accepted work; it is not a per-attempt completion denominator. Harness rows describe
capture only, independently of submission and processing. With startup collection
off they show disabled until a queued attempt supplies progress.

A successful refresh requires capture/submission success, no pending
work, matching active/target generations in a consistent status snapshot, and a
successful dashboard read of the current published revision. Acceptance alone never
claims refreshed usage. Failures stop the spinner and remain visible with saved
usage; unavailable status cannot claim success.

Automatic reads coalesce committed revisions and reject stale responses. Rows,
totals and coverage update from one consistent snapshot while retaining filters,
sort, focused row identity, scroll position where possible, and drawer drafts. An
empty saved snapshot says usage will appear automatically during refresh; definitive
empty-state guidance follows refresh completion. `--sync=false` skips startup
capture and visibility waiting, but existing durable processing resumes and can
refresh the displayed revision. TUI reload is query-only. Viewer filters never restart
capture or select processing work. Remote TUI is unavailable. Browse opens hosted browser login without submitting.

Local visibility reads count pending scopes and failed pending scopes in the same
dataset/generation snapshot, using existing durable scope error codes. A recorded
failure in retry backoff returns `processing_failed` promptly, including after
owner restart. Due retries get a chance to finish before reporting an old failure,
so finite commands can recover transient errors without rebuilding a generation.
The bounded visibility deadline returns `processing_timeout`; caller cancellation
and storage query errors retain their own classification. Native query interruption
uses the caller's cancellation/deadline when present. Neither failure exposes
raw database errors or source metadata in the TUI. Background viewers preserve
published history and report processing failures without blocking interaction.
Reprocessing remains explicit and keeps the old generation until replacement is
complete. `data wait` resumes an interrupted rebuild without creating a new one.
No automatic reset, index repair, or generation rebuild follows a processing error.

Native publication verification covers bulk insert constraint failures and
cancellation after projection writes, rollback, owner reopen and retry. Failed
transactions must preserve all fact IDs/components, provenance, outcomes, scope
progress, metadata and immutable receipts; retry/replay must publish exactly once.

`sync` starts a finite detached distributed worker. Parent commits a
job to separate operational SQLite, passes credentials through inherited private
pipes, and waits only for startup ACK. `--print` also submits and reserves stdout for
the canonical URL. `--wait` waits for acceptance. `--debug` requires read+ingest and
observes each accepted receipt, never the unrelated global queue; non-TTY output is
plain progress. Requests/receipts and fixed errors survive in jobs; tokens do not.
An OS lock serializes workers without holding a jobs transaction during networking.
Later workers drain earlier queued requests with the same endpoint/credential.
Abandoned running claims become interrupted; pending evidence stays in the outbox.

Local viewers own no operational jobs store or handoff loop. Startup collection
runs once; restart the viewer to recollect. `--sync=false` suppresses capture while
durable processing resumes; `--full-refresh` and `--source-dir` affect startup
capture only. Viewer filters and TUI reload remain query-only. Remote plugins invoke
`sync --harness`; TypeScript adapters coalesce overlaps and retain a follow-up
pass without forwarding payloads. Existing jobs storage/remote job format stays
unchanged. Removed nonterminal local requests remain stored and are reported as
unsupported, never executed remotely. Collector evidence/batches/receipts remain
untouched and available to viewer startup replay.

Development fixture setup uses a guarded internal capture entrypoint restricted to
`.tokeninsights-dev`, composed from the same local runtime and collector. No public
headless local-sync command is introduced.

Legacy `service`, `server`, and `collector` commands and the hidden daemon runner
are removed. Local viewers use only command-owned foreground runtimes; finite
`data reprocess/wait` provides local maintenance. Development
fixtures use the shared database ownership locks. Public local
web has no ingestion/admin/progress-write routes; remote admin remains private.

Hosted administrator creates/disables users and creates/revokes scoped tokens
through the private owner socket. Hosted users sign in with username/password, then
manage their own password and API keys through browser settings. Signup defaults off;
hosted composition resolves `--signup` / `TOKENINSIGHTS_SIGNUP_ENABLED`. Personal
composition automatically creates one default user/dataset and mounts no login,
signup or account-management routes; its UI has no account controls.

`GET /api/v2/auth` is public and advertises only server kind, login methods and signup
availability. It exposes no user or dataset; authenticated `/api/v2/instance` retains
its existing capability/permission contract. The UI resolves this policy and the
current account at its application boundary. Route mounting and server-side
authorization enforce policy independently of hidden UI controls.

Hosted usernames are trimmed, lowercase ASCII, 3–32 characters: first character
alphanumeric, remaining characters alphanumeric, underscore or hyphen. Passwords
are 15–128 Unicode characters and retain whitespace. `accounts` owns validation and
Argon2id hashing with unique salts, 19 MiB memory, two iterations and one lane.
Storage and HTTP adapters do not invent credential policy. Signup uses recoverable
inactive-user/dataset provisioning; successful activation binds one stable user ID
to one stable dataset ID. Credentials never select or replace that dataset.

`POST /api/v2/auth/session` exchanges username/password for an opaque, 24-hour,
HttpOnly/SameSite=Lax host-only cookie (Secure for HTTPS browser origins); DELETE logs
out. Browser sessions are independent of API keys and permit reads and the current
user's account settings, but cannot ingest or administer other users. Self-service
password changes require the current password. Changing or privately resetting a
password invalidates all browser sessions while leaving API keys active. Disabling
a user blocks every credential. Password recovery remains private-socket only;
there are no public administrator roles or administration UI.

Named API keys have labels of at most 200 Unicode characters and 256-bit random
secrets stored only as digests. Creation reveals
the secret once; settings list metadata and support revocation. `read`/`ingest`
scopes authorize data routes; the sync preset includes both for receipt inspection.
Keys have no default expiry. Key revocation takes effect immediately without ending
browser sessions. API keys cannot change passwords or manage keys. All self-service
operations derive ownership from the authenticated session, never client-supplied
user or dataset IDs. Browser settings bind account reads and mutations to the
resolved dataset using optional `X-TokenInsights-Dataset-ID`. The server authenticates
first, then compares this expectation without using it to select a dataset. A
mismatch rejects with 409 before the operation; the UI clears account caches and
requires login. This prevents another tab's shared-cookie identity change from
applying settings to a different account. Future federated login may attach verified provider identities
to existing user IDs; email matching alone must not link accounts. No OAuth code or
generic provider framework is introduced.

Validate same-origin browser state changes against the request Host, or an
explicit optional public origin. HTTP and HTTPS browser origins are supported;
TLS/ingress remains deployment-owned; external deployments should use HTTPS.
Explicit proxy CIDR policy may
resolve client addresses for login admission; headers never confer identity or
select origin. Fixed auth errors expose no credentials. Static login shell/health are
public; authenticated failures render login and clear account-specific/in-flight
query caches, including placeholder data.

Dashboard reads honor request cancellation. Canceled requests produce no query
failure log or error response; query timeouts and storage failures remain visible.

Bound admission before body decoding: four global acceptance slots, one active
acceptance per hosted user, per-user 10 batches/second with burst 20, password
authentication/signup five attempts/minute per client address and four global
password-work slots. Rate limits return 429/Retry-After; global busy
returns 503. Saved requests remain unacknowledged for manual retry.

## Package and deployment boundaries

For every non-trivial change, apply the architecture checklist in
[`AGENTS.md`](../AGENTS.md#architecture). Identify the owning module and semantic
contract before choosing implementation details. Resolve mode policy and adapter
selection in composition roots; keep domain behavior independent of deployment.
Review affected modes/backends and failure lifecycles, not just the successful
request path. ADRs 0010 and 0011 define the principles and contract guarantees;
dependency tests and real-adapter suites enforce their executable portions.

[ADR 0009](adr/0009-single-process-and-distributed-compositions.md) preserves
processing semantics while changing composition. `collector.Delivery` has direct and
HTTP adapters; `analytics.Repository` shares direct/HTTP query semantics. `accounts.Repository`
and its adapters isolate application persistence. Dataengine owns processing contracts; storage implementation remains behind these boundaries.

[ADR 0011](adr/0011-storage-adapter-contracts.md) defines separate token/account contracts. [ADR 0012](adr/0012-sqlite-and-postgres-persistence.md) implements SQLite and PostgreSQL adapters. Shared relational transaction code owns behavior; small engine-specific SQL fragments remain private. Account and token transactions stay separate even when they share a physical database.

| Package | Behavior boundary |
| --- | --- |
| `evidence`, `publication` | Sanitized wire records and stable contribution contracts |
| `pipeline` | Capture/native readers, continuity and safe enrichment |
| `rawcollectorstore` | Collector capture/checkpoint/outbox transactions and immutable dataset-bound requests |
| `collector` | Delivery orchestration with injected destination transport; no local-service discovery |
| `clientworkflow` | Remote endpoint resolution and authenticated descriptor preflight |
| `processor` | Pure evidence interpretation; no host reads, SQL, network or wall clock |
| `dataengine` | Transport-independent work/processing orchestration and retry scheduling |
| `datastore` | Shared relational token transactions and dataset-scoped persistence |
| `analytics` | Backend-independent query contracts, results and query policy |
| `adapters/sqlanalytics` | Dataset-scoped SQL queries and receiver/query composition |
| `server`, `ingestionhttp` | Authorized REST/asset and ingestion adapters |
| `clientaddress` | Explicit trusted-proxy client address resolution; no environment, account or storage dependencies |
| `serverfeatures` | Typed kind/capability policy |
| `collectorprogress` | Sanitized progress values, registry and HTTP reads; no capture dependencies |
| `processusage` | Own-process CPU/RSS sampling and cache; Web composition injects read-only snapshots |
| `accounts` | Credential contracts, shared policy and dataset provisioning contract |
| `adapters/accountsql` | Shared account transactions and recoverable provisioning |
| `adapters/sqlite`, `adapters/sqliteaccounts`, `appstore` | SQLite opening and physical application pairing |
| `adapters/postgres`, `persistence/postgres` | Paired PostgreSQL composition, schemas, connections and ownership |
| `syncjob` | Durable finite jobs, native detachment and delivery retries |
| `localruntime` | Command ownership and background startup lifetime, direct ingestion/query, local requests and development fixtures |
| `serverruntime` | Shared worker/readiness/listener lifecycle through interfaces |
| `remoteserver` | Authenticated remote composition |

`querymodel` owns storage-independent query values. Direct queries receive an
analytics repository and instance identity; they do not construct an HTTP app.
Storage supplies semantic receiver operations; `ingestionhttp` mounts HTTP routes.
The collector requires an injected delivery adapter before capture. Single-process
composition supplies direct delivery; distributed discovery supplies HTTP delivery.
No core discovers a daemon or substitutes a transport.

`server.DataSource` supplies dataset-bound `evidence.Receiver` and
`analytics.Repository` ports. HTTP scopes both from the authenticated principal;
it cannot construct adapters or choose another backend from request values.
`dataengine.Metadata` carries storage-independent publication identity/revisions.
Missing receipts use `evidence.ErrReceiptNotFound`, never a driver sentinel.
The shared runtime consumes worker/readiness interfaces; embedded initialization
and filesystem locks belong to adapter/composition code. Import rules keep SQL
and concrete storage out of accounts, analytics, HTTP and shared runtime.

Reusable `storagecontract` suites run against real adapter fixtures, including
durable reopen, acceptance replay/isolation, publication fences, query components
and account revocation. SQL rollback/fault-injection tests stay beside each adapter.
Shared storage benchmarks use these same real fixtures for growing history,
independent datasets, retained generations, reopen, incremental acceptance and
queries during processing. Fixture setup stays outside timing; every sample keeps
accounting/receipt invariants. Benchmark smoke checks execute all families without
performance thresholds. Physical size probes stay inside adapter fixtures, not
production contracts.

Built client/server deployment scenarios run against both hosted backends: native
harness capture, copied/concurrent clients, collector rebuild, lost-response replay,
and server restart with accepted work or a staged replacement generation. They
assert HTTP totals, session/fact identities, exact retry bytes and immutable
receipts. Restart fixtures commit work through real adapters while the binary is
stopped; they do not race the worker or add production test switches. In-process
composition remains SQLite-only. See the [coverage map](collector-ingestion-tests.md).

The real Docker image has a separate `pnpm run test:container` contract on both
backends: environment configuration, secret-file startup, non-root/read-only root,
alternate-port readiness, private admin, ingestion, replacement durability,
revocation and SIGTERM. Local pre-push runs this alongside native deployment contracts. CI is limited to native OS/architecture coverage unavailable on one developer host; new CI checks require a documented reason.

### Runtime resource ownership

Composition acquires storage, lifetime locks and listeners and cleans up every
partially completed startup. Invalid listener bindings reject before starting
workers or taking listener ownership. A failed ready callback or listener failure
shuts down all started listeners and preserves the originating error.

`serverruntime.Serve` owns HTTP lifetime only; `Run` also starts, cancels and joins
processing. Neither closes the caller's store. Shutdown cancels request contexts
and starts draining every listener concurrently under one 15-second deadline, so
a slow public request cannot leave the private admin listener accepting work.
Listener loops join before return. Deadline expiry returns an error and forcibly
closes connections; it cannot force a Go handler to return. Handlers must honor
request cancellation and keep storage work within their request lifetime.

Local runtime stops admitting collection, cancels and joins active collection,
processing and queued-job workers, interrupts progress, then closes databases and
releases the lifetime lock. Web drains its HTTP server before closing that runtime.
Hosted composition joins server processing and account cleanup before closing its
databases and releasing ownership; it also removes the private admin socket.
Failed startup must permit a subsequent owner to open the same resources.

Shutdown does not wait for the durable processing queue to become empty. Accepted
evidence and immutable receipts survive cancellation; the next owner resumes
pending projection work. Tests cover startup failures, multi-listener draining,
forced connection close, ownership during collection shutdown and receipt/totals
recovery, alongside processor join and transactional cancellation tests.

[ADR 0010](adr/0010-current-contracts-and-boundaries.md) defines the cleanup and
testing rules. Import tests enforce pure processing, capture/processing separation,
and storage/HTTP separation. Real-adapter contract tests enforce the behaviors that
types and import rules cannot prove.

One process owns the shared database; two bounded compute workers. Docker packages
the foreground executable with committed assets, non-root Debian runtime, CA/timezone
data, persistent SQLite `/data` and private `/run/tokeninsights` admin
socket. `/healthz` is liveness; `/readyz` checks initialized storage/auth/routes,
not absence of pending work. SIGTERM closes listeners, joins worker and closes
storage. See [deployment](deployment.md) for TLS, provisioning and backup.

The executable owns flag/environment precedence, secret-file resolution and
validation. Composition receives typed settings; domains never inspect deployment
environment. Flags override nonempty environment values, then defaults apply.
Secrets have no argv representation; DSN and DSN_FILE are mutually exclusive.
Settings are immutable for one process lifetime. The binary's `healthcheck`
subcommand probes HTTP readiness without reopening storage or reading secrets.

Hosted composition wraps only its public handler with `clientaddress.Policy`.
The zero policy ignores forwarded addresses. Configured CIDRs permit walking
X-Forwarded-For right to left through trusted hops; the first untrusted address is
the client. Invalid/absent chains and more than 32 hops fall back to the socket
peer. Account admission consumes the resolved RemoteAddr and knows nothing about
proxy configuration. Forwarded host/proto never override the request Host or optional origin pin. Private admin and local HTTP retain their existing policy. See
[ADR 0013](adr/0013-server-deployment-boundary.md).

Collector JSONL replacement, per-user DBs, organizations, external queues,
replicas and OAuth/OIDC are out of scope. Hosted username/password signup is
operator-controlled and defaults off.

Go embeds committed web assets. Production builds use pure Go (`CGO_ENABLED=0`); native CI/release
Linux/macOS amd64/arm64 runners. Production needs no JavaScript runtime.
Verification: format/lint/schema/API, native semantic fixtures, full/race tests,
native build/JS-absent smoke and browser E2E.

Application pairing also persists `<canonical-token-path>.application.json`, containing
only the application instance ID. Keep this guard with both databases in stopped
backups. A missing/replaced app database fails closed to preserve credential revocations. Restore the matched set; do not delete the guard to bypass recovery.

## Initial server bootstrap and background plugins

[ADR 0015](adr/0015-server-bootstrap-and-background-plugins.md) supersedes mandatory
HTTPS/public-origin deployment and synchronous completion hooks. The server accepts
HTTP on any configured IPv4 port; flags override TOKENINSIGHTS_LISTEN, then
TOKENINSIGHTS_SERVER_PORT, then 8766. Healthcheck shares this resolution. Compose defaults
to SQLite with separate persistent token/account files and host-loopback publishing.

[ADR 0016](adr/0016-hosted-user-management.md) supersedes ADR 0015's token bootstrap
and browser-login rules. The executable resolves optional
`TOKENINSIGHTS_BOOTSTRAP_USERNAME` and `TOKENINSIGHTS_BOOTSTRAP_PASSWORD` (or
`TOKENINSIGHTS_BOOTSTRAP_PASSWORD_FILE`) into typed hosted settings. Both username
and password are required when either is supplied. Before readiness, accounts
idempotently provisions a fixed ordinary bootstrap user/dataset. The initial
password is hashed; later env changes never replace it or reactivate disabled users.
An incompatible username conflicts rather than silently renaming or reassigning
the account. Bootstrap creates no API key; users create keys in settings. Legacy
`ADMIN_ACCOUNT`/`ADMIN_TOKEN` reject. Deployments may omit bootstrap and provision
through the private socket, including password reset through stdin.

Plugins trigger background sync without --wait. Codex/Claude return empty decision
JSON immediately after shell-background launch; Pi/OpenCode callbacks return while
the enqueue subprocess runs. The CLI persists jobs, starts an independently
detached finite worker and owns retry/delivery state. Successful startup does not
imply ingestion or processing completion. Plugin cleanup stops unfinished enqueue
children, not workers already detached. Contract tests use fake host APIs, blocked
executables and synthetic CLI/container fixtures; no installed harness or user
registration is used. Real event delivery and artifact flush timing remain assumed.
