# Docker and hosted deployment

`tokeninsights-server` runs the foreground server without collecting harness data.
It serves the committed embedded dashboard and authenticated REST API. One container
owns token and account state in SQLite or PostgreSQL. Collectors run
on client machines; the server has no harness mounts, collector daemon or analytics TUI.

## Build

```sh
docker build -t tokeninsights-server:local .
export TOKENINSIGHTS_PUBLIC_URL=https://usage.example.com
docker compose -f deploy/compose.hosted.yaml up -d --build
```

Remote startup defaults to hosted authentication. The unauthenticated personal
container composition is absent; use `tokeninsights web` for local foreground usage.
`TOKENINSIGHTS_PUBLIC_URL` or `--public-url` supplies the canonical HTTPS browser origin.

The Dockerfile pins Go and Debian image digests. It builds with `CGO_ENABLED=0`
and smoke-tests `--version`; runtime dependencies are CA certificates, timezone
data and curl for health checks. There is no C/C++ database library or JavaScript
runtime. Builds consume committed Go source and embedded browser assets.

## Select storage

SQLite is the default, including the Compose file above. `--server-db-path` is
required; `--app-db-path` defaults to `app.sqlite` beside it. All local in-process
commands use SQLite regardless of the remote server's backend.

For PostgreSQL, provision an empty PostgreSQL **18** database. No extension is
required. The server role needs CONNECT and CREATE on that database and ownership
of the two application namespaces it creates. Use a dedicated database/role;
application clients never receive database credentials. Startup creates
`tokeninsights_data` and `tokeninsights_accounts` together. Existing incompatible or
partial namespaces fail without repair; unrelated namespaces remain untouched.

Supply a DSN through the process environment or deployment secret injection, never
as a command argument. Configure TLS in the DSN (`sslmode=verify-full` and a trusted
CA for network deployments). The server honors pgx connection/TLS parameters and
does not print the DSN. Do not put credentials in tracked Compose files.

```sh
# Set TOKENINSIGHTS_POSTGRES_DSN using your secret provider.
tokeninsights-server --storage-backend postgres \
  --listen 0.0.0.0:8765 --public-url https://usage.example.com \
  --admin-socket /run/tokeninsights/admin.sock

# Same server against an externally provisioned database:
docker compose -f deploy/compose.postgres.yaml up -d --build
```

`TOKENINSIGHTS_STORAGE_BACKEND` supplies the backend default; the flag overrides
it. PostgreSQL requires an explicit absolute admin socket and rejects SQLite path
options. SQLite rejects a supplied PostgreSQL DSN. Configuration/connection errors
never select another backend. Both token and account domains use the selected
engine, through separate interfaces and transaction boundaries.

## Hosted server

Provide a canonical HTTPS dashboard URL. A TLS reverse proxy on the host forwards
that origin to `127.0.0.1:8765`; provision certificates and proxy separately.
Forward the original `Host` header and preserve request path/body. No externally
published admin endpoint is needed.

```sh
export TOKENINSIGHTS_PUBLIC_URL=https://usage.example.com
docker compose -f deploy/compose.hosted.yaml up -d --build
```

The URL configures expected browser origin and secure session behavior, not the
internal bind address. The hosted database starts fresh. Do not reuse a personal
volume: stored server-kind mismatch rejects rather than assigning personal history
to an account. Every hosted user has one dataset; all users share `/data/server.sqlite`; accounts/credentials live in `/data/app.sqlite`.

The equivalent native process is:

```sh
tokeninsights-server --listen 0.0.0.0:8765 \
  --server-db-path /var/lib/tokeninsights/server.sqlite \
  --app-db-path /var/lib/tokeninsights/app.sqlite \
  --public-url https://usage.example.com \
  --admin-socket /run/tokeninsights/admin.sock
```

Provision through the running owner process's private mode-0600 socket. With the
Compose example:

```sh
docker compose -f deploy/compose.hosted.yaml exec tokeninsights \
  tokeninsights-server admin --admin-socket /run/tokeninsights/admin.sock user create Alice
```

Copy the returned `userId`, then create a token:

```sh
docker compose -f deploy/compose.hosted.yaml exec tokeninsights \
  tokeninsights-server admin --admin-socket /run/tokeninsights/admin.sock \
  token create USER_ID --scopes read,ingest
```

Token creation prints the secret once; persistent authentication storage retains
its digest. Hosted `user reprocess USER_ID` schedules reprocessing only for that
user's server-resolved dataset through the same socket. Optional `--expires RFC3339` limits token lifetime. Use `token revoke
TOKEN_ID` or `user disable USER_ID` through the same socket to block access. Tokens
can rotate without resetting dataset history or collector delivery progress.

On each collector machine:

```sh
tokeninsights config set mode distributed
tokeninsights config set server-url https://usage.example.com
tokeninsights config set server-token
tokeninsights sync
tokeninsights web
```

`config set server-token` reads stdin or prompts securely. Passing a token value
as an argument is rejected; `config get server-token` never prints it. The private
home config file stores credentials. `TOKENINSIGHTS_ACCESS_TOKEN` provides an
environment override. Endpoint/configuration errors never trigger local fallback.

Hosted `web` syncs with terminal progress and opens the browser. Browser token
login requires read permission and exchanges the token for an opaque 24-hour,
HttpOnly/Secure/SameSite=Lax cookie. Sessions are read-only; logout revokes the
session. User disable/source-token revocation invalidate access. Bearer tokens are
not passed in URLs or retained in browser localStorage. Hosted browsers query
processing lag but expose no collector-progress feature. Hosted TUI is rejected,
including `--sync=false`.

## Storage, health and shutdown

The image runs as UID/GID `10001:10001`. `/data` contains SQLite files when that backend is selected. Docker named volumes inherit the image's directory
ownership; an existing bind mount must be writable by that UID. The private
socket lives in `/run/tokeninsights`, which is recreated on container replacement.
SQLite foreground servers default to `DB_PATH.admin.sock`; `--admin-socket` overrides it. PostgreSQL requires the explicit socket path.
No host socket mount is needed when administering with `docker compose exec`.

`GET /healthz` reports process liveness. `GET /readyz` reports initialized server
storage/auth/routes without user metadata. Pending asynchronous processing alone
does not make the server unready. The image health check uses `/readyz` on internal
port 8765. Override the health-check URL if changing that port.

SIGTERM shuts down HTTP, joins processing and closes storage. Compose allows
30 seconds before forced termination. Observe shutdown before starting a new
owner. Both engines enforce one server owner. SQLite uses filesystem ownership;
PostgreSQL uses a dedicated session advisory lock. All PostgreSQL writes execute
on that session; connection loss stops the server, and the writer never reconnects.
Read pools may reconnect. Do not deploy multiple replicas against one database.

Stop the server before backups or restore. For SQLite, preserve the complete matched
set: `app.sqlite`, `server.sqlite`, their WALs, and the token `.application.json`
pairing guard. Never copy only a live main file while its WAL contains commits.
For PostgreSQL, back up and restore both namespaces together using one consistent
database backup, preserving both metadata identities. Restore into a separate
database and verify with the matching binary before switching deployment.
Keep personal and hosted storage separate. Incompatible schemas reject; there are
no migrations, old-engine imports, automatic resets or downgrade readers. Existing
obsolete database files are never deleted by startup.

Compose `down` retains named volumes. `down --volumes` deletes persistent history,
accounts and receipts; use it only when intentionally discarding that deployment.
There is no published image, automatic deployment, external queue or horizontal
scaling configuration in these examples.


SQLite application pairing persists `<canonical-token-path>.application.json`,
containing only the application instance ID. Missing/replaced application storage
fails closed to preserve revocations; restore the matched set rather than deleting
the guard. PostgreSQL keeps equivalent pairing identities inside its namespaces.
