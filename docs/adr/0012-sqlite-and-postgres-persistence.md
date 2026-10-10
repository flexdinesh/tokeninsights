# SQLite and PostgreSQL persistence

Status: Accepted. Date: 10 October 2026. Schema implementation explicitly approved.

## Decision

SQLite and PostgreSQL are the supported engines. Single-process composition uses
SQLite for every persistence role. Hosted composition selects SQLite or PostgreSQL
for both token and account storage. Collector outbox and operational jobs stay
local SQLite. DuckDB and TimescaleDB have no adapters, dependencies or migration
paths. Existing files remain untouched; incompatible files reject without mutation.
This supersedes the engine choices in ADRs 0007–0011, retaining their semantic
contracts and composition boundaries.

The backend is deployment configuration, independent of mode/capability policy.
Only composition roots select adapters and own their lifetimes. HTTP, processing,
analytics and account consumers retain the semantic interfaces in ADR 0011.
Accounts and tokens have separate contracts even on one physical PostgreSQL
database. Provisioning remains pending-account → ensure-dataset → activate, with
idempotent restart recovery; consumers cannot demand a cross-domain transaction.

## Implementation boundaries

- `datastore` owns shared relational token transactions and dataset handles.
- `adapters/sqlanalytics` owns SQL analytics, with explicit engine-specific calendar
  and aggregation expressions. Filtering, grouping and pagination stay in SQL.
- `adapters/accountsql` owns shared account transactions and provisioning. Credential
  generation, digests, permissions and expiration policy remain in `accounts`.
- `adapters/sqlite` / `adapters/sqliteaccounts` and `adapters/postgres` construct
  implementations. `appstore`, `persistence/sqlitecore` and `persistence/postgres`
  own physical opening, validation, identity and connection behavior.
- `persistence/sqlutil` only numbers bound SQL parameters. It is neither a generic
  repository nor an extensible dialect framework. Domain interfaces expose no SQL.

DRY shares transaction rules and semantic oracles, without merging collector,
token, account and job lifecycles. An engine difference stays inside adapters.
Adding an engine requires implementation plus the same executable contracts.

## Durability, ownership and failure

SQLite token storage uses strict tables, WAL, FULL synchronous writes, one immediate
writer and pooled read snapshots. Each role validates actual schema objects against
its embedded definition before writable opening. The token role is version 1;
collector 20, application 2 and jobs 1 retain their existing contracts. Application
identity is paired with token identity and the `.application.json` guard.

PostgreSQL 18 is the supported major version. Two fixed namespaces,
`tokeninsights_data` and `tokeninsights_accounts`, initialize atomically with paired
identities and independent version-1 metadata. Startup inspects columns, constraints,
indexes, views, relation properties and unexpected triggers/functions against the
committed catalog contract. Unrelated namespaces remain untouched; a missing half
or mismatched pair fails closed. Fresh-database tests also verify the catalog
snapshot against the authoritative SQL.

One server owns a database. PostgreSQL uses a session advisory lock on a reserved
connection; every token and account write runs on that connection under a shared
mutex. The owner never replaces a lost connection. Loss cancels server lifetime;
another owner may then start. Reader connections can reconnect independently.
This provides ownership fencing, not horizontal scaling or distributed scheduling.

Multi-statement reads use REPEATABLE READ. Serialized writes use READ COMMITTED;
there is no speculative serialization retry layer. A failed transaction rolls back.
The durable worker retries pending processing; clients replay exact accepted batch
bytes after an uncertain response. Never retry an unknown commit with new identity.
Readiness checks initialized paired storage, not queue emptiness. Shutdown joins
workers and handlers before releasing storage ownership.

Both engines preserve integer token bounds, exact request bytes, dataset-qualified
keys, null/unknown semantics, stable ordering, active-generation visibility and
session-peak medians. SQLite calendar functions use Go timezone rules; PostgreSQL
uses named-zone SQL expressions. Shared tests cover DST, fixed offsets and calendar
boundaries. No time-series extension is needed by the current contract.

## Verification and constraints

SQL-free shared token/account suites run against real SQLite and PostgreSQL.
Hosted HTTP tests repeat isolation, spoof rejection, replay, provisioning and
restart checks for both. Engine tests cover schema rejection, rollback, snapshots
and PostgreSQL ownership loss. Preserve pure accounting oracles and focused failure
tests; do not replace them with mocks or assertions about internal call order.

`pnpm test` and `pnpm test:race` require live PostgreSQL. The runner starts a pinned
disposable container unless `TOKENINSIGHTS_TEST_POSTGRES_DSN` is provided. Fixtures
create random databases and drop only those databases. Local pre-push runs the live
contracts; CI covers native OS/architecture builds and focused storage tests.
Production builds need no CGO or JavaScript runtime; race tests still require CGO.

No migration/import support, automatic fallback, multi-server scheduling or
cross-engine data transfer is promised. Future major PostgreSQL versions require
catalog and behavioral verification before widening support.
