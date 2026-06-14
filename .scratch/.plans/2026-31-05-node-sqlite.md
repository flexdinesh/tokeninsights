# Replace better-sqlite3 with node:sqlite

## Summary

Migrate plugin SQLite writes from `better-sqlite3` to Node's built-in `node:sqlite` and require Node.js 25 or newer for the repo and plugins.

## Implementation

- Replace plugin DB imports with `DatabaseSync` from `node:sqlite`.
- Keep synchronous write behavior for OpenCode request storage, OpenCode token worker storage, and Pi extension storage.
- Replace `better-sqlite3` transaction helper usage with explicit `BEGIN`, `COMMIT`, and `ROLLBACK` logic.
- Update Pi positional statement bindings from array arguments to spread arguments.
- Remove `better-sqlite3` and `@types/better-sqlite3` from plugin dependencies.
- Remove `better-sqlite3` from plugin bundle externals and generated dist package metadata.
- Set root and plugin Node engine requirements to `>=25`.
- Set plugin `tsup` targets to `node25`.
- Refresh workspace lockfile and remove stale package-local Pi lockfile.
- Update docs to describe `node:sqlite` and Node.js 25 requirement.

## Tests

- Run `pnpm install`.
- Run `pnpm run build:plugins`.
- Run `pnpm run check-schema`.
- Run `pnpm run build`.
- Run `pnpm run test`.

## Decisions

- Root `package.json` engine updated to `>=25`, not just plugin packages.
- No schema changes.
- Historical `.ai/plans` references to `better-sqlite3` remain unchanged.

## Tradeoffs And Risks

- Node.js 25 is now required for plugin runtime and repo builds.
- `node:sqlite` has no `better-sqlite3` transaction helper, so transactions are managed manually.
- Removing native sqlite deps reduces install/build friction but depends on Node's bundled SQLite API.

## Open Questions

- None.

## Execution Guidance

If execution deviates, update this plan to reflect the latest approved plan and surface the deviation to the user.
