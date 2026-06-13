# tokeninsights

Sync durable local harness data into TokenInsights SQLite and view canonical token usage in an interactive terminal table.

## Install

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@dev
```

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
tokeninsights sync --all
tokeninsights sync --harness opencode
tokeninsights sync --harness pi --source-dir /path/to/source-root
tokeninsights sync --all --dry-run
tokeninsights sync --all --no-normalize
```

When `--source-dir` is provided, sync first looks for a harness subdirectory such as `/path/to/source-root/opencode`; otherwise it scans the provided directory directly.

OpenCode sync reads modern SQLite sources named `opencode.db` or `opencode-<channel>.db`. Pi sync reads JSONL session files from `~/.pi/agent/sessions`, or from a provided Pi source directory. Codex sync reads rollout JSONL session files from `${CODEX_HOME:-~/.codex}/sessions`, parsing structured `event_msg` token-count records.

`normalize`

Rebuild canonical facts from existing raw facts.

```sh
tokeninsights normalize
tokeninsights normalize --harness codex
tokeninsights normalize --dry-run
```

`reset-canonical`

Delete canonical sessions, messages, token usage, and normalization diagnostics while keeping raw facts and observations.

```sh
tokeninsights reset-canonical
tokeninsights reset-canonical --confirm
```

`reset-all`

Delete and recreate the TokenInsights database plus SQLite sidecars.

```sh
tokeninsights reset-all
tokeninsights reset-all --confirm
```

`view`

Open the interactive terminal UI over canonical token usage.

```sh
tokeninsights view
tokeninsights view --today
tokeninsights view --yesterday
tokeninsights view --year --bucket month
tokeninsights view --month --provider openai --model gpt-5
```

Running `tokeninsights` without a command still opens `view`.

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

`--yesterday`

Show data from yesterday.

`--week`

Show data from the current calendar week.

`--month`

Show data from the current calendar month. This is the default Date Range Filter for interactive view.

`--year`

Show data from the current calendar year.

`--all-time`

Show all canonical data with no period filter.

`--bucket day|week|month|year`

Set the Tokens tab Time Bucket. The default bucket is `day`; week buckets start on Monday.

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

Press `q` to quit. Use tab/shift-tab or number keys 1-5 to switch Aggregation Tabs. Use `d` for Date Range Filter, `g` for Time Bucket, `s` for sort, and `p`, `m`, or `h` for provider, model, or harness filters. Use `up/down` or `j/k` to scroll vertically, `left/right` to scroll horizontally, and `home/end` to jump to the start or end of the horizontal table viewport.

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

The active Aggregation Tabs are Tokens, Models, Providers, Harnesses, and Sessions. TPS, request, and tool domains remain future-compatible data domains, but they are not active empty viewer tabs.

Session IDs are shortened in table output. Model names with `/` are shortened to the last path segment where a compact display is needed.
