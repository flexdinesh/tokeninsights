# Storage adapter contracts

Engine details below are historical; [ADR 0012](0012-sqlite-and-postgres-persistence.md) supersedes them with SQLite/PostgreSQL.
Status: Accepted. Date: 10 October 2026.

## Decision

PostgreSQL is a planned remote backend for both token evidence/analytics and
accounts, through separate interfaces. This supersedes the deferral of alternate
backends in ADR 0009 and the hypothetical-backend assumption in ADR 0010. Current
production adapters remain DuckDB for tokens and SQLite for accounts. This change
does not implement or advertise PostgreSQL support, change schemas or introduce
migrations. Collector/outbox and operational jobs remain local SQLite.

Interfaces describe observable behavior and transaction outcomes. They do not
expose SQL, driver types, filesystem paths, transaction handles or a generic CRUD
repository. Mode policy chooses behavior; composition selects and owns adapters.

| Boundary | Contract and ownership |
| --- | --- |
| Acceptance | `evidence.Receiver`: one authorized dataset, exact request identity, atomic evidence/receipt/pending-work commit, immutable replay, portable missing-receipt error |
| Processing | `dataengine.Backend` / `ConcurrentBackend`: consistent connected work, atomic projection/outcome/provenance publication, generation/revision/membership fences, durable retry state |
| Queries | `analytics.Repository`: dataset-scoped rows/totals/metadata in one snapshot, bounded pagination, distinct confirmed/estimated usage, shared token/calendar semantics |
| Accounts | `accounts.Repository`: credential transactions, source-token-bound sessions, immediate revocation checks, stable user/dataset identity and recoverable provisioning |
| Provisioning | `accounts.Datasets`: idempotent dataset creation, existence and explicit reprocessing; no account SQL in the token adapter |
| HTTP | `server.DataSource`: receiver/query ports scoped only after authentication; both identify the same dataset/store; no fallback or implicit creation |
| Runtime | `serverruntime.Readiness` / `Worker`: initialized storage readiness, cooperative cancellation and joined processing; composition owns opening/closing |

`analytics` and `accounts` contain contracts, values and domain policy. DuckDB
query SQL lives in `adapters/duckdb`; token persistence remains in `datastore`.
SQLite account SQL lives in `adapters/sqliteaccounts`, using `appstore` for the
physical database and pairing guard. Credential generation/digests/permission
validation remain shared account policy. Local and hosted roots construct adapters;
HTTP handlers and shared runtime cannot import them. Transitive import tests enforce
these boundaries. Do not add forwarding compatibility APIs for former locations.

## Constraints for PostgreSQL

- **Transactions:** evidence/receipts/queue are one acceptance transaction;
  facts/estimates/provenance/outcomes/revisions are one publication transaction.
  A lost response or cancellation racing commit may leave a committed acceptance.
  Resolve uncertainty by replaying identical request bytes, never new identities.
- **Snapshots:** every multi-statement dashboard/facets/status operation must use
  an appropriate shared snapshot. PostgreSQL READ COMMITTED can observe different
  snapshots per statement; its adapter must select isolation accordingly and retry
  entire transactions where serialization failure requires it. See PostgreSQL's
  [isolation documentation](https://www.postgresql.org/docs/current/transaction-iso.html).
- **Independent stores:** PostgreSQL may hold both domains physically, but callers
  cannot require a transaction spanning their interfaces. Provisioning persists a
  pending account with fixed dataset identity, ensures that dataset, then activates
  the account. Resume is idempotent and cannot reenable disabled ready accounts.
- **Ownership:** continue one server owner and one bounded processing dispatcher.
  `LoadWork` and in-memory exclusions are not durable multi-process leases. A future
  PostgreSQL composition must enforce ownership and stop on ownership loss; it
  cannot reuse a local file lock as cross-host exclusion. Session-level advisory
  locks require dedicated-connection/pool care. See PostgreSQL's
  [locking documentation](https://www.postgresql.org/docs/current/explicit-locking.html).
- **Representation:** retain exact accepted bytes and hashes independently of any
  parsed JSON representation. Keep dataset-qualified keys, stable native identities,
  integer bounds, null/unknown distinctions, time buckets, deterministic ordering
  and separate estimates. Backend SQL and indexes may differ.
- **Lifecycle:** readiness means initialized/available storage, not an empty queue.
  Request cancellation propagates to drivers; workers join before storage closes.
  A forced HTTP close does not guarantee arbitrary handler termination.
- **Physical contracts:** each adapter owns schema validation and durable instance
  identity. PostgreSQL must supply the pairing guarantees currently provided by
  embedded metadata and the application sidecar without imposing filesystem paths
  on its interfaces. Schema implementation needs separate explicit approval.

Multi-server processing, distributed admission limits, migrations and switching
existing data between engines are separate work. PostgreSQL support alone must not
silently promise any of them.

## Verification and delivery

`storagecontract` supplies SQL-free suites parameterized by real adapter factories
and reopen operations. Token tests cover replay/conflict rejection, tenant isolation,
pending restart, cancellation, revision/generation fences, all token components,
pagination and internally consistent concurrent reads. Account tests cover durable
identity, credential scopes, session revocation, logout and disabled-account recovery.
SQL-specific fault injection and provisioning interruption tests remain with adapters;
pure accounting and existing direct/HTTP conformance tests remain intact.

These suites are an initial executable contract, not proof of an unimplemented
backend. PostgreSQL delivery must run them against real PostgreSQL and add its own
transaction rollback, serialization retry, owner-loss and schema tests. New semantic
cases extend the shared suite so both supported adapters must satisfy them.

Next implementation: PostgreSQL token and account adapters, explicit backend
configuration, durable identity/pairing and ownership, schema validation, deployment
readiness, and real-database verification. No compatibility shims or generic SQL
dialect layer are required by this decision.

Unresolved questions: none for the boundary refactor. PostgreSQL driver, deployment
version and ownership implementation are decided with that adapter's implementation.
