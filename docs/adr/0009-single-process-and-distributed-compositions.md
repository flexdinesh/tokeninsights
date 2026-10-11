# Single-process and distributed compositions

Command selection/config and local handoff are superseded by [ADR 0014](0014-command-owned-client-compositions.md); retained domain/storage contracts still apply.

Engine details below are historical; [ADR 0012](0012-sqlite-and-postgres-persistence.md) supersedes them with SQLite/PostgreSQL.
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
or periodic source scanning. Local TUI and Web open after storage initialization
and display command-owned collection and processing progress alongside saved usage.
Distributed delivery normally finishes at acceptance. Local-only capability flags control collector progress and
dashboard Reload; Reload refreshes queries only. This Web startup refinement was
approved on 9 October 2026, including additive processing-status fields and local
machine hostname semantics; it changes no database schema.

The user-approved TUI refinement on 9 October 2026 supersedes the initial
fresh-data startup wait: show saved committed usage immediately, with a persistent
refresh strip while capture, submission and processing run in the owning process.
Refresh published revisions automatically without resetting viewer controls.
Acceptance is not completion; TUI completion requires processing visibility and a
successful dashboard read of the current published revision. Failure/quarantine
preserves saved usage and shows an incomplete refresh. `--sync=false` skips startup
capture but resumes durable processing. Shared local startup orchestration owns
cancellation and joins background tasks before storage closes. This refinement
changes no schema, accounting semantics, capture scope or hosted capabilities.

## Rollout

The [design contract](../design.md) specifies runtime boundaries; the
[development guide](../development.md) defines verification gates.
All compositions use shared semantic adapters. Application SQLite and finite sync
jobs have separate role schemas. [ADR 0010](0010-current-contracts-and-boundaries.md)
removes compatibility paths and defines current-schema rejection without mutation.
Stable identities, exact pending requests, receipts and credential revocations remain
required within supported contracts.

## Unresolved questions

None. Alternate storage, additional containers and richer plugin input deferred.
