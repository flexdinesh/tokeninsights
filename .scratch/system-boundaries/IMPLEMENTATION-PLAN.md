# System boundaries: implementation plan

Status: Complete. Schema/wire contracts explicitly approved 7 October 2026. Validation recorded in [EXECUTION.md](EXECUTION.md).
Parallel implementation/integration underway; completion awaits the execution
verification gates. See [EXECUTION.md](EXECUTION.md) for current ownership/status.
Date: 7 October 2026.

## Accepted product decisions

- **Personal server**: unauthenticated, one dataset; default client/server kind. May run as the managed local service or an explicit foreground server. TUI supported.
- **Hosted server**: authenticated, multiple users; one instance and one shared DuckDB database. Each user owns one isolated dataset. TUI unsupported.
- `tokeninsights web` syncs by default, then opens the dashboard. `--sync=false` skips collection/submission.
- Hosted `web` supports terminal sync progress and opens the hosted dashboard. Hosted browser UI never shows collector progress.
- Hosted users/tokens are administrator-created. Browser login exchanges a read-capable token for an authenticated session. No public signup/password/OIDC flow in this release.
- Collector retains SQLite. No JSONL rewrite in this implementation.
- Server startup never discovers sources or collects. CLI workflows own collection; plugins keep invoking the same Go `sync`.
- Evidence/acceptance receipts remain immutable; facts/estimates remain replaceable, versioned projections.

This plan replaces the earlier proposed database-per-tenant option. No per-user database files.

## Current seams and target ownership

Existing code already provides durable raw acceptance, idempotent delivery, deterministic interpretation, asynchronous projection, REST analytics and embedded assets. Preserve those semantic contracts while extracting boundaries.

| Current seam | Change |
| --- | --- |
| `pipeline/process_evidence.go` invokes adapters that also discover/read host files | Extract pure interpretation and its dependencies into `processor` |
| `pipeline/extract.go` exposes SQLite transactions throughout capture | Put atomic capture/checkpoint behind collector-state operations |
| `collector/raw.go` discovers local service and builds Unix transport | Resolve destination in client composition; delivery receives transport |
| `datastore/store.go` owns SQL, processing, queue, admission | Separate engine orchestration from DuckDB implementation; retain atomic operations |
| `datastore/http.go` mounts ingestion, legacy and admin together | Separate ingestion/query/admin HTTP groups, each capability/authorization gated |
| `server/duck_queries.go` contains analytics SQL | Move transport-independent queries/results into analytics adapter |
| `service/runtime.go` and `remoteserver/server.go` duplicate lifecycle | Shared server runtime; local discovery/control remain a wrapper |
| `server` contains current and SQLite legacy query paths | Isolate legacy compatibility; current runtime never chooses backend via nil-store fallback |
| `config` imports HTTP `server` for network defaults/validation | Move network preferences into a small dependency shared by config/runtime |
| `CONTEXT.md` still describes canonical-only ingestion | Reconcile glossary, design, README, architecture references |

Keep the existing Go module and pnpm workspace. These are Go packages, not independent services or new modules. Change imports/call sites incrementally; avoid a broad directory-only rewrite.

Implementation packages under `packages/cli/internal/` (retain existing names
where they already provide the behavior boundary):

| Package | Responsibility and boundary |
| --- | --- |
| `evidence` | Whitelist, sanitized source-shaped records, stable observation helpers; no CLI/filesystem/storage |
| `publication` | Existing canonical contribution types/identity; no host discovery |
| `processor` | Deterministic `Process(records) -> projection`; no SQL, filesystem, Git, networking or wall-clock reads |
| `pipeline` | Capture: source discovery, consistent native reads, extraction/context, continuity verification, safe location enrichment; legacy normalization delegates pure helpers |
| `rawcollectorstore` | Collector state: capture/checkpoint transaction, retained outbox, immutable prepared requests, dataset-bound receipts |
| `collector` | Delivery orchestration: protocol negotiation, submission, receipt checks, fixed failure codes; injected transport/state, no service discovery |
| `clientworkflow` | Destination resolution; sync/view startup; progress fan-out; browser launch delegated to existing browser helper |
| `serverfeatures` | Server kinds, typed capability set, validated feature policy |
| `dataengine` | Transport-independent worker orchestration, processing and retry scheduling against atomic adapter operations; no HTTP/SQL |
| `datastore` | DuckDB adapter: SQL, contract validation/upgrades/import, write coordinator and dataset-scoped atomic operations |
| `analytics` | Query/filter/result contracts; DuckDB queries independent of HTTP |
| `server`, `ingestionhttp` | Thin query/asset and ingestion REST adapters; explicit route groups |
| `serverruntime` | Open/own engine, workers, listeners, readiness, graceful shutdown |
| `accounts` | Hosted users, tokens, browser sessions and principal resolution |
| `service`, `remoteserver` | Managed personal and foreground personal/hosted composition roots |
| Existing explicit legacy adapters | Retained protocol-1 requests/receipts, legacy import and maintenance compatibility in their current packages; no directory-only relocation |

Use small interfaces at consumers. Introduce each when needed; no generic backend/plugin framework. DuckDB remains the only production data adapter. A future backend must implement semantic atomic operations, not arbitrary CRUD.

Capture operation: validate continuity, append observations, save next checkpoint/context in one transaction. Prefer a source capture session with explicit commit/rollback over leaking `*sql.Tx`.

Server operations express acceptance, receipt lookup, work loading, projection
publication, snapshot reads and reprocessing. Existing `datastore` operation names
remain where appropriate; `dataengine` coordinates worker execution against its
adapter boundary. Contracts include dataset scope, exact request identity,
revision fences and atomic state changes. HTTP never reopens database paths.

## Server kinds, configuration and capabilities

Client config adds `server-kind: personal|hosted`, default `personal`, and `server-token`. Keep `server-url`, `host`, `port`, role-specific DB paths and existing XDG path.

Example hosted client preferences:

```json
{
  "server-kind": "hosted",
  "server-url": "https://usage.example.com",
  "server-token": "<user-access-token>"
}
```

`host` is local bind address; `server-url` is destination. No token in URLs. Existing precedence stays flags > environment > file > defaults. Add `TOKENINSIGHTS_SERVER_KIND` and `TOKENINSIGHTS_ACCESS_TOKEN`; do not reuse removed `TOKENINSIGHTS_SERVER_TOKEN` server authentication behavior. No token CLI flag; support secure interactive/stdin entry through `config set server-token` while retaining file configuration. Configuration writes remain atomic/mode-0600; `config get server-token` reports configured/unset, never the secret. Validate hosted URL/token together at runtime, allowing individual config-set operations while setup is incomplete.

Hosted client configuration requires URL and token. Token with an empty/local destination is invalid, preventing credentials reaching a local listener. Personal endpoints may be remote; `server-kind` is deployment policy, not a test for localhost.

Client kind is an expectation, never an authority. Compare it with the server descriptor. Mismatch fails before collection/viewer launch, with a fixed actionable error and no automatic fallback. Configuring hosted rejects TUI immediately, including `--sync=false`; remote personal TUI also requires actual terminal-dashboard capability.

Foreground executable adds `--kind personal|hosted`, default personal. Existing foreground invocation remains valid. Hosted requires shared data path and canonical HTTPS public URL, normally behind a TLS proxy. Client preferences never enable hosted mode in the local daemon.

Create typed `ServerKind`, `Capability`, `Capabilities`, and `FeaturePolicy` primitives. Profile defaults plus available components determine capabilities; optional server-side disables can remove features. Validation prohibits hosted terminal dashboard or collector progress, and prohibits disabling hosted authentication/user isolation. Commands cannot enable server features through client config.

| Capability | Managed personal | Foreground personal | Hosted |
| --- | --- | --- | --- |
| `usage`, `facets`, `web-dashboard` | Yes | Yes | Yes, authenticated |
| `raw-ingestion` | Private Unix transport | HTTP | HTTP bearer token |
| `terminal-dashboard` | Yes | Yes | No |
| `collector-progress` | Yes | No: no local publisher transport | No |
| `reprocess` | Private admin | Operator admin | Operator admin |

`collector-progress` means server-observable host capture/submission progress. Processing lag is separate and remains available to all authorized viewers. Foreground personal CLI still displays its own progress.

Use the same capability policy for route mounting, handler guards, command preflight and browser feature selection. Validate dependencies: terminal/web dashboards need usage/facets; collector progress needs its publisher/read components. Reprocess never follows implicitly from raw-ingestion permission. Personal public queries stay available with `--host 0.0.0.0`; public local writes/admin remain absent.

Expose a new `GET /api/v2/instance` descriptor, preserving `/api/v1/instance` for personal compatibility. Include kind, capabilities, database/dataset identity, instance identity, readiness, reporting timezone and viewer defaults. Hosted descriptor is available to any valid user token/session, including ingest-only tokens, and returns the caller's permissions separately from supported features. `sync` requires ingest permission; query viewers require read permission. Login shell/static assets and minimal health endpoints are public. Request transport and principal scopes still authorize each operation. A missing new descriptor yields an upgrade-required error; do not infer personal kind from an older server's hostname or successful query.

Wire capabilities are bounded string identifiers with a typed known registry. Ignore unknown names; never infer a missing capability. Unknown kinds reject. This avoids the current strict generated capability enum preventing additive feature evolution. OpenAPI remains the source of generated Go/TypeScript contracts.

## Shared database and user isolation

One server process, one shared `server.duckdb`, one global write coordinator and one processing worker initially. Add an `accounts` logical schema in the same file; no second account database in this release.

Keep native evidence/fact identities unchanged within a dataset. Use composite database keys with `dataset_id`; do not add user identity to source-native hashing. Identical native records dedupe across a user's collectors; two users with identical records remain independent.

An authenticated `Principal` carries user ID, dataset ID and token/session scopes. Dataset comes from server accounts, never from a query parameter or unverified header. A required scoped engine handle makes omission difficult: there is no public unscoped query/receipt/processing API.

Every acceptance lookup, outcome, parent dependency, recursive component traversal, revision fence, projection deletion/insertion, legacy coverage check, generation activation, facet, summary and page is scoped. Bind dataset values as SQL parameters. Joins match dataset on both sides. Tests must use colliding identities, not just different user names.

Snapshot identity becomes instance/database/dataset/generation/published revision. Browser query keys include it. Clear all user-specific query state on login/logout/account change, including in-flight requests and placeholder results. Return user-scoped pending counts, revisions and hostname labels; do not expose other users' activity through shared metadata.

Processing selects eligible work fairly across datasets; one busy/failing user must not indefinitely starve another. Dependencies cannot connect datasets. Generation builds/activation are per dataset; new data for user A cannot delay user B's cutover. Keep one compute worker; do not add an external queue/backend.

## Approved schema and wire contract

The user explicitly approved these schema/wire contracts on 7 October 2026 before
implementation. Versions: collector 18, DuckDB 2, raw protocol 3, read API v2;
extractor/processor versions stay unchanged unless interpretation semantics change.
Any further contract change outside this approval still requires explicit approval.

### DuckDB 2

- Global server metadata is `ingestion.instance` (role/version/database/kind); dataset metadata remains `ingestion.metadata` (dataset ID, active/target generations, input/published revisions, timestamps).
- Add `accounts.users` (opaque user ID, display name, active status, creation time), `accounts.tokens` (token ID/user ID, unique digest, scopes, expiry/revocation), and `accounts.sessions` (unique session digest/user ID/source token ID, expiry/revocation). Enforce token/session ownership and one dataset per hosted user; personal dataset remains `default` with no account owner.
- `raw.evidence`: key `(dataset_id,evidence_id)`; scope index includes dataset.
- `ingestion.batches` and legacy receipts: key `(dataset_id,stream_id,batch_id)`; preserve exact request/receipt bytes.
- `ingestion.items`: key `(dataset_id,stream_id,sequence)`.
- `ingestion.batch_items`: key `(dataset_id,stream_id,batch_id,sequence)`; evidence index includes dataset.
- `processing.scopes`: key `(dataset_id,scope)`.
- `processing.dependencies`: key `(dataset_id,child,parent)`.
- `processing.outcomes`: key `(dataset_id,generation,evidence_id)`.
- `analytics.generations`: key `(dataset_id,generation)`.
- Facts/estimates: key `(dataset_id,generation,fact_id)`.
- Provenance: key `(dataset_id,generation,fact_id,evidence_id)`.
- Legacy baseline: key `(dataset_id,fact_id)`; coverage: `(dataset_id,generation,fact_id)`.
- Confirmed/estimated views join dataset metadata on dataset and active generation. They expose dataset columns; scoped analytics queries must still filter them explicitly.

Use opaque, server-generated IDs and validated dataset ownership. Token/session records are authentication state, not source evidence; raw privacy whitelist remains unchanged.

### Collector 18

Add dataset ID to raw destinations and saved batches; saved batches also retain protocol version. Existing rows migrate to `default`/protocol 2 without changing request bytes, hashes, stream IDs, sequences, receipt bytes or cursors. Extend request immutability protections to binding/version columns.

New destination identity includes canonical endpoint and authenticated dataset; local identity also includes database ID for deliberate retained-history replay. Reuse verified migrated default bindings rather than creating a new cursor. A database replacement at an existing remote binding still rejects. Token rotation retains identity; switching users creates independent delivery progress. Dataset is operational delivery metadata, not copied into captured source records.

### Raw protocol 3

Add `/api/v3/ingestion/capabilities`, batches and receipt lookup. Batch envelope explicitly binds `databaseId` and `datasetId`; receipt validates both against the saved request and authenticated destination. Keep extractor version/whitelist, contiguous sequence rules, limits, acceptance-only completion and processing dispositions.

Hosted accepts protocol 3 only. Mismatched dataset versus authenticated principal rejects before mutation. Receipt lookups are scoped to principal; foreign receipts return not found. A bearer token cannot choose another dataset through request bytes.

Personal keeps protocol 1/2 adapters for saved requests; new clients use protocol 3. Existing saved requests replay via their original adapter and bytes before preparing newer batches for that destination. Legacy adapters are never mounted on hosted HTTP. Retained personal legacy batches stay with their existing destination and are never reinterpreted as hosted batches. Legacy-only history whose sources vanished needs a separately specified import, not automatic transfer.

### Read API compatibility

Keep personal v1 read responses unchanged for existing clients. Add v2 instance/status/usage/facets responses with dataset-aware snapshot identity. The analytics service is shared; version adapters translate envelopes. Hosted exposes authenticated v2 reads only. New CLI/browser use v2. Existing v1 instance/sync structs are strict, so do not silently add required fields to them.

Separate `GET /api/v2/status` (readiness, generation, input/published revisions, processing lag) from `GET /api/v2/collector-progress` (personal local capture/submission). Hosted mounts no collector-progress endpoint and returns 404 there. Keep v1 sync status as a personal compatibility adapter.

### Upgrade safety

- Verified DuckDB 1 upgrades to personal DuckDB 2, assigning all rows/generations to dataset `default`; preserve database identity, IDs, five components/totals, exact receipts/requests and interrupted processing.
- Build a staged new file from a stopped, read-only source. Checkpoint it, verify counts/components/identities/receipts, then atomically publish while retaining a recoverable old file. Recovery after interruption must select a fully verified source/target; never overwrite the sole recoverable copy.
- Hosted starts with a fresh schema-2 database. Reject opening personal data as hosted or hosted data as personal. Automatic assignment of existing shared history to a hosted user is out of scope.
- Preserve SQLite legacy import behavior and source bytes; import into personal dataset only. Legacy `schema/server.sql` format/version stays unchanged.
- Collector 17 upgrade is transactional/additive; retain verified 16-to-17 bridge before upgrading to 18. Update inspect/role constants, compatible-version rules and schema-copy checks together.
- Downgrade rejects without mutation. Rollback uses retained verified files; do not claim older binaries can read new schemas.

## CLI workflows and progress

Common workflow: resolve preferences -> ensure local service when personal/local -> authenticate/negotiate descriptor -> enforce kind/capabilities -> capture -> submit -> open viewer. `sync --dry-run` remains source-only and needs no reachable server. `--publish-only` bypasses capture. Source errors still permit submitting already committed outbox data; report both stages.

| Command | Personal | Hosted |
| --- | --- | --- |
| Bare invocation | Ensure/report local, or report configured personal URL | Report configured endpoint; never start local |
| `sync` | Capture/submit | Same collector/transport contract with bearer |
| `tui` | Sync/loading screen then REST dashboard | Usage error before collection, including query-only mode |
| `web` | Open browser early enough to show personal progress; run initial sync with terminal progress | Sync with terminal progress; open hosted dashboard/login; no browser collector progress |
| `web --sync=false` | Open saved dashboard | Open hosted dashboard/login |
| `service ...` | Explicit local lifecycle/maintenance | Still local administration; never routes to hosted |

For default personal `web`, ensure and validate server, begin a progress attempt, open dashboard, then collect/submit. For hosted `web`, finish the terminal sync attempt before opening; do not transfer the CLI token to browser URLs. If sync fails, preserve evidence, report failure and still offer/open saved dashboard. Browser launch failure prints usable URL; a successful sync stays successful. Headless/non-TTY progress uses concise lines.

`web --host` controls managed local binding. If already running with another explicit binding, preserve current policy: explain `service restart`; do not silently restart or kill it. Hosted `web --host` is invalid. Query-only flags never bypass kind/capability checks.

TUI and web presenters consume the same semantic progress stream from clientworkflow. Remove obsolete `Normalize`/`--no-normalize` coupling from new capture options; keep its CLI compatibility handling explicit until documented removal.

TUI queries available saved data after durable acceptance. While its accepted work is pending, display processing state and refresh reads. This does not change `sync` into a projection wait or wait for the entire hosted queue. Web polling similarly reacts to published revision; Reload remains query-only.

### Personal browser progress transport

The server does not collect. CLI workflows publish sanitized progress to a dedicated private control route; runtime stores an in-memory attempt registry. Public personal GET exposes only bounded sanitized state.

Attempt state: opaque ID, stage, per-harness status, acknowledged batches/entries, known pending count, fixed error code and lease timestamps. No source paths, source JSON, prompt/tool text, endpoint credentials or exception text. Never invent total percentages.

Begin/update/heartbeat/finish messages are instance-verified on the existing mode-0600 socket. Heartbeats use the existing five-second status cadence; lease expires after three missed intervals. Crash/cancellation marks interrupted, never successful. Bound registry size and terminal-state retention with named constants; attempts from concurrent commands remain separate. A contender may report waiting without overwriting another attempt's counts.

TUI, `web` and manual/plugin `sync` use the same publisher when a managed personal destination advertises this capability. Progress reporting is ancillary: reporting failure cannot invalidate accepted delivery or prevent capture. Hosted composition never mounts publisher/read routes or initializes the registry. Remote foreground personal clients show terminal progress only.

Browser mounts/polls collector-progress only when capability present. Do not hide hosted markup while continuing to poll. Processing lag/estimates remain common features. Cancellation of an old attempt or a replaced service cannot update the new instance's registry.

## Hosted access and administration

Administrator operations use a private mode-0600 Unix socket. Add foreground executable subcommands for user creation/disable and token creation/revocation, operating through the running owner process. A second CLI process never opens the writable DuckDB file. Print newly created raw tokens once; subsequent reads expose only IDs/scopes/status.

Random bearer tokens contain 256 bits of entropy; store digests, not plaintext. Initial token scopes are `read` and `ingest`; client token generally has both. Users can hold multiple tokens; datasets persist across rotation/revocation. User disable blocks tokens/sessions; source-token revocation invalidates sessions issued from it. Authentication failures expose fixed errors without credentials.

Browser token login uses a same-origin POST exchange and opaque server-side session. Session cookie is HttpOnly, Secure, SameSite=Lax, host-only; no bearer persistence in browser localStorage or query strings. Initial session lifetime is 24 hours; logout revokes it. Validate canonical public origin for state-changing requests and retain Go cross-origin protection. Login/static assets are public; authenticated API failures lead to login, not an endless connection retry screen. Browser sessions have read permission and cannot ingest or administer.

All hosted API route groups resolve principal before data access, including capabilities, receipts and status. Descriptor is authenticated metadata; analytics/receipt/status reads require read permission, batches require ingest. No global v1 fallback or personal private transport bypass. Avoid accepting untrusted proxy headers as identity/origin; canonical public URL supplies expected external origin.

Use bounded, named-config admission policies: existing four global ingestion slots, at most one active acceptance per hosted user, and a per-user batch token bucket (default 10 requests/second, burst 20). Login attempts are bounded per client address (default five/minute). Enforce before body decoding/expensive work. Return 429 with Retry-After for rate limits, 503 for global busy; keep exact requests unacknowledged for subsequent manual retry. Fair worker selection and read deadlines limit cross-user interference. No billing, organizations, user self-service, external broker or horizontal scaling.

Auth/account writes use the shared coordinator; creating user/dataset commits together. Auth reads fail closed when storage unavailable. Runtime manages token/session cleanup without deleting users' evidence/history.

## Deployment and Docker

Keep `tokeninsights-server` as the deployable foreground binary for either kind. It hosts the same assets/API/engine as managed local composition; never depends on CLI viewer/source packages.

Add a multi-stage Dockerfile rooted at the repo, using pinned compatible Go/C/C++ build tooling. Build from committed embedded assets; Node/pnpm are not runtime dependencies. Choose a glibc-compatible non-root runtime and verify actual dynamic dependencies of the DuckDB binary. Include CA certificates and timezone data. Pin image versions/digests during implementation using current primary documentation.

Mount shared data at `/data`; private admin socket under writable `/run/tokeninsights`. One instance owns each database. SIGTERM stops HTTP, joins processor and closes/checkpoints storage. `/healthz` reports process liveness; `/readyz` verifies initialized storage/auth/routes. Pending processing alone does not make readiness fail. Health responses reveal no user data.

Add a Compose example for personal and a hosted example with canonical HTTPS URL/TLS proxy instructions. Document admin provisioning using the private socket inside the container. Do not publish an external admin listener or silently deploy an image/site. Document volume ownership, upgrade recovery files and backup of stopped/checkpointed data.

## Implementation slices and dependencies

| Slice | Scope | Acceptance evidence |
| --- | --- | --- |
| 01 Boundary contract/docs | Record personal/hosted glossary, capability policy, invariants and approval proposal; ADR amendment | Product matrix and schema/wire decisions reviewed |
| 02 Pure processor | Extract interpretation/helpers out of mixed adapters; legacy normalization reuses shared pure helpers | Existing native goldens unchanged; dependency check excludes storage/host/network |
| 03 Capture/delivery seams | Introduce source capture transaction/state, injected destination transport and common progress events | Incremental/rewrite/partial-tail, lost receipt, collection-error/submission tests pass |
| 04 Analytics/HTTP/runtime | Scoped interface scaffolding for personal/default; extract SQL/services/route groups and lifecycle | Local/foreground semantic equivalence; private public policy; worker teardown/ownership tests |
| 05 Capability discovery/config | Kind/feature primitives, v2 descriptor, endpoint negotiation, expected-kind/token resolution, command guards | Hosted TUI rejection before capture; disabled route/feature tests; config precedence |
| 06 Approved schema upgrades | DuckDB 2, collector 18, staged upgrade/inspect/import, dataset keys/views | Old bytes/IDs/totals preserved; interruption/newer-schema rejection; check-schema |
| 07 Dataset engine/raw v3/read v2 | Scoped acceptance/receipts/work/generations/analytics; protocol-3 binding and legacy personal adapters | Colliding two-user identities isolated; per-dataset revisions/fences/cutovers; check-api |
| 08 Hosted accounts/runtime | Admin socket, provisioning, token/session auth, admission, fair work; actual hosted composition | Unauthenticated/foreign requests fail; revoked user/token/session fail; two-user E2E |
| 09 `web`/personal progress | Shared startup presenter, private progress sink/public read, browser capability gates | Personal browser progress live; hosted terminal-only; crash/overlap/reload semantics |
| 10 Hosted browser access | Token login/logout, identity-aware caches, common processing lag UI | No account crossover/placeholder leak; no collector-progress requests; login/revocation E2E |
| 11 Docker/docs/release | Container/Compose, health/readiness, native smoke, docs/tooling/plugins/assets | Persistent two-user restart, SIGTERM, JS-absent binaries/container; full gates |

02 and 03 can be separate surgical refactors; 04 follows their contracts. 05 can ship personal discovery before hosted storage. 06 uses the recorded explicit schema approval. 07 follows 06. 08 requires 05/07. 09 uses 03/04/05; hosted paths complete with 08/10. Each slice must preserve a runnable personal server; do not advertise hosted support before isolation/auth tests pass.

The user explicitly requested independent background agents in parallel.
Implementation uses the shared `codex/system-boundaries` worktree, fetched before
creation through Worktrunk. Processor, storage, delivery, client, browser, hosted,
progress, analytics and deployment owners work within exclusive file boundaries;
root sequences shared contracts, integration fixes and full verification. See
[EXECUTION.md](EXECUTION.md). Remove completed worktrees through Worktrunk.

## Verification contract

Preserve semantic fixtures, not only HTTP success assertions. Compare five components and totals, native contribution identities, provenance/outcomes, request hashes/bytes, receipts, acknowledged cursors and REST summaries.

Required focused cases:

- Duplicate/intrabatch/overlap/concurrent batches; identical accepted replay; changed identity bytes conflict atomically.
- Collector deletion/rebuild and lost-response retry retain exactly counted native contributions.
- Append, truncation, replacement, partial JSON tail, invalid safe counters and mutable OpenCode records preserve capture contracts.
- Same user/two collectors dedupe; different users with identical evidence/session/fact/stream/batch IDs remain independent.
- Cross-user parent identities never connect; receipt lookup cannot cross users; spoofed dataset rejects without mutation.
- Token rotation keeps cursor; account switch gets independent cursor; saved batch cannot be replayed into another dataset.
- Busy/failing user does not starve another; generation activation and pending status are dataset-local.
- Full query surface is scoped: totals, charts, pagination, facets/search, session counts, context, locations, estimates, unresolved, revisions and producer labels.
- Hosted TUI rejection for sync and query-only; kind mismatch; missing/disabled/unknown capabilities; localhost hosted still disallows TUI.
- Personal/public listener cannot write/reprocess/report progress; private socket verifies instance; hosted mounts no collector-progress route.
- Progress crash/heartbeat expiry, cancellation, overlap, ancillary reporting failure; query Reload never collects.
- Browser login/logout/user switch/revocation clear cached/in-flight data; no secrets in URLs/logs/storage; hosted never requests collector progress.
- Verified current and legacy upgrades preserve immutable bytes/history and active/building generations; rejected modes/versions do not mutate.
- Built server never starts/discovers collector sources. Direct native build and runtime work without Node/npm/pnpm.

For changed code run repository formatting/lint before verification, then focused Go/web tests, schema/API consistency, full tests, race tests and build. Use root scripts:

All TypeScript changes/tests retain the repository prohibition on `any`, non-null assertions and type assertions. Regenerate API contracts; never patch generated files by hand.

```sh
pnpm run format
pnpm run lint
pnpm run check-schema
pnpm run check-api
pnpm run test
pnpm run test:race
pnpm run build
pnpm run check-web
pnpm run test:web-e2e
```

After browser assets/build/Docker changes, build both binaries directly from `packages/cli`; run them with a restricted PATH containing no JS tooling. Verify TUI/web using `./packages/cli/bin/tokeninsights`, isolated HOME/XDG/source fixtures and the built server. Use repository browser tests; manual automation follows the machine's agent-browser instructions. No live user DB/source/config mutation during verification.

Update README, `CONTEXT.md`, `docs/design.md`, `docs/system.md`, an ADR for this decision, OpenAPI/generated clients, schema embeds/version constants, development/mock/Playwright fixtures and root start:web script in affected slices. Historical documents remain explicitly historical. ADR 0007's auth-deferred/shared-default/application-store decisions are superseded only by the reviewed hosted contract; acceptance/projection/privacy invariants remain.

## Explicit exclusions

JSONL collector replacement, database per tenant, organization datasets, signup/password/OIDC, external queue, alternative database backend, replicas, retention/compaction, background periodic collection, browser-triggered collection, new token/TPS semantics and automatic personal-to-hosted history transfer.

## Remaining questions and completion

- No unresolved product questions. Personal/hosted naming and collector-18/DuckDB-2/raw-3/read-v2/progress/auth contracts approved.
- Implementation and verification gates complete; deployment defaults are not measured capacity claims.
