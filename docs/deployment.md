# Docker and hosted deployment

`tokeninsights-server` runs the foreground server without collecting harness data.
It serves the committed embedded dashboard and authenticated REST API. One container
owns writable DuckDB token data and paired SQLite application state. Collectors run
on client machines; the server has no harness mounts, collector daemon or analytics TUI.

## Build

```sh
docker build -t tokeninsights-server:local .
export TOKENINSIGHTS_PUBLIC_URL=https://usage.example.com
docker compose -f deploy/compose.hosted.yaml up -d --build
```

Remote startup defaults to hosted authentication. The unauthenticated personal
container composition is retired; use `tokeninsights web` for local foreground usage.
`TOKENINSIGHTS_PUBLIC_URL` or `--public-url` supplies the canonical HTTPS browser origin.

The Dockerfile pins multi-platform manifest digests for Go 1.26.8/trixie and
Debian trixie-20261005-slim, verified from the registry. Official build recipes
provide CGO's GCC/G++ toolchain; the runtime uses matching glibc plus libstdc++, CA
certificates, timezone data and curl for health checks. The image build checks
`ldd` for missing libraries and runs `--version`. See the primary
[Go image metadata](https://github.com/docker-library/official-images/blob/master/library/golang),
[Go image recipe](https://github.com/docker-library/golang/blob/master/1.26/trixie/Dockerfile)
and [Debian image metadata](https://github.com/docker-library/official-images/blob/master/library/debian).

Builds consume committed Go source/generated contracts/embedded browser assets.
No Node, npm or pnpm exists in the runtime image. Dashboard source edits must run
the repository web build first so its committed embedded assets match source.
Build natively for Linux amd64/arm64 with a compatible CGO toolchain; do not force
`CGO_ENABLED=0` or use an Alpine/musl runtime for this image.

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
to an account. Every hosted user has one dataset; all users share `/data/server.duckdb`; accounts/credentials live in `/data/app.sqlite`.

The equivalent native process is:

```sh
tokeninsights-server --listen 0.0.0.0:8765 \
  --server-db-path /var/lib/tokeninsights/server.duckdb \
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

The image runs as UID/GID `10001:10001`. `/data` contains shared storage and any
upgrade recovery files. Docker named volumes inherit the image's directory
ownership; an existing bind mount must be writable by that UID. The private
socket lives in `/run/tokeninsights`, which is recreated on container replacement.
Native foreground servers default to `DB_PATH.admin.sock`; `--admin-socket` overrides
it.
No host socket mount is needed when administering with `docker compose exec`.

`GET /healthz` reports process liveness. `GET /readyz` reports initialized server
storage/auth/routes without user metadata. Pending asynchronous processing alone
does not make the server unready. The image health check uses `/readyz` on internal
port 8765. Override the health-check URL if changing that port.

SIGTERM shuts down HTTP, joins processing and closes storage. Compose allows
30 seconds before forced termination. Observe shutdown before starting a new
owner; multiple replicas must not open the same writable DuckDB file. This
embedded ownership constraint follows [DuckDB concurrency](https://duckdb.org/docs/current/connect/concurrency).

Stop the container before file-level backups/upgrades. Preserve the complete data
directory, including `app.sqlite`, `server.duckdb`, WALs and retained upgrade
source/recovery files. App/token files are paired by database identity; restore them
together. Never copy only a live database while its WAL holds committed state. Keep separate personal and
hosted volumes. Verify backups by reopening a copy with the matching binary/kind.
A schema downgrade rejects; rollback uses the retained verified older file and
matching binary, not an older binary pointed at newly upgraded data.

Compose `down` retains named volumes. `down --volumes` deletes persistent history,
accounts and receipts; use it only when intentionally discarding that deployment.
There is no published image, automatic deployment, external queue or horizontal
scaling configuration in these examples.


Existing hosted DuckDB accounts migrate once into paired SQLite, preserving IDs,
credential digests, expiration and revocation. A completed migration marker prevents
stale DuckDB records restoring revoked credentials. Original account tables remain
read-only recovery sources. Stop before backup or binary rollback; keep a matched
pre-upgrade copy. Local user setup creates one default account automatically.

Application pairing also persists `<canonical-token-path>.application.json`, containing
only the application instance ID. Keep this guard with both databases in stopped
backups. A missing/replaced app database fails closed to preserve credential revocations. Restore the matched set; do not delete the guard to bypass recovery.
