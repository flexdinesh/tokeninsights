# Personal/hosted composition, capabilities and user datasets

Engine details below are historical; [ADR 0012](0012-sqlite-and-postgres-persistence.md) supersedes them with SQLite/PostgreSQL.
Composition and account-storage decisions superseded by [ADR 0009](0009-single-process-and-distributed-compositions.md); capability/authentication wire contracts remain.
Status: **Accepted**. Date: 7 October 2026.
Schema and wire contracts explicitly approved before implementation.

Supersedes ADR 0007's shared-default-only deployment, deferred authentication and
future separate application SQLite decisions. Its immutable evidence/receipt,
replaceable projection, privacy, acceptance and processing invariants remain.
[Design](../design.md) specifies the current contracts.

## Context

The same collection, interpretation and analytics need to support a one-command
personal TUI/web experience and a hosted server for multiple users. Embedding local
service discovery in delivery, host readers in interpretation and HTTP in storage
obscures ownership. A hosted browser cannot observe its user's collector as if it
were a local service. Multiple users need isolation even when their source-native
IDs, stream IDs and fact IDs collide.

## Decision

Use two explicit server kinds: `personal` (default, unauthenticated, one dataset)
and `hosted` (authenticated, one isolated dataset per user). Both compose the same
pure processor, dataset-scoped data engine, analytics, REST/assets and runtime.
Managed personal adds owner discovery, private control and an in-memory collector
progress registry. Foreground hosted adds accounts, principal resolution, sessions
and bounded admission. One instance owns one shared DuckDB file for all users.

Typed capabilities exposed by REST determine routes and viewer behavior; caller
permissions separately authorize supported operations. Hosted cannot enable TUI
or collector progress. Client kind is an expectation, checked before collection.
Capabilities do not allow a client to select or bypass authentication.

Scope deduplication, receipt identities, ancestry, processing work/generations and
all analytics by dataset. Keep native hashes unchanged inside a dataset. Assign
dataset identity from authenticated accounts; reject a contradictory batch binding.
Multiple collectors of one user dedupe; identical evidence from different users
remains independent. Bind collector acknowledgements to endpoint/database/dataset,
so token rotation resumes and switching users cannot skip delivery.

Store accounts and credential digests in paired SQLite. Recoverable provisioning
creates the DuckDB dataset before activating the user. Administrators
provision users and scoped tokens through a private socket. Browser token login
creates a read-only opaque secure cookie; never persist bearer credentials in
browser URLs or localStorage. Initial deployment is one instance behind TLS.

`tokeninsights tui` composes personal startup/sync/query. `tokeninsights web`
composes startup/sync/browser launch for either kind. Managed personal opens early
and exposes bounded sanitized capture/submission progress; hosted syncs in the
terminal and opens authenticated saved analytics without collector progress.
Server startup and Reload remain collection-free. Plugins keep invoking Go sync.

Retain collector SQLite. Replacing it with JSONL would still require atomic
capture/checkpoints, exact saved request recovery, per-dataset acknowledgements,
locking and compaction; evaluate a journal only after a smaller state boundary
exists and equivalent crash semantics are measured.

Current schema/protocol contracts are in [design.md](../design.md). [ADR 0010](0010-current-contracts-and-boundaries.md) removes compatibility adapters and migrations.

## Consequences

Shared behavior remains DRY without coupling commands to listeners/storage.
Capabilities provide one enforceable feature policy. Dataset keys and scoped APIs
make isolation explicit while keeping a single transaction domain.

Authentication state shares the embedded database's availability/ownership limits.
There is no horizontal scaling guarantee. Container runtime includes native DuckDB
libraries, committed embedded assets and no JavaScript toolchain. Backends, queues,
organizations and public signup are deferred. Facts remain revisable when later
evidence changes interpretation; immutable evidence does not imply immutable totals.

Unresolved questions: none.
