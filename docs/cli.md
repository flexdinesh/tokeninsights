# CLI reference

`tokeninsights --help` lists commands. Bare invocation opens the desktop
app. The default install includes it; `--cli-only` installs use `tui`/`web`.

## Viewers

```sh
tokeninsights
tokeninsights connect local
tokeninsights connect ssh://you@devbox
tokeninsights tui
tokeninsights tui --capture=false
tokeninsights web
tokeninsights web --open=false
tokeninsights web --host 127.0.0.1 --port 9000
```

An unavailable desktop fails; no browser fallback. Launch returns after the app
acknowledges a live requested session. `web --open` is boolean: true opens the system browser,
false prints its loopback URL. Web's default port is 0 (available port); host must
be loopback. The command stays foreground and Ctrl+C releases its lease.

Viewer flags: `--capture` (default true), `--full-refresh`, `--source-dir`,
`--connection`, `--collector-db-path`, `--server-db-path`.
`--full-refresh` requires capture. Path flags select local storage only.
Source overrides on SSH must be absolute; TCP permits default capture only.

Launch and explicit **Refresh** capture on the backend. Filters, browser reload,
TUI `r` and automatic revision reads only query saved usage. No periodic scanning.
`--capture=false` skips launch capture while pending durable processing resumes.

## TUI filters

```sh
tokeninsights tui --today                      # also --yesterday --week --month --year --all-time
tokeninsights tui --filter-day-from 2026-01-01 --filter-day-to 2026-01-31
tokeninsights tui --bucket week                # day|week|month|year
tokeninsights tui --harness pi,codex            # also --provider --model --session-id; repeat/comma-separate
```

Weeks start Monday; calendar filters use the backend timezone and never restrict
capture. TUI results use one consistent snapshot with a 100,000-row limit.

| Key | Action |
| --- | --- |
| `tab` / `1`-`7` | Switch view |
| `d` / `g` / `s` | Date range / bucket or group / sort |
| `p` / `m` / `h` / `f` | Provider / model / harness / all filters |
| `j` `k` / `PgUp` `PgDn` | Row / page |
| `←` `→` / `Home` `End` | Scroll columns |
| `Space` / `Enter` / `Esc` / `c` | Toggle / apply / cancel / clear |
| `r` | Reload saved queries |
| `R` | Refresh sources on the selected backend |
| `?` | Help |
| `q` / `Ctrl+C` | Quit |

## Connections

```sh
tokeninsights tui --connection ssh://you@devbox
tokeninsights web --connection ssh://you@devbox:2222
tokeninsights config set remote.command /home/you/.local/bin/tokeninsights
tokeninsights remote
tokeninsights remote --listen 0.0.0.0:9000 --advertise devbox.example
```

SSH discovers/starts the target backend through system SSH. TCP requires a running
`remote` gateway. Its default bind is `0.0.0.0:0`; `--advertise` accepts a
reachable host or host:port. It prints a complete shell-quoted connect command.
Use that full URI as `connect STRING` or `tui|web --connection STRING`.
Keep its temporary secret private. Stopping the gateway invalidates its strings
and connections; other leases continue. Same-machine use stays TCP.
See [remote connections](remote.md).

## Configuration

```sh
tokeninsights config set KEY VALUE
tokeninsights config get KEY
tokeninsights config remove KEY
tokeninsights --config-file PATH tui
```

| Key | Environment | Default |
| --- | --- | --- |
| `collector.db-path` | `TOKENINSIGHTS_COLLECTOR_DB_PATH` | collector.sqlite in data directory |
| `in-process.server-db-path` | `TOKENINSIGHTS_SERVER_DB_PATH` | server.sqlite in data directory |
| `in-process.host` | `TOKENINSIGHTS_HOST` | 127.0.0.1 |
| `in-process.port` | `TOKENINSIGHTS_PORT` | 0 |
| `remote.command` | `TOKENINSIGHTS_REMOTE_COMMAND` | tokeninsights on remote PATH |

Flags > environment > config > defaults. Config defaults to
`${XDG_CONFIG_HOME:-~/.config}/tokeninsights/config.json`;
`--config-file` or `TOKENINSIGHTS_CONFIG_PATH` overrides it.
A saved `viewer.open` is obsolete: remove it by hand; the file stays unchanged.
Connections are invocation-only; there is no saved remote endpoint.

Retired application/distributed settings, `remote.endpoint`, old endpoint/access
environment variables and obsolete viewer modes fail with repair guidance.
Edit/remove obsolete settings explicitly; failures neither print secret values
nor rewrite configuration. `--sync` is replaced by `--capture`;
`--endpoint` by `--connection`. Sync, browse, hosted server and completion
plugins are removed. Existing user files remain untouched.

## Maintenance

```sh
tokeninsights data reprocess
tokeninsights data wait
tokeninsights data wait --connection ssh://you@devbox
tokeninsights tui --collector-db-path PATH --server-db-path PATH
tokeninsights --version
```

Maintenance supports local/SSH only. TCP viewer credentials cannot run it.
Reprocessing preserves published usage until replacement is complete.
Both maintenance operations use a 30-second visibility wait.

Defaults: `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/collector.sqlite`
and `server.sqlite`. Role/path checks reject mismatches; removed app/jobs files
are never migrated or deleted. See [design](design.md).
