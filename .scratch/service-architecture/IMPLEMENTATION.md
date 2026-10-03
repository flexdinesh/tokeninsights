# TokenInsights service implementation plan

Status: Implemented and validated. Revalidated after compaction, then implemented under the user's authorization. No unresolved product decisions or SQLite schema changes.

Final package/API simplifications, operational details, measurements, and validation limits are recorded in [VALIDATION.md](VALIDATION.md). Its implementation reconciliation supersedes the proposed type names and wire details below; `docs/design.md` is the shipped design contract.

This plan implements [the architecture proposal](PRD.md). It fixes the interfaces, state transitions, ordering, compatibility behavior, and validation needed before coding. Keep changes within the existing Go module and ship the command migration only after service ownership, refresh, and viewer behavior work together.

## Scope and release contract

The release adds managed `service start|stop|restart|status`, foreground `service run`, and `refresh [--wait]`. Bare invocation ensures the service and prints status. `view` remains a local TUI, requests refresh on opening, and keeps direct read-only SQLite analytics. Web opening reads saved usage; Refresh explicitly ingests and normalizes.

First startup initializes an empty DB. Startup/restart never syncs or repairs existing data. `--host` binds only the web UI/API listener, with default `127.0.0.1:8765` and explicit IPv4/wildcard support. Remote browsers use the service machine's sources. Authentication and login/reboot autostart remain out of scope.

Include `--open` for explicit browser launch, `status --json`, and `restart --reload-sources` for deliberate source-root changes. Do not add refresh harness/reason flags yet; the internal request model can support future hints without shipping harness plugins. No periodic sync, filesystem watching, SSE, cloud export, remote TUI, new token metrics, SQL pagination rewrite, or history-retention policy is included.

Do not modify SQLite tables, columns, indexes, schema version 14, or data generation 5. Public HTTP contract additions below are transport metadata. If source refactoring cannot preserve stored identities, stop that implementation slice and propose the required compatibility change for explicit approval; never quietly bump parser identity or generation.

## Current code and required changes

| Current location | Concrete change |
| --- | --- |
| `cmd/tokeninsights/main.go` | Context-aware signal handling and typed exit-code mapping, including stopped status |
| `internal/cli/command.go` | Register service/refresh; route bare and leading startup flags to service start; preserve help/version |
| `internal/cli/flags.go` | Separate viewer flags from service flags; retain existing DB-path defaults |
| `internal/cli/command_serve.go` | Deprecated foreground wrapper around service runtime; remove startup ingest/browser policy |
| `internal/cli/command_view.go` | Assemble local observation client; do not launch a daemon synchronously before the TUI appears |
| `internal/cli/table.go` | Request/observe refresh instead of invoking pipeline; allow controls on saved data |
| `internal/cli/sync_coverage.go` | Separate viewer observation cancellation, requested-check state, and durable publication |
| `internal/cli/command_sync.go`, normalization/reset handlers | Typed application actions; online forwarding or offline execution |
| `internal/server/server.go` | HTTP handlers only; application dependency replaces process-local syncer/state and lifecycle |
| `internal/server/api_transport.go` | Map application status plus snapshot metadata into generated models |
| `internal/server/data.go` | Retain SQL/aggregation; return transaction revision and protected snapshot identity |
| `internal/server/network.go` | Move listener binding/display helpers to service; retain IPv4 validation behavior |
| `internal/server/port_conflict.go` | Delete managed listener takeover path and `lsof`/termination helpers |
| `internal/server/browser.go` | Move browser-launch helper to `internal/browser`; invoke from CLI only |
| `internal/db/writer_lock.go` | Share canonical path resolver; retain temporary writer-lock behavior |
| `internal/db/open.go` | Add context-aware reset-all entry point without changing reset SQL |
| `internal/pipeline/types.go`, adapters, recovery | Explicit source configuration, job identity in results/progress, byte-identical fingerprint behavior |
| `docs/openapi.yaml`, generated transport models | Instance/data identity, readiness, and pending-refresh state |
| Web API/controller/header/coverage/mocks | Explicit Refresh, epoch-aware queries, first-start and queued feedback |
| Development/E2E launchers | Foreground service without startup sync; fixture data setup becomes explicit |

## Package boundaries and types

Add `internal/dbpath` with `Canonical(path string) (string, error)`, extracted from `db.canonicalDBPath`. Preserve existing absent-file/parent-symlink behavior. Add cycle detection for dangling symlink chains rather than unbounded recursion. Writer locking and service identity use this same function.

Add `internal/app` for application-owned actions, refresh scheduling, and status. It imports pipeline/db, not CLI, service, HTTP models, or Bubble Tea. Add `internal/service` for manager, private control transport, process startup, filesystem configuration, and runtime composition. The public server receives narrow application methods; the service composes app and server.

Use these concrete application concepts:

- `ActionKind`: sync, normalize, reset-canonical, reset-all. Refresh is a scheduling request that results in a normalized all-harness sync action.
- `SyncRequest`: selected harnesses, explicit source configuration, normalization/full-refresh flags, and observation time. No function values on the wire.
- `NormalizeRequest`: selected harnesses, source configuration for any recovery, and observation time.
- `Operation`: ID, kind, queued/running/succeeded/failed/cancelled state, acceptance time, optional durable job ID, summary, safe failure, and reset sequence.
- `RefreshReceipt`: service instance ID, coalesced operation ID, acceptance time, and whether work is pending or already assigned.
- `SnapshotIdentity`: random service instance ID, random data epoch, and durable revision. IDs are opaque strings; revisions retain signed integer semantics.
- `Failure`: typed code and safe message, with local mapping to the existing recovery/cancellation sentinel errors.
- `Status`: service phase, data readiness, active action, pending refresh, requested-check time, snapshot identity, and existing durable sync status when readable.

Expose `RequestRefresh`, `SubmitSync`, `SubmitNormalize`, `SubmitReset`, `Operation`, `CancelOperation`, `Status`, and `Close` on the application controller. Methods accept observation contexts for bounded calls; operation execution uses its own controller/action context. Do not pass the request context into accepted shared refresh.

Use small interfaces only at consumers or necessary test seams. Scheduler tests inject runner functions and clocks; process tests use real subprocesses. Do not create a generic dependency container or application-wide event bus.

## Explicit source configuration

### Preserve resolution and identity

Currently four adapters independently read `HOME`, `XDG_DATA_HOME`, `CODEX_HOME`, and `CLAUDE_CONFIG_DIR`, while `recoverySourceKey` resolves them again. `Normalize` can expand into all-harness recovery using the daemon's environment. Forwarding current CLI options without fixing this would change the caller's requested scope.

Introduce `pipeline.SourceConfig`, resolved once at the CLI/service setup boundary. Carry it through `SyncOptions`, `NormalizeOptions`, and discovery. Its descriptor contains:

- Mode: default sources or a source-dir override.
- Original caller base directory, captured once.
- Source home used by existing location display/resolution, where needed.
- The five ordered root slots: OpenCode, Pi, Codex sessions, Codex archives, Claude Code projects.
- Each root's absolute I/O path and its original lexical identity path. Unconfigured roots remain explicitly absent.
- Override root's original lexical spelling and absolute I/O path; harness subdirectory/fallback policy remains the existing one.

Use absolute paths for file access, but construct source IDs using the existing lexical inputs. For example, OpenCode currently hashes both source path and root. Derive the same legacy identity path from the configured lexical root plus relative file path, compute IDs unchanged, then attach the absolute I/O path. Pi/Codex/Claude relative source keys and discovery ordering must also remain unchanged.

No process-wide `os.Setenv` or `os.Chdir` is allowed for a job. Audit location resolution and Codex ancestry lookup for remaining environment/CWD dependencies. Resolve their operational paths from the job descriptor without changing stored location or fact identities.

Keep the recovery fingerprint's exact `rebuild-sources-v1` layout, mode marker, five default root slots, and lexical absolute normalization. Do not symlink-resolve source roots for that fingerprint: directory availability must not change a pending rebuild's scope. The DB identity resolver is a different concern.

Nil source configuration on low-level pipeline options preserves current environment resolution for existing package callers/tests. Resolve it once at public pipeline entry, including dry-run, rather than separately inside each adapter. Adapter discovery tests can use the same resolver directly. CLI online requests transmit the resolved descriptor; ordinary dashboard refresh uses the saved descriptor.

### Required parity tests

Compare legacy and explicit resolution for defaults, absent HOME, individual harness overrides, relative environment paths, single-harness custom directories, multi-harness custom roots, missing subdirectories, symlinked roots, and Codex archives. Assert exact source IDs, raw keys, recovery fingerprint, canonical totals, diagnostics, and continuity reuse.

Create a pending recovery with the old fingerprint algorithm, then resume with the new configuration representation. Test that changing the descriptor fails before writes. A newly created source directory must not change that descriptor's fingerprint.

## CLI parsing and compatibility

Continue using Go `flag.FlagSet`. No CLI framework is needed. Service parsing must not reuse `parseViewerOptions`, which currently registers unrelated filters and `--no-sync`.

| Invocation | Accepted options |
| --- | --- |
| Bare/start | `--db-path`, `--host`, `--port`, `--open` |
| Restart | `--db-path`, `--host`, `--port`, `--reload-sources` |
| Stop | `--db-path` |
| Status | `--db-path`, `--json` |
| Foreground run | `--db-path`, `--host`, `--port` |
| Refresh | `--db-path`, `--wait` |
| View | Existing viewer flags, including `--no-sync`; no host/port |

Canonical syntax places action before flags: `service start --host ...`. Do not add flags-before-subcommand parsing. `service` alone and unknown actions print concise usage errors. Per-command help exits zero without setup. Root help/version also remain zero and side-effect free.

Track explicit host/port presence through `FlagSet.Visit` or optional fields so omitted defaults do not overwrite saved settings. Explicit port zero remains zero in configuration and reports the assigned runtime port. Empty host normalizes to loopback; validation remains existing IPv4 rules.

Root viewer flags return a migration error with `tokeninsights view ...`. Do not accept `--no-sync` at root/start: start never syncs. During one deprecation release, `serve` routes to foreground `service run`; accept its old `--no-sync` as a warning/no-op, but reject old viewer filters with a migration hint. This is an intentional compatibility change, not a second server implementation.

`main` maps usage to 2, stopped `service status` to 3, general failure to 1, and successful actions to 0. Stopped status prints its structured/text result once and produces no duplicate stderr error. Refresh waiting on failed/cancelled work returns nonzero. Async acceptance is success, not a claim ingestion succeeded.

Implement signal-derived command contexts. TUI exit cancels local requests/queries only. Ctrl+C on an exclusive forwarded maintenance action requests cancellation with a fresh short-lived context; waiting on a shared refresh never cancels it. Preserve existing summaries and confirmation messages.

## Filesystem and instance discovery

### Paths and formats

Compute a stable database key as SHA-256 of canonical DB path: compare the full 64-hex digest and use its first 32 hex characters for filenames. Separately, generate a random 128-bit runtime `instanceId` on every start; this is also the control nonce. Generate independent random 128-bit data epochs. Never use the stable database key as the browser's runtime identity. Compare full DB identity during handshake and fail on any filename collision. User directories already scope instances to the current user.

Persistent sidecars are `<db>.service.op.lock` and `<db>.service.lock`; retain `<db>.lock` for existing writer ownership. Never unlink lock files. Existing hard-linked database aliases are unsupported: reject databases with multiple hard links before managed ownership, and revalidate file identity before startup completes. This closes separate-path ownership for already-existing aliases; external file replacement while running is unsupported and diagnosed.

Use the PRD's XDG config/state/runtime layout. Validate a nonempty runtime directory is absolute and current-user owned, then create a private TokenInsights child directory. If XDG runtime is unavailable use private state/runtime. Discovery must also consider the known fallback directory when the caller's XDG runtime availability changes. An ownership lock held without a matching reachable record is unresponsive, not stopped.

Config format version 1 stores DB identity, requested bind/port, and source descriptor. Discovery format version 1 stores service nonce, PID, version/commit, protocol/action versions, requested bind, actual listener address/port, local dashboard URL, socket location, and startup time. It stores no transcripts or entire environment. Config and runtime files use strict decoding, bounded size, private permissions, and atomic temporary-file replacement.

Status probes never create directories/locks or clean stale records. Setup actions validate trusted directory ownership, symlinks, record identity, socket length, and hard-link rules before writing. Test user-controlled paths as ordinary inputs; avoid copying filesystem details into public API errors.

### Admission ordering

The lifecycle operation lock serializes starts, stops, restarts, and online/offline mutation routing. The daemon alone holds its lifetime ownership lock. Pipeline/reset code acquires the existing writer lock only for actual writes.

An explicit mutating CLI action takes the operation lock, probes service ownership, then chooses a route. If a compatible service runs, submit the action and release the lock before waiting. If ownership is free, hold the operation lock for the entire standalone mutation so a service cannot start midway. If ownership is held but unreachable/incompatible, refuse bypass. Read-only/dry-run commands do not acquire or create service locks.

Managed startup and foreground run use the same admission rules. The managed child does not reacquire its parent's operation lock: the parent holds it until readiness. In particular, never hold the writer lock while calling a pipeline method that acquires it again.

This coordinates the new executable's paths. Older binaries lack the admission protocol and may still contend using the writer lock. Never infer that their running job satisfies the current refresh; schedule the requested work after writer availability. Do not promise perfect reset notifications for old binaries bypassing service coordination.

## Startup and shutdown protocol

### Manager and child

`Manager.Ensure` resolves configuration under the operation lock and probes the private socket. A matching active service is reused. Explicit conflicting host/port returns current settings and a restart hint. A healthy service with a compatible control/action/schema/data contract can be reused across release versions; version alone is diagnostic.

Spawn the absolute current executable with a hidden `__service-run` entry point. Pass the startup descriptor and readiness channel as inherited file descriptors 3 and 4 using `Cmd.ExtraFiles`. No shell, serialized source paths in argv, or environment mutation. Set Unix `SysProcAttr.Setsid`, stdin to the null device, and startup stdout/stderr to private files. Use `exec.Command`, not a lifetime-bound `CommandContext`. [Go subprocess contract](https://pkg.go.dev/os/exec@go1.26.0#Cmd), [Unix process attributes](https://pkg.go.dev/syscall@go1.26.0#SysProcAttr).

Child startup order:

1. Decode/validate descriptor and acquire lifetime lock nonblocking.
2. After acquiring lifetime ownership, validate and remove stale discovery/socket artifacts only for this database identity in its private directory. Never unlink ownership/writer lock files or remove a socket while lifetime ownership is held by another process. Bind private and public listeners. No HTTP application requests accepted yet.
3. Inspect existing DB without modifying it. For a missing DB only, acquire writer lock, recheck, initialize the current empty schema, close connection, release lock.
4. Reject corrupt/unrecognized/newer data. Recognized older/metadata-pending/rebuild-pending data allows runtime startup with analytics blocked and recovery controls available.
5. Assemble controller/handlers and start listeners with service phase starting.
6. Publish discovery, then saved configuration, each by atomic replacement after successful initialization; mark running and send ready descriptor. Child commits these records, avoiding a parent-death gap after readiness. The parent still owns the operation lock during normal startup.
7. Parent validates nonce through control, prints actual status, closes bootstrap descriptors, and returns. One background `Wait` reaps this child if the parent remains alive, such as a TUI.

The two records are not an atomic filesystem transaction. Retain prior config bytes until startup finishes. On a caught initialization failure, close listeners, clean only this child's runtime artifacts, restore prior config if this startup replaced it, release ownership, and return a structured failure. If no prior config existed, remove only this startup's config. Ownership and record matching must be verified before cleanup. A process crash between replacements may leave validated new configuration; status still checks live ownership/readiness and never treats that configuration as a running service. Fresh DB initialization may have occurred before a later filesystem failure; report it truthfully rather than deleting the DB as cleanup.

The child writes a bounded readiness message then closes that channel. An abruptly dead parent must not cause an endless pipe wait; a fully initialized child may remain discoverable. Explicit parent cancellation stops only its own child, waits for cleanup, and never kills a reused instance. Use the child process handle, not sidecar PID matching.

### Stop and restart

Stop takes the operation lock, probes validated identity, POSTs shutdown, then waits for lifetime ownership release and TCP listener closure. A stopped instance succeeds without creating state. A responsive service transitions once to stopping: admission closes, queued work is cancelled, active work is cancelled, request contexts are cancelled, HTTP drains, bookkeeping completes, sockets/discovery are removed, then ownership releases.

Return shutdown acceptance before closing its control listener. `http.Server.Shutdown` cannot wait on the same still-running shutdown handler; launch runtime shutdown after the response is sent.

Do not offer force stop in V1. If deadline expires and ownership remains, report unresponsive/stopping and leave identity artifacts intact. Never start a replacement merely because HTTP disconnected. Restart holds one operation lock around verified stop and new startup. Validate proposed settings before stopping; failed replacement leaves old saved settings and a truthful stopped failure.

Foreground run participates in ownership/discovery and can be stopped by the manager. It logs to the caller as well as bounded service logs, never auto-opens a browser, and exits on its signal context.

### Named limits

Initial engineering defaults, subject to measurements: operation-lock wait 30 seconds; startup 45 seconds, allowing existing 30-second writer contention; stop 15 seconds, covering existing 10-second HTTP drain and five-second failure bookkeeping; short control calls three seconds; control body 256 KiB; 16 queued exclusive actions; 128 retained completed operations for ten minutes; 256 refresh request aliases with a ten-minute TTL; private log rotation at four MiB with three files. Keep existing 30-second analytics deadline.

These are operational bounds, not calculation shortcuts or service-level promises. Define named constants in the owning packages, inject shorter timings in tests, and tune only with evidence. Long refresh itself has no new overall timeout. Startup does not wait for ingest. Log through a bounded writer; redirected emergency stderr is private and must not become an unbounded normal logging path.

## Private control contract

Use HTTP/1.1 JSON over the private Unix socket with a dedicated `http.Transport.DialContext`. Disable proxy environment use and redirects; the socket path, not a network URL, selects the destination. Close idle client connections on viewer exit. Handler bodies reject unknown fields, multiple JSON documents, invalid discriminators, and oversized input.

| Route | Request and response |
| --- | --- |
| `GET /control/v1/instance` | Service/database identity, versions/capabilities, bind, readiness; independent of compatible analytics |
| `GET /control/v1/status` | Full application status; no writes |
| `POST /control/v1/refresh` | Caller-generated request ID; coalesced receipt |
| `POST /control/v1/operations` | Request ID plus exactly one typed sync/normalize/reset payload; queued operation receipt |
| `GET /control/v1/requests/{requestId}` | Resolve a retained caller request to its receipt after a lost POST response |
| `GET /control/v1/operations/{id}` | Safe outcome, progress/job identity, summary, or unknown/expired |
| `POST /control/v1/operations/{id}/cancel` | Cancel queued/active exclusive work; reject shared-refresh cancellation |
| `POST /control/v1/shutdown` | Expected instance nonce; accepted shutdown |

Successful mutation admission returns 202. Invalid request returns 400, identity/contract/reset conflict 409, missing/expired operation 404, capacity exhaustion 429, and stopping/unavailable 503. Public handlers use their generated error model; private typed failures can have richer codes without exposing them remotely.

For exclusive actions, operation ID equals caller request ID, scoped to the service instance. Active/queued IDs cannot be evicted; completed IDs use the bounded outcome registry. Ordinary refresh maps caller IDs to the coalesced operation through a separate bounded LRU/TTL alias cache. Evicting an alias never cancels its operation. Bound ID length and validate its format before allocation; compare decoded typed payloads for retained-ID replay.

While retained, replaying the same ID/payload returns its receipt; reusing an ID with a different payload fails. A timed-out POST queries the request route before it has a receipt, and the operation route afterward. Unknown/expired lookup means unknown outcome, never permission to automatically replay destructive work. Deduplication ends on eviction, expiry, or instance exit; document that limit. A changed instance likewise yields unknown outcome. Test thousands of aliases against fixed storage bounds, payload conflicts, lost responses, and expiry. No pending actions or deduplication survive crashes.

Cancel permission is ownership policy: private operations are either exclusive or shared refresh. The same local user has filesystem authority, but an observer cannot cancel shared refresh through this endpoint. Reset confirmations are still required in CLI and in the typed control request; no reset route exists publicly.

Handshake carries control protocol 1, action contract 1, supported schema/data generation, capabilities, and build diagnostics. Read identity/status and stop remain available on recognized data-incompatible instances. Mutations require compatible action/storage contracts. Newer service versions are never silently downgraded.

## Scheduler state machine

### Work admission and bounded state

One controller worker executes all mutations. Protect admission/state with one mutex; never hold it during pipeline, SQLite, HTTP, or callbacks into a client. Status returns copied maps/structs. Progress updates update bounded latest state rather than append an event per source.

Maintain an active work item, FIFO deque of exclusive actions, at most one pending ordinary refresh item, reset barrier, closed flag, and bounded completed-operation registry. Multiple callers receive the same coalesced refresh operation ID rather than allocating an unbounded ticket per caller. Their response acceptance times remain specific to the request.

Ordinary refresh scope is immutable saved configuration, all supported harnesses, normalization on, full-refresh off. Exclusive sync options are never folded into this work automatically. Full refresh, no-normalize, custom roots, or targeted CLI sync remain explicit actions with their own outcome. This deliberately avoids ambiguous cancellation/joining behavior.

FIFO admission determines work order. The one pending refresh occupies a queue position; later requests merge into it without changing its position. Exclusive actions cannot starve behind a continually replaced refresh. Reject excess exclusive requests immediately. Future plugin priorities/debounce are outside this release.

### Snapshot boundary and coalescing

Before the active ordinary refresh begins source discovery, another ordinary request can share it. Set a `captureStarted` flag atomically from the first discovering/resetting/rebuilding event; pipeline must publish that event before it obtains any source snapshot. A request racing that flag is either admitted before capture or assigned the pending follow-up.

Once capture begins, every new request joins/creates one pending follow-up. Mark capture conservatively at recovery entry. Do not use wall-clock freshness comparisons to decide whether a request was satisfied; use operation assignment and explicit capture state.

If pipeline is waiting for an external writer before discovery, callers can share the upcoming service work. The service does not join the external job: its full option strength cannot be proven from current durable fields, and accepting a late request must cover sources already checked externally.

At terminal completion, finish the active operation, retain its outcome, then promote next queued work. Pending refresh runs even after ordinary partial failure because it was explicitly requested. There is no self-scheduled retry, except existing pipeline recovery behavior within the requested action. If no pending demand exists, the worker sleeps on a notification channel.

### Durable jobs and outcomes

Preserve pipeline job transactions and publication points. Add durable job ID to pipeline summary/progress so the controller can associate an operation after the job is created, including post-reset recovery. Existing CLI summary formatting need not print this field.

Use `db.ReadSyncStatus` as authority once operational tables exist. Process state contributes queued/admission information, operation ownership, and recovery feedback before those tables are readable. Never increment a separate local analytics revision or reuse a previous job's progress for a newly queued check.

At execution, use current actual time for observation unless an explicit CLI action carries its existing invocation time; keep pipeline wall-clock start/completion timing unchanged. Use server acceptance/capture time for requested-check presentation, not browser time.

Classify failures centrally: recovery required, metadata upgrade, rebuild pending/scope mismatch, cancelled, writer unavailable, source failure, database failure, and internal failure. Return safe strings across both transports; log scrubbed details. Map recognized failures back to sentinel errors for existing CLI recovery guidance. Do not serialize Go error objects or trust arbitrary error text to contain no private content.

### Reset barrier and cancellation

On acceptance of explicit reset, set the barrier and cancel/remove pending ordinary refresh demands before it can execute. Refuse new refresh demands while reset is queued/running; allow status/read observation. Earlier exclusive FIFO actions finish first. Reject a second reset while the barrier is active.

Reset acquires the normal writer lock through existing reset actions, applies existing SQL, invalidates snapshot identity, and releases the barrier only after terminal bookkeeping. A pending pre-reset refresh must not repopulate data after reset. Cancelling a queued reset releases the barrier but does not silently recreate cancelled refresh demand.

Recovery's internal reset is distinct: it invalidates data identity but keeps same-source follow-up demand, since recovery is itself the requested refresh. Do not clear it as if the user requested empty storage.

Cancelling an exclusive queued action removes it; cancelling a running action cancels its action context and waits for pipeline rollback/completion bookkeeping. Accepted shared refresh survives HTTP/TUI disconnect. Controller shutdown cancels both active and queued work. All outcomes remain observable until eviction or instance exit.

## Online and offline maintenance

Factor action execution out of CLI rendering. Sync/normalize requests carry resolved caller source configuration; daemon execution must not substitute its saved roots. The selected DB always belongs to the receiving service; a control request cannot override DBPath.

Expose context-aware `db.ResetAllContext`; retain the existing wrapper if current callers need it. For canonical reset, move the current lock/read-validation/writable-open/reset sequence into a shared application action. Confirmation previews stay entirely in CLI and do not submit work or start a service.

The router follows the admission protocol above. Offline actions use current invocation context and progress notices. Online actions poll operation status, reconstruct typed summary/failure, and print the same summary once. Early errors and non-confirmed resets preserve current output behavior.

Dry-run executes locally with resolved caller configuration, never enqueues, creates a service, writes operational jobs, or upgrades DB metadata. No generalized service-owned preview queue is needed.

An explicit sync/normalize using caller roots that differ from saved roots is allowed if existing recovery rules allow it. It never rewrites service configuration. The next ordinary refresh still uses saved roots.

## Data readiness and snapshot invalidation

Data readiness is an enum: ready, metadata-upgrade-required, recovery-required, rebuild-pending, unavailable. Service lifecycle is a separate starting/running/stopping enum. A ready empty DB is valid and its last successful refresh is absent.

Startup calls existing compatibility inspection but does not call recovery or metadata upgrade on existing files. Refresh/maintenance preserves the pipeline's existing upgrade/rebuild rules. Analytics queries continue using `db.BeginAnalyticsRead` inside the actual transaction.

Controller data epoch is a random opaque token, regenerated on reset/recovery replacement and uncertain compatibility change. Instance ID changes on each runtime start. Revision comes from SQLite. Do not depend on revision monotonicity across resets or build identity across dev binaries.

Public analytics acquire a context-aware read permit from the controller before opening the DB. At reset/recovery-replacement execution, close read admission and wait for existing readers, subject to existing query deadlines. This read gate is separate from the refresh admission barrier: a reset waiting behind earlier exclusive work does not block saved-data reads yet. A read holds the permit until its consistent response is assembled and tagged; no open read transaction survives the response. This prevents a response from being labelled with a newer epoch than its data. Use a bounded notification-based gate, not an uncancellable `RWMutex.Lock` hidden beneath HTTP deadlines.

Do not infer destructive recovery from asynchronously displayed progress. Add a narrowly scoped pipeline option hook invoked synchronously before existing recovery reset/replacement code can write. Its context-aware error aborts that destructive step; propagate the hook through normalization's recovery path. The application hook closes the gate, drains readers, changes the epoch before the first destructive write, and marks analytics unavailable. Explicit reset calls the same application transition. Hold that gate until action bookkeeping establishes compatibility/readiness, including failure paths; reopening permits never implies incompatible data became ready. Offline callers leave the hook absent and preserve existing pipeline behavior. Tests pause at the exact before-write boundary to prove old reads drain and new reads do not enter.

Return revision from the same analytics/facet transaction as data. Clients reject superseded responses by query/snapshot identity. Existing compatible ordinary refresh still publishes during the job; ordinary writes do not hold the reset barrier.

Local TUI SQLite reads retain their own consistent transactions. When a service is reachable, sample its instance/epoch/readiness before and after each reload; discard the result if identity changed or analytics became unavailable. Tag reload messages with that identity and viewer selection generation; also discard messages superseded by newer status/reset/filter transitions. A status epoch change clears old rows/facets and loads a new snapshot. This preserves local SQL without claiming instantaneous cross-process reset notification: a reset beginning after the final sample is learned on the next status observation. `--no-sync` can read an existing local socket for reset identity but never creates runtime state; if no service exists, keep current durable-status observation and read-only reload behavior.

## Public API changes

Keep existing endpoint paths and API version v1. Add optional fields in the OpenAPI document, always emitted by the new runtime, then generate committed Go and TypeScript models. New browser code requires the service metadata it uses; a stale page on an old server should show version/reload feedback rather than silently trust an empty identity.

| Response | Added fields |
| --- | --- |
| Instance | `instanceId`, `dataEpoch`, `dataReadiness` |
| Sync | `instanceId`, `dataEpoch`, `dataReadiness`, `pendingRefresh`, `checkRequestedAt` |
| Usage and facets | `instanceId`, `dataEpoch`, transaction `revision` |

Reuse existing sync phases: waiting for queued work; resetting/rebuilding for recovery; ready/failed/cancelled/interrupted for terminal outcomes. Do not add an unrelated service lifecycle enum to durable tables. `running` means a check is active or pending, including the gap before a durable job exists; `progress` belongs only to the actual durable active/latest job.

`checkRequestedAt` is the latest outstanding ordinary request's server acceptance time, or zero when none remains. Combined with assigned job progress, it suppresses old checked/empty coverage during a follow-up. Failure never upgrades that timestamp into a successful check.

POST sync returns 202 for accepted work, with merged active/pending status. It returns 503/unavailable during explicit reset or shutdown. Do not expose private operation IDs, source descriptors, lifecycle routes, or destructive actions publicly.

These additions are compatible in intent, but old validators using closed objects can reject new fields. Update built browser assets/mocks together; document that stale pages need reload. Do not promise arbitrary old consumers accept the extended object.

Retain generated validation and existing error model. Update examples and conformance tests; assert no full operational paths or private config are returned. All API writes stay within authorized local refresh semantics.

## HTTP binding and browser access

Service runtime owns listeners; `server.NewHandler` accepts DB/viewer defaults and application access only. Default initial selection remains month/day with no filters. Neither host nor port affects local control or TUI queries.

Bind default port exactly; any conflict fails without takeover or random fallback. Explicit zero permits dynamic selection. Use actual listener address for status and browser URL. Wildcard local URL uses loopback, with interface URLs shown as optional network candidates. No IPv6/DNS bind expansion.

Add Go cross-origin protection for public mutations; it checks browser request provenance without adding authentication. Loopback binding restricts Host to loopback names/addresses. Explicit wildcard/trusted-network binding supports direct IP or DNS access and must not demand a new authentication/hostname setup flow. Tests cover remote same-origin POST, hostile cross-origin POST, and Vite proxy Origin/Host forwarding. [Go HTTP protection](https://pkg.go.dev/net/http@go1.26.0#CrossOriginProtection).

No CORS access is added. Public routes are explicitly registered; private paths remain JSON not-found. Unknown SPA routes must not accidentally reveal control responses. Keep saved ingest hostname semantics unchanged.

Browser helper keeps current SSH/display checks. Invoke only from CLI `--open` after service readiness/reuse, never from daemon or foreground run. Failure prints URL and warning while leaving service running. It is an explicit convenience; default root invocation exits after status.

## TUI implementation

Launch Bubble Tea immediately, then issue an asynchronous ensure-and-refresh command alongside initial saved-data loading. This preserves visible startup progress while a daemon starts. Inject a small local service client at the model boundary; tests never accidentally launch a daemon from an ordinary model update.

Replace `syncCmd` and source-event channel consumption with messages for service connection, refresh receipt, shared status, operation completion, and observer failure. Rename `cancelSync` to observation cancellation. Existing source progress rows can be populated from durable status while preserving named recovery phases.

Use explicit state for viewer loading, refresh submission, refresh running/pending, analytics eligibility, and observation failure. Stop overloading `syncing` as both local goroutine ownership and a keyboard lock. `syncInFlight` no longer implies this TUI owns ingestion.

Compatible saved rows remain navigable during ordinary refresh. If no saved rows exist, keep progress, quit, and retry available. Recovery clears/hides analytics until ready. `r` reloads committed data; `u` submits one refresh demand and ensures service if needed. Never post refresh from status polling, redraw, resize, filtering, or sorting.

Use a one-second active status poll and five-second idle observation, plus existing animation only while visually active. One outstanding status read and one reload per current selection; use selection generation to discard old messages. Preserve the current invocation-time calendar anchor for this migration; daemon lifetime must not dictate TUI date filters. Automatic midnight rollover is a separate viewer change.

If ensure/refresh fails but compatible data exists, show data and retry feedback. If service disappears after acceptance, show interrupted/unknown observation without cancelling data reads. Explicit stop must remain stopped until `u` or another default TUI opening. On quit, report unresolved failure concisely where current exit behavior requires, but never send shared-operation cancellation.

`--no-sync` validates existing DB before launch and retains no-create/no-repair semantics. Default first opening now may initialize an empty DB after the TUI starts; update tests expecting local TUI-owned creation rather than preserving that old timing as a requirement.

## Web implementation

Keep TanStack Query as data owner and `useDashboardSync` as coordination boundary. No refresh POST in mount, reconnect, focus, filter, or route effects.

Query keys use instance/data epoch and durable revision, in addition to existing filter scope. On instance/epoch change cancel and clear old usage/facet/instance queries; same-scope placeholder data must never cross epoch. On ordinary revision change preserve existing same-scope placeholder behavior and invalidate bootstrap metadata. API responses with a different epoch than their request context are superseded, not displayable rows.

Retain cancellation-before-POST-publication behavior already protecting status reads. Scope the status query to the bootstrap instance; an old GET from a former process must not overwrite a receipt/status from the replacement. On reconnect, fetch instance and status before resuming analytics. Response identity validation is runtime typed validation, not `as` casts or non-null assertions.

Expose active, pending, and submitting separately. The Refresh action is disabled while POST is submitting or a follow-up is already pending; while an active run has no follow-up, it may enqueue one additional check. Display Refreshing / Refresh queued without offering cancellation. Reload data remains a read-only operation and can remain available during ordinary refresh.

Update `DashboardHeader`, `SyncCoverage`, failure/retry copy, and `App` eligibility. Empty first-start data explains Refresh; it must not say service is broken or invent checked-empty days. Use `checkRequestedAt` and durable progress for current-check confirmation, never browser wall clock.

Remove unconditional one-second analytics polling during active work if epoch/revision-driven invalidation covers publications. Status polls at one second while active/pending and five seconds idle; suspend intervals in hidden documents while keeping focus/reconnect read refresh. No event stream is required.

Update strict MSW response fixtures, POST coalescing simulation, controller race tests, snapshot-boundary tests, and browser labels together. UI architecture stays unchanged; no dashboard redesign or new authentication flow.

## Development and test isolation

`dev:server` runs foreground service against existing sanitized fixture DB. `dev:cli` remains `view --no-sync`. `start:web` builds and calls managed start with `--open`. Vite keeps its same-origin `/api` proxy.

Before `setup-dev-data.ts` removes `.tokeninsights-dev`, verify the fixture instance is stopped and no writer holds its lock. Stop only its verified TokenInsights service, wait for child exit, then recreate. Never remove a live lifetime/writer lock inode. Refuse recreation when ownership cannot be proven free. This guard is essential because the existing setup unconditionally removes the fixture directory.

Browser E2E launcher creates sources, explicitly prepares ordinary fixture analytics with a CLI sync, then starts foreground service. Dedicated first-start and explicit-refresh tests use separate empty instances rather than depending on startup ingestion. Root defaults changed from serve/view must not silently alter production data in tests.

Set isolated HOME, XDG config/state/runtime/data, Codex, Claude, DB path, and source roots for every subprocess test. Register cleanup immediately after successful spawn; stop/reap before directory deletion. Launch one test binary built once per integration suite. Use real loopback/Unix listeners, polling with deadlines, and synchronization channels instead of fixed sleeps. Run macOS and Linux ownership/detachment tests; compile every existing release target.

## Work packages and completion gates

| Package | Files and deliverable | Tests and gate |
| --- | --- | --- |
| 01 Source configuration parity | Pipeline types/resolution, four adapters, recovery, normalization/location dependencies | Exact IDs/fingerprints/fixtures unchanged; no schema or generation change |
| 02 Application actions and scheduling | New app actions/controller/status, context-aware resets, pipeline job identity | Deterministic capture races, bounded queue, cancellation, FIFO, reset barrier, durable outcome tests |
| 03 Filesystem and admission | dbpath extraction, service config/identity/locks, online/offline route admission | Symlinks/missing paths, hard-link rejection, private paths, read-only status, startup/write race |
| 04 Runtime and private control | Service manager/runtime/process/control/logging, server handler extraction | Real concurrent starts, readiness errors, detach/reap, stop/restart, stalled owner, port conflicts, request deduplication |
| 05 Public transport and web | OpenAPI/generated types, server snapshots, React controller/header/mocks | API conformance, old-response/reset races, no mount POST, pending refresh, empty/recovery/failure states |
| 06 CLI and local TUI migration | New commands, dispatcher/help/exit codes, browser helper, maintenance forwarding, TUI observation | Option matrix, live/offline summaries, confirmation, local-only host behavior, quit/demand ownership |
| 07 Development and release validation | Dev/E2E launchers, embedded assets, README/glossary/design/ADR and integration checks | Isolated browser/PTY flows, no runtime JS tools, release-target builds, stopped fixture recreation |

Packages 01–03 establish semantics. Package 04 integrates them; 05 and 06 consume the same controller/identity. Package 07 finalizes the user-visible migration. These are ordered review slices, not permission to ship partial command behavior. All changes can remain on one feature branch until the end-to-end contract passes.

Move or rewrite existing process-local server sync tests into application tests. Preserve SQL/query/coverage fixtures; rewrite only assertions whose ownership/startup behavior intentionally changes. Do not delete failing correctness tests or make them assert the old pipeline's mistakes. Add an ADR for explicit service lifetime/refresh ownership and update the stale implicit-view-sync glossary definition.

Each work package must update affected docs/tests/contracts with its code. New Go files need `gofmt` even before Git tracking: the root format script currently formats only `git ls-files` Go files. Run root formatting/lint before relevant verification; do not assume untracked source was formatted by that script.

## End to end acceptance matrix

| Scenario | Evidence required |
| --- | --- |
| Empty first start | Empty current DB; zero sync jobs/ingest rows; listeners ready |
| Repeated root/start | Same instance/PID/listener; no new sync jobs or source work |
| Web open/reconnect/filter | Reads only; explicit POST creates/queues check |
| Default TUI open/quit | One demand; local read works; service/refresh survives quit |
| Read-only TUI absent service | No DB/config/runtime/lock creation or normalization |
| Late source completion | Change already-checked source, submit demand, follow-up imports it once |
| Many demands and maintenance | One pending refresh; exclusive queue bounded; FIFO completion |
| Explicit reset with pending check | Pending check cancelled; no automatic reimport; old cache responses discarded |
| Caller-specific source roots | Online and offline results match; saved refresh roots untouched |
| Running external old writer | Wait then check desired sources; no falsely joined job |
| Wildcard web listener | Remote same-origin API works; TUI still reads local DB/socket; private admin routes unavailable over TCP |
| Startup race or dead parent | One owner; complete discoverable config or clean failure; no wait on unread pipes |
| Crash leaves socket/config artifacts | New owner safely removes only matching stale runtime files; status never claims config alone is live |
| Graceful stop versus crash | Cancelled/interrupted durable outcome; no lost commits or false freshness |
| Installed/daemon version skew | Compatible reuse; incompatible mutation fails; explicit restart adopts current binary |
| Historical compatibility recovery | Existing same-scope reset/resume rules preserved; analytics blocked until complete |
| Direct native binary | Start/status/refresh/stop and embedded dashboard work with JS runtimes absent |

## Verification and documentation

After code changes: root `pnpm run format`, `pnpm run lint`, focused package tests, `pnpm run check-api`, `pnpm run check-schema`, `pnpm run test`, affected web E2E, then `pnpm run build`. Add focused Go race-detector runs for app/service concurrency. Broaden/repeat only for failures or new changes.

Build directly from `packages/cli` against committed generated/static assets. Run service lifecycle and fixture refresh with Node/npm/pnpm absent from PATH; keep any tools the existing location resolver actually requires separate from the JS-runtime test. Verify the built project-local binary `./packages/cli/bin/tokeninsights` in a PTY using isolated fixture paths for TUI behavior. No unrelated terminal launchers are needed.

### Scale and resource measurements

Use reproducible synthetic sources and record fixture size, runtime/build, elapsed time, bytes read, allocations, RSS, open descriptors, and SQLite/operational-table sizes. Compare before/after on the same fixtures; results become release evidence, not invented latency guarantees.

- Warm repeated start/status and browser reads must produce zero source discovery, new sync jobs, or normalized writes. Measure cold startup separately from unchanged refresh.
- Exercise existing analytics at 10,000 and 100,000 sessions across harnesses, with concurrent tabs and TUI reads during refresh. Measure the Go aggregation-before-pagination cost and cancellation responsiveness. Keep SQL pagination/index changes as measured follow-up work requiring the existing schema approval process where applicable.
- Submit thousands of refresh requests during a paused source capture. Assert fixed active/pending work, bounded request aliases/outcomes/exclusive queue, and bounded logs. After several large refreshes, confirm source handles, parse/ancestry caches, goroutines, and query transactions return to idle ownership. Do not retain per-job source snapshots in the daemon.
- Confirm idle service performs no periodic pipeline work. UI status reads remain bounded; record sleep/resume behavior and stop latency under active parsing and blocked writers.
- Report unchanged refresh I/O honestly: persistent service removes unwanted sync and startup, but existing continuity/content verification can still scale with historical source bytes. Do not weaken those proofs to make a benchmark faster.

Update README command examples; design architecture/lifecycle/API sections; glossary implicit refresh terms; public API examples/generated models; dev instructions; and CLI migration guidance. Explicitly document saved versus caller sources, pending request loss on daemon crash, reset barriers, IPv4 trusted-network scope, local-only TUI, and deferred auth/autostart.

Implementation and validation evidence are tracked in VALIDATION.md. No SQLite schema or token-semantics changes are required.

## Unresolved questions

None. The operational limits above are proposed initial constants to validate, not missing product decisions. Schema approval remains required only if implementation evidence proves a storage contract change necessary.
