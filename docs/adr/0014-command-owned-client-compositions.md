# Command-owned client compositions

Status: Accepted. Date: 11 October 2026. User approved implementation.

## Decision

Commands choose composition, independent of saved credentials. TUI and Web own
in-process capture, direct ingestion, processing and queries. Sync owns finite
authenticated hosted submission. Browse opens the configured hosted URL without
capture, storage, capability discovery or authentication preflight. Browser login
continues owning read authentication. Data maintenance stays local; the dedicated
server remains hosted-only and never collects.

This supersedes command/config selection, local sync handoff and distributed Web
in ADRs 0009–0010, plus the hosted default listen port in ADR 0013. Accounting,
storage, wire, dataset isolation and lifecycle guarantees from ADRs 0010–0013
remain. No database schema, read API or ingestion protocol changes.

## Configuration and boundaries

Group client preferences as collector (shared continuity/outbox path), in-process
(local token/account paths and bind), and distributed (URL/token). Use dotted config
keys. Flags override environment, then file, then defaults. Strict parsing rejects
unknown/null/duplicate fields recursively; runtime validation is scoped to the
command. Local viewers never consume remote credentials. Sync requires URL/token;
status and dry-run do not submit. Browse requires only URL. Flat configuration
rejects without mutation; explicit user regrouping replaces automatic conversion.
Mode settings, flags and environment selection are removed.

Executable boundaries resolve distinct typed local, sync and browse settings.
Composition constructs adapters and owns startup/shutdown. Core modules receive
explicit policy/dependencies; they do not infer mode or read deployment settings.
Hosted server flags/env/secret files remain independent of client preferences.

## Lifetimes and coexistence

Local viewers own no operational sync queue. Capture starts once in background;
restart the viewer to recollect. Full-refresh/source-dir are startup controls;
filters and Reload query only. Saved-only viewers resume processing. Cancellation
joins capture and processing before storage closes.

Distributed jobs retain their current storage/request shape, private credential
pipes, bounded retries and immutable receipts. Obsolete nonterminal local job
records remain stored and are reported as unsupported; they never dispatch remotely.
Evidence and destination bindings remain available for replay. Plugins are
distributed-only. Development fixtures use guarded internal local composition.

Local Web defaults to 8765; hosted native/image/Compose to 8766. Explicit occupied
ports fail. Personal and hosted token/account storage remain distinct. Capture/
delivery serialize through the shared collector lock; each endpoint/database/
dataset binding owns its acknowledgement progress. Each invocation has one
destination; no fan-out, daemon, timer or remote-to-local fallback.

## Verification

Prove command-scoped configuration, URL-only browse without network/storage,
local direct queries with distributed credentials, shared-outbox local/hosted
coexistence and separate totals/receipts on SQLite/PostgreSQL. Preserve replay,
restart, failure/cancellation, tenant isolation and accounting oracles. Dependency
guards exclude operational jobs from localruntime. Native/image health and
shutdown use the same listen defaults. Checks remain in existing local gates.

Unresolved questions: none.
