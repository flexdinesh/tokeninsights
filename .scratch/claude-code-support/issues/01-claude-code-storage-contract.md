# Add the `claude-code` harness storage contract

Type: AFK

Status: ready-for-agent

## Parent

Claude Code Harness Support PRD

## What to build

Make `claude-code` a valid persisted harness value across the schema contract, command validation, supported harness registry, and product documentation. The completed slice should let the CLI and database contract recognize Claude Code as a harness even before full transcript parsing is implemented.

This is a storage compatibility break by project decision: bump the schema version and rely on the existing reset-all instruction for older local databases.

## Acceptance criteria

- [ ] The schema contract accepts `claude-code` for every persisted harness column.
- [ ] The schema version is bumped and all schema copies/constants agree.
- [ ] Harness validation accepts `claude-code` for sync, normalize, and viewer filters.
- [ ] Supported harness enumeration includes Claude Code in `sync --all`.
- [ ] README and design documentation describe Claude Code as supported in the storage contract and note that older databases require reset-all.
- [ ] Schema validation passes.
- [ ] Existing OpenCode, Pi, and Codex tests continue to pass.

## Blocked by

None - can start immediately
