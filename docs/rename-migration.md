# Migration Guide: tokeninspector → tokeninsights

This project has been renamed from **tokeninspector** to **tokeninsights**. This guide covers everything you need to do manually to continue using the plugins and CLI.

---

## 1. Environment Variables (REQUIRED)

The environment variable prefixes have changed from `TOKENINSPECTOR_` to `TOKENINSIGHTS_`.

| Old | New |
|---|---|
| `TOKENINSPECTOR_DB_PATH` | `TOKENINSIGHTS_DB_PATH` |
| `TOKENINSPECTOR_RETENTION_DAYS` | `TOKENINSIGHTS_RETENTION_DAYS` |

**Action:** Update your shell profile (`.bashrc`, `.zshrc`, etc.) or any scripts that set these variables.

```bash
# Before (OLD — no longer works)
export TOKENINSPECTOR_DB_PATH=/path/to/custom.db
export TOKENINSPECTOR_RETENTION_DAYS=90

# After (NEW)
export TOKENINSIGHTS_DB_PATH=/path/to/custom.db
export TOKENINSIGHTS_RETENTION_DAYS=90
```

---

## 2. CLI Binary (REQUIRED)

The CLI binary name has changed from `tokeninspector-cli` to `tokeninsights-cli`.

**Action:** Update any shell aliases, scripts, or documentation that reference the old binary name.

```bash
# Before (OLD)
tokeninspector-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --today

# After (NEW)
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --today
```

If you had the old binary in your PATH, rebuild from source:

```bash
pnpm run build:cli
```

---

## 3. Database File Migration (REQUIRED — existing data)

The default database file name and state directory have changed:

| Old | New |
|---|---|
| `~/.local/state/tokeninspector/tokeninspector.sqlite` | `~/.local/state/tokeninsights/tokeninsights.sqlite` |
| `$XDG_STATE_HOME/tokeninspector/tokeninspector.sqlite` | `$XDG_STATE_HOME/tokeninsights/tokeninsights.sqlite` |
| `$PWD/.tokeninspector-state/tokeninspector.sqlite` | `$PWD/.tokeninsights-state/tokeninsights.sqlite` |

**Action:** Migrate your existing database file to the new location:

```bash
# Create the new state directory
mkdir -p ~/.local/state/tokeninsights

# Move the existing database
mv ~/.local/state/tokeninspector/tokeninspector.sqlite \
   ~/.local/state/tokeninsights/tokeninsights.sqlite

# Optional: remove the old empty directory
rmdir ~/.local/state/tokeninspector 2>/dev/null || true
```

If you use a custom DB path via `TOKENINSIGHTS_DB_PATH` (or `TOKENINSPECTOR_DB_PATH` before the rename), you do not need to move the file — just update the environment variable (see §1).

---

## 4. OpenCode Plugin Configuration (REQUIRED for OpenCode users)

### 4a. Remove the old plugin entry

Remove any `oc-tokeninspector` entry from your `~/.config/opencode/opencode.jsonc`.

### 4b. Link the plugin packages

The OpenCode plugins are package directories. Link them with pnpm instead of symlinking individual files:

```bash
cd packages/plugins/opencode-server
pnpm install
pnpm link --global

cd ../opencode-tui
pnpm install
pnpm run build
pnpm link --global
```

### 4c. Update OpenCode config

Add the new linked package entries. The plugin ID has changed from `oc-tokeninspector` to `oc-tokeninsights` (`~/.config/opencode/opencode.jsonc`):

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": [
    "@tokeninsights/opencode-server",
    "@tokeninsights/opencode-tui"
  ]
}
```

> **Do not** add `oc-tokeninsights-writer.ts` or `writer-client.ts` to the config. The writer is a worker module loaded internally by the server plugin package.

---

## 5. Pi Extension (REQUIRED for Pi users)

The Pi extension package name has changed:

| Old | New |
|---|---|
| `pi-tokeninspector` | `pi-tokeninsights` |

**Action:** Link the package and add it to Pi settings:

```bash
cd packages/plugins/pi
pnpm install
pnpm link --global
```

Pi settings (`~/.pi/agent/settings.json`):

```json
{
  "packages": ["pi-tokeninsights"]
}
```

---

## 6. Git Remote (REQUIRED for contributors)

If you push to the GitHub repository, update the remote URL after the repository is renamed on GitHub:

```bash
git remote set-url origin git@github.com:flexdinesh/tokeninsights.git
```

> This step requires the repository owner to rename the GitHub repository first.

---

## Quick Checklist

- [ ] Rename environment variables (`TOKENINSPECTOR_*` → `TOKENINSIGHTS_*`)
- [ ] Update CLI binary references (`tokeninspector-cli` → `tokeninsights-cli`)
- [ ] Move database file to new location (if using default path)
- [ ] Link OpenCode plugin packages with `pnpm link --global`
- [ ] Update OpenCode config with package names (`@tokeninsights/opencode-server`, `@tokeninsights/opencode-tui`)
- [ ] Link Pi package with `pnpm link --global` and add `pi-tokeninsights` to Pi `packages` settings
- [ ] Update git remote URL (after GitHub repo rename)
- [ ] Rebuild CLI from source: `pnpm run build:cli`
