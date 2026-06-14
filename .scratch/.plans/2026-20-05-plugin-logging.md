# Add shared file logging foundation for OpenCode server and Pi plugins

## Summary

Add a reusable logger package at `packages/logger` and wire it into the OpenCode server plugin and Pi extension. Logs go to daily files under the XDG state directory:

- `$XDG_STATE_HOME/tokeninsights/logs/{harnessname}-{YY-MM-DD}.log` when `XDG_STATE_HOME` is set and non-empty
- `$HOME/.local/state/tokeninsights/logs/{harnessname}-{YY-MM-DD}.log` when `XDG_STATE_HOME` is unavailable
- `./.tokeninsights-state/logs/{harnessname}-{YY-MM-DD}.log` as a final fallback

Debug logs are written only when `TOKENINSIGHTS_DEBUG=1`. Current day + previous 2 calendar days are retained; older log files are pruned automatically.

## Key implementation changes

### 1. New shared logger package

Add `packages/logger` with `src/index.ts` and package metadata.

Logger API:

```ts
const logger = createTokenInsightsLogger({
  harness: "opencode-server" | "pi",
})
```

Methods:

- `debug(message, fields?)`
- `info(message, fields?)`
- `warn(message, fields?)`
- `error(message, fields?)`
- `flush()` / `close()` if needed for shutdown paths

Logger behavior:

- Default format: `logfmt`
- Code-level constant: `LOG_FORMAT`, initially set to `logfmt`, so JSONL can be tried later
- Best-effort only; logging errors never break plugin hooks
- No prompt text, message content, request headers, tool args, or tool output in logs
- Metadata only

### 2. Workspace/package wiring

Update:

- `pnpm-workspace.yaml` to include `packages/logger`
- root `package.json` with `build:logger`
- OpenCode server package dependency on `@tokeninsights/logger`
- Pi package dependency on `@tokeninsights/logger`

### 3. OpenCode server plugin logging

Wire logger into:

- `packages/plugins/opencode-server/src/index.ts`
- `packages/plugins/opencode-server/src/writer-client.ts`
- `packages/plugins/opencode-server/src/oc-tokeninsights-writer.ts`

Add:

- `info` log when plugin loads
- `debug` log for every hook: `chat.params`, `chat.headers`, `tool.execute.before`, `tool.execute.after`, and top-level `event`
- `debug` log when a new session starts via `session.created`
- `debug` logs for key event cases: `message.updated`, `message.part.updated`, `session.idle`, `session.deleted`
- `error` logs replacing or augmenting current `console.error` paths: DB init failure, worker init failure, worker error, flush failure, request insert failure

### 4. Pi extension logging

Wire logger into `packages/plugins/pi/src/index.ts`.

Add:

- `info` log when extension loads
- `debug` log for every hook: `session_start`, `turn_start`, `before_provider_request`, `message_update`, `tool_execution_start`, `tool_execution_end`, `message_end`, `session_shutdown`
- `debug` log whenever a new session starts
- `debug` logs for skipped paths: missing session ID, non-assistant messages, missing usage, DB init disabled
- `error` logs replacing or augmenting current `console.error` paths: DB init failure, request insert failure, token insert failure, TPS insert failure, tool call insert failure

### 5. Docs

Update:

- `README.md`: log path, `TOKENINSIGHTS_DEBUG=1`, retention behavior, example `tail` command
- `docs/design.md`: shared logger package, log retention, metadata-only logging privacy rule, OpenCode server and Pi plugin logging behavior

## Verification

Run:

```sh
pnpm run build:logger
pnpm run build:opencode-server
pnpm run build:pi
pnpm run build
```

Manual smoke test:

```sh
TOKENINSIGHTS_DEBUG=1 opencode ...
TOKENINSIGHTS_DEBUG=1 pi ...
ls ~/.local/state/tokeninsights/logs
tail -f ~/.local/state/tokeninsights/logs/opencode-server-$(date +%y-%m-%d).log
```

## Decisions made

- Scope: OpenCode server plugin and Pi extension only
- Harness filenames: `opencode-server-{YY-MM-DD}.log`, `pi-{YY-MM-DD}.log`
- Format: logfmt by default
- Format toggle: code constant `LOG_FORMAT` in logger package
- Debug mode: only `TOKENINSIGHTS_DEBUG=1`
- Retention: current day + previous 2 calendar days
- Add a log entry whenever a new session starts
- Use XDG state directory resolution instead of hardcoded `~/.local/state`
- Update docs

## Tradeoffs / risks

- `message_update` can be noisy in Pi when debug is enabled, but debug logging is explicitly opt-in
- File logging is best-effort; lost log lines are preferable to blocking plugin execution
- Logs must avoid raw payloads to prevent leaking prompts/tool outputs

## Remaining open questions

None.

## Execution guidance

If implementation needs to deviate from this plan, update this saved plan first, surface the deviation to the user, and capture updated decisions, tradeoffs, and risks here before continuing.
