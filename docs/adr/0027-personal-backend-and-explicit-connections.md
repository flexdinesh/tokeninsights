# Personal backend and explicit connections

Status: Accepted. Date: 5 November 2026. User approved the redesign, breaking
CLI/config cleanup and retired product/schema removal.

## Context

Hosted accounts, distributed publication/jobs, PostgreSQL and deployment products
obscured the personal usage workflow. Viewer lifetime and dashboard listeners were
mixed with backend compatibility. Desktop URL handoff could not safely carry
self-contained connection credentials or prove the requested app was ready.

## Decision

One personal Go backend per storage identity owns SQLite capture, direct acceptance,
durable processing, queries and shutdown. Desktop, built on MyGo, is the only desktop product; TUI
and explicit web remain. Desktop installation includes CLI/backend; the CLI-only
product remains. `tokeninsights-backend` exposes only `internal-serve`.

Bare `tokeninsights` opens desktop. `connect STRING` opens exactly
local/SSH/TCP; `tui|web --connection STRING` explicitly selects a target. No saved
remote default, transport fallback, local-data fallback or browser fallback.
An unavailable desktop fails with actionable guidance.

There is no desktop selector: `--open` on bare/`connect` and `viewer.open` are
removed; `web --open` stays boolean. A saved `viewer` config block fails with
obsolete-config manual repair guidance, file unchanged.

Local sessions use private Unix HTTP/2 protocol 3, requiring exact equality.
The descriptor carries protocol, instance ID and collector/token settings only.
There is no backend dashboard listener or listener compatibility state.
Viewer composition owns loopback dashboard bridges. Each session/window holds a
lease; final disconnect plus grace stops admission, joins capture/processing and
closes storage. Compatible startup is serialized. Accepted work resumes on reopen.

Capture runs on launch and explicit Refresh, coalescing overlap and bounded stronger
follow-up. No periodic scanning or completion hooks. `--capture=false` skips launch
capture while processing resumes. Filters, TUI reload and browser reload query saved
usage only. Refresh completion requires visibility and a current successful query.

SSH uses system SSH and may discover/start the target backend without a gateway.
TCP requires foreground `remote`, binds `0.0.0.0:0` by default and supports
`--listen`/`--advertise`. Print a shell-quoted connect command containing:

```text
tokeninsights://connect/v1?address=HOST:PORT#pin=HEX_SHA256_SPKI&token=BASE64URL_SECRET
```

TLS identity and token are ephemeral. Verify the pin before submitting the token;
authenticate every request, including descriptor/private round trips. Pin one backend
instance; redact storage paths. Permit reads, progress, leases and default capture;
reject ingestion, path overrides and maintenance. Same-host use stays TCP.
Stop closes gateway connections and invalidates strings; other local/SSH leases survive.
No pairing, saved profiles or durable credentials.

Loopback bridges provide redacted connection metadata and same-origin JSON
`POST /api/viewer/refresh` for default capture only. Browser JavaScript receives
no TCP secret. Native desktop rendezvous keeps launch specifications/credentials on
a private Unix socket; OS URLs contain only rendezvous IDs. Return after the selected
desktop acknowledges a live requested session, not OS opener success.
Each window displays requested transport/target, verified backend identity and state.

Remove hosted accounts/auth/UI, PostgreSQL, external ingestion/sync/jobs, completion
plugins, server installers and Docker deployment products. Retain local Dagger
validation and real SSH/network tests. `data reprocess|wait` remains local/SSH only.
Retired settings return safe actionable errors without values or rewrites.

Collector schema 20 and SQLite token schema 1, paths, history, stable IDs, evidence,
receipts and recovery remain unchanged. Historical SQL dataset/kind/destination
fields remain contract details. Approved removal includes application/account/jobs
schemas and pairing code, but existing user files remain untouched. No migration,
import or deletion of user files. Metadata-only privacy remains mandatory.

## Superseded decisions

This supersedes hosted/distributed/account/PostgreSQL product decisions in ADRs
0007 through 0016; generated/tap distribution in ADRs 0001, 0002, 0017 and 0019;
account/jobs portions of ADR 0020; and conflicting command, dashboard listener,
transport, opener and product choices in ADRs 0023 through 0026.
ADRs remain historical records. Preserve their applicable accounting, semantic
boundary, transaction/replay, identity, snapshot, ownership and cancellation rules.
ADR 0018's aggregation distinction and ADRs 0021/0022's local task/engine principles
remain, with retired product coverage superseded here.

## Verification and consequences

Preserve five token components, session identity, ancestry ambiguity, excluded
estimates, provider inference, exact receipt replay and replacement generations.
Use real SQLite rollback/reopen/concurrency contracts and independent accounting
oracles. Prove local/SSH/TCP query and Refresh parity, real OpenSSH/network access,
same-host TCP, bad token/pin/expiry/protocol rejection, shared startup/final leases,
native secret handoff/acknowledgement, unavailable apps and installed product boundaries.

Format/lint first. Final validation requires full local `check:pr` and docs build on
clean committed HEAD. Keep managed Dagger and
dedicated caches; never interfere with another invocation or poll long checks.

Unresolved questions: none.
