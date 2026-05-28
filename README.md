# tokeninsights

Local token usage tools for OpenCode and Pi. The OpenCode server plugin and Pi extension write token/TPS/request/tool-call data to SQLite; the Go CLI reads aggregate tables.

See [`docs/design.md`](docs/design.md) for full architecture, schema contract, event flow, and invariants.

## Code Organization

- `packages/plugins/opencode-server/` — OpenCode server plugin package (`@tokeninsights/opencode-server`)
- `packages/plugins/pi/` — Pi extension package
- `packages/logger/` — shared file logger for plugin diagnostics
- `packages/cli/` — Go CLI (`tokeninsights-cli`) that queries the SQLite DB
- `packages/schema/schema.sql` — single source of truth for SQLite schema
- `packages/scripts/check-schema.ts` — cross-language schema contract validator

## Development

### When to run what

| You changed                                                    | Run                                                               |
| -------------------------------------------------------------- | ----------------------------------------------------------------- |
| `packages/schema/schema.sql`                                   | `pnpm run check-schema`                                           |
| Any `.ts` in `packages/plugins/`                               | `pnpm run build`                                                  |
| Any `.go` in `packages/cli/`                                   | `pnpm run test && pnpm run build:cli`                             |
| Storage, schema, events, SQL, aggregation, rendering, or tests | Update **both** plugin and CLI; `pnpm run build && pnpm run test` |

> ⚠️ **Schema changes are user-approved only.** Never modify `packages/schema/schema.sql` without explicit user approval, even for additive changes. Always explain the rationale and ask first.

### Build everything from a fresh checkout

TokenInsights targets Node 24+. The OpenCode server plugin uses native Node TypeScript at runtime.

```sh
pnpm install
pnpm run check-schema
pnpm run build
pnpm run test
```

`build` builds the OpenCode server plugin, Pi extension, and CLI. `test` runs all tests.

### Plugin builds

```sh
pnpm run build:opencode-server
pnpm run build:pi
```

### CLI verification

```sh
pnpm run test
pnpm run build:cli
```

### Smoke test against real DB

Build the CLI first, then run against your local database:

```sh
pnpm run build:cli
./packages/cli/tokeninsights-cli --today
```

## Install OpenCode Plugin

Install dependencies and link the local server plugin package with pnpm:

```sh
cd packages/plugins/opencode-server
pnpm install
pnpm link --global
```

Configure OpenCode to load the linked package names rather than file paths (`~/.config/opencode/opencode.jsonc`):

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["@tokeninsights/opencode-server"],
}
```

The server package contains its Node worker-thread module internally. Do not add `oc-tokeninsights-writer.ts` or `writer-client.ts` as separate plugins.

Default DB path: `~/.local/share/tokeninsights/tokeninsights.sqlite`

Environment overrides for writers:

- `TOKENINSIGHTS_DB_PATH` — absolute path, or relative to the TokenInsights data directory
- `TOKENINSIGHTS_RETENTION_DAYS` — retention window for pruning durable rows

## Plugin Logs

The OpenCode server plugin and Pi extension write best-effort metadata logs to daily files:

```text
$XDG_STATE_HOME/tokeninsights/logs/{harness}-{YY-MM-DD}.log
```

If `XDG_STATE_HOME` is unset, the default path is `~/.local/state/tokeninsights/logs/`. Log files keep the current day plus the previous 2 calendar days; older TokenInsights log files are pruned automatically.

Debug logs are written only when `TOKENINSIGHTS_DEBUG=1` is set. Logs use logfmt format and intentionally avoid prompt text, message content, request headers, tool args, and tool output.

Examples:

```sh
TOKENINSIGHTS_DEBUG=1 opencode
ls ~/.local/state/tokeninsights/logs
tail -f ~/.local/state/tokeninsights/logs/opencode-server-$(date +%y-%m-%d).log
```

## Install Pi Extension

Install dependencies, link the local Pi package with pnpm, and add the linked package name to Pi settings:

```sh
cd packages/plugins/pi
pnpm install
pnpm link --global
```

Pi settings (`~/.pi/agent/settings.json`):

```json
{
  "packages": ["@tokeninsights/pi"]
}
```

The Pi extension writes to the same TokenInsights DB as the OpenCode server plugin (`~/.local/share/tokeninsights/tokeninsights.sqlite`) but stores data in the `pi_*` table family. The CLI reads both `oc_*` and `pi_*` tables and shows a `harness` column (`oc` or `pi`) to distinguish sources.

Tool calls are tracked as lifecycle rows (`started`, `completed`, `error`). The CLI `tool calls` tab shows started-call counts plus error counts per normal group; `tool breakdown` adds per-tool grouping.

## CLI Usage

```sh
tokeninsights-cli --today
tokeninsights-cli --week
tokeninsights-cli --month
tokeninsights-cli --all-time
tokeninsights-cli --week --group-by=hour
tokeninsights-cli --week --group-by=session
tokeninsights-cli --week --provider openai --model gpt-5.5
tokeninsights-cli --week --harness pi
tokeninsights-cli --month
tokeninsights-cli --all-time --filter-day 2026-04-24,2026-04-23
```

Interactive keys:

- `tab` / `shift+tab` — switch tabs (tokens, tps, requests, tool calls, tool breakdown)
- `g` — open grouping popup
- `f` — open filter popup for provider or harness
- `↑/↓` / `j/k` — scroll vertically or move cursor in popup
- `←/→` / `h/l` — scroll the table horizontally
- `home` / `end` — jump to the start/end of the horizontal table viewport
- grouping popup: `space` / `enter` selects grouping mode
- filter popup: `space` / `enter` enters value selection; in value selection, `space` toggles values and `enter` applies
- `esc` in a popup closes without applying staged filter changes
- `q` / `esc` / `ctrl+c` — quit
