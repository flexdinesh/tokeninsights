# Docker and hosted deployment

`tokeninsights-server` runs the foreground server without collecting harness data.
It serves the embedded dashboard and authenticated REST API. One container
owns token and account state in SQLite or PostgreSQL. Collectors run
on client machines; the server has no harness mounts, collector daemon or analytics TUI.

## Build

```sh
docker build -t tokeninsights-server:local .
# Optionally supply TOKENINSIGHTS_BOOTSTRAP_USERNAME and TOKENINSIGHTS_BOOTSTRAP_PASSWORD.
docker compose up -d --build
```

Remote startup defaults to hosted authentication. The unauthenticated personal
container composition is absent; use `tokeninsights web` for local foreground usage.
The server runs HTTP on the configured port without requiring a domain or TLS.
Ingress and TLS are deployment-owned; use HTTPS for external deployments.
`TOKENINSIGHTS_PUBLIC_URL` or `--public-url`
optionally pins the browser origin; otherwise browser authentication matches the
request Host and supports HTTP or HTTPS origins.

The Dockerfile pins Node, Go and Debian image digests. A build stage installs the
workspace dependencies with pinned pnpm and the frozen lockfile, then builds the
dashboard from source with `node tools/build/src/build-web.ts`; the image has no mise. Generated host assets and
`node_modules` are excluded from the build context; the embedded dashboard excludes
the development mock service worker. Go builds with `CGO_ENABLED=0`
and smoke-tests `--version`; runtime dependencies are CA certificates, timezone
data. The binary itself probes readiness; no curl is required. There is no C/C++ database library or JavaScript
runtime. Builds consume committed Go and dashboard source; build tools stay in the
build stages.

## Server configuration

Native and container deployments run the same foreground `tokeninsights-server`.
Flags override nonempty environment values; empty environment values use defaults.
Explicit empty flags clear environment defaults and still undergo validation.
Configuration is read once at startup. Restart to apply changes or rotate the
database secret. No config-file discovery or runtime reload is performed.

| Environment variable | Flag | Default / requirement |
| --- | --- | --- |
| `TOKENINSIGHTS_STORAGE_BACKEND` | `--storage-backend` | `sqlite`; alternatively `postgres` |
| `TOKENINSIGHTS_LISTEN` | `--listen` | `0.0.0.0:8766`, IPv4 address and port; overrides `TOKENINSIGHTS_SERVER_PORT` |
| `TOKENINSIGHTS_SERVER_PORT` | None | `8766`; also controls Compose port publishing |
| `TOKENINSIGHTS_PUBLIC_URL` | `--public-url` | Optional HTTP/HTTPS browser origin pin |
| `TOKENINSIGHTS_SIGNUP_ENABLED` | `--signup` | `false`; enable hosted username/password registration |
| `TOKENINSIGHTS_BOOTSTRAP_USERNAME` | None | Optional initial ordinary user; requires bootstrap password |
| `TOKENINSIGHTS_BOOTSTRAP_PASSWORD` | None | Initial password; mutually exclusive with password file |
| `TOKENINSIGHTS_BOOTSTRAP_PASSWORD_FILE` | None | Alternative readable password secret file |
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

Bootstrap password secret files are bounded to 64 KiB; a trailing LF or CRLF is
removed while password spaces are preserved. Passwords have no argv representation.
Supply username with exactly one password source, or omit all bootstrap settings.
Legacy `ADMIN_ACCOUNT`/`ADMIN_TOKEN` reject; they are no longer login or bootstrap
credentials. Compose exposes optional username/password and signup environment
settings; secret-file deployments must also mount the file read-only and provide its
container path.

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
  --listen 0.0.0.0:8766 --public-url https://usage.example.com \
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

## Same-machine SQLite server

Root `compose.yaml` includes the SQLite deployment with a stable `tokeninsights`
project name. Supply `TOKENINSIGHTS_BOOTSTRAP_USERNAME` and
`TOKENINSIGHTS_BOOTSTRAP_PASSWORD` through your process environment or secret manager,
then:

```sh
docker compose up -d --build
tokeninsights config set distributed.server-url http://127.0.0.1:8766
tokeninsights browse
```

Sign in with username/password. Open the profile icon in the top right, then create
a named API key in settings with read and ingest scopes. Copy its secret when shown:

```sh
tokeninsights config set distributed.server-token  # enter the API key
tokeninsights sync --wait
```

No Basic auth, domain, public URL, reverse proxy or TLS
configuration is required. Compose publishes only on host loopback. To change
ports, supply `TOKENINSIGHTS_SERVER_PORT=9000` to Compose and configure the client URL as
`http://127.0.0.1:9000`. The native binary also honors `TOKENINSIGHTS_SERVER_PORT`;
`--listen` and `TOKENINSIGHTS_LISTEN` override it. Healthcheck uses the same resolution.
Local `tui/web` remain in-process; remote `sync` shares collector continuity but
keeps independent delivery progress and never opens local token/account storage.

Startup optionally bootstraps one fixed ordinary user/dataset before readiness.
Usernames are trimmed, lowercase ASCII, 3–32 characters, beginning with an
alphanumeric character and continuing with alphanumerics, underscores or hyphens.
Passwords are 15–128 Unicode characters, including preserved whitespace, and stored
as salted Argon2id hashes. Repeated startup preserves identity, data and changed
passwords. Changing the environment password does not reset an existing password;
changing the bootstrap username conflicts. Disabled users remain disabled. Bootstrap
creates no key and grants no public administrator privileges. Deployments may omit
bootstrap and provision through the private socket.

Signup defaults off. Set `TOKENINSIGHTS_SIGNUP_ENABLED=true` or pass `--signup` to
allow registration. Settings are self-service only; there is no public admin UI.
Personal `tui/web` automatically use a default identity, hide account controls and
do not mount signup/login/settings routes.

## Optional ingress

A deployment may place a TLS reverse proxy in front of `127.0.0.1:8766`.
Provision certificates and proxy separately. Forward the original `Host` header
and preserve request path/body. No externally published admin endpoint is needed.

Within an existing TLS proxy's virtual host, an Nginx location can forward requests
as follows (certificate/server configuration remains deployment-owned):

```nginx
location / {
    proxy_pass http://127.0.0.1:8766;
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
or browser origin. An optional `TOKENINSIGHTS_PUBLIC_URL` pins the browser origin; otherwise the
request Host supplies same-origin matching;
proxy headers never grant account or dataset access.

```sh
# Optional origin pin for this TLS deployment:
export TOKENINSIGHTS_PUBLIC_URL=https://usage.example.com
docker compose up -d --build
```

The optional URL pins the browser origin, not the internal bind address.
Session cookies are Secure for HTTPS browser origins and support plain HTTP
origins; cookies remain HttpOnly/SameSite=Lax. Forwarded host/proto do not select
the origin. Cross-origin login/logout/signup and account mutations reject. The hosted database starts fresh. Do not reuse a personal
volume: stored server-kind mismatch rejects rather than assigning personal history
to an account. Every hosted user has one dataset; all users share `/data/server.sqlite`; accounts/credentials live in `/data/app.sqlite`.

The equivalent native process is:

```sh
tokeninsights-server --listen 127.0.0.1:8766 \
  --server-db-path /var/lib/tokeninsights/server.sqlite \
  --app-db-path /var/lib/tokeninsights/app.sqlite \
  --public-url https://usage.example.com \
  --admin-socket /run/tokeninsights/admin.sock
```

Provision through the running owner process's private mode-0600 socket. With the
Compose example:

```sh
docker compose exec tokeninsights \
  tokeninsights-server admin user create alice
```

Private user creation treats its name argument as a username and leaves password
login unavailable until a password is set. Copy the returned `userId`, then use
private recovery to set/reset its password. Supply the new password over stdin;
do not include it in command arguments:

```sh
docker compose exec -T tokeninsights \
  tokeninsights-server admin user reset-password USER_ID < /secure/new-password
```

Password reset invalidates every browser session for that user while leaving their
API keys active. Revoke keys separately if needed. The same command provides
operator-only recovery; email/password-reset links are not implemented.

Alternatively create an API key through the socket:

```sh
docker compose exec tokeninsights \
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
tokeninsights config set distributed.server-url https://usage.example.com
tokeninsights config set distributed.server-token
tokeninsights sync
tokeninsights browse
```

`config set distributed.server-token` reads stdin or prompts securely. Passing a token value
as an argument is rejected; `config get distributed.server-token` never prints it. The private
home config file stores credentials. `TOKENINSIGHTS_ACCESS_TOKEN` provides an
environment override. Endpoint/configuration errors never trigger local fallback.

`browse` opens the configured URL without syncing or requiring a token. `sync` uploads
independently; local `web` always serves in-process usage on port 8765. Browser password
login creates an opaque 24-hour HttpOnly/SameSite=Lax cookie (Secure for HTTPS).
Sessions authorize reads and the current user's password/key settings, but cannot
ingest or administer users. API keys authorize only their read/ingest scopes and
cannot manage accounts. A key's revocation blocks it immediately without ending
browser sessions. Password changes require the current password and invalidate all
browser sessions without revoking API keys. Logout revokes the current session;
user disable blocks all credentials. Key creation reveals a 256-bit secret once;
persistence holds only its digest. Settings list named-key metadata and allow
revocation; keys have no default expiry. Bearer tokens are not passed in URLs or
retained in browser localStorage. Hosted browsers query
processing lag but expose no collector-progress feature. Hosted TUI is rejected,
including `--sync=false`.

Public `GET /api/v2/auth` exposes server kind, login methods and signup availability
without user or dataset information. Authenticated `/api/v2/instance` still exposes
caller capabilities and permissions. Login/signup admission permits five attempts
per minute per client address and four concurrent password operations globally;
trusted-proxy policy determines that address as described above. Authentication
failures expose no credentials.

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

Hosted SQLite account schema is 3 and PostgreSQL account schema is 2; token schemas
remain 1. Older hosted account storage rejects without mutation or migration.
Personal SQLite account schema 2 remains compatible with existing local viewers.

Compose `down` retains named volumes. `down --volumes` deletes persistent history,
accounts and receipts; use it only when intentionally discarding that deployment.
There is no published image, automatic deployment, external queue or horizontal
scaling configuration in these examples.

## Deployment verification

Run `mise run test:container` after changing the image or deployment boundary.
It builds the actual image and checks both SQLite and PostgreSQL: non-root startup,
read-only root, an alternate listen port, readiness, private administration, real
client ingestion, receipts/totals after container replacement, revocation and clean
SIGTERM exit. It owns disposable networks, volumes and a pinned PostgreSQL container.
The pre-push hook runs this target; Docker is required. Native deployment tests exercise environment-only startup and
mounted secret files; HTTP boundary tests enforce trusted-proxy login isolation and
request-host origin protection, password bootstrap/login, signup policy, independent
sessions and keys, HTTP login and HTTPS cookie policy.

Future integrations should add typed settings at the executable boundary and
inject explicit adapters through hosted composition. OAuth/OIDC, provider discovery,
hot reload and generic integration registries are not implemented by this setup.

SQLite application pairing persists `<canonical-token-path>.application.json`,
containing only the application instance ID. Missing/replaced application storage
fails closed to preserve revocations; restore the matched set rather than deleting
the guard. PostgreSQL keeps equivalent pairing identities inside its namespaces.
