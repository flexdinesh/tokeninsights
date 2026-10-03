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

Ingest local harness sources into raw tables and normalize pending canonical work by default. `view` requests an all-harness background refresh on opening, so use explicit sync for targeted harness refreshes, dry runs, full refreshes, no-normalize workflows, or custom source directories.

```sh
tokeninsights sync --all
tokeninsights sync --harness opencode
tokeninsights sync --harness pi --source-dir /path/to/source-root
tokeninsights sync --all --dry-run
tokeninsights sync --all --full-refresh
tokeninsights sync --all --no-normalize
```

With `sync --all --source-dir`, each harness is read only from its own subdirectory, such as `/path/to/source-root/opencode`; missing subdirectories are skipped. Single-harness sync first looks for a matching harness subdirectory, then falls back to scanning the provided directory directly.

OpenCode sync reads modern SQLite sources named `opencode.db` or `opencode-<channel>.db`, including sessions marked archived in those databases. V1 `message` and V2 `session_message` layouts share the SQLite row ID as message identity; usable V2 data replaces matching V1 data. Pi sync reads JSONL session files from `~/.pi/agent/sessions`, or from a provided Pi source directory; Pi has no harness archive and OS trash is excluded. Codex sync reads rollout JSONL session files from `${CODEX_HOME:-~/.codex}/sessions` and `${CODEX_HOME:-~/.codex}/archived_sessions`, parsing structured `event_msg` token-count records. Claude Code sync reads retained local JSONL transcript files from `${CLAUDE_CONFIG_DIR:-~/.claude}/projects` regardless of UI/server archive state; cloud-only archives are excluded. Successful refreshes persist markers: matching content and current location attribution skip parsing, regardless of source age. Eligible Pi files verify the processed prefix and parse only appended records. Size and mtime alone never prove reuse. `sync --dry-run` previews the same checks without writing; `sync --full-refresh` ignores markers for the requested harness scope without requeueing existing raw facts for canonical rebuild.

Harness discovery runs concurrently. Source preparation uses a worker pool sized by Go's available CPU budget (`GOMAXPROCS`), with at most twice that many outstanding sources. Reads, parsing, and Git inspection run outside write transactions; one writer commits sources in discovery order and batches unchanged-source bookkeeping. Loading status includes content verification, so an unchanged refresh still performs reads. Service startup performs no source reads. No schema upgrade is required.

Token components are additive across harnesses: input excludes cache read/write, and output excludes separately reported reasoning. Pi legacy inclusive-input rows are corrected when their source total proves the old layout. Inconsistent source totals fall back to component sums with diagnostics; non-integer and overflowing counters are rejected.

Codex forks/subagents reuse versioned markers only when their content, location, parser/collector identity, and complete parent chain match. Existing fork sources parse once to establish these markers. Missing, conflicting, cyclic, unreadable, or unstable ancestry falls back to parsing; shared parent parses and verification are cached within the run. Explicit ancestry plus matching turn, provider/model, and complete last/cumulative token metadata identifies replay; rewritten timestamps are not match keys. Verified copies retain the original parent fact identity/time, while uncertain history is retained with diagnostics. Distinct snapshots use deterministic identity fingerprints, with line identity when cumulative identity is absent.

`normalize`

Process pending canonical work from existing raw facts. After `reset-canonical`, this rebuilds canonical facts from requeued raw token facts.

Raw provider and model names remain unchanged. Canonical rows used by queries map Pi `openai-codex` to `openai`, map `fireworks-ai` to `fireworks`, and strip `accounts/fireworks/models/` from Fireworks model names. Normal sync and `normalize` also refresh previously stored canonical names.

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

Open the local interactive terminal UI over canonical token usage. By default, `view` ensures the service and requests all-harness refresh asynchronously; compatible saved data remains usable. Closing the TUI cancels observation only. Use `--no-sync` to skip raw ingest and normalization and open existing canonical data read-only.

```sh
tokeninsights view
tokeninsights view --no-sync
tokeninsights view --today
tokeninsights view --yesterday
tokeninsights view --year --bucket month
tokeninsights view --month --provider openai --model gpt-5
```

Bare `tokeninsights` ensures the background service and prints its URL. Move root viewer flags to `tokeninsights view ...`; `--host`/`--port` are service options only.

Every tab's pinned summary shows `sessions <shown> shown / <synced> synced`, followed by the row count and, except in Context, the filtered token total. `shown` counts distinct sessions matching all active filters across the full result, not just the visible scroll viewport. `synced` counts all distinct sessions with countable canonical usage in this database across all dates and harnesses, ignoring viewer filters. Sessions spanning multiple dates or models are counted once; sessions without countable usage are excluded from both counts.

The default current-month filter can show a small subset of synced sessions. Compare `view --no-sync --month` with `view --no-sync --all-time` using the same `--db-path` to inspect date filtering without changing the database. All time removes the preset date restriction but keeps dimension filters and any explicit custom date bounds.

`service start|stop|restart|status|run`

```sh
tokeninsights                         # ensure service, print URL; no sync
tokeninsights service start --open
tokeninsights service start --host 0.0.0.0 --port 8765
tokeninsights service status --json
tokeninsights service restart --reload-sources
tokeninsights service stop
tokeninsights service run             # foreground; Ctrl+C stops
```

Start/run accept `--db-path`, `--host`, `--port`; start also accepts `--open`. Restart accepts bind flags and `--reload-sources`. Status accepts `--db-path` and `--json`; stop accepts `--db-path`. Put flags after the service action. Default binding is `127.0.0.1:8765`; port zero reports the assigned port. Omitted settings reuse saved configuration; changing a running bind requires restart. Busy ports fail without process takeover. Start/restart never ingest or repair existing storage. `serve` is a deprecated foreground alias; its old `--no-sync` is a no-op, and viewer filters must move into the browser URL/UI.

Saved service roots come from the setup environment. Ordinary dashboard refresh uses those roots. Explicit sync/normalize use the current caller's roots and preserve their options, forwarding to the live service or running standalone while ownership is free. An unreachable owner blocks mutation; it never allows a second writer to bypass the service. Status is read-only; a stopped result exits 3, usage exits 2, other failures exit 1.

`refresh [--wait]`

```sh
tokeninsights refresh
tokeninsights refresh --wait --db-path /path/to/usage.sqlite
```

Refresh ensures the service and requests all-harness ingest with normalization. Acceptance returns immediately; `--wait` reports completion/failure. Disconnecting or Ctrl+C while waiting does not cancel shared work. Requests arriving after capture queue one coalesced follow-up; there is no automatic retry without demand. Explicit reset cancels queued ordinary refresh and rejects new requests until complete. Operations and request deduplication are bounded and process-local; restart/crash loses pending demand.

The web displays saved data on opening. **Refresh** requests sync; **Reload Data** only rereads SQLite. `--open` uses the actual assigned URL and skips SSH/headless browser launch. Administration is local-only over a private Unix socket, even with wildcard web binding. Authentication and login/reboot autostart remain out of scope.

The web dashboard provides Tokens, Models, Providers, Harnesses, Sessions, Context, and Repo views, summary cards, charts, faceted multi-select filters, custom/open-ended dates, session-ID search, column sorting/visibility, pagination, and system/light/dark themes. Repo groups by repository or directory and lists providers, harnesses, and models in each row. Location filters apply only to Repo; missing values display as **unknown**. Unknown rows start collapsed; when recorded directories contribute, an inline control reveals them all and any missing-directory note. Unknown directory groups have no recorded directories and show **No recorded directory**. Date Range Filters and Time Buckets use the server's local timezone and Monday-start weeks. Browser URLs preserve filters, tab, bucket, sorting, and pagination; browser back/forward restores them. **Restore CLI defaults** restores the startup filters.

The dashboard queries only the server serving its page, using same-origin API requests. Open the server directly by IP or DNS name; `service start --host 0.0.0.0` remains supported. Sync records the ingesting machine’s hostname in each ingest run, including unchanged-source checks. The UI displays the latest completed run’s saved hostname and the page address, with no server selector or saved host list. Older data reports `unknown` until synced again; serving a copied database does not relabel it with the server’s hostname.

The REST server has no authentication and does not advertise cross-origin browser access. Use network bindings only on trusted networks.

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

The service initializes an empty missing database without sync. Normal `view` ensures that service; sync/normalize/reset retain their existing create/recovery behavior. `view --no-sync` rejects missing, incompatible, or rebuild-pending data without creating files. Existing recognized older data can start the service with analytics blocked until explicit refresh/recovery.

### Compatibility Recovery

V11–V13 databases upgrade additively to V14, preserving usage and leaving historical ingest hostnames unknown. Schema V14 uses data generation 5 and retains the durable rebuild-pending marker and recovery source hash. `database_lifecycle.rebuild_source_key` is NULL when ready and nonempty while pending; source artifact paths are never stored. Directory display paths use `~/` where a home directory can be identified; otherwise full directory paths may be stored. Compatibility depends on schema/data contracts, not release version numbers. Normal `sync`, `normalize`, and requested dashboard refresh recover recognized older schema/data by transactionally resetting application tables in place, importing all configured local harnesses, and normalizing. This reset can lose usage when original artifacts were deleted. Compatible updates preserve data; newer, unknown, or corrupt databases are rejected without automatic deletion.

Recovery notices appear on stderr for `sync`/`normalize`, and as resetting/rebuilding progress in the TUI/web viewer. Missing harness installations are normal skips. A failed rebuild retains partial imports and pending state. Retry with the original `--db-path`, `--source-dir` if used, and source environment settings to resume without resetting again. Default-source recovery can resume through normal TUI opening, explicit web Refresh, or `sync --all`; custom-source recovery requires `sync --all --source-dir <original-root> --db-path <original-database>`. A mismatched source configuration is rejected before data writes. Full paths cannot be recovered from the stored hash. Analytics reads validate compatibility inside their read snapshot and stay unavailable until success. Only retained local sources can reconstruct history.

Targeted default-source commands first recover all default harnesses. `sync --all --source-dir <root>` stays bounded to `<root>/<harness>` during recovery. Single-harness custom-source commands defer recovery without changing the database; use an eligible default-source or all-harness command first. Recovery always normalizes, even before satisfying `--no-normalize`. `--dry-run` previews reset/resume without database writes or stale refresh-state suppression. `--no-sync` never repairs data. Help/version commands never access the database.

Source-scope matching uses normalized configuration: default recovery fingerprints the effective OpenCode, Pi, Codex, and Claude Code roots, including environment overrides; custom all-harness recovery fingerprints the canonical absolute root. Equivalent normalized roots match. Changing roots cannot complete an existing pending rebuild.

## View Arguments

`--no-sync`

Open existing canonical data read-only without starting a service or processing pending normalization work. This mode disables refresh requests, including `u`.

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

The Instrument desk layout prioritizes full-screen terminals (120×35 or larger): machine/sync status, seven-view navigation, scope controls, a token readout strip, full-width table, pinned coverage, and a short key guide. Canvas, readouts, and drawers use the terminal background; foreground colors adapt to light/dark terminals; bracketed active navigation and a row cursor also identify selection without color. Smaller terminals compact navigation and omit the readout strip when space is scarce; table metrics remain accessible by scrolling.

Press `q` or Ctrl+C to quit. Use tab/shift-tab or number keys 1-7 to switch Aggregation Tabs. Use `d` for Date Range Filter, `g` for Time Bucket or Repo grouping, `s` for sort, and `p`, `m`, or `h` for provider, model, or harness filters. `f` opens a filter menu; Repo adds repository and directory facets there. `?` opens the keyboard guide. Drawers open on the right while the dashboard stays visible. Use up/down or j/k to navigate, Space to toggle multi-select values, Enter to apply, and Escape to cancel without changing scope or table position. `c` clears the draft selection; applying an empty selection includes all values. Choosing a date preset replaces custom date bounds.

Use `up/down` or `j/k` to move through rows, PageUp/PageDown to move a page, `left/right` to scroll horizontally, and `home/end` to jump to the start or end of the horizontal table viewport. `r` reloads canonical usage read-only; it does not sync sources. Failed reads offer this retry action.

The six readouts show total, input, output, reasoning, cache read, and cache write tokens summed from the full filtered result, independently of scrolling. Loading uses placeholders rather than stale values. Context shows an explanation of session peaks instead of additive token readouts.

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

The active Aggregation Tabs are Tokens, Models, Providers, Harnesses, Sessions, Context, and Repo. Repo groups optional fact-level location attribution by repository or directory. Repo rows list distinct providers, harnesses, and models in the same row. Repository identity combines clones with the same remote. Missing location values display as **unknown**. TPS, request, and tool domains remain future-compatible data domains, but they are not active empty viewer tabs. The Sessions tab also shows derived `ctx used`, the peak prompt-side context load for the session. Context groups session peaks by harness, provider, and model, showing `sessions`, `avg ctx`, `median ctx`, and `max ctx`.

Session IDs are shortened in table output. Model names with `/` are shortened to the last path segment where a compact display is needed.
