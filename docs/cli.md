# CLI

Run `tokeninsights <command> --help` for every flag. Harnesses: `opencode`, `pi`, `codex`, `claude-code`.

## Dashboards

```sh
tokeninsights tui                              # terminal dashboard
tokeninsights tui --sync=false                 # saved data only, no capture
tokeninsights tui --full-refresh               # reread sources, retry quarantine
tokeninsights tui --today                      # also --yesterday --week --month --year --all-time
tokeninsights tui --filter-day-from 2026-01-01 --filter-day-to 2026-01-31
tokeninsights tui --bucket week                # day|week|month|year
tokeninsights tui --harness pi,codex           # also --provider --model --session-id; repeat or comma-separate

tokeninsights web                              # browser dashboard at http://127.0.0.1:8765
tokeninsights web --port 9000                  # other port; 0 picks a free one
tokeninsights web --host 0.0.0.0               # read-only dashboard on all IPv4 interfaces
tokeninsights web --open=browser               # auto|mygo|electron|browser|false
tokeninsights web --open=false                 # print URL only
tokeninsights web --sync=false                 # saved data only
```

## Remote machine over SSH

```sh
tokeninsights tui --endpoint ssh://you@devbox
tokeninsights web --endpoint ssh://you@devbox  # local read-only proxy, browser only
tokeninsights data wait --endpoint ssh://you@devbox:2222
tokeninsights tui --endpoint=local             # override saved endpoint
tokeninsights config set remote.endpoint ssh://you@devbox
tokeninsights config set remote.command /home/you/.local/bin/tokeninsights  # if not on remote PATH
```

## Desktop apps

```sh
tokeninsights web --open=mygo
tokeninsights web --open=electron
tokeninsights config set viewer.open mygo      # default for web/browse
tokeninsights-mygo --endpoint=ssh://you@devbox # Linux; macOS: open -a "TokenInsights MyGo" --args --endpoint=ssh://you@devbox
```

## Sync to a hosted server

```sh
tokeninsights config set distributed.server-url https://usage.example.com
tokeninsights config set distributed.server-token  # prompts; never pass as argument
tokeninsights sync                             # background submission
tokeninsights sync --wait                      # wait for acceptance
tokeninsights sync --print                     # submit; stdout is remote URL only
tokeninsights sync --debug                     # show capture and receipt progress
tokeninsights sync --harness pi --source-dir /path/to/source
tokeninsights sync --all --source-dir /path/to/all-harnesses
tokeninsights sync --publish-only              # resend retained evidence, no capture
tokeninsights sync --full-refresh              # reread sources, retry quarantine
tokeninsights sync --dry-run                   # parse only, no submit
tokeninsights sync status                      # latest job; --json for machines
tokeninsights browse                           # open hosted dashboard
tokeninsights browse --open=false              # print hosted URL
```

## Config

```sh
tokeninsights config set KEY VALUE
tokeninsights config get KEY
tokeninsights config remove KEY
tokeninsights --config-file PATH tui           # or TOKENINSIGHTS_CONFIG_PATH
```

Keys: `collector.db-path`, `in-process.server-db-path`, `in-process.app-db-path`, `in-process.host`, `in-process.port`, `distributed.server-url`, `distributed.server-token`, `viewer.open`, `remote.endpoint`, `remote.command`.

Env: `TOKENINSIGHTS_ENDPOINT`, `TOKENINSIGHTS_REMOTE_COMMAND`, `TOKENINSIGHTS_SERVER_URL`, `TOKENINSIGHTS_ACCESS_TOKEN`, `TOKENINSIGHTS_COLLECTOR_DB_PATH`, `TOKENINSIGHTS_SERVER_DB_PATH`, `TOKENINSIGHTS_APP_DB_PATH`. Precedence: flags > env > config > defaults.

## Data

```sh
tokeninsights data reprocess                   # rebuild processed usage
tokeninsights data wait                        # wait for pending processing
tokeninsights tui --collector-db-path PATH --server-db-path PATH --app-db-path PATH
tokeninsights --version
```

Default data dir: `${XDG_DATA_HOME:-~/.local/share}/tokeninsights/`.

## TUI keys

| Key                             | Action                                   |
| ------------------------------- | ---------------------------------------- |
| `tab` / `1`-`7`                 | switch tab                               |
| `d` / `g` / `s`                 | date range / bucket or group / sort      |
| `p` / `m` / `h` / `f`           | provider / model / harness / all filters |
| `j` `k` / `PgUp` `PgDn`         | move row / page                          |
| `←` `→` / `Home` `End`          | scroll columns                           |
| `Space` / `Enter` / `Esc` / `c` | toggle / apply / cancel / clear          |
| `r`                             | reload saved data                        |
| `?`                             | help                                     |
| `q` / `Ctrl+C`                  | quit                                     |

## Hosted server

```sh
tokeninsights-server --listen 127.0.0.1:8766 --server-db-path /var/lib/tokeninsights/server.sqlite --public-url https://usage.example.com
tokeninsights-server --storage-backend postgres --admin-socket /run/tokeninsights/admin.sock  # DSN via TOKENINSIGHTS_POSTGRES_DSN
tokeninsights-server admin --admin-socket PATH user create alice
tokeninsights-server admin --admin-socket PATH user reset-password USER_ID < new-password
tokeninsights-server admin --admin-socket PATH user disable USER_ID
tokeninsights-server admin --admin-socket PATH user reprocess USER_ID
tokeninsights-server admin --admin-socket PATH token create USER_ID --scopes read,ingest
tokeninsights-server admin --admin-socket PATH token revoke TOKEN_ID
```

See [deployment](deployment.md) for Docker, storage and backups.
