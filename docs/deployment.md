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
data. The binary itself probes readiness; no curl is required. There is no C/C++ database library or JavaScript
runtime. Builds consume committed Go source and embedded browser assets.

## Server configuration

Native and container deployments run the same foreground `tokeninsights-server`.
Flags override nonempty environment values; empty environment values use defaults.
Explicit empty flags clear environment defaults and still undergo validation.
Configuration is read once at startup. Restart to apply changes or rotate the
database secret. No config-file discovery or runtime reload is performed.

| Environment variable | Flag | Default / requirement |
| --- | --- | --- |
| `TOKENINSIGHTS_STORAGE_BACKEND` | `--storage-backend` | `sqlite`; alternatively `postgres` |
| `TOKENINSIGHTS_LISTEN` | `--listen` | `0.0.0.0:8765`, IPv4 address and port |
| `TOKENINSIGHTS_PUBLIC_URL` | `--public-url` | Required canonical HTTPS origin |
| `TOKENINSIGHTS_SERVER_DB_PATH` | `--server-db-path` | Required for SQLite; forbidden for PostgreSQL |
| `TOKENINSIGHTS_APP_DB_PATH` | `--app-db-path` | SQLite: `app.sqlite` beside token database |
| `TOKENINSIGHTS_ADMIN_SOCKET` | `--admin-socket` | SQLite: beside token database; PostgreSQL: required absolute path |
| `TOKENINSIGHTS_TRUSTED_PROXIES` | `--trusted-proxies` | Empty: trust no forwarded client addresses; comma-separated CIDRs |
| `TOKENINSIGHTS_POSTGRES_DSN` | None | PostgreSQL connection secret |
| `TOKENINSIGHTS_POSTGRES_DSN_FILE` | None | Alternative: path to a readable secret file |

The image sets the admin socket to `/run/tokeninsights/admin.sock`. The `admin`
subcommand also honors `TOKENINSIGHTS_ADMIN_SOCKET`. Settings and proxy CIDRs
validate before opening listeners or storage; storage validation then rejects
incompatible databases. Startup failure exits nonzero without backend fallback.
`--version`, `--help` and `healthcheck` do not require database credentials.

Use exactly one nonempty PostgreSQL secret source. Secret files may end with a
newline; whitespace is trimmed. Empty, unreadable or oversized files (over 64 KiB)
reject. Neither secret contents nor the secret-file path appear in read errors.
DSNs have no command-line flag and must never be logged.

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
export TOKENINSIGHTS_POSTGRES_DSN_FILE=/secure/tokeninsights/postgres-dsn
docker compose -f deploy/compose.postgres.yaml up -d --build
```

For this Compose example, `TOKENINSIGHTS_POSTGRES_DSN_FILE` names a **host** file;
Compose mounts it read-only at `/run/secrets/postgres_dsn` and supplies that
**container** path to the binary. Provision the file with your secret manager,
readable by UID 10001 (for example, owned by 10001 with mode 0400). Compose file
secrets retain host file permissions. Do not commit the file or put its contents
in `.env`. PostgreSQL credentials are not embedded in the image or Compose YAML.

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

Within an existing TLS proxy's virtual host, an Nginx location can forward requests
as follows (certificate/server configuration remains deployment-owned):

```nginx
location / {
    proxy_pass http://127.0.0.1:8765;
    proxy_set_header Host $http_host;
    proxy_set_header X-Forwarded-For $remote_addr;
    client_max_body_size 1m;
}
```

At the public edge, **overwrite** incoming `X-Forwarded-For`; do not allow callers
to choose their client address. Set `TOKENINSIGHTS_TRUSTED_PROXIES` to only the
actual proxy peer CIDR seen by the server. A native loopback proxy can use
`127.0.0.1/32`; Docker forwarding may present the bridge gateway instead. Determine
that address for your host/network; do not assume loopback or trust all private
networks. Keep the published port bound to host loopback as in these examples.

With no trusted proxies, login limits use the socket peer, so proxied users share
that limit. With explicit trust, the server walks `X-Forwarded-For` right to left
through trusted hops and uses the first untrusted address. Missing, malformed or
overlong chains fall back to the socket peer. At most 32 hops are accepted.
`Forwarded`, `X-Real-IP`, forwarded host and forwarded scheme do not select identity
or browser origin. `TOKENINSIGHTS_PUBLIC_URL` remains the browser origin authority;
proxy headers never grant account or dataset access.

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
tokeninsights-server --listen 127.0.0.1:8765 \
  --server-db-path /var/lib/tokeninsights/server.sqlite \
  --app-db-path /var/lib/tokeninsights/app.sqlite \
  --public-url https://usage.example.com \
  --admin-socket /run/tokeninsights/admin.sock
```

Provision through the running owner process's private mode-0600 socket. With the
Compose example:

```sh
docker compose -f deploy/compose.hosted.yaml exec tokeninsights \
  tokeninsights-server admin user create Alice
```

Copy the returned `userId`, then create a token:

```sh
docker compose -f deploy/compose.hosted.yaml exec tokeninsights \
  tokeninsights-server admin \
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
The Compose root filesystem is read-only; `/run/tokeninsights` and `/tmp` are
ephemeral writable tmpfs mounts. Only the SQLite example mounts persistent `/data`;
PostgreSQL persistence belongs to the external database deployment.
SQLite foreground servers default to `DB_PATH.admin.sock`; `--admin-socket` overrides it. PostgreSQL requires the explicit socket path.
No host socket mount is needed when administering with `docker compose exec`.

`GET /healthz` reports process liveness. `GET /readyz` reports initialized server
storage/auth/routes without user metadata. Pending asynchronous processing alone
does not make the server unready. The image runs `tokeninsights-server healthcheck`,
which probes `/readyz` using `TOKENINSIGHTS_LISTEN` (wildcard maps to loopback).
It has a three-second timeout, bypasses HTTP proxy environment variables, rejects
redirects and exits nonzero unless ready. It never opens storage or reads secrets.
When overriding the server port with a flag, also set the probe to
`tokeninsights-server healthcheck --listen 127.0.0.1:PORT`. Prefer the environment
setting so both processes share it; update port publishing to match.

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

## Deployment verification

Run `pnpm run test:container` after changing the image or deployment boundary.
It builds the actual image and checks both SQLite and PostgreSQL: non-root startup,
read-only root, an alternate listen port, readiness, private administration, real
client ingestion, receipts/totals after container replacement, revocation and clean
SIGTERM exit. It owns disposable networks, volumes and a pinned PostgreSQL container.
The pre-push hook runs this target; Docker is required. Native deployment tests exercise environment-only startup and
mounted secret files; HTTP boundary tests enforce trusted-proxy login isolation and
unchanged origin protection.

Future integrations should add typed settings at the executable boundary and
inject explicit adapters through hosted composition. OAuth/OIDC, provider discovery,
hot reload and generic integration registries are not implemented by this setup.

SQLite application pairing persists `<canonical-token-path>.application.json`,
containing only the application instance ID. Missing/replaced application storage
fails closed to preserve revocations; restore the matched set rather than deleting
the guard. PostgreSQL keeps equivalent pairing identities inside its namespaces.
