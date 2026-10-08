# Runtime compositions

Status: complete

## Authorization and decisions

User approved this architecture and schema changes on 8 October 2026. Implement
after planning; no additional approval gate. Preserve accounting and retained
history; schema-breaking permission does not authorize deleting user data.

- Single-process and distributed compositions share interpretation and queries.
- Existing durable DuckDB processing queue is sufficient in both compositions.
- One local viewer per database; one remote server container.
- Storage access uses semantic contracts and adapters for future replacement.
- Distributed sync defaults to finite background execution. `--print` also
  submits and writes only the remote URL to stdout. Job diagnostics use stderr.
- Plugins initially provide only a harness selector. No event payload/counters.
- SQLite owns application accounts; DuckDB owns evidence and analytical data.
- No collector daemon, external broker, replicas, or automatic source watcher.

## Command contract

`mode` is `single-process` (default) or `distributed`. Existing hosted config maps
to distributed; existing personal config without a URL maps to single-process.
Explicit remote configuration never falls back locally. Distributed requires URL
and token; the server requires authenticated hosted policy and canonical HTTPS
origin. IP destinations work with valid TLS/trusted certificates.

- Bare invocation: help; never starts a service.
- `tui`: local only; acquire runtime, collect, directly accept, await visible
  processing, then query directly. Quit cancels and joins work and releases stores.
- `web`: local collects/processes then starts foreground web serving; browser
  failure prints URL and leaves listener usable. Distributed synchronously submits
  and opens remote dashboard; never starts a local listener.
- `web --host 0.0.0.0`: local listener binding; browser opens loopback. Public
  routes are dashboard/query only. `--host/--port` reject distributed mode.
- `tui/web --sync=false`: saved data; still command-owned local lifetime.
- Local `sync`: finite collect/accept/process; no HTTP listener.
- Distributed `sync`: durable invocation + detached finite worker. Return after
  startup acknowledgement, not remote acceptance. No false success claim.
- `sync --print`: same job submission, remote URL on stdout; local rejects.
- `sync --wait`: foreground delivery; usable in scripts, exit reflects acceptance.
- `sync --debug`: foreground bounded terminal progress including receipt processing;
  non-TTY fallback uses plain text. This is not the remote analytics TUI.
- `sync status`: latest durable invocation state without starting work.
- `config set/get/remove`: mode, URL/token, bind preferences, role-specific paths;
  current precedence and private atomic home config remain.
- Server executable: authenticated foreground runtime supervised by Docker; no
  collection. Existing private operator socket remains for administration.

## Invariants

Keep five token components, stable session/contribution identities, independent
equal-valued requests, source snapshot precedence, dataset isolation, ancestry,
confirmed/estimated separation, native ambiguity, and TPS concepts unchanged.
Never infer identity from delivery IDs, job IDs, equal counters, or payload hash.

Capture and source checkpoint commit together. Persist exact request bytes before
submission; acknowledge only a matching durable receipt. Acceptance atomically
commits evidence, mappings, receipt, dependency/scope work. Projection atomically
fences generation/component/revisions and publishes facts, estimates, provenance,
outcomes. Late context invalidates dependent outcomes. Restart resumes pending work.

Direct and HTTP adapters use identical batch validation and acceptance operations.
Direct execution does not bypass bounds, dataset checks, receipts, or replay.
Local delivery identity uses durable database/dataset identity, independent of port.
Retained protocol 1/2 requests finish unchanged before new protocol 3 delivery.

## Contracts and package changes

1. Collector `Delivery` port: negotiate, submit immutable bytes, legacy submission,
   receipt lookup. HTTP adapter encapsulates network/auth/error mapping. Direct
   adapter wraps an authorized data-store handle; no HTTP server/roundtripper.
2. Query port: instance/status/usage/facets with existing typed response semantics;
   TUI receives a query implementation. Shared query application service feeds
   direct TUI and REST; SQL stays within the DuckDB adapter.
3. Data store port: scoped acceptance/receipt, generation/status, work loading,
   fenced publication, retry recording, query snapshots, readiness and close.
   Preserve semantic operations; do not invent generic CRUD/transaction callbacks
   as a claim that all backends are interchangeable.
4. Runtime: lifetime ownership and worker cancellation independent of listeners.
   Local composition owns collector + application + token stores. Remote owns
   application + token stores + HTTP/admin adapters. Join before closing stores.
5. Accounts: semantic repository for provisioning, token/session lifecycle and
   authentication; SQLite adapter. No DuckDB SQL in account service.
6. Invocation tracking: bounded local durable jobs, atomic claim, status, failure
   codes, retry/resumption. Configuration secrets never enter job/log payloads.

## Implementation tracking

Issues live in [issues/](issues/), in dependency order. Stage 01 is the initial
reviewable implementation: transport/query adapters and listener-free runtime.
Public command migration and schema changes follow in separate stages; do not
claim the redesigned CLI is available until those stages pass their gates.

## Storage and migration

Retain `collector.sqlite` for continuity/outbox. Introduce role-validated
`app.sqlite` beside `server.duckdb` for accounts and provisioning. Invocation state
uses a separate role-validated `<collector-db-path>.jobs.sqlite`, keeping enqueue
independent of the collector delivery writer lock and token acceptance transactions. Keep
raw evidence, receipts, work/retries, generations and provenance in DuckDB.

Move existing account rows by verified, resumable copy preserving user/dataset IDs,
token/session digests, revocation and expiry. Bind application storage to token
database identity; reject mismatched restoration. Default local user owns existing
`default` dataset. Do not change raw/read protocols solely for storage changes.

Provisioning: persist inactive user + fixed dataset ID, idempotently create token
dataset, activate account. Restart resumes. Authentication denies inactive users.
No cross-engine transaction claim. Coordinated stopped backup preserves both stores.
Newer incompatible schema rejects without mutation; retained old copies support
rollback with matching binary. Keep current legacy import and replay coverage.

## Ownership and request handoff

Use OS-backed lifetime locks. One local viewer owns writable token storage. A
standalone sync owns it when available. If a viewer owns it, persist a request in
local operational SQLite; owner executes collection/acceptance inside its process.
No HTTP ingestion or second DuckDB opener. Owner services explicit queued requests
only, without scanning sources periodically. On exit unclaimed work remains.

Requests retain resolved mode, endpoint/dataset expectation, source selector and
safe options. Credential material goes through private inherited configuration,
never argv/log/job fields. Recovered remote work must not silently retarget to a
different account or endpoint after config edits.

Coalesce identical concurrent demands but retain a follow-up when a new event
arrives after an active scan began. Do not lose distinct harness requests. Lock
ordering, owner startup/handover and stale jobs require real concurrency tests.

## Completion and failures

Local viewing waits for its accepted inputs to be query-visible, including target
generation activation. Durable unusable/ambiguous outcomes are terminal diagnostics.
Timeout/failure permits saved-data viewing; never silently claim current totals.
Distributed wait ends at acceptance; debug observes relevant receipt processing,
never global queue emptiness. Preserve dataset and generation snapshot checks.

Background state: queued -> running -> accepted / failed / interrupted. Completion
does not imply every input counted. Bounded transient retries honor Retry-After;
auth/identity/privacy/version failures stop. Worker exit is finite; later explicit
invocation resumes interrupted work. No reboot/eventual-delivery promise without
another invocation. Startup handshake has a deadline; child owns detached stdio.

## Implementation sequence and gates

1. Extract delivery, query, storage and runtime contracts; retain existing behavior.
   Gate: direct/HTTP parity and existing immutable-replay tests.
2. Add local runtime and explicit mode/config. Switch TUI/web/sync to direct local
   composition; foreground lifecycle and listener isolation.
   Gate: no local ingestion HTTP; ownership, cancellation and freshness tests.
3. Add application SQLite/account migration and provisioning recovery.
   Gate: existing hosted auth/isolation plus migration/restart/mismatch tests.
4. Add finite background sync, status/debug/wait/print and owner handoff.
   Gate: process startup failures, durable job recovery, concurrent demands,
   exact stdout contract and credentials absent from persisted jobs/logs.
5. Update minimal plugin selectors and coalescing; Docker/auth defaults and tooling.
   Gate: plugin contracts, authenticated remote smoke and native runtime checks.
6. Update README, design, glossary, ADR, CLI help and deployment; retire obsolete
   public service paths while preserving required import/maintenance adapters.
   Gate: format, lint, schema/API checks, focused semantic tests, full/race tests,
   build, web checks as applicable, native binary with JS tools absent from PATH.

## Verification matrix

- Direct vs HTTP: exact identities/components, receipts, errors, replay/duplicates.
- Crash boundaries: capture checkpoint, request save, accepted response lost,
  projection publication, invocation claim and parent/child startup handshake.
- History: source disappearance, collector deletion, late parent, ambiguity,
  conflicting revisions, generation rebuild/restart, retained legacy requests.
- Isolation: colliding native/delivery IDs across users; token rotation/account
  switch; revoked users/tokens/sessions; restored app/data mismatch.
- Local: one owner, no ingestion listener, foreground web shutdown, TUI direct
  queries, wildcard bind/browser URL, queued plugin request during viewer lifetime.
- Distributed: mandatory auth, background stdout/stderr, print also submits,
  foreground debug/wait, durable error status, finite retry, no local fallback.
- Migration: interrupted account copy/provisioning, default user, identities and
  exact receipts preserved; verified backup and incompatible version rejection.
- Production: committed browser assets, native Go binaries and single container;
  no runtime Node/npm/pnpm dependency.

## Validation and current implementation

Stages 01–06 implemented on `codex/runtime-compositions`:
- Direct/HTTP delivery, shared queries, unchanged processing and identity contracts.
- Command-owned local TUI/web/sync; read-only foreground web, no ingestion HTTP.
- Paired SQLite accounts, one-time legacy migration, restartable provisioning.
- Durable finite jobs, private native startup handshake, bounded retries and local handoff.
- Background sync/print/wait/debug/status; harness-specific supervised plugins.
- Authenticated container defaults; finite data maintenance and old-service stop migration.

Application pairing includes a token-side `.application.json` guard containing the
application instance ID. It prevents a deleted/replaced SQLite database from silently
re-importing stale legacy credentials. Back up this guard with both databases.
Local visibility waits include existing pending work/generation recovery; distributed
debug polls only its saved receipts. Existing DuckDB processing queue is retained.

Validation completed:
- Root `pnpm run format`, `pnpm run lint`, `pnpm run test`, `pnpm run build`.
- Schema/API generation consistency; native Go and committed browser/plugin builds.
- Focused race tests: accounts, appstore, localruntime, syncjob, collector, server, CLI.
- Browser E2E: 26 passed, including foreground web shutdown, GET-only Reload,
  authenticated sessions, tenant isolation and revocation.
- Docker build and smoke: required auth, five-component ingestion, background
  `--print` submission/status, credential revocation across restart, no Node/npm/pnpm.
- Direct Go build and native CLI execution with JavaScript tools absent from PATH.
- Fixture setup twice; native TUI showed 896 total (640 input, 146 output,
  26 reasoning, 71 cache read, 13 cache write) and exited cleanly.
- Relative application paths including URI-special characters retain pairing and
  saved data across reopen. Final regression and full suite passed after the fix.
- `git diff --check` clean. Native all-harness oracles remain 12 contributions /
  1102 tokens across all five components; replay/rebuild do not inflate totals.

Completed invocation history retains the latest 1,000 terminal jobs; queued/running
work is never pruned. No daemon, external broker or additional server replica added.
Implementation and validation completed on `codex/runtime-compositions`; PR creation
authorized by the user after completion.

## Unresolved questions

None. Existing queue and one viewer/container approved. Minimal plugin harness
selector is the initial contract; richer evidence and alternate storage deferred.
