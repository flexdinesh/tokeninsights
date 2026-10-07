# Execution

Status: Complete. Schema/wire contracts explicitly approved 7 October 2026.

Worktree: `codex/system-boundaries`, fetched `origin/main` first.
Implementation plan: [IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md).

## Parallel ownership

| Agent | Exclusive initial ownership |
| --- | --- |
| processor | Pure interpretation extraction, pipeline helpers/tests, new processor |
| storage | DuckDB schema/embed, datastore except http.go; dataset isolation/upgrades |
| delivery | Collector schema/db upgrade, evidence protocol, rawcollectorstore, collector transport |
| client | serverfeatures, network preferences, config, CLI, queryclient |
| browser | OpenAPI/generated clients, web source/tests |
| hosted | accounts, shared serverruntime, remoteserver, server executable |
| progress | collectorprogress, service runtime/client and personal progress controls |
| analytics | analytics extraction, server data/duck_queries/api_transport |
| deployment | Docker/Compose, root package scripts, product docs/glossary/ADR |
| root | HTTP integration, shared contract decisions, cross-agent fixes and full verification |

No commits/push/PR until integration. Shared worktree; avoid owned files and communicate interfaces before cross-cutting edits. Dependencies are sequenced by messages; independent implementation runs in parallel.

## Wire integration decisions

- Kinds: `personal`, `hosted`. Capabilities: `usage`, `facets`, `web-dashboard`, `raw-ingestion`, `terminal-dashboard`, `collector-progress`, `reprocess`.
- `GET /api/v2/instance`: current instance fields plus `serverKind`, `datasetId`, capabilities and caller `permissions` (read/ingest). `apiVersion: v2`. Features and permission are distinct.
- `GET /api/v2/status`: instance/dataEpoch/datasetId/readiness, generation/targetGeneration/inputRevision/revision/pending. No collector status. Usage/facets v2 same existing response plus datasetId; api query parameters remain unchanged.
- Raw v3: existing v2 envelope plus required datasetId; routes `/api/v3/ingestion/{capabilities,batches,batches/{stream}/{batch}}`. Existing raw v2/legacy v1 personal-only and byte-preserving.
- Login: `POST /api/v2/auth/session` body `{token: string}`; creates read-only browser cookie; `DELETE /api/v2/auth/session` logs out. Success returns 204. Login screen on 401; browser clears all account caches. Never return/embed raw token.
- Personal progress GET `/api/v2/collector-progress`; private control writes `/control/v1/collector-progress`. Hosted both absent. Body/attempt API owned by progress agent, published before browser/client integration.
- Hosted runs one shared DuckDB2, scoped keys/views and per-user datasets. No DB per tenant. Hosted admin uses private socket.

## Completion gates

Format/lint; focused semantic tests; schema/API checks; full/race/browser tests; embedded asset/native build; JS-absent smoke; Docker persistence/auth/restart if Docker available. No user DB/config/source mutations.

All gates passed 7 October 2026:

| Gate | Evidence |
| --- | --- |
| Formatting/lint | Root `pnpm run format`, `pnpm run lint`, `pnpm run format:check`; pinned Go lint reports zero issues; `git diff --check` clean |
| Contracts | `pnpm run check-schema`, `pnpm run check-api`; generated Go/TypeScript contracts and embedded schema copies agree |
| Functional tests | Root `pnpm run test`: 31 build-tool tests, 61 browser unit tests, all Go packages pass |
| Concurrency | Root `pnpm run test:race` passes; final focused worker/storage/accounts/CLI/delivery suites also pass |
| Build/assets | Root `pnpm run build`, `pnpm run check-web`; both native binaries also built directly from `packages/cli` |
| Browser runtime | Root `pnpm run test:web-e2e`: 26 pass against final binaries; real hosted HTTPS sessions/user switch/revocation and one-command managed web/progress |
| Native UX | Isolated HOME/XDG and empty PATH: personal web initializes, Pi sync accepts two entries, totals 120, repeated sync remains 120, query-only TUI renders and quits; service stopped; final native binaries run without Node/npm/pnpm |
| Container | Final image `sha256:645368174d4b7e422b8f7c1d474842e497fb9d1a87564b8831bfaac6ea30b907`; both Compose configs validate; native libraries resolve; non-root/JS-free runtime; personal/private admin and hosted shared-user isolation, replay, scoped reprocess, revocation, SIGTERM/restart persistence pass |

Semantic coverage includes all five token components, native identity collisions, exact retained protocol-2 bytes and receipts, independent dataset generations, ancestry, fair work, every dashboard tab/chart/facet, admission-before-body validation, raw-contract preflight-before-capture, collector-17 maintenance, immutable outbox recovery, and schema-1 staged upgrade/rejection.

Final boundary follow-ups extracted `dataengine` worker orchestration through dataset-scoped semantic backend operations, removed remaining HTTP SQL, shared terminal progress, and wired operator reprocessing for both foreground kinds. Existing package names retained for capture (`pipeline`), collector state (`rawcollectorstore`), delivery (`collector`) and DuckDB (`datastore`).

Safe upgrade constraint: schema-1 storage with a nonempty WAL must be stopped/checkpointed before upgrade; rejection preserves source bytes. Verified upgrades retain the original `.schema1-backup`.

Disposable runtime fixtures, browser processes, smoke containers and volumes cleaned. Implementation remains as reviewable local changes in the worktree above; no live user storage/config/source changed.

## Unresolved questions

None. Integration details resolved by root/owners within approved design.
