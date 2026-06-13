# tokeninsights

Local token usage tracking for OpenCode, Pi, and Codex.

TokenInsights is now sync-first: the Go CLI ingests durable local harness data into SQLite, normalizes it into canonical token facts, and opens an interactive terminal view over the canonical tables.

Default DB:

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

Set `TOKENINSIGHTS_DB_PATH` to use a different database.

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

Schema changes require explicit approval before editing `packages/schema/schema.sql`.

The schema source of truth is `packages/schema/schema.sql`; the CLI embeds a checked copy and validates `PRAGMA user_version` before reading or writing.

See [`docs/design.md`](docs/design.md) for architecture, schema, invariants, and pipeline details.
