# Host collector and normalized server ingestion

Status: **Accepted**. Date: 5 October 2026. Implementation: PR #53.
Supersedes collection ownership, mixed storage and refresh/viewer boundaries in
[ADR 0005](0005-persistent-service-and-explicit-refresh.md).

## Context

The prior service could discover machine-local harness files, capture raw facts,
normalize them and serve analytics from one SQLite file. A remote server cannot
read each producer machine's files. Sharing collector parsing with server query
state also made ingestion failures and duplicate accounting hard to isolate.

The desired primary experience remains manual collection and a local dashboard.
Future central servers should accept multiple producer machines without requiring
a collector-state copy or duplicate contributions after producer reinstall.
The implementation must make durable failure boundaries independently testable.

## Decision

The native Go binary has a host collector, a managed local server, a foreground
remote-ready server composition and HTTP query clients. Local/remote share the
same ingestion/query core; selecting a remote URL does not start or relay through
a local server. Source discovery, adapters, ancestry, continuity, normalization
and diagnostics belong entirely to the collector. Server startup and viewers do
not collect.

Create fresh `collector.sqlite` and `server.sqlite`; leave the old
`tokeninsights.sqlite` untouched. No legacy import or identity-alias migration is
part of this decision. Independent SQLite application IDs and supported versions
prevent wrong-role opening/recovery. Collector/server paths cannot alias,
including symlinks and existing hard links. Server history never uses producer
reset/rebuild behavior.

Only collector schema 16 and server schema 2 are supported. Previous schemas
reject without mutation; there are no metadata migrations or schema-reset
fallbacks. Older data generations within current collector schema can rebuild
from retained sources; pending current-generation rebuilds resume with the same
scope, and newer generations reject. Explicit collector resets accept only
current-role/current-schema storage or a brand-new empty file.

Collector SQLite retains metadata-only raw and normalized facts. Canonical
normalization and publication journal snapshots commit together. An immutable
pending request is saved before delivery. Each destination has its own durable
server binding, pending batch and contiguous acknowledgement cursor. A manual
`sync` collects, normalizes and delivers; `sync --publish-only` resumes delivery.
Failed collection does not discard committed publication; failed delivery does
not discard local normalized work.

Server SQLite contains normalized canonical query tables, durable database
identity, revision, producer labels and committed receipts. One database binds
one owner, initially `default`; several machines may publish for that owner.
The server recomputes stable session/message/fact/location IDs from deterministic
JSON-array SHA-256 tuples. Fact identity uses harness/native session/message/
request/scope, independently of installation, stream, batch, hostname or local
row IDs. Copied histories and a recreated collector therefore dedupe.

Codex retains its adapter-derived immutable snapshot witness rather than
pretending it has universal native request revisions. Missing stable evidence is
withheld with diagnostics. Only Claude native message/request facts with source
timestamp evidence use `claude-source-timestamp-v1`: newer replaces one
contribution, older is a stale no-op, equal timestamp with changed value conflicts.
Other changed values conflict. Arrival time and collector sequence do not decide
source precedence. No retraction or backflow is introduced.

Ingestion validates strict allowlisted normalized JSON and commits the complete
batch plus receipt atomically. Exact stream/batch replay returns its original
receipt; changed bytes under that identity conflict. Fact dedupe is separate from
batch replay. Success means committed and queryable, not accepted into an
asynchronous inbox. The collector advances only after validating receipt identity,
range, exact request hash and counts. Unknown response outcomes retain exact
request bytes for a later manual retry.

Admission is bounded at four concurrent requests with one SQLite connection.
Batches are at most 1 MiB/256 facts; strings at most 256 UTF-8 bytes. Protocol
integers and countable aggregate components/totals remain within the exact
JavaScript safe-integer domain. Protocol/identity/semantics versions are all 1
and require exact support; incompatible versions return 422 without mutation.
Journal and receipts are retained indefinitely initially.

TUI and browser query REST snapshots. `tui` is read-only by default, with
`--sync` explicit. TUI reload and browser Reload only fetch saved data. Public
sync POST and private collection actions are removed; sync GET remains read-only
readiness/revision status. Instance/database/revision metadata guards multi-page
snapshots. Producer labels and last ingestion describe saved available data,
never source completeness. Server reporting timezone governs client displays;
IANA names preserve historical DST where available. TPS concepts remain intact.

Root commands are `service`, `sync`, `tui` and `server`; advanced producer
maintenance is grouped under `collector normalize`, `collector reset-canonical`
and `collector reset-all`. Previous commands and `--db-path` / `--no-sync` are
removed. Collector maintenance does not delete server history.

Retain detached native startup, lifecycle/admission locks, private directories,
private Unix administration, occupied-port checks and ownership protection.
Loopback is default. Every non-loopback bind requires a token, irrespective of
local/remote command. Public assets and APIs accept configured authentication;
remote TLS/setup/credential provisioning and multi-tenant authorization follow
later. Browser assets remain prebuilt/embedded; production requires no Node.

Initial Codex, Claude, Pi and OpenCode adapters are thin bounded invocations of
the same collector CLI. They do not parse, normalize or maintain a separate
trigger/delivery queue. Their 60-second host deadline permits interruption;
retained durable work resumes on manual sync.

## Consequences and verification

The collector takes on persistence and delivery state; the server becomes
independent of machine-local source formats. Two local databases and indefinite
journal/receipt retention cost disk space. Synchronous ingestion gives a precise
commit acknowledgement and bounded overload failures without another inbox.
Conflicts preserve existing accounting and stop the unacknowledged suffix for
diagnosis rather than inventing precedence.

Reproducibility requires retained sources and ancestry under supported rules.
Missing identities, collisions within the fixed namespace, corruption and lost
undelivered sources are not repaired by retries. A local replacement database
creates a new binding and journal replay; remote replacement at the same URL
requires a deliberate binding/configuration change.

[Architecture](../collector-server-architecture.md) supplies stable G01–G11;
[failure contract](../collector-ingestion-tests.md) supplies F01–F14 and fixtures.
Tests must verify identities, each component/total, references, durable receipts,
cursor/revision and REST results with real SQLite/HTTP. Named crash boundaries
cover capture, canonical/journal commit, batch preparation, server commit and
collector acknowledgement. Actual coverage and unresolved failures are recorded
in [VALIDATION](../../.scratch/collector-ingestion/VALIDATION.md), rather than
inferred from acceptance of this ADR.

Future work includes retention/compaction, explicit migrations, profile
namespaces, remote provisioning, richer timing and optional asynchronous hooks.
No unresolved decisions remain for the accepted implementation scope.

### TUI startup update

`tui` now runs collection and publication in a loading screen before querying
the dashboard. `tui --sync=false` reads saved data only. Dashboard Reload remains
GET-only. Startup failures offer Retry, View saved data, and Quit; acknowledged
work remains durable. See [design](../design.md) for the current command contract.
