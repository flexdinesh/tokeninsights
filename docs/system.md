# System design

[ADR 0008](adr/0008-personal-hosted-composition-and-capabilities.md) defines
personal/hosted composition. [Design](design.md) specifies storage, identity,
processing and query contracts; [deployment](deployment.md) covers containers.

## Shared behavior, explicit composition

A collector reads Durable Sources, extracts whitelisted native evidence, commits
capture/checkpoints in SQLite and delivers immutable saved requests. The server
accepts evidence/receipts/work atomically, interprets outside its write transaction,
and publishes revision-fenced facts/estimates/outcomes atomically. Evidence and
receipts are immutable; counted facts are replaceable projections.

    Sources -> capture -> collector state -> delivery -> scoped acceptance
      -> pure processor -> scoped facts/estimates -> analytics -> REST -> viewers

CLI workflows own destination resolution and collection. HTTP adapters own routes,
transport validation and principal resolution. Runtime owns storage, workers,
listeners and shutdown. The same processor and analytics implement every server
composition. `pipeline` owns capture, `rawcollectorstore` collector state and
`collector` delivery; `clientworkflow` supplies destination transport. Pure
`processor` interprets, `dataengine` orchestrates work/retries and `datastore`
implements DuckDB persistence. `analytics` supplies query contracts/SQL, while
`server`/`ingestionhttp` adapt HTTP. `serverruntime` owns lifecycle. Neither a server
nor browser Reload starts collection.

| Composition | Data | Access | Viewers/progress |
| --- | --- | --- | --- |
| Managed personal | One dataset, one DuckDB file | Public query/assets; private Unix ingestion/control | TUI/web; sanitized browser collector progress |
| Foreground personal | One dataset, one DuckDB file | HTTP query/ingestion; private administration | TUI/web; terminal collector progress |
| Hosted | One shared DuckDB file; one isolated dataset per user | Bearer ingestion/queries; read-only browser session; private admin socket | Web; terminal collector progress only |

Kinds are `personal` and `hosted`; personal is the default. Location does not
choose kind: a personal server may be remote, and a hosted server may be reachable
through localhost. A database's stored kind must match the requested composition.

## Capabilities and permissions

`GET /api/v2/instance` exposes kind, dataset identity and capabilities. A shared
typed policy controls mounted routes, CLI preflight and browser behavior. Unknown
capability names are ignored; missing required features and unknown kinds fail.
Client configuration is an expectation checked against the descriptor.

Capabilities are `usage`, `facets`, `web-dashboard`, `raw-ingestion`,
`terminal-dashboard`, `collector-progress` and `reprocess`. Hosted forbids terminal
dashboard and collector progress. Managed personal advertises collector progress
only with its private publisher and public read components. Reprocessing uses a
separate administration transport.

Caller permissions (`read`, `ingest`) authorize operations independently of these
features. Dataset identity is server-assigned from authentication, never trusted
from client query parameters. Receipt identity and raw protocol 3 bind the request
to that dataset; all lookups/joins/processing/query snapshots share the scope.

## Client workflows

Client preferences live in `${XDG_CONFIG_HOME:-~/.config}/tokeninsights/config.json`.
Flags > environment > file > defaults. `server-kind` defaults to `personal`;
`server-url` selects the destination; `server-token` supplies hosted credentials.
`host`/`port` bind the managed local listener. Explicit empty URL selects local
only with valid personal preferences. Hosted requires URL and token.

`sync` negotiates the destination, captures and submits all harnesses by default.
`--dry-run` stays source-only; `--publish-only` retries retained evidence.
`tui` performs the same workflow in its loading screen, then queries personal
analytics; hosted rejection occurs before collection. `web` syncs by default;
`--sync=false` opens saved usage. Managed personal web opens early to show progress;
hosted web syncs in the terminal before opening browser login. No CLI token is
transferred in browser URLs. Failed browser launch prints a usable URL.

Bare invocation ensures/reports local personal service or reports the configured
endpoint. Remote failure never starts local. `service` always manages local
personal composition, regardless of client routing. Bind changes require explicit
restart; `web --host` applies only to managed local composition. Plugins invoke
this same Go `sync`, without separate collection behavior.

Progress describes capture/submission attempts, with leases and bounded retention.
It travels through the private owner socket and exposes sanitized status only.
Source paths, native content and credentials never enter progress. Delivery success
means durable acceptance; asynchronous processing lag remains distinct. Queries
refresh published revision without waiting for every user's global queue.

## Hosted ownership and deployment

One foreground process owns one writable DuckDB file and worker. Account creation
commits user/dataset together. Users may rotate/revoke multiple tokens without
changing dataset identity. Admin-created tokens have read/ingest scopes. Browser
login exchanges a read token for an opaque, read-only HttpOnly/Secure/SameSite
session; token revocation or user disable also invalidates dependent sessions.

Canonical HTTPS public URL supplies the expected browser origin. TLS proxy and
external supervision belong to deployment. Private mode-0600 admin socket controls
provisioning/reprocessing through the running owner process. No second writable
process, public admin listener, per-user database, external queue or replica model.

Collector SQLite 18 retains evidence, continuity and exact delivery state. Server
DuckDB 2 retains accounts and dataset-scoped evidence, receipts, processing and
analytics. Verified personal upgrades/imports preserve identities, bytes, totals
and recoverable source files. Personal history is not automatically assigned to a
hosted account. See [design](design.md) for migration rules.

JSONL collector replacement, organizations, signup/OIDC, alternate backends,
compaction and horizontal scaling remain out of scope.
