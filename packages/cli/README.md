# tokeninsights-cli

Sync durable local harness data into TokenInsights SQLite and view canonical token usage in an interactive terminal table.

Supported harness IDs:

```text
opencode
pi
codex
```

## Commands

`sync`

Ingest local harness sources into raw tables and normalize to canonical facts by default.

```sh
tokeninsights-cli sync --all
tokeninsights-cli sync --harness opencode
tokeninsights-cli sync --harness pi --source-dir /path/to/source-root
tokeninsights-cli sync --all --dry-run
tokeninsights-cli sync --all --no-normalize
```

When `--source-dir` is provided, sync first looks for a harness subdirectory such as `/path/to/source-root/opencode`; otherwise it scans the provided directory directly.

OpenCode sync reads modern SQLite sources named `opencode.db` or `opencode-<channel>.db`. Pi sync reads JSONL session files from `~/.pi/agent/sessions`, or from a provided Pi source directory. Codex fixture ingestion currently reads metadata-only JSONL/NDJSON sources.

`normalize`

Rebuild canonical facts from existing raw facts.

```sh
tokeninsights-cli normalize
tokeninsights-cli normalize --harness codex
tokeninsights-cli normalize --dry-run
```

`reset-canonical`

Delete canonical sessions, messages, token usage, and normalization diagnostics while keeping raw facts and observations.

```sh
tokeninsights-cli reset-canonical
tokeninsights-cli reset-canonical --confirm
```

`reset-all`

Delete and recreate the TokenInsights database plus SQLite sidecars.

```sh
tokeninsights-cli reset-all
tokeninsights-cli reset-all --confirm
```

`view`

Open the interactive terminal UI over canonical token usage.

```sh
tokeninsights-cli view
tokeninsights-cli view --today
tokeninsights-cli view --week --group-by=hour
tokeninsights-cli view --week --group-by=session
tokeninsights-cli view --month --provider openai --model gpt-5
```

Running `tokeninsights-cli` without a command still opens `view`.

## Database

Default path:

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

Override it with `--db-path` or `TOKENINSIGHTS_DB_PATH`.

The CLI creates a missing database for `sync`, `normalize`, and reset workflows. `view` opens the database read-only and rejects missing or incompatible databases with a reset instruction.

## View Arguments

`--today`

Show data from today.

`--week`

Show data from the current calendar week. This is the default period for interactive view.

`--month`

Show data from the current calendar month.

`--all-time`

Show all canonical data with no period filter.

`--group-by=hour|session`

Split the selected period by hour or session. The default grouping is day.

`--session-id ID`

Filter by canonical harness session ID. Can be repeated or comma-separated.

`--provider ID`

Filter by provider. Can be repeated or comma-separated.

`--model ID`

Filter by model. Can be repeated or comma-separated.

`--harness ID`

Filter by `opencode`, `pi`, or `codex`. Can be repeated or comma-separated.

`--filter-day-from YYYY-MM-DD`

Inclusive local-day lower bound.

`--filter-day-to YYYY-MM-DD`

Inclusive local-day upper bound.

## Interactive Keys

Press `q` to quit. Use `up/down` or `j/k` to scroll vertically, `left/right` or `h/l` to scroll horizontally, and `home/end` to jump to the start or end of the horizontal table viewport.

## Metrics

Token columns come from countable rows in `canonical_token_usage`:

```text
input
output
reasoning
cache read
cache write
total
```

Missing provider or model values are normalized to `unknown`.

TPS, request, and tool-call tabs remain part of the UI surface, but sync-first V1 only guarantees token usage where durable local sources expose it. Sparse or unavailable domains render empty instead of failing token viewing.

Session IDs are shortened to the last 8 characters in table output. Model names with `/` are shortened to the last path segment.
