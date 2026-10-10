# Current contracts and composition boundaries

Engine details below are historical; [ADR 0012](0012-sqlite-and-postgres-persistence.md) supersedes them with SQLite/PostgreSQL.
Status: Accepted. Date: 9 October 2026.
User explicitly approved schema/API removal and breaking changes before launch.

## Decision

Support the current system only: collector SQLite 20, token DuckDB 3, application
SQLite 2, jobs SQLite 1, raw ingestion protocol 3 and read API v2. Remove the
normalized collector pipeline, SQLite analytics server, old wire routes, retained
request adapters, imports, migrations and associated fixtures. Existing incompatible
databases reject without mutation. This supersedes compatibility provisions in
ADRs 0007–0009; it does not authorize deleting users' files.

Single-process and distributed are compositions of shared behavior. Local commands
own capture, direct submission, processing and queries; web additionally owns a
read-only listener. The remote process owns authenticated acceptance, processing
and queries, never capture. Mode selects composition once; core modules receive
explicit collaborators. Remote discovery validates capabilities and permission
before local capture. A failed remote operation never chooses local storage.

## Principles and their limits

- Share domain decisions, validation and atomic operations. DRY does not require
  sharing every similar line or merging independent lifetimes. Keep capture
  parsing separate from accounting interpretation.
- Encapsulate behavior and ownership, not merely types. Storage owns transactions;
  adapters own transport; composition owns process lifetime, policy and resources.
- Define contracts at consumer boundaries. Prefer Accept, Receipt, ProcessNext and
  Dashboard to generic repositories exposing CRUD. Interfaces must express real
  interchangeability; do not add a backend abstraction without a concrete need.
- Capabilities describe available features; permission authorizes a caller;
  dataset scope determines accessible data. None substitutes for another.
- Treat acceptance, processing visibility and displayed revision as distinct
  milestones. Retries preserve bytes and identity; cancellation joins work before
  closing storage.
- Evidence is immutable; projections are replaceable. Preserve native identity,
  component totals, provenance, generation fences and atomic publication.
- Reject unsupported contracts before mutation. A schema version alone is
  insufficient: inspect role, actual structure and required constraints.
- Delete an obsolete path with its schemas, routes, flags, docs and tests. Never
  retain a second production pipeline solely to make old fixtures pass.
- Production remains Go-only; committed browser assets run in the browser.

## Contracts and verification

Types establish shape; tests establish semantics.

| Boundary | Required evidence |
| --- | --- |
| Capture → outbox | Metadata-only bytes, checkpoint/outbox atomicity, quarantine, restart and partial-tail behavior |
| Outbox → receiver | Exact retry bytes, receipt binding, account isolation, rejection without acknowledgement |
| Direct ↔ HTTP adapters | Matching acceptance, rejection and query results against real stores |
| Evidence → projection | Native identity, all token components, ambiguity diagnostics and deterministic interpretation |
| Projection → storage | Real transaction rollback, restart, revision/generation fences and atomic publication |
| Storage → queries | Dataset isolation, consistent snapshot, pagination and filter semantics |
| Composition → capabilities | Local read-only routes, authenticated remote routes, no remote capture, ownership and shutdown |
| Composition → resource lifetime | Startup cleanup, cancellation and worker joins, concurrent listener draining with a shared deadline, ownership held through storage close, accepted work resumed after reopen |
| Package dependencies | Pure processing; capture does not interpret usage; storage does not mount HTTP |

Keep focused pure tests for arithmetic, native formats, identity, time boundaries,
and state transitions. Boundary tests complement these; replacing every unit test
with integration tests would make failures slower and less precise. Remove tests
of deleted behavior and redundant implementation-shaped mocks. Migrate useful
viewer oracles to current DuckDB fixtures.

Import rules follow transitive repository dependencies in internal/architecture.
The progress registry is independent of capture; localruntime owns the observer
that maps collection/delivery events into registry messages. Semantic contracts remain
beside their consuming packages. Generated API/schema checks prevent drift;
deployment tests exercise built binaries and durable process boundaries.

## Next work

Keep the supported surfaces small. New features must identify their owning module,
composition policy and observable contract before implementation. Add abstractions
when a second real consumer/adapter exposes a shared behavior, not to anticipate
hypothetical storage or deployment modes.

Unresolved questions: none for this cleanup.

PostgreSQL for remote token and account storage is now a concrete requirement.
[ADR 0011](0011-storage-adapter-contracts.md) defines the separate interfaces,
adapter conformance suites and implementation constraints.
