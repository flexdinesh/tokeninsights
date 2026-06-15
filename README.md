# tokeninsights

Local token usage tracking for OpenCode, Pi, and Codex.

TokenInsights is a sync-first Go CLI. It ingests durable local harness data into SQLite, normalizes raw facts into canonical token usage, and opens an interactive terminal view over canonical tables.

Default DB:

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

Use `--db-path` or `TOKENINSIGHTS_DB_PATH` to choose another database.

## CLI

Install the latest stable release:

```sh
brew install flexdinesh/tap/tokeninsights
```

Alternative stable install with Go:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
```

Install the development version from the `dev` branch:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@dev
```

Build:

```sh
pnpm run build:cli
```

Sync local data, then view it:

```sh
./packages/cli/bin/tokeninsights sync --all
./packages/cli/bin/tokeninsights view
```

Sync one harness:

```sh
./packages/cli/bin/tokeninsights sync --harness opencode
./packages/cli/bin/tokeninsights sync --harness pi
./packages/cli/bin/tokeninsights sync --harness codex
```

Useful sync options:

```sh
./packages/cli/bin/tokeninsights sync --all --dry-run
./packages/cli/bin/tokeninsights sync --all --no-normalize
./packages/cli/bin/tokeninsights sync --harness opencode --source-dir /path/to/fixtures
```

With `sync --all --source-dir <root>`, harnesses read from `<root>/<harness>` and skip missing harness subdirectories.

OpenCode sync reads modern SQLite sources named `opencode.db` or `opencode-<channel>.db`. Pi sync reads JSONL session files from `~/.pi/agent/sessions`, or from a provided Pi source directory. Codex sync reads rollout JSONL session files from `${CODEX_HOME:-~/.codex}/sessions`, parsing structured `event_msg` token-count records.

Canonical maintenance:

```sh
./packages/cli/bin/tokeninsights normalize
./packages/cli/bin/tokeninsights normalize --dry-run
./packages/cli/bin/tokeninsights reset-canonical --confirm
./packages/cli/bin/tokeninsights reset-all --confirm
```

View filters:

```sh
./packages/cli/bin/tokeninsights view --today
./packages/cli/bin/tokeninsights view --yesterday
./packages/cli/bin/tokeninsights view --year --bucket month
./packages/cli/bin/tokeninsights view --month --harness pi
./packages/cli/bin/tokeninsights view --week --provider openai --model gpt-5
```

`view` opens on the Tokens aggregation tab for this month with day buckets. Tabs switch between Tokens, Models, Providers, Harnesses, and Sessions. Date range presets are `--today`, `--yesterday`, `--week`, `--month`, `--year`, and `--all-time`; token time buckets are `--bucket day|week|month|year`.

Running without a command still opens the interactive view for compatibility:

```sh
./packages/cli/bin/tokeninsights --week
```

## Development

```sh
pnpm run check-schema
pnpm run test
pnpm run build
```

Schema changes require explicit approval before editing `packages/schema/schema.sql`. The CLI embeds a checked schema copy and validates `PRAGMA user_version`.

See [`docs/design.md`](docs/design.md) for architecture, schema, invariants, and pipeline details.
