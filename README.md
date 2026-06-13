# tokeninsights

Local token usage tracking for OpenCode, Pi, and Codex.

TokenInsights is a sync-first Go CLI. It ingests durable local harness data into SQLite, normalizes raw facts into canonical token usage, and opens an interactive terminal view over canonical tables.

Default DB:

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

Use `--db-path` or `TOKENINSIGHTS_DB_PATH` to choose another database.

## CLI

Build:

```sh
pnpm run build:cli
```

Sync local data, then view it:

```sh
./packages/cli/tokeninsights-cli sync --all
./packages/cli/tokeninsights-cli view
```

Sync one harness:

```sh
./packages/cli/tokeninsights-cli sync --harness opencode
./packages/cli/tokeninsights-cli sync --harness pi
./packages/cli/tokeninsights-cli sync --harness codex
```

Useful sync options:

```sh
./packages/cli/tokeninsights-cli sync --all --dry-run
./packages/cli/tokeninsights-cli sync --all --no-normalize
./packages/cli/tokeninsights-cli sync --harness opencode --source-dir /path/to/fixtures
```

With `sync --all --source-dir <root>`, harnesses read from `<root>/<harness>` and skip missing harness subdirectories.

Canonical maintenance:

```sh
./packages/cli/tokeninsights-cli normalize
./packages/cli/tokeninsights-cli normalize --dry-run
./packages/cli/tokeninsights-cli reset-canonical --confirm
./packages/cli/tokeninsights-cli reset-all --confirm
```

View filters:

```sh
./packages/cli/tokeninsights-cli view --today
./packages/cli/tokeninsights-cli view --week --group-by=session
./packages/cli/tokeninsights-cli view --month --harness pi
./packages/cli/tokeninsights-cli view --week --provider openai --model gpt-5
```

Running without a command still opens the interactive view for compatibility:

```sh
./packages/cli/tokeninsights-cli --week
```

## Development

```sh
pnpm run check-schema
pnpm run test
pnpm run build
```

Schema changes require explicit approval before editing `packages/schema/schema.sql`. The CLI embeds a checked schema copy and validates `PRAGMA user_version`.

See [`docs/design.md`](docs/design.md) for architecture, schema, invariants, and pipeline details.
