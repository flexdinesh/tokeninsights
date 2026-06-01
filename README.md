# tokeninsights

Local token usage tracking for OpenCode and Pi. Plugins write to SQLite; the Go CLI reads aggregated token, TPS, request, and tool-call data.

Default DB:

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

## Local plugin setup

This is for local development. The plugins are not published to npm yet, so use local symlinks.

From this repo:

```sh
pnpm install
pnpm run build:plugins
```

Requires Node.js 25 or newer for local builds and Pi runtime. The OpenCode server plugin runs under Bun.

### OpenCode

Symlink the built server plugin into OpenCode's local plugin directory:

```sh
mkdir -p ~/.config/opencode/plugins
ln -sfn "$PWD/packages/plugins/opencode-server/deploy" ~/.config/opencode/plugins/tokeninsights
```

OpenCode auto-discovers server plugins from `~/.config/opencode/plugins`.

### Pi

Symlink the built Pi extension into Pi's extension directory:

```sh
mkdir -p ~/.pi/agent/extensions
ln -sfn "$PWD/packages/plugins/pi/dist" ~/.pi/agent/extensions/tokeninsights
```

Pi auto-discovers extensions from `~/.pi/agent/extensions`. Run `/reload` or restart Pi after rebuilding.

## CLI

Build and run:

```sh
pnpm run build:cli
./packages/cli/tokeninsights-cli --today
```

Common filters:

```sh
./packages/cli/tokeninsights-cli --week
./packages/cli/tokeninsights-cli --month
./packages/cli/tokeninsights-cli --all-time
./packages/cli/tokeninsights-cli --week --group-by=session
./packages/cli/tokeninsights-cli --week --harness pi
./packages/cli/tokeninsights-cli --week --provider openai --model gpt-5.5
```

## Development

```sh
pnpm run check-schema
pnpm run build
pnpm run test
```

For local plugin testing with a temporary database, start OpenCode or Pi with `TOKENINSIGHTS_DB_PATH` set:

```sh
TOKENINSIGHTS_DB_PATH=/tmp/tokeninsights-test.sqlite opencode
TOKENINSIGHTS_DB_PATH=/tmp/tokeninsights-test.sqlite pi
```

Then point the CLI at the same database:

```sh
tokeninsights-cli --db-path /tmp/tokeninsights-test.sqlite --today
```

Useful targeted commands:

```sh
pnpm run build:plugins
pnpm run build:opencode-server
pnpm run build:pi
pnpm run build:cli
```

Schema changes require explicit approval before editing `packages/schema/schema.sql`.

## Logs and env

Logs:

```text
~/.local/state/tokeninsights/logs/{harness}-{YY-MM-DD}.log
```

Environment overrides:

- `TOKENINSIGHTS_DB_PATH`
- `TOKENINSIGHTS_RETENTION_DAYS`
- `TOKENINSIGHTS_DEBUG=1`

See [`docs/design.md`](docs/design.md) for architecture and schema details.
