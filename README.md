# TokenInsights

Local token usage dashboard for OpenCode, Pi, Codex, and Claude Code.

Open the interactive terminal dashboard to see your usage. TokenInsights automatically refreshes data from your installed coding tools before showing the dashboard—no separate sync command is needed.

![TokenInsights TUI showing token usage by model](assets/tokeninsights-view-models.png)

## Install

### Homebrew

```sh
brew install flexdinesh/tap/tokeninsights
```

### Go

```sh
# stable release
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
```

See the [Release Guide](docs/release.md) for version-specific and development installs.

## Quick Start

```sh
tokeninsights view
```

The dashboard opens with this month's usage grouped by day. Running `tokeninsights` without a command does the same thing. Press `q` to quit.

```sh
# view preset date ranges
tokeninsights view --today
tokeninsights view --yesterday
tokeninsights view --week
tokeninsights view --month
tokeninsights view --year
tokeninsights view --all-time
```

Date ranges use your local time zone. Week, month, and year mean the current calendar period; weeks start on Monday.

## Explore Your Usage

Switch between tokens, models, providers, harnesses, sessions, and context tabs. The header shows the date range, hostname, and last sync time. The pinned summary below the table compares matching sessions with all synced sessions, for example `sessions 5 shown / 214 synced`. It also shows the row count and token total for the full filtered result; the context tab omits the token total.

“Shown” counts distinct sessions matching the current date, provider, model, harness, and session filters, including rows outside the scroll viewport. “Synced” counts all distinct sessions with countable usage in this database across all dates and tools, regardless of those filters. A session used across multiple dates or models is counted once. To compare the default month with all time without changing the data, use the same database:

```sh
tokeninsights view --no-sync --month --db-path /path/to/tokeninsights.sqlite
tokeninsights view --no-sync --all-time --db-path /path/to/tokeninsights.sqlite
```

In the dashboard, press `d` to change the date range. Switching to all time keeps any provider, model, harness, or session filters in place.

Cache columns use `cache R` (read) and `cache W` (write). The sessions tab's `ctx used` column shows peak prompt-side token load, excluding assistant output and reasoning tokens. The context tab compares session peaks by harness, provider, and model using `avg ctx`, `median ctx`, and `max ctx`.

```sh
# filter by harness, provider, or model
tokeninsights view --harness pi
tokeninsights view --week --provider openai --model gpt-5

# group this year's usage by month
tokeninsights view --year --bucket month

# open previously synced data without refreshing it (read-only)
tokeninsights view --no-sync
```

Viewer filters affect displayed data only: `view --harness pi` still refreshes all supported tools. `--no-sync` requires an existing, compatible TokenInsights database.

| Key | Action |
|-----|--------|
| Tab / Shift+Tab or 1–6 | Switch tabs |
| `d` | Choose a date range |
| `g` | Choose a time bucket |
| `s` | Change sorting |
| `p` / `m` / `h` | Filter providers / models / harnesses |
| ↑ / ↓ or `j` / `k` | Scroll vertically |
| ← / →, Home / End | Scroll horizontally |
| `q` | Quit |

See the [CLI reference](packages/cli/README.md) for all flags, including custom date bounds and session filters.

## Local Data

TokenInsights reads local session files and databases. Here, “sync” means importing usage into a local SQLite database, not uploading it to a service. No API keys are needed.

| Tool | Default source |
|------|----------------|
| OpenCode | `${XDG_DATA_HOME:-~/.local/share}/opencode/opencode.db` and `opencode-<channel>.db` (V1 and V2) |
| Pi | `~/.pi/agent/sessions` |
| Codex | `${CODEX_HOME:-~/.codex}/sessions` and `${CODEX_HOME:-~/.codex}/archived_sessions` |
| Claude Code | `${CLAUDE_CONFIG_DIR:-~/.claude}/projects` |

The parsers extract token counts, timestamps, model/provider names, and session/message identifiers. Session files may contain conversations; conversation text is not copied into token usage rows.

### Data Coverage

“All time” means all usage imported into this local database, not your provider account's lifetime total. Usage that is no longer available locally or was recorded on another machine is not automatically recovered.

Local archived history is included when the harness keeps it in a durable source: OpenCode archived sessions remain in SQLite, Codex scans both `sessions` and `archived_sessions`, and Claude Code reads retained local `projects` transcripts regardless of UI/server archive state. Pi has no harness archive. OS trash and cloud-only archives are excluded.

Codex fork/subagent history is counted once when explicit ancestry and matching token metadata identify the original parent facts, even if replay timestamps changed. Uncertain history is retained with diagnostics; missing parent transcripts can limit reconciliation.

### Automatic Compatibility Recovery

Normal sync, normalize, and dashboard startup automatically rebuild recognized older database formats or incompatible token identities from all configured local harnesses. Compatible updates keep existing data. Recovery resets application tables transactionally inside the existing SQLite file, then reimports and normalizes retained sources. Only locally retained history can be reconstructed.

If recovery fails, retry with the same database and source configuration. For default sources, use `tokeninsights sync --all` or reopen the dashboard with the original `--db-path` and source environment settings. For custom sources, repeat `sync --all --source-dir <root> --db-path <database>`. Partial imports are retained and recovery resumes without another reset. A stored hash enforces matching source roots; full paths are not persisted or reconstructed automatically. Mismatched retries are rejected before data writes. `--no-sync` stays read-only and rejects unfinished recovery; `--dry-run` previews recovery without writing. Unknown, corrupt, or newer databases are rejected without automatic deletion.

### Database Location

The default TokenInsights database is:

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

Override it with `--db-path` or `TOKENINSIGHTS_DB_PATH`.

## Advanced Usage

### Manual Sync

Use `sync` for targeted refreshes, previews, or custom source directories. Normal viewing handles ingestion and normalization automatically. Subsequent refreshes can skip old, unchanged sources; see the [Design Guide](docs/design.md#sync-pipeline) for refresh behavior.

```sh
# refresh all supported tools, or just one
tokeninsights sync --all
tokeninsights sync --harness pi

# preview without writing to the TokenInsights database
tokeninsights sync --all --dry-run

# parse all discovered sources, ignoring saved refresh state
tokeninsights sync --all --full-refresh

# import a custom source directory
tokeninsights sync --harness pi --source-dir /path/to/pi/sessions
```

With `sync --all --source-dir /path/to/sources`, put each tool's sources in its own subdirectory: `opencode`, `pi`, `codex`, or `claude-code`. Missing subdirectories are skipped.

When compatibility recovery is needed, targeted default-source sync first rebuilds all default harnesses. All-harness custom-root recovery stays within that root. Single-harness custom-root commands defer recovery; run an eligible all-harness command first.

If ordinary automatic sync fails, the terminal dashboard exits with an error. Refresh unaffected harnesses with `tokeninsights sync --harness <harness>`, then use `tokeninsights view --no-sync` to view available data. Unfinished compatibility recovery must complete before viewing data.

### Maintenance & Debugging

Process pending normalization work from already-ingested data, for example after `sync --all --no-normalize`:

```sh
tokeninsights normalize
tokeninsights normalize --dry-run
```

#### Rebuild Canonical Tables

Purge normalized canonical facts and diagnostics without deleting raw ingested facts, observations, or source refresh state. Existing raw token facts are requeued so `tokeninsights normalize` can rebuild canonical data.

This requires compatible, fully recovered data; it cannot repair incompatible raw usage or identities.

```sh
tokeninsights reset-canonical --confirm
tokeninsights normalize
```

#### Reset Local Database

Transactionally reset application tables in the existing SQLite file to start fresh. This clears raw facts, canonical facts, pending normalization work, and source refresh state. Explicit reset remains useful when local source availability changes, such as a Codex parent transcript becoming available after ambiguous history was imported.

```sh
tokeninsights reset-all --confirm
```

## Development

Development uses pnpm and Node 26+. Go remains independently buildable for production.

```sh
# Install workspace dependencies
pnpm install

# Format all Go and TypeScript/JavaScript/React code
pnpm run format

# Verify formatting without changes, then lint all code
pnpm run format:check
pnpm run lint

# Verify Go embeds and schema synchronization
pnpm run check-schema

# Run all tests across the repository packages
pnpm run test

# Build local binaries
pnpm run build

# Build a deterministic sanitized fixture database for development
pnpm run dev:data

# Open the TUI against the fixture database
pnpm run dev:cli

# Serve the web dashboard against the fixture database on loopback
pnpm run dev:web

# Install the locally compiled CLI binary
pnpm run install:cli
```

The development commands use the shared fixture under `packages/cli/testdata/conformance/sync-first-basic/source/`. It contains compact, harness-native structures for OpenCode, Pi, Codex, and Claude Code, but every value is synthetic. `dev:data` recreates the ignored `.tokeninsights-dev/` directory, materializes the OpenCode SQLite source from reviewable SQL, and writes `.tokeninsights-dev/tokeninsights.sqlite`. Do not commit raw harness databases or transcripts. The existing `pnpm run start:web` remains unchanged and uses normal local sources.

This is a pnpm monorepo with Go and TypeScript packages. Browser code uses Vite and React. Node scripts use native, erasable TypeScript on Node 26+. Root tooling lives in the private `@tokeninsights/build-tools` workspace package under `tools/build`; root pnpm commands are stable orchestration aliases. Node and pnpm are needed only for builds, tests, and development. Production is one native Go binary: committed browser assets are embedded with `go:embed`, served by Go, and executed only by the browser. The host running `tokeninsights` needs no JavaScript runtime or `node_modules`.

### Important Documentation

For deeper details, refer to:

- **[Design Guide](docs/design.md)**: Core architecture, SQLite schema definition, data pipelines, and canonical invariants.
- **[Web Visual Language](DESIGN.md)**: React design principles, semantic tokens, shared components, accessibility, and responsive rules.
- **[Development Guide](docs/development.md)**: Comprehensive setup, local testing, and package structure details.
- **[Release Guide](docs/release.md)**: Details on CLI releases, tagging rules, and CI automation workflows.
