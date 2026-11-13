# TokenInsights design

[ADR 0027](adr/0027-personal-backend-and-explicit-connections.md) defines the current
personal backend and explicit connections. Earlier ADRs retain decision history.
The [whitelist](raw-ingestion-whitelist.md) defines metadata-only extraction.

## Ownership

One personal Go backend per canonical storage identity owns capture, direct
acceptance, durable processing, queries and shutdown. TUI, web and desktop
hold sessions; they never open usage stores.
Collector discovery reads OpenCode/Pi/Codex/Claude Code Durable Sources, captures
whitelisted metadata into SQLite and preserves continuity and local location
enrichment. The processor alone normalizes counters and resolves ancestry.

    Sources -> whitelist extraction -> Collector SQLite -> direct acceptance
      -> Token SQLite evidence/receipts/scopes -> processor -> facts/estimates -> views

Delivery IDs identify operational stream/sequence/batch state. Evidence IDs name
qualified native records plus preserved snapshots/context. Missing native record
IDs use lineage/order as witnesses only. Contributions use stable native
harness/session/message/request rules, independent of installation, row IDs,
capture times or deliveries. Equal counters/hash alone never identify shared
consumption. Dataset-qualified keys and the personal dataset `default` remain in
the unchanged SQL contract; no account system is implied.

## Storage contracts

| Role | Default | Authoritative schema | Version |
| --- | --- | --- | --- |
| Collector | collector.sqlite | schema/schema.sql | 20 |
| Token history | server.sqlite | schema/data.sql | 1 |

SQLite is the sole backend. Defaults remain
`${XDG_DATA_HOME:-~/.local/share}/tokeninsights/`.
`--collector-db-path` / `TOKENINSIGHTS_COLLECTOR_DB_PATH` and
`--server-db-path` / `TOKENINSIGHTS_SERVER_DB_PATH` override paths.
Flags precede environment, file and defaults. Parse command flags once.
Canonical paths cannot alias. Reject wrong roles, newer schemas and changed
contracts without mutation. Validate role, version and actual schema objects
read-only before writable opening; new files initialize atomically.

There are no account, application, pairing or jobs stores. Removed product files,
including existing user databases and former pairing guards, remain untouched.
This redesign changes neither retained SQL schema nor version, imports no data,
runs no migrations and deletes no user files. Existing explicit older-storage
recovery ([ADR 0020](adr/0020-reset-older-local-storage.md)) still requires terminal
confirmation: move only affected collector/token files and WAL/SHM companions to
versioned backups. Foreign, newer, mismatched or owned files block reset.
Without a terminal report the issue and change nothing. Rebuilding both stores
cannot recover usage whose native sources were deleted; preserve backups.

SQLite uses one immediate writer, WAL, FULL synchronous durability and pooled
snapshot reads. Ownership excludes another writer process. Accepted evidence,
receipts, pending scopes, published history and replacement generations survive
shutdown. Processing upgrades use durable replacement generations, never schema
migration or evidence deletion.

Token tables retain their existing meanings:

- `raw_evidence`: immutable sanitized JSON and qualified scope.
- `ingestion_instance`: role/version/database/kind identity.
- `ingestion_metadata`: dataset identity, active/target generations, revisions and times.
- `ingestion_batches`, `ingestion_items`, `ingestion_batch_items`: exact requests/receipts, immutable stream/sequence bindings and every mapping.
- `processing_scopes`, `processing_dependencies`, `processing_outcomes`: durable queue/revisions, retry state, ancestry and dispositions.
- `analytics_generations`, `analytics_facts`, `analytics_estimates`, `analytics_provenance`: typed usage and evidence edges.

All keys remain dataset-qualified; active views join the published generation.
Metadata reads retrieve active/target states, active-generation count and newest
processor version within the caller's snapshot. Acceptance and projection have
separate atomic transactions. No cross-store transaction or external broker exists.

## Capture and reliable submission

SQLite evidence_state/sources/outbox/destinations/batches retain lineage/context,
immutable observations, monotonic sequences, bindings, exact saved requests and
acceptance receipts. Capture/checkpoint commit together.

Collector evidence_quarantine retains only source key/format, an opaque fingerprint of local device/inode/size/mtime, parser policy version, fixed error code, byte offset and capture time. It stores no source path or record contents. Deterministic record-limit failures preserve the previous evidence/checkpoint and remain incomplete on later captures while their fingerprint/policy matches. File changes, parser policy changes and launch `--full-refresh` retry capture; successful capture clears quarantine. Cancellation, transient I/O failures and source changes during capture remain retryable without quarantine.

JSONL validates saved byte prefix, parses appended records, then verifies captured
bytes and inode before commit. Rewrite/truncation rotates lineage without deleting
evidence. Partial JSON tail waits; complete JSON without newline is captured but
keeps checkpoint before the last line. Full refresh preserves verified lineage
and deduplicates observations. Prefix verification reads old bytes for continuity and reuses that pass for the captured-prefix hash, retaining final byte/inode verification.

Raw Codex discovery lists files without the legacy ancestry-header scan; ancestry remains sanitized evidence interpreted by the backend.

At most min(CPU count, 8) readers prepare sources concurrently with one SQLite writer. Sources commit in discovery order; up to four completed sources per reader may wait on disk behind a slow source so readers do not idle. JSONL records retain at most 16 MiB per reader; native decoding overhead varies with record shape, so worker admission is a nominal allowance rather than a hard heap cap. SQLite scans retain their existing snapshot/row behavior; sanitized metadata-only temporary spools keep prepared results off the heap. Spools use private permissions and are removed on completion/failure/cancellation. Disk use follows active source sizes; no total disk cap is claimed. Sources commit atomically after continuity verification; one source failure does not discard another source. Location resolution caches are shared and concurrency-safe.

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
become backend diagnostics rather than client repairs.

Unknown/private fields never cross acceptance. Source paths/cursors/Git URLs stay
local; enrichment contains stable hashes, basenames, repository names/provenance.
Collector locks serialize capture and journal writes. Direct delivery never holds
the capture lock across acceptance. Persist request bytes before acceptance;
validate the receipt independently, then advance retained delivery progress
transactionally. Cancellation or lost acknowledgement replays identical bytes.
While one batch is accepted, delivery encodes the next in memory without writing;
preparation persists it only if a fresh read would encode the same entries for the
same binding, otherwise it re-encodes. Acknowledgement reuses the prepared batch
only when stored request bytes are identical. Pending counts derive from the
contiguous outbox sequence instead of scanning it.

Raw/outbox/old generations retain indefinitely initially; no compaction. Replay
can repair only captured fields; whitelist changes may require source rereads.
Bindings retain database identity and the default dataset; collector rebuild,
source deletion and retries must preserve stable fact identities and totals.
Historical destination fields remain part of collector schema 20, without external
submission or fan-out. Batch construction encodes each entry once, accounts for
the exact envelope/byte limit, then validates and persists one final request.
Retried bytes remain immutable.

## Direct acceptance

The backend accepts sanitized evidence directly. This is an internal evidence
contract, with no network ingestion routes. Local session protocol 3 is a separate
transport contract; removed raw HTTP protocols have no adapters.

Exact saved request bytes bind database, default dataset, stream, batch and sequence.
Strict decoding rejects unknown/private fields, duplicate keys, invalid UTF-8,
trailing values and malformed envelopes before mutation. Bounds remain 1 MiB,
256 entries and 256 UTF-8 bytes per metadata string. Safe semantic problems remain
accepted evidence for asynchronous diagnosis.

One transaction commits exact request/hash, every submitted mapping, deduplicated
evidence, immutable receipt and pending scopes. Overlapping/concurrent duplicates
succeed. Changed batch bytes or changed evidence for an existing stream/sequence
reject atomically. Receipt identity, contiguous range/count and acceptance revision
are checked independently of mutable processing outcomes. Lost acknowledgement
replays identical requests.

Acceptance does not imply query visibility. Saved published history remains readable
through capture, processing, failure and quarantine. Refresh completes only after
processing is visible and a successful query displays the current revision.
`data wait` explicitly waits up to 30 seconds for visibility.

## Processing

One backend owns the file/lifetime lock. Connections share its engine; viewers never
reopen paths. Four admission slots, shared short write transactions, two processing workers. One dispatcher owns fair dataset selection and claims whole dataset-qualified components; components connected to an in-flight claim wait. A 32 MiB estimated raw-JSON admission budget bounds concurrent loading; a component over budget runs alone.
Scopes are dataset-qualified native sessions or unresolved source lineages; Codex
ancestry connects dependencies only within that dataset. Pending counts, revision
fences, generation activation and receipt outcomes are dataset-local. Worker
selection proceeds fairly across retained dataset-qualified work; retained dataset boundaries remain enforced. Late parent arrivals invalidate terminal dependent outcomes.

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
classifications, and no evidence classification or storage contract changes.

## Dashboard presentation

The shared React dashboard uses the White First design system in root `DESIGN.md`,
locally bundled DM Sans, accessible keyboard controls, responsive filters and
calendar navigation. Daily/weekly/monthly/yearly periods follow the backend timezone;
weeks start Monday. Custom bounds are inclusive and optionally open. Date changes
reset pagination while preserving dimensions and the selected view. Confirmed and
excluded usage retain their own labeled totals, URL/history state and recovery.

Connection metadata shows requested transport/target, verified backend hostname/
identity and state for each window. It never includes a TCP token or pin.
`GET /api/viewer/connection` returns the bridge's redacted metadata.
The status popup separates capture progress, processing lag, failures and last
acceptance. Sanitized source counts measure sources, never tokens or sessions.
Process CPU/RSS measurements are transient; CPU 100% means one core.
Status polling may query saved revisions; it never scans source files.

## Generations and preserved history

`data reprocess` explicitly queues a replacement generation. Published data remains
queryable until every scope covers the latest accepted inputs. Activation atomically
advances generation/revision; evidence arriving during a build must be covered.
Restart resumes pending work. Processor upgrades retain interrupted builds and use
a fresh current generation; the old published generation stays active until ready.
`data wait` resumes visibility waiting without starting another rebuild.
Processing errors never trigger automatic reset, index repair or reprocessing.

## Querying

Typed columnar SQL filters/groups/sorts/pages active facts;
dashboard queries never scan raw JSON. Raw tables' mere presence does not slow
analytics scans. Read transaction covers metadata/summary/rows/facets. Responses
identify instance/database/dataset/generation/published revision/input revision; browser guards
snapshot identities. Values are bound; identifiers are fixed selections.
Clamp page before offset arithmetic; reject unsafe JavaScript integer aggregates.
Row counts group only visible identities, using the same grouping definition as
row aggregates. Fact counts share the filtered summary scan; saved-session counts
remain dataset-wide. Counts, page clamping, summaries and rows stay in the same
read transaction in SQLite.
IANA/fixed local offsets apply to calendars. Pages 50 default/200 max, dimensions
12 chart groups, time charts 1000 buckets, session facets 100.
The direct query adapter offers bounded complete results for TUI through the same
analytics contract and SQL as paginated HTTP reads. It loads metadata, summaries,
counts, chart and all requested rows within one read transaction, avoiding repeated
dashboard aggregation per page. The TUI retains its 100,000-row limit; HTTP keeps
its existing page limits. Adapter selection stays in command composition and the session
contract still governs terminal access. Date range filters constrain analytics,
never collector discovery or capture.
Repo-only location filters; unknown visible. Context: per-session prompt-side
peak input+cache read+cache write, then average/median/max.

One connected session/ancestry component is processed in memory. Huge components,
partitioning/preaggregations and retention need measured follow-up, not benchmark
claims in this change.

## Sessions, capture and lifetime

Local launchers serialize discovery/startup by canonical storage identity. A
private user Unix socket carries HTTP/2 session protocol **3**, with exact version
equality. The descriptor contains protocol version, instance ID and collector/token
settings only. Existing compatible ownership wins. Local storage conflicts and
version mismatch fail without forced restart or fallback. Remote sessions compare
protocol, never local storage paths. Backend configuration is resolved once.

Every TUI session, desktop window, foreground web command and browser tab retains
a lease. The backend has no dashboard TCP listener or dashboard-binding state.
It serves private assets/API reads and lifetime/capture operations on its socket.
Loopback bridges belong to viewer composition. Browser lease streams tolerate
reload through reconnect grace and do not depend on background JavaScript timers.

Launch and explicit **Refresh** request capture. Identical overlapping requests
coalesce; a stronger compatible request retains bounded follow-up work. Conflicting
source-directory requests reject busy. There is no periodic scanning, completion
plugin, login service or distributed sync. `--capture=false` suppresses launch
capture while durable processing resumes. Filters, TUI `r`, automatic revision
reads and browser reload query saved usage only. `--full-refresh` rereads sources
and retries quarantine; `--source-dir` applies to launch capture.

Closing one viewer does not cancel work serving another. After the final lease and
reconnect grace, stop admission, cancel/join capture and processing, drain requests,
close storage and release ownership. Startup grace covers a crashed launcher before
attachment. Shutdown need not empty the durable queue; the next owner resumes it.
Partial startup cleans every acquired resource and permits reopen.

## Explicit connections

`endpoint` owns pure Local/SSH/TCP values, parsing, serialization and safe labels.
`local` is the default. `connect STRING` opens exactly that target;
`tui|web --connection STRING` explicitly selects another backend. No environment
variable or saved profile selects a remote default. Failure cannot choose another
transport or local data. Running `tui` inside SSH still opens that machine's TUI.

SSH uses system SSH through `sshtransport`, with
`ssh://[user@]host[:port]`. The remote executable discovers/starts its own backend
and splices stdio to the private socket; `remote` need not be running.
A single connection carries session protocol, lease, query, progress and capture.
Unsafe SSH option tokens, password-bearing authorities and query settings reject.
`remote.command` / `TOKENINSIGHTS_REMOTE_COMMAND` selects a locally configured
remote executable path, never a command from a connection string.
Remote source overrides must be absolute; local database path flags reject.
Read and write failures preserve bounded SSH stderr and remote-command repair
guidance after waiting at most the existing exit grace for process/stderr completion.
Pipe deadline errors retain their timeout classification.

TCP uses a running foreground `remote` gateway and pinned TLS through
`tcptransport`. The self-contained versioned URI is:

```text
tokeninsights://connect/v1?address=HOST:PORT#pin=HEX_SHA256_SPKI&token=BASE64URL_SECRET
```

Unknown/duplicate keys and missing/malformed fields reject without echoing secrets.
Verify the server public-key pin before sending the credential. Every request,
including descriptor and private round trips, authenticates. The gateway pins one
backend instance and redacts storage paths. Credentials grant session reads,
progress, leases and default capture only; ingestion, path overrides, general
control and maintenance reject. A TCP string used on the serving host remains TCP.

`remote` binds `0.0.0.0:0` by default. `--listen HOST:PORT` chooses its bind;
`--advertise HOST[:PORT]` chooses the reachable address. Without an override use
the SSH_CONNECTION server address, then a nonloopback interface, then loopback.
Wildcard advertisements reject. Print a complete shell-quoted
`tokeninsights connect 'URI'` command. Identity, pin and access secret are ephemeral.
Stopping the gateway closes its listener/connections and invalidates its strings;
other local/SSH leases survive. The gateway owns no storage or durable credentials.

## Loopback bridges and native desktop launch

`dashboardproxy` serves each viewer's selected session on a loopback HTTP bridge,
default available port. It forwards bounded dashboard/API reads and lease streams,
with host/origin checks and sanitized headers. Its only capture write is
`POST /api/viewer/refresh`, requiring same origin and JSON and forwarding default
capture only. No source paths, maintenance or general private control are exposed.
Browser JavaScript never receives a TCP credential. The caller owns the session
and bridge lifetime. `web --host` accepts loopback only; use `remote` for TCP access.

Bare `tokeninsights` opens desktop; `web` explicitly opens a browser and
`web --open=false` prints its URL without opening one. An unavailable desktop
fails with repair guidance. `--app=native` on bare or `connect` selects the
experimental native app instead (ADR 0030).
No browser, transport, other-app or local-data fallback is permitted.

`desktoplaunch` owns native Unix-socket rendezvous. The launch specification and
TCP secret stay native; the OS URL carries only the selected app's scheme and an
opaque rendezvous ID. Launch returns after that app acknowledges a live requested
session, not after the OS opener exits. No credential files are written. Each
window keeps its own target/session. Desktop composes the shared dashboard/bridge
without owning storage. Native (`packages/desktop-native`) renders MyGo native UI
over its window's `localclient` session with no bridge; DESIGN.md roles are its
guarded visual contract. Its formatting, calendar, query, status, column, chart
and filter rules come from pure `dashboardview`, pinned to the web by shared
fixtures in `packages/contracts/fixtures/dashboard-view` (ADR 0032). Its theme
choice persists app-wide in the app's user data `preferences.json`. Each
window's model polls status and collector progress every 5 seconds while the
window shows, follows snapshot identity/revision as the web dashboard does
and derives its header status from `dashboardview`. Neither shell imports
the other.

## Packages and contracts

Apply the architecture checklist in [AGENTS.md](../AGENTS.md#architecture).
ADRs 0010/0011 retain principles for semantic operations, atomicity, identity,
cancellation and ownership; ADR 0027 supersedes their retired product boundaries.

| Owner | Responsibility |
| --- | --- |
| `pipeline`, `rawcollectorstore`, `collectordb` | Whitelist capture, continuity, outbox and collector schema |
| `collector`, `evidence`, `publication` | Direct delivery, sanitized contracts and stable contributions |
| `processor`, `dataengine` | Pure interpretation; durable work and retry orchestration |
| `datastore`, `adapters/sqlanalytics` | SQLite acceptance/publication and consistent analytics |
| `analytics`, `application` | Semantic query contracts and portable viewer operations |
| `localruntime`, `localbackend` | Composition, ownership, shared capture and session lifetime |
| `localprotocol`, `localclient` | Protocol 3, compatible startup and session operations |
| `endpoint`, `sshtransport`, `tcptransport` | Pure target values and explicit transport adapters |
| `remoteaccess` | Temporary authenticated TLS gateway over one session |
| `dashboardproxy`, `desktoplaunch` | Loopback dashboard bridge and private acknowledged launch |
| `dashboardview` | Pure native dashboard presentation rules; no UI toolkit, shell or transport |
| CLI/TUI/desktop/native | Executable preferences and presentation composition |

Resolve executable flags/env once into typed settings. Domains neither read
deployment settings nor construct another layer's adapters. Keep SQL/driver types
inside persistence; clients cannot open backend storage. Pure accounting tests
complement real SQLite transaction/restart/concurrency contracts.

## Progress and recovery

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
empty-state guidance follows refresh completion. `--capture=false` skips startup
capture and visibility waiting, but existing durable processing resumes and can
refresh the displayed revision. TUI reload is query-only. Viewer filters never restart
capture or select processing work. Local, SSH and TCP viewers query the same selected backend without transport fallback.

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

## Configuration, products and verification

Supported settings: `collector.db-path`, `in-process.server-db-path`,
`in-process.host`, `in-process.port`, `remote.command`.
Storage names and defaults stay unchanged. Retired application/distributed settings,
implicit endpoints and saved viewer settings return actionable errors without
printing values, rewriting configuration or deleting files. Users repair their
configuration explicitly. There is no automatic migration.

The default installer installs the desktop product, which includes CLI/backend, and
adds `~/.local/bin` to the shell profile when PATH lacks it. The CLI-only product
remains behind `--cli-only` for hosts without a display. `install-dev.sh` builds
desktop or CLI from any canonical ref on the installing host; `install-dev-native.sh`
builds only the experimental native app beside them, and it never owns the command.
Either product install can take command ownership from the former MyGo product only
when the CLI symlink targets its known bundled path and its product directory has
the matching installer ownership marker. Retired product files and launchers remain
untouched; failed publication restores the previous command.
Go products need no host JavaScript runtime.
Shared React presentation lives in packages/dashboard, browser composition in
packages/web and TypeScript contracts in packages/contracts. The backend embeds
ignored generated browser assets. No binaries/bundles/generated assets are committed.
Only stable source tags publish prebuilt archives; other refs build locally from source
(ADR 0031). See [release](release.md). There is no hosted server, PostgreSQL, account UI,
server installer, Docker product deployment, completion plugin or sync job product.

Validation remains local through isolated Docker Compose projects (ADR 0029),
including real OpenSSH and isolated TCP/network validation. Preserve
accounting/privacy, receipt replay, source deletion/rebuild, publication rollback,
restart and snapshot oracles. Add shared owner/startup races, final lease shutdown/
reopen, query/Refresh parity, bad pin/token/expired URI/protocol mismatch, serving-host
TCP, native secret handoff/desktop acknowledgement and unavailable-app checks.

Format/lint before verification. Final PR evidence requires committed clean HEAD:
full `mise run check:pr` and
`mise run build:docs`. Never replace these with uncommitted checks. Use retained
artifacts; never poll long checks or interfere with another invocation's resources.
