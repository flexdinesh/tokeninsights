# Remote connections

A connection selects one machine's personal SQLite backend. It does not upload or
merge evidence between machines. Requested transport/target, verified backend
hostname/identity and state remain visible per window. Failure never switches
transport or falls back to local data.

## SSH

Install CLI/backend on the source machine and a compatible viewer locally:

```sh
tokeninsights connect ssh://you@devbox
tokeninsights tui --connection ssh://you@devbox
tokeninsights web --connection ssh://you@devbox:2222
tokeninsights data wait --connection ssh://you@devbox
```

System SSH supplies authentication and host verification. It discovers or starts
the remote backend; no `remote` gateway is needed. Protocol versions must match.
If the CLI is outside remote PATH:

```sh
tokeninsights config set remote.command /home/you/.local/bin/tokeninsights
```

This locally configured executable path is never supplied by a connection string.
The target uses its own storage configuration. Local database path flags reject.
An SSH `--source-dir` override must be absolute and refers to the target machine.
Running bare `tokeninsights tui` inside an SSH shell still opens that machine's TUI.

## Temporary TCP access

On the machine holding the usage:

```sh
tokeninsights remote
tokeninsights remote --listen 0.0.0.0:9000 --advertise devbox.example
```

Run one command at a time. Default bind: `0.0.0.0:0`, an available network port.
Without `--advertise`, choose the SSH_CONNECTION server address, otherwise a
nonloopback interface, otherwise loopback. Override with a reachable host or
host:port; wildcard addresses cannot be advertised.

The command prints a shell-quoted `tokeninsights connect 'URI'` command. Copy it
to the viewing machine. The URI carries its address, temporary public-key pin and
access token:

```text
tokeninsights://connect/v1?address=HOST:PORT#pin=HEX_SHA256_SPKI&token=BASE64URL_SECRET
```

Use the complete URI, including fragment. `connect` opens the selected desktop;
`tui|web --connection 'URI'` selects the same target for those presentations.
The native client verifies the pinned TLS key before sending the credential and
authenticates every request. Same-host use still traverses TCP.

Keep the printed string private. It permits saved reads, progress, leases and
default Refresh; it cannot ingest evidence, override storage/source paths or run
maintenance. Stopping `remote` closes its connections and invalidates its strings.
Other local/SSH leases survive. Start a new gateway to obtain a new string.
No saved profile, account, pairing or durable credential is required.

## Browser and desktop boundaries

All browser dashboards use viewer-owned loopback bridges. Do not expose them with
a wildcard `web --host`; use the temporary TLS gateway for network access.
Browser JavaScript receives only redacted connection metadata, never the TCP token.
Refresh requires same-origin JSON and forwards only default capture.

MyGo/Electron launch specifications stay native. OS links contain only a private
rendezvous ID; launch succeeds after the selected desktop acknowledges its live
requested session. An unavailable app fails with repair guidance, without browser
fallback. See [CLI](cli.md) and [design](design.md).
