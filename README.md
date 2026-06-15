# TokenInsights

Local token usage tracking for OpenCode, Pi, Codex, and Claude Code.

TokenInsights is a sync-first Go CLI. It ingests durable local harness data into SQLite, normalizes raw facts into canonical token usage, and opens an interactive terminal view over canonical tables.

![TokenInsights TUI showing token usage by model](assets/tokeninsights-view-models.png)

## Install

### Homebrew (stable)

```sh
brew install flexdinesh/tap/tokeninsights
```

### Go (stable)

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@latest
```

### Go (development branch)

```sh
go install github.com/flexdinesh/tokeninsights/packages/cli/cmd/tokeninsights@dev
```

## Usage

### Primary Commands

#### 1. Ingest Data (`sync`)

Ingest raw local data from supported harnesses (`opencode`, `pi`, `codex`, or `claude-code`).

- **Sync all supported harnesses:**

  ```sh
  tokeninsights sync --all
  ```

- **Sync a specific harness:**

  ```sh
  tokeninsights sync --harness pi
  ```

- **Useful Sync Options:**

  ```sh
  # Discover and parse files without writing to the database
  tokeninsights sync --all --dry-run

  # Skip automatic canonical normalization after ingestion
  tokeninsights sync --all --no-normalize

  # Override default harness source directory
  tokeninsights sync --all --source-dir /path/to/custom/fixtures
  ```

#### 2. Open TUI View (`view` or no command)

Launch the interactive terminal user interface (TUI) to analyze canonical tables. You can pre-filter the data or set time buckets.

- **Open default view (this month, daily buckets):**

  ```sh
  tokeninsights view
  ```

- **View preset date ranges:**

  ```sh
  tokeninsights view --today
  tokeninsights view --yesterday
  tokeninsights view --week
  tokeninsights view --year
  tokeninsights view --all-time
  ```

- **Filter view data and choose time buckets:**

  ```sh
  tokeninsights view --month --bucket day
  tokeninsights view --week --provider openai --model gpt-5
  tokeninsights view --harness pi
  ```

### Rarely Needed Commands / Maintenance & Debugging

These commands are primarily used for pipeline debugging, diagnostics, or manual database management.

#### Rebuild Canonical Tables (`normalize`)

Rebuild canonical facts and diagnostic records from already-ingested raw facts. Typically run automatically by `sync`.

```sh
# Normalize all harnesses
tokeninsights normalize

# Dry-run normalization to preview canonical changes
tokeninsights normalize --dry-run
```

#### Purge Canonical Tables (`reset-canonical`)

Purge normalized canonical facts and diagnostics without deleting raw ingested facts.

```sh
tokeninsights reset-canonical --confirm
```

#### Reset Local Database (`reset-all`)

Completely wipe and recreate the local SQLite database and its sidecars to start fresh.

```sh
tokeninsights reset-all --confirm
```

### Default Database Path

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

_You can customize the DB path using the `--db-path` flag or the `TOKENINSIGHTS_DB_PATH` environment variable._

## Development

```sh
# Verify Go embeds and schema synchronization
pnpm run check-schema

# Run all tests across the repository packages
pnpm run test

# Build local binaries
pnpm run build

# Install the locally compiled CLI binary
pnpm run install:cli
```

### Important Documentation

For deeper details, refer to:

- **[Design Guide](docs/design.md)** — Core architecture, SQLite schema definition, data pipelines, and canonical invariants.
- **[Development Guide](docs/development.md)** — Comprehensive setup, local testing, and package structure details.
- **[Release Guide](docs/release.md)** — Details on CLI releases, tagging rules, and CI automation workflows.
