# Move TokenInsights default DB path to local share

## Summary

Hard switch all TokenInsights plugins and the CLI to default to:

```text
~/.local/share/tokeninsights/tokeninsights.sqlite
```

When `XDG_DATA_HOME` is set, use:

```text
$XDG_DATA_HOME/tokeninsights/tokeninsights.sqlite
```

No automatic fallback or migration from `~/.local/state/tokeninsights/tokeninsights.sqlite` will be added.

## Key implementation changes

- Update plugin default path helpers in:
  - `packages/plugins/opencode-server/src/index.ts`
  - `packages/plugins/opencode-tui/src/index.tsx`
  - `packages/plugins/pi/src/index.ts`
- Replace `XDG_STATE_HOME` with `XDG_DATA_HOME`.
- Replace `~/.local/state/tokeninsights` with `~/.local/share/tokeninsights`.
- Keep `TOKENINSIGHTS_DB_PATH` override behavior unchanged:
  - absolute configured paths stay absolute
  - relative configured paths resolve under the TokenInsights data directory
- Add a CLI default DB path in `packages/cli/internal/cli/flags.go`.
- Make `--db-path` optional.
- Allow `tokeninsights-cli` with no args to use the default DB and default current-week period.
- Update CLI usage text and tests.
- Rebuild tracked outputs:
  - `packages/plugins/opencode-tui/dist/index.js`
  - `packages/cli/tokeninsights-cli`
- Update user-facing docs and agent guidance:
  - `README.md`
  - `packages/cli/README.md`
  - `docs/design.md`
  - `AGENTS.md`
  - `docs/rename-migration.md` where needed

## Tests / verification

Run:

```sh
pnpm run test
pnpm run build
```

Check for stale current references:

```sh
grep -R ".local/state/tokeninsights\|XDG_STATE_HOME" README.md AGENTS.md docs packages
```

Historical decision logs and old plan files may still mention prior paths.

## Decisions made by the user

- Hard switch to `~/.local/share` / `XDG_DATA_HOME`.
- Do not auto-discover, fallback to, or migrate from `~/.local/state`.

## Tradeoffs and risks

- Existing users must move their DB manually or set `TOKENINSIGHTS_DB_PATH`.
- Missing one component could split reads and writes across different DB files, so plugin sources, CLI defaults, docs, tests, dist output, and binary output must be updated together.

## Remaining open questions

None.

## Execution guidance

If execution deviates from this plan, update this plan file to reflect the latest approved plan, and surface the deviation to the user with the reason.
