# Collector/server execution plan

Status: Implementing approved fresh-database storage/publication contract. All implementation belongs to [PR #53](https://github.com/flexdinesh/tokeninsights/pull/53).

This plan expands the [PRD](PRD.md) into parallel implementation boundaries. The [architecture proposal](../../docs/collector-server-architecture.md), [failure matrix](../../docs/collector-ingestion-tests.md), and [identity audit](IDENTITY-AUDIT.md) supply stable Gxx, Fxx, and CFIxxx references. Concrete approved storage and wire contracts supersede candidate JSON traces; update the traces explicitly when a decision changes them. User approved fresh collector/server databases; original storage remains untouched.

## Result and boundaries

One binary supplies the host collector, local server lifecycle, and API clients. A second foreground server command can compose the same ingestion/query core within the existing Go module. One server process runs per deployment. Local and remote servers are destinations, not required paired processes or an automatic relay.

The collector retains existing adapters, metadata-only raw facts, normalization, source fingerprints, and eligible byte cursors. It publishes normalized session-centric facts through a durable journal and saved immutable HTTP batches. The server retains normalized query data and durable ingestion receipts. Server queries, startup, and public endpoints never discover source files, parse harness artifacts, normalize raw data, or run Git. A single trusted owner belongs to each initial server database; several machines can publish into that owner. Hostname and installation identity must not prevent copied native facts from deduping.

Manual `sync` remains primary. Hook adapters use the same collector operation. Offline collection commits locally; delivery failure leaves publication pending. A later manual invocation retries. Query clients show committed available data; neither source coverage nor complete lifetime history is claimed. No retractions, reverse repair, autonomous uploader, source watcher, producer relay, or multi-account provisioning is introduced.

## Inspected dependencies

| Current boundary | Required change and implication |
| --- | --- |
| `internal/cli/command_sync.go` delegates writes to `service.Mutate` | Replace with host-owned collector orchestration; delivery follows local normalization. Server lifecycle admission cannot own collector mutation. |
| `internal/app/app.go` owns `pipeline.SourceConfig`, sync actions, reset/recovery scheduling | Keep collector orchestration on producer side; remove source-aware controller from server composition. A bounded ingestion admission queue replaces server collection scheduling. |
| `internal/service/files.go`, `manager.go`, `runtime.go` persist sources and expose private refresh/action routes | Version saved server config/discovery; retain lifecycle locks, private directory protections, readiness, detached launch, and stop semantics. Remove source configuration and collection routes. |
| `internal/server/server.go` implements `POST /api/v1/sync` and reads controller state | Replace collection endpoint/capability with read-only ingestion status and canonical ingestion POST. No endpoint launches a collector. |
| `internal/server/data.go` reads canonical queries plus `LastCompletedSync`, `ViewerDayCoverage`, `LoadSyncStatus` | Reuse canonical analytics, but replace local-source status with server revision and last committed ingestion. Omit source-day coverage rather than implying absent uploads prove empty days. |
| `internal/server/server.go` gets hostname from raw `ingest_runs` | Producer labels come from approved normalized provenance, not a raw-table query. Keep deployment identity separate; never substitute the serving machine's hostname for missing producer identity. More than one producer label can display "multiple machines". |
| `internal/cli/table.go` directly opens SQLite for rows, counts, status, filters | Route dashboard and facet reads through a typed HTTP client. Remove SQLite/probe fallback from the active viewer path; local/remote viewers use one protocol. |
| Existing `/usage` response has numeric rows for seven active tabs and summary `sessions`/`syncedSessions` | TUI session counts already fit the summary. Map active rows instead of duplicating SQL. API pagination and revision consistency need explicit handling. |
| `packages/web/src/useDashboardSync.ts` posts sync and polls local job state | Replace with server-status observation and query Reload. Web collection instructions point to `tokeninsights sync`; Reload never POSTs collection. |
| Root development scripts prepare one mixed database | Prepare separate collector/server fixture databases and publish through production ingestion. Update dev server, CLI preview, Playwright setup, schema/API checks, and build tooling. |

No repository-wide adapter rewrite is required. Keep one Go module and existing canonical query implementations where the approved server projection permits reuse. Separate storage open/migration paths are necessary: server history cannot use producer reset/resync recovery.

## Accepted command and configuration workflow

Use these defaults; routine command details do not require another approval. Storage/publication contract changes still require explicit approval.

| Command | Behavior |
| --- | --- |
| `tokeninsights` | Ensure local query/ingestion service; print status/URL. No source collection. |
| `tokeninsights sync` | Default all harnesses: collect changed sources, normalize, journal changes, prepare/send pending batches, report collection and delivery separately. `--harness` narrows collection. |
| `tokeninsights sync --publish-only` | Send previously journaled work without source discovery; useful during source loss or delivery repair. |
| `tokeninsights normalize` | Normalize retained local raw work and journal canonical changes. Publication occurs on `sync`; no server normalization action. |
| `tokeninsights view` | Ensure local server, then query API. Read-only default: no startup collection. `r` reloads committed data. Explicit `--sync` performs caller-side sync before opening. |
| `tokeninsights service start\|stop\|restart\|status\|run` | Manage local server and server database only. Remove `--reload-sources`; every non-loopback bind requires `--token` or `TOKENINSIGHTS_SERVER_TOKEN`. |
| `tokeninsights server run` | Foreground composition with canonical ingestion/query core. Bind loopback by default; every non-loopback exposure requires a token. Remote provisioning/TLS deployment remains later work. |
| Completion plugin invocation | Run the existing `tokeninsights sync` directly with a bounded host deadline. No separate hook CLI or durable trigger queue. |
| Collector reset commands | Scope explicitly to collector state. Resetting/deleting collector cannot delete server history. Server destructive reset is not an alias for producer reset. |

Use unambiguous `--collector-db-path` and `--server-db-path` flags rather than one `--db-path` changing ownership by command. Use fresh `collector.sqlite` and `server.sqlite` files. Leave original `tokeninsights.sqlite` untouched; legacy import/migration is outside this implementation. Reject aliasing the two files, including canonical symlink aliases and existing hard links. Saved server config contains bind/database/auth settings only; private producer config can contain source roots and selected destination. Never persist the full process environment.

An explicit `--server-url`/destination selects remote transport and skips local discovery/start even when delivery fails. Explicit flags override environment, then saved config, then defaults. Credentials come from caller configuration/environment; exclude them from database journal payloads, logs, printed URLs, and replay identity. Transport configuration is separate from canonical entity identity.

Delivery summaries distinguish discovered/parsed/normalized counts, pending publication, committed batches/entities, duplicate/no-op facts, and failure stage/code. Earlier acknowledged batches remain committed when a later suffix fails. Do not label partial progress as full success or erase pending work after conflict.

## TUI and browser API transition

Keep the seven current active token tabs, repo/directory grouping, filters, context aggregates, session counts, sorting, formatting, selection cancellation, and stale-response protection. TPS remains a documented metric capability with `tps avg`, `tps mean`, and `tps median` concepts; do not remove timing render/model concepts because initial durable timing is sparse. Do not invent timing values from token counts. Add timing query/transport fields only when an implemented durable timing domain needs them; unavailable metric tabs stay inactive as today.

Use one Go query client for local and remote responses with generated response types. The API already carries summary session counts. The TUI loads bounded pages and requires the same server/database identity and revision across pages plus a final instance check. Retry at most three times if ingestion changes that snapshot; fail explicitly rather than mix revisions. Restore selection only when query/identity still match. Cancel obsolete HTTP requests on filters/tab changes and quitting.

Keep API server timezone as reporting timezone for this release, and display it consistently in TUI/browser. Occurrence times cross ingestion as UTC epoch values; collection host timezone cannot change IDs. Tests use a pinned reporting timezone and date-boundary fixtures. Configurable client-specific timezone is a later extension.

Browser Sync becomes Reload. Show a concise empty-state/help command: `tokeninsights sync`. Replace "Synced", "Syncing", "days checked", and "No usage found" claims derived from producer discovery with server receipt/revision semantics. Refreshing queries cannot assert source completeness or start producer work. Polling is optional observation only; server restart/data identity changes clear stale cache before new results render.

## Thin harness adapters

Servediff's inspected `docs/architecture.md`, `docs/system.md`, `docs/plugins.md`, manifests, and `packages/plugin-*` provide prior art. Transfer native registration and small completion adapters, not servediff's expiring/supersedable pending diff snapshots: TokenInsights historical pending facts must remain durable.

| Harness | Verified completion surface | TokenInsights adapter scope |
| --- | --- | --- |
| Codex | `Stop`, command hook JSON with common session fields; servediff has a native marketplace manifest and hook definition | Invoke `tokeninsights sync`; discard hook stdin and CLI output. Return empty decision JSON; no continuation decision. |
| Claude Code | `Stop` after normal main-agent response; interrupted/API-error completion differs | Invoke `tokeninsights sync`; completion is best effort. Manual sync catches retained records later. |
| Pi | `agent_settled` is final notification after automatic continuation; servediff Pi package registers it | Register one completion handler; launch the Go command with argument array and `shell: false`; no parser or HTTP logic. |
| OpenCode | V2 event subscription with cleanup; servediff adapter observes `session.status` where status is idle | Pin/test supported V2 package/event schema. Subscribe, type-guard payload, invoke Go, abort on plugin cleanup. Do not invent a generic turn-end hook or claim V1 parity. |

Primary references checked during planning: [Codex hooks](https://developers.openai.com/codex/hooks), [Claude hooks](https://code.claude.com/docs/en/hooks), [Pi extension lifecycle](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md), and [OpenCode V2 events](https://opencode.ai/v2/docs/build/plugins/#events). Current upstream docs may evolve; supported minimum releases require pinned SDK/types and fixture verification in the adapter task. The existing servediff baseline is Codex 0.160.0, Claude 2.1.265, Pi 1.0.0, OpenCode 2.0.22; do not advertise TokenInsights support solely from those labels.

Hook events select a completion opportunity, not accounting inputs. Discard prompt/assistant text, tool data, request headers, transcript paths, and session fields rather than forwarding them to the collector. Do not restrict accounting to a hook's session without preserving required Codex ancestry/cumulative context. Initial wrappers invoke default all-harness sync; normal continuity prevents rereading unchanged eligible sources. A hook may fire before final usage is durably flushed; no hook promises complete turn attribution. Later hook/manual collection processes retained data.

Initial adapters run the collector directly with a 60-second host deadline. Native command hooks wait for the finite process; Pi/OpenCode runners must bound child lifetime and abort active work on teardown. Existing collector writer ownership serializes concurrent collection; repeated uploads converge through canonical fact and batch dedupe. OpenCode can coalesce overlapping idle callbacks within its adapter, without another durable queue. Timeout can leave collection incomplete; committed transactions and publication journal work survive, and later manual sync resumes. Hook launch failure cannot guarantee a trigger was recorded. No separate `internal/hooks` scheduler, detached worker, private trigger store, perpetual retry, silent expiration of undelivered journal entries, or replacement of historical facts is required. Diagnostics use fixed host-log markers, with manual CLI output for details; conversation content is never logged. Commit installable native manifests and standalone package artifacts; do not edit user harness configuration automatically. A durable asynchronous trigger engine is future follow-up scope only.

## Dependency graph and parallel ownership

| Task | Primary files/packages | Dependencies | Trace IDs and acceptance |
| --- | --- | --- | --- |
| T01 Fix native identity/accounting | `internal/pipeline/opencode_sqlite.go`, `claude_code_jsonl.go`, `normalize.go`, real fixture tests | Reviewed adapter rules; identity-generation approval when required | G01/G02; CFI007 distinct equal-valued OpenCode requests; CFI008 partial→complete Claude; retain CFI001–006 and CFI009 raw evidence. Preserve proven copies/V1-V2 overlap; no largest-counter universal rule. |
| T02 Approve storage/wire contract | proposed contract/ADR, source schema + embed/constants, OpenAPI | Read-only proposal and explicit user approval before contract edits | G01–G11; specify ID encoding, entity boundaries, value precedence, receipt/cursor, limits, compatible/incompatible versions, owner. |
| T03 Collector journal/store | producer storage package, pipeline normalization transaction, collector exporter | T01/T02 | G03/G04/G07/G10; F01/F06/F11/F14. Canonical change and journal atomic; unchanged values do not grow journal; saved batches immutable; destination markers independent. |
| T04 Shared normalized protocol/client | narrow `internal/ingestion` package, codec/client tests, OpenAPI generated types | T02 | G05–G09/G11; F03/F04/F08–F12. Allowlist only; exact integer rules; bounded body/client deadline; typed failure codes; receipt validation; retries use identical saved bytes. |
| T05 Server store/core | server-only storage open/migration, `internal/ingestion` server application/store | T02/T04 | G05/G06/G08/G09/G11; F01–F05/F07–F12/F14. Transactional stable uniqueness and receipts; deterministic conflicts; no raw/source imports or filesystem dependency. |
| T06 Local/remote composition | `internal/service/{manager,files,runtime,client}.go`, server HTTP handlers, foreground command | T05 | G06/G10; F02/F05/F11/F14. Retain lock/private-directory safeguards; canonical-only startup; bounded admission; server restart preserves receipts. Remote selection never boots local. |
| T07 Host sync and command/config UX | `internal/collector`, CLI commands/flags/help/summary | T03/T04/T06 | G03/G04/G07/G10/G11; F01/F02/F06/F11/F13/F14. Real sync publishes; offline collection then manual resume; partial suffix failure reported; producer resets preserve server. |
| T08 API clients/TUI | Go query client, `internal/cli/{command_view,table,sync_coverage,desk}.go` | Approved API/T06; parallel skeleton after T02 | G06/G10; remote read without local DB; all active tabs/facets/sorting parity; pinned multi-page reads; no implicit collection or fallback SQLite reads. |
| T09 Browser | `packages/web/src/{api,useDashboardSync,contracts}.ts*`, dashboard header/coverage/empty state, mocks/e2e | Approved query/status API; implement parallel with T08 | G06/G10; Reload only GETs; cache observes server revision; no source-completeness claims; served data remains usable collector absent. |
| T10 Hooks/plugins | `packages/plugin-{codex,claude,pi,opencode}`, marketplace manifests, scripts | T07 stable `sync` entry; package source/adapters can begin after interface freeze | G01/G05/G10/G11; synthetic events/executable paths; no transcript forwarding; direct bounded sync, repeated/concurrent dedupe; lost upload retained; timeout/teardown contains child lifetime; Pi/OpenCode SDK type checks and standalone artifacts; no Node required by Go product. |
| T11 Compatibility/roles | separate producer/server role checks, DB-path alias checks, incompatible-schema fixtures | T01/T02/T03/T05 | G01/G02/G09/G10; F01/F09/F13. Fresh collector.sqlite/server.sqlite; original tokeninsights.sqlite untouched. Preserve server facts/receipts; wrong role or incompatible schema rejected without deletion. No legacy import. |
| T12 Full failure integration | production HTTP + SQLite tests, isolated subprocess driver, synthetic golden fixtures | T03–T07/T11; test skeleton/oracles parallel | Every F01–F14 and G01–G11. Real two-DB reconstruction and REST totals; barriers rather than timing sleeps; representative process kills; exact failure correlation. |
| T13 Documentation/release/tooling | README/design/ADR, PRD/implementation/failure matrix, build/dev fixture tasks | Integrate each task incrementally; final after T08–T12 | Current design matches runtime; no stale server-sync docs; native Go/install checks, API/schema/asset consistency, plugin artifacts reproducible; PR title/body final scope. |

T03 and T05 can run independently after T02 because their stores differ. T08 and T09 can run independently against approved query DTOs. T10 adapters can use a fake executable while T07 is underway. T12 owns integration/fault seams but must coordinate with store owners; tests execute production implementations, not a second simulated ingestion engine. One integration owner changes shared schema/OpenAPI/generated files and runs root formatting; agents do not overwrite each other's files. Shared package interfaces are frozen and communicated before dependent code begins.

## Compatibility acceptance

Create fresh `collector.sqlite` and `server.sqlite` under the selected data directory. Original `tokeninsights.sqlite` remains untouched. No legacy bridge, import, or ambiguous identity migration is required. Do not copy old canonical keys into server rows before publishing reconstructed stable keys: that can double usage.

Reject opening a collector database as server storage and vice versa; reject identical/symlink/hard-linked database aliases. Server schema/data compatibility checks preserve historical facts and receipts; they cannot assume producer artifacts remain available or use destructive reset/resync recovery. Rebuilding/deleting collector storage replays retained sources without retracting existing server history. Unsupported schema/data generations fail clearly without changing original storage.

## Real verification gates

1. **Identity:** CFI001–009 run through real adapters/persistence. Both reds become green with reviewed expected identities/counters, plus adjacent copy/revision cases. Add exact stable publication IDs to independent expected facts. Missing-native-ID ambiguity stays explicit; do not turn it into an arbitrary count oracle.
2. **Stores:** inject SQL/commit failures at capture, normalization+journal, batch preparation, server fact+receipt, and collector acknowledgement. Reopen both databases/WAL; inspect identity/value sets and progress. Check bounded limits at limit and limit+1; unchanged imports create no new journal work.
3. **Protocol/core:** F01–F14 use real SQLite and HTTP. Include same batch/equal facts across batches, conflicting repeated facts within a batch, two owners in controlled test composition, shuffled entity order, changed-payload arrival orders, numeric extremes, private extra fields, and concurrent duplicate barriers. Receipt equality survives restart and lost response; invalid batches have no visible prefix or success receipt.
4. **Manual workflow:** built Go binary, isolated XDG/source/database directories. Publish known fixtures; query hand-authored totals; stop server, append usage and sync offline; restart and sync; delete only collector DB, reparse, and verify existing server fact set/totals unchanged. Source removal preserves server history. Direct remote URL skips local bootstrap. Observe failures through safe IDs/stages/codes.
5. **Viewer/plugin:** API TUI parity with all active tabs/facets and reporting timezone boundaries; multi-page revision changes cannot mix rows. Browser Reload causes no collection POST, and unavailable collectors do not block saved analytics. Plugin tests use fake executables and synthetic hook payloads, including spaces/metacharacters in executable paths and private event fields; no native host state edited.
6. **Repository:** run `pnpm run format`, `pnpm run lint`, focused tests, `pnpm run test`, `pnpm run build`. Then `mise run check:push` including race/e2e and schema/API/web checks. Direct native Go builds and `--help` with JavaScript tooling absent from PATH. Regenerate and commit embedded web/plugin artifacts. No skip/expected-value weakening or push-hook bypass once runtime work claims completed.

Update [VALIDATION.md](VALIDATION.md) with actual commands and outcomes, and the failure matrix with production test names. Fixtures/oracles precede fixes; assertions check stable identity, every token component, countability, references, receipts, revisions, and final REST totals. Preserve red tests for newly discovered semantic bugs until resolved or explicitly recorded as blockers. Completion means full runtime path and gates, not passing fixture-integrity checks alone.

## Unresolved questions

None for current scope. Fresh database roles, publication contract, native identity/revision policies, and CLI defaults are decided. Remote provisioning and any future legacy migration remain later work.
