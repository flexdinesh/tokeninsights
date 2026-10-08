# Single-process and distributed compositions

Status: **Accepted; implemented and validated**. Date: 8 October 2026.
User approved schema changes and implementation after architecture review.

## Decision

Compose the same Collector, acceptance, processing and analytics through direct
calls locally and authenticated HTTPS remotely. Single-process commands own their
database/worker lifetime. Only local `web` opens a public listener; local `tui`
queries directly. The remote deployment is one authenticated container and has
no terminal analytics dashboard. Docker supervises its foreground process.

Retain durable evidence/outbox, immutable receipts, dataset isolation, stable
contribution identities, dependency/revision fences, processing generations and
separate estimates. Keep the existing durable processing queue in both modes;
do not introduce an external broker. Token evidence and accounting transactions
stay in DuckDB. Account/system management moves to a paired SQLite adapter with
recoverable provisioning; no cross-engine transaction guarantee.

Use semantic delivery, processing, query and account contracts. DuckDB remains
the initial token adapter; future backends must satisfy the same atomicity,
isolation, replay and consistent-query contracts. Generic CRUD is insufficient.

Distributed `sync` defaults to a finite background job with durable local status.
`sync --print` also submits and prints the remote URL. `--wait` supplies foreground
acceptance and `--debug` supplies foreground progress/receipt observation. Early
exit reports successful startup, never remote acceptance or fresh analytics.
Plugins initially select their harness only. No plugin event/counter payload.

One local viewer owns a database. Concurrent sync/plugin requests use durable
local handoff, executed by that owner without HTTP ingestion. No collector daemon
or periodic source scanning. Local viewer startup awaits query visibility;
distributed delivery normally finishes at acceptance.

## Rollout

The [implementation plan](../../.scratch/runtime-compositions/PRD.md) and six
dependency-ordered issues specify lifecycle, command, migration and test gates.
All command compositions now use the shared adapters. Application SQLite and finite
sync jobs have separate role schemas; DuckDB remains version 2 with read-only legacy
account tables. ADR 0008's daemon composition and shared account-storage decisions
are superseded; its wire capabilities, authentication and dataset isolation remain.

Schema approval is recorded; it does not authorize silent deletion of history.
Preserve stable database/dataset IDs, exact pending requests, acknowledgements,
account identities and revocations. Retain recoverable migration copies.

## Unresolved questions

None. Alternate storage, additional containers and richer plugin input deferred.
