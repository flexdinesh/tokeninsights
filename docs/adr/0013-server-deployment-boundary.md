# Server deployment boundary

Command selection/config and local handoff are superseded by [ADR 0014](0014-command-owned-client-compositions.md); retained domain/storage contracts still apply.

Status: Accepted. Date: 10 October 2026.

## Decision

Optimize first deployment for one Linux host, Docker Compose and a TLS reverse
proxy. The dedicated Go server binary is the native executable and container
entrypoint. It never collects local harness data. Storage remains SQLite or
PostgreSQL through the separate token/account contracts in ADR 0012.

Executable code resolves flags, environment defaults and mounted secret files
into typed settings before composition opens resources. Flags win; empty env
values use defaults. Configuration is startup-only. Secrets have no flags or
diagnostic representation. Direct DSN and DSN_FILE conflict; bad files reject.
Adding an integration means explicit typed configuration and an injected adapter,
not environment reads in domain code or a generic plugin registry.

Hosted composition owns proxy policy at its public HTTP boundary. Trust no proxy
by default. Explicit CIDRs permit X-Forwarded-For traversal from the socket peer
through trusted hops to the first untrusted address. Malformed/absent chains and
more than 32 hops fall back to the socket peer. The proxy edge must overwrite
untrusted incoming headers. Accounts consume a resolved client address for login
admission, never proxy configuration. Forwarded headers cannot select browser
origin, authenticate users or select datasets. Local and private-admin boundaries
do not inherit hosted proxy trust.

The image runs non-root with a read-only Compose root, ephemeral runtime mounts,
and backend-specific persistence. It contains the same binary, embedded assets,
CA certificates and timezone data. Its healthcheck subcommand probes readiness
over HTTP without reading secrets or opening databases. Env-configured listen
ports apply to server and probe. SIGTERM drains the existing shared lifecycle;
one server owns either database backend.

## Contracts and scope

Configuration tests prove precedence and pre-open rejection. HTTP tests prove
proxy address isolation, spoof resistance and unchanged origin policy. Native
deployment fixtures use env-only startup. Real-image contracts repeat ingestion,
receipts, totals, private admin, replacement durability, revocation, alternate-port
readiness and SIGTERM on both backends. Dependency guards keep proxy handling out
of account/storage code. All locally reproducible checks, including real-image
contracts, run in pre-push. CI additions require documented coverage unavailable
locally; native OS/architecture verification is the current exception. No schema
or wire contract changes are needed.

OAuth/OIDC implementation, dynamic reload, image publishing, automatic deployments,
replicas and server tuning remain separate product work. This milestone adds the
deployment boundary those features can use without introducing speculative APIs.
