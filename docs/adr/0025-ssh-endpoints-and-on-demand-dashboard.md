# SSH endpoints and on-demand local dashboard listener

Status: Accepted. Date: 3 November 2026. User approved the execution plan.

## Decision

The personal backend stays private and lease-scoped. Local clients use its
user-private Unix socket. SSH is the only remote transport for a personal
backend; authenticated TCP remains the hosted server's job. No TCP control API,
personal-mode authentication, long-running daemon, or database schema change is
introduced.

This supersedes, from ADR 0024, the always-bound public listener and listener
host/port as backend compatibility settings; and, from the design contract,
"Remote TUI is unavailable" for personal backends reached over SSH. Hosted
composition, accounting, receipts, dataset isolation and storage contracts are
unchanged.

### On-demand dashboard listener

Backend compatibility covers storage identity only. Each viewer lease is a
control lease or a dashboard lease. TUI, MyGo and maintenance hold control
leases and bind no TCP port. `web`, Electron, browser tabs and the remote web
proxy hold dashboard leases that request a host/port. The first dashboard lease
binds the public read-only listener; equal requests share it; a different
request while bound rejects as a settings mismatch. After the last dashboard
lease and the reconnect grace the listener drains; a request racing the drain
waits, then binds again. Browser tabs lease only the binding serving them.
The descriptor reports the current dashboard URL and listen request, or none.
Each dashboard lease stream reports its URL as the first event. The private
socket also accepts prior-knowledge HTTP/2 and serves the read-only dashboard
and API v2 behind the same instance check, so private bridges need no TCP
listener. The local-client protocol is version 2; clients and backends require
exact equality and name both versions on mismatch.

### SSH endpoints

`--endpoint`, `TOKENINSIGHTS_ENDPOINT` and config `remote.endpoint` select
`local` (default) or `ssh://[user@]host[:port]`; flags win over environment,
then file. `--endpoint=local` forces local for one invocation. The client runs
the system `ssh` with `<remote.command|tokeninsights> system dial-stdio`; the
remote command resolves its own configuration, attaches to or starts its
backend under the startup lock, checks compatibility, then splices stdio to the
private socket. One SSH process carries the lease, queries and capture over
HTTP/2; its exit releases the lease, and the session never reconnects silently.
TUI, MyGo, `data reprocess|wait` and `web` accept remote endpoints. Remote `web`
binds a local loopback proxy that forwards only read-only dashboard requests and
browser leases, and opens the browser only. Electron stays local. Remote
failures never fall back to local storage. Remote `--source-dir` must be
absolute and is interpreted on the remote machine. Local storage path flags
reject with a remote endpoint.

## Tradeoffs and verification

SSH reuses host keys, agents, ProxyJump and user configuration without a new
credential system or open port. Non-interactive PATH may omit the installer's
link directory; `remote.command` overrides it. Exact protocol equality makes
skew a clear error. Listener churn after the last dashboard lease favors
minimal exposure; reload relies on the reconnect grace.

Prove the listener state machine, shared and conflicting requests, occupied
ports, unbind after the last lease, private HTTP/2 dashboard access and
instance pinning with real storage. Prove SSH sessions with a fake `ssh`
executable against real backend processes, plus a real OpenSSH container in the
deployment validation group. Preserve ADR 0024 lifetime, capture and parity
guarantees.

Unresolved questions: none.
