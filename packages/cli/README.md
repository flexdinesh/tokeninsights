# tokeninsights

View local token usage in an interactive terminal table. Run `tokeninsights view` to automatically refresh all supported harnesses and open the dashboard; no separate sync command is needed.

## Install

```sh
brew install flexdinesh/tap/tokeninsights
```

Alternative stable install with Go:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
```

Development version from `dev`:

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@dev
```

Supported harness IDs:

```text
opencode
pi
codex
claude-code
```

## Commands

`sync`

Ingest local harness sources into raw tables and normalize pending canonical work by default. `view` runs an implicit all-harness sync before opening, so use explicit sync for targeted harness refreshes, dry runs, full refreshes, no-normalize workflows, or custom source directories.

```sh
tokeninsights sync --all
tokeninsights sync --harness opencode
tokeninsights sync --harness pi --source-dir /path/to/source-root
tokeninsights sync --all --dry-run
tokeninsights sync --all --full-refresh
tokeninsights sync --all --no-normalize
```

With `sync --all --source-dir`, each harness is read only from its own subdirectory, such as `/path/to/source-root/opencode`; missing subdirectories are skipped. Single-harness sync first looks for a matching harness subdirectory, then falls back to scanning the provided directory directly.

OpenCode sync reads modern SQLite sources named `opencode.db` or `opencode-<channel>.db`, including sessions marked archived in those databases. Pi sync reads JSONL session files from `~/.pi/agent/sessions`, or from a provided Pi source directory; Pi has no harness archive and OS trash is excluded. Codex sync reads rollout JSONL session files from `${CODEX_HOME:-~/.codex}/sessions` and `${CODEX_HOME:-~/.codex}/archived_sessions`, parsing structured `event_msg` token-count records. Claude Code sync reads retained local JSONL transcript files from `${CLAUDE_CONFIG_DIR:-~/.claude}/projects` regardless of UI/server archive state; cloud-only archives are excluded. After a successful OpenCode SQLite or Pi/Codex/Claude Code JSONL refresh, old unchanged sources can be skipped by Recent Source Refresh; recent or changed sources are still parsed. The freshness window is 48 hours before the last successful source refresh. `sync --dry-run` previews skips without writing, and `sync --full-refresh` ignores source refresh state for the requested harness scope without requeueing existing raw facts for canonical rebuild.

Codex forks/subagents are reparsed each sync, with linked parent parses cached within the run. Explicit ancestry plus matching turn, provider/model, and complete last/cumulative token metadata identifies replay; rewritten timestamps are not match keys. Verified copies retain the original parent fact identity/time, while uncertain history is retained with diagnostics. Ordinary Codex sources still use Recent Source Refresh. Distinct snapshots use deterministic identity fingerprints, with line identity when cumulative identity is absent.

`normalize`

Process pending canonical work from existing raw facts. After `reset-canonical`, this rebuilds canonical facts from requeued raw token facts.

```sh
tokeninsights normalize
tokeninsights normalize --harness codex
tokeninsights normalize --dry-run
```

`reset-canonical`

Delete canonical sessions, messages, token usage, and normalization diagnostics while keeping raw facts, observations, and source refresh state. Existing raw token facts are requeued for normalization.

Requires compatible data with no unfinished recovery. It cannot repair incompatible raw token semantics or identities.

```sh
tokeninsights reset-canonical
tokeninsights reset-canonical --confirm
```

`reset-all`

Transactionally recreate application tables inside the existing SQLite file. This clears raw facts, canonical facts, pending normalization work, and source refresh state. A subsequent sync imports retained local sources. Explicit reset remains available when source availability changes and previously ambiguous Codex history needs reconciliation.

```sh
tokeninsights reset-all
tokeninsights reset-all --confirm
```

`view`

Open the interactive terminal UI over canonical token usage. By default, `view` opens into a sync progress screen, refreshes all supported Durable Sources, and processes pending normalization work before rendering the dashboard. Use `--no-sync` to skip raw ingest and normalization and open existing canonical data read-only.

```sh
tokeninsights view
tokeninsights view --no-sync
tokeninsights view --today
tokeninsights view --yesterday
tokeninsights view --year --bucket month
tokeninsights view --month --provider openai --model gpt-5
```

Running `tokeninsights` without a command still opens `view`, including implicit sync. `tokeninsights --no-sync` is equivalent to `tokeninsights view --no-sync`.

Every tab's pinned summary shows `sessions <shown> shown / <synced> synced`, followed by the row count and, except in Context, the filtered token total. `shown` counts distinct sessions matching all active filters across the full result, not just the visible scroll viewport. `synced` counts all distinct sessions with countable canonical usage in this database across all dates and harnesses, ignoring viewer filters. Sessions spanning multiple dates or models are counted once; sessions without countable usage are excluded from both counts.

The default current-month filter can show a small subset of synced sessions. Compare `view --no-sync --month` with `view --no-sync --all-time` using the same `--db-path` to inspect date filtering without changing the database. All time removes the preset date restriction but keeps dimension filters and any explicit custom date bounds.

`serve`

Serve the embedded React dashboard over IPv4. By default the listener binds `0.0.0.0` and prints three URLs: the primary LAN address (for example `http://10.0.1.151:8765`), `http://localhost:8765`, and `http://0.0.0.0:8765`. LAN discovery excludes Docker/virtual/VPN interface names from the printed list; the wildcard listener still accepts connections on all IPv4 interfaces. If no LAN address is available, only localhost and `0.0.0.0` are printed. Use `--host <ipv4>` to restrict binding to a specific address; then only that address is printed. Default port: `8765`; `--port 0` chooses an available port. Ctrl+C shuts down the server.

```sh
tokeninsights serve --week
tokeninsights serve --no-sync --port 8080
tokeninsights serve --host 10.0.1.151 --week
tokeninsights serve --month --bucket week --provider openai --harness pi
```

All viewer arguments below also work with `serve`. They initialize browser filters rather than restricting which harnesses sync. Startup refreshes all supported harnesses with normalization; `--no-sync` skips startup sync and requires an existing compatible, fully recovered database. **Sync now** runs a shared refresh across browser clients, including clients using this server as a remote source; **Reload data** only rereads SQLite. Ordinary sync failures keep the server available for retry or explicitly inspecting existing data. Failed compatibility recovery offers retry and hides **Inspect existing data** until recovery completes.

The web dashboard provides Tokens, Models, Providers, Harnesses, Sessions, and Context views, summary cards, charts, faceted multi-select filters, custom/open-ended dates, session-ID search, column sorting/visibility, pagination, and system/light/dark themes. Date Range Filters and Time Buckets use the server's local timezone and Monday-start weeks. Browser URLs preserve filters, tab, bucket, sorting, and pagination; browser back/forward restores them. **Restore CLI defaults** restores the startup filters.

The page origin is the initial local data source. The source selector accepts reachable HTTP(S) TokenInsights servers and normalizes bare `host:port` values to HTTP. It validates compatibility before saving, labels sources by hostname, deduplicates normalized URLs, and persists the list and active selection in browser `localStorage`. Switching sources preserves current viewer state. Requests and query caches are source-specific; sources are selected individually and data is never merged. An unavailable saved source remains selected with an error and recovery controls. Browser mixed-content rules can block HTTP from an HTTPS page, and invalid or untrusted certificates can block HTTPS sources.

The REST server permits unauthenticated `GET`, `POST`, and preflight `OPTIONS` requests from every browser origin without credentials. This enables remote reads and **Sync now**, but any website able to reach the listener can read usage metadata and trigger local ingestion. Use only on trusted networks.

The versioned API has no unversioned compatibility routes:

| Endpoint | Purpose |
|----------|---------|
| `GET /api/v1/instance` | Identity, versions, capabilities, timezone, viewer defaults |
| `GET /api/v1/sync` | Current shared sync state |
| `POST /api/v1/sync` | Start or join shared sync |
| `GET /api/v1/usage` | Filtered summary, chart, and paginated rows |
| `GET /api/v1/usage/facets` | Filter facets and session search |

[`docs/openapi.yaml`](../../docs/openapi.yaml) is the authoritative, repository-only contract and is not served at runtime. `pnpm run generate:api` creates committed Go transport models plus TypeScript types/Zod schemas; `pnpm run check-api` detects contract drift. Direct Go builds use committed generated output and need no Node runtime.

The Sessions card shows distinct sessions matching the filters alongside all synced sessions. Every table summary leads with `Sessions <shown> shown / <synced> synced`, including Context and empty filtered results. Counts use the same canonical query as the TUI, exclude empty/non-countable-only sessions, and cover every page. The synced count ignores all viewer filters. Context compares in-range session peaks as average, median, and maximum; its table summary shows session coverage and row count without an additive token total.

## Database

Default path:

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

Override it with `--db-path` or `TOKENINSIGHTS_DB_PATH`.

The CLI creates a missing database for `sync`, implicit `view`/`serve` sync, `normalize`, and reset workflows. Fresh databases start current and need no recovery. `view --no-sync` and `serve --no-sync` validate existing data read-only and reject missing, incompatible, or rebuild-pending databases.

### Compatibility Recovery

Schema V8 tracks data generation 1, a durable rebuild-pending marker, and a hash of the recovery source configuration. `database_lifecycle.rebuild_source_key` is NULL when ready and nonempty while pending; full source paths are never stored. Compatibility depends on schema/data contracts, not release version numbers. Normal `sync`, `normalize`, `view`, and `serve` automatically recover recognized older schema/data by transactionally resetting application tables in place, importing all configured local harnesses, and normalizing. Compatible updates preserve data; newer, unknown, or corrupt databases are rejected without automatic deletion.

Recovery notices appear on stderr for `sync`/`normalize`, and as resetting/rebuilding progress in the TUI/web viewer. Missing harness installations are normal skips. A failed rebuild retains partial imports and pending state. Retry with the original `--db-path`, `--source-dir` if used, and source environment settings to resume without resetting again. Default-source recovery can resume through normal dashboard startup or `sync --all`; custom-source recovery requires `sync --all --source-dir <original-root> --db-path <original-database>`. A mismatched source configuration is rejected before data writes. Full paths cannot be recovered from the stored hash. Analytics reads validate compatibility inside their read snapshot and stay unavailable until success. Only retained local sources can reconstruct history.

Targeted default-source commands first recover all default harnesses. `sync --all --source-dir <root>` stays bounded to `<root>/<harness>` during recovery. Single-harness custom-source commands defer recovery without changing the database; use an eligible default-source or all-harness command first. Recovery always normalizes, even before satisfying `--no-normalize`. `--dry-run` previews reset/resume without database writes or stale refresh-state suppression. `--no-sync` never repairs data. Help/version commands never access the database.

Source-scope matching uses normalized configuration: default recovery fingerprints the effective OpenCode, Pi, Codex, and Claude Code roots, including environment overrides; custom all-harness recovery fingerprints the canonical absolute root. Equivalent normalized roots match. Changing roots cannot complete an existing pending rebuild.

## View Arguments

`--no-sync`

Skip the implicit all-harness sync before opening the TUI. This preserves read-only viewing of existing canonical data and does not process pending normalization work.

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

Filter by `opencode`, `pi`, `codex`, or `claude-code`. Can be repeated or comma-separated.

`--filter-day-from YYYY-MM-DD`

Inclusive local-day lower bound.

`--filter-day-to YYYY-MM-DD`

Inclusive local-day upper bound.

## Interactive Keys

Press `q` to quit. Use tab/shift-tab or number keys 1-6 to switch Aggregation Tabs. Use `d` for Date Range Filter, `g` for Time Bucket, `s` for sort, and `p`, `m`, or `h` for provider, model, or harness filters. Use `up/down` or `j/k` to scroll vertically, `left/right` to scroll horizontally, and `home/end` to jump to the start or end of the horizontal table viewport.

Tables fit the current terminal width where possible. Summary columns with multiple model, provider, or harness values stack those values vertically within the row instead of collapsing them to a count. Long model, provider, and harness values are truncated in-place; horizontal scrolling is used only when the minimum readable table width is still wider than the viewport.

## Metrics

Token columns come from countable rows in `canonical_token_usage`:

```text
input
output
reasoning
cache R
cache W
total
```

Missing model values are normalized to `unknown`. Missing provider values are normalized to `unknown`, except Claude Code artifact-derived rows, which appear as `maybe-anthropic` with inferred provider provenance.

The active Aggregation Tabs are Tokens, Models, Providers, Harnesses, Sessions, and Context. TPS, request, and tool domains remain future-compatible data domains, but they are not active empty viewer tabs. The Sessions tab also shows derived `ctx used`, the peak prompt-side context load for the session. Context groups session peaks by harness, provider, and model, showing `sessions`, `avg ctx`, `median ctx`, and `max ctx`.

Session IDs are shortened in table output. Model names with `/` are shortened to the last path segment where a compact display is needed.
