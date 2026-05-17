# Simplify pnpm Workspace Scripts

## Summary

Simplify root package scripts to use pnpm workspace filters instead of `cd`, while preserving plugin package names and `pnpm link --global` behavior from each plugin directory.

## Key implementation changes

- Add workspace package globs for `plugins/*` and `cli` in `pnpm-workspace.yaml`.
- Add `cli/package.json` with `tokeninsights-cli` package metadata and Go `build`/`test` scripts.
- Update root `package.json` scripts to keep one schema check script, one build script per plugin/CLI, one aggregate `build` script, and one aggregate `test` script.
- Update `plugins/opencode-server/package.json` so its single `build` script validates both the server entrypoint and writer.
- Update `plugins/pi/package.json` so its build command is exposed as `build`.
- Update README, docs/design.md, and AGENTS.md script references to the simplified command names.

## Tests and verification

Run:

```sh
pnpm run check-schema
pnpm run build
pnpm run test
```

## Decisions made

- Use pnpm workspace filters for plugins and CLI.
- Add the Go CLI as a pnpm workspace package only for script orchestration.
- Use root script name `test`, not `test:all`.
- Keep plugin package names, exports, and link behavior unchanged.

## Tradeoffs and risks

- Root `pnpm install` may update the lockfile with plugin/CLI workspace importers; this is expected for a fuller pnpm workspace setup.
- `pnpm link --global` from plugin directories remains supported because package names and manifests remain unchanged.

## Execution guidance

If execution deviates from this plan, update this plan file to reflect the latest approved plan and surface the deviation to the user.
