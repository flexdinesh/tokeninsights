# Initial server bootstrap and background completion sync

Status: Accepted. Date: 13 October 2026. User requested implementation.

Account/token bootstrap and token-login decisions are superseded by [ADR 0016](0016-hosted-user-management.md). Deployment and background-plugin decisions remain current.

## Decision

Server startup requires only storage and HTTP listen configuration, not a domain,
canonical HTTPS origin or ingress setup. TLS is deployment-owned. Listen flags
and environment select the port, shared by readiness probing. An optional public
origin remains available for existing deployments; otherwise login/logout match
browser Origin host to request Host. HTTPS browser origins get Secure cookies;
HTTP origins can use the same HttpOnly/SameSite session flow. Forwarded host/proto
never grant identity or bypass same-origin protection.

Resolve ADMIN_ACCOUNT/ADMIN_TOKEN at the executable boundary. Bootstrap one stable
admin user/dataset and read+ingest credential before readiness through the shared
account adapter. Digest-only persistence reuses existing tables in both engines.
Startup is idempotent and resumes incomplete provisioning. Supplied token changes
replace the bootstrap digest on restart; existing sessions keep their lifetime.
Disable/revocation remain effective. This initial bootstrap adds no Basic auth,
public administrator role or user-management UI. Future management is separate.

Completion hooks delegate to durable background CLI sync and never wait for remote
acceptance. Codex/Claude launch enqueue in the shell background and return empty
JSON; Pi/OpenCode return control while observing an enqueue child. CLI-owned jobs
and detached finite workers own retry/ingestion, independently of harness lifecycle.
Plugin diagnostics contain no event or subprocess output. Host registration and
event delivery are assumptions verified later manually; tests never use installed
harnesses or modify user registrations.

Collector capture/journal writes use the existing writer lock. Delivery holds a
separate endpoint/database/dataset lock, releasing the capture lock around network
I/O. Local aliases of the same database/dataset share a delivery lock. Persisted
requests and validated acknowledgement semantics remain unchanged. A stalled
remote request cannot block capture or an independent local destination.

## Verification

Shared SQLite/PostgreSQL account contracts prove bootstrap, reopen, stable identity,
sessions and retained disable/revocation. HTTP tests prove plain HTTP login and
HTTPS cookie policy without an origin pin, and reject cross-origin attempts.
Real-image tests prove bootstrap login/read/ingest after container replacement.
Fake harness APIs and blocked executables prove nonblocking callbacks/hooks, safe
arguments and cleanup. Collector tests block remote HTTP while capture and local
delivery continue; existing replay, identity, receipt and accounting oracles remain.
All checks run through existing local gates. No schemas, wire contracts or CI changes.

Unresolved questions: none.
