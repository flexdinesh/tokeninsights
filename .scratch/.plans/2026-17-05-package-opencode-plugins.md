# Package OpenCode Plugins for pnpm link

## Summary
Restructure `plugins/opencode-server/` and `plugins/opencode-tui/` to be self-contained package directories like `plugins/pi/`, remove reliance on `plugins/shared/`, rename entrypoints to `index.ts` / `index.tsx`, and update docs/scripts for `pnpm link` package installation.

## Key implementation changes

1. Add package manifests:
   - `plugins/opencode-server/package.json` with name `@tokeninsights/opencode-server`, entry/export `./index.ts`.
   - `plugins/opencode-tui/package.json` with name `@tokeninsights/opencode-tui`, entry/export `./index.tsx`.
2. Move/duplicate shared files:
   - Server package gets local copies of `types.ts`, `schema-migrate.ts`, `writer-client.ts`, and `oc-tokeninsights-writer.ts`.
   - TUI package gets only the local types it needs.
3. Rename entrypoints:
   - `plugins/opencode-server/oc-tokeninsights-server.ts` to `plugins/opencode-server/index.ts`.
   - `plugins/opencode-tui/oc-tokeninsights.tsx` to `plugins/opencode-tui/index.tsx`.
4. Update imports and paths:
   - Server imports change from `../shared/*` to `./*`.
   - Server worker URL changes to `new URL("./oc-tokeninsights-writer.ts", import.meta.url)`.
   - TUI imports local `./types.ts`.
5. Remove obsolete `plugins/shared/` after references are removed.
6. Update root scripts for new entrypoint paths and use `pnpm` for Pi typecheck.
7. Update `scripts/check-schema.ts` to validate TS row types from `plugins/opencode-server/types.ts`.
8. Update Pi workflow to `pnpm` and remove the npm lockfile.
9. Update `README.md`, `docs/design.md`, and `docs/rename-migration.md` for package-based install/linking and new file organization.

## Verification
Run:

```sh
npm run check-schema
npm run smoke:plugins
```

Optionally full verification:

```sh
npm run verify:all
```

## Decisions made by user
- Use scoped package names.
- Duplicate only the files each plugin needs.
- Rename OpenCode plugin entrypoints to package-style `index.ts` / `index.tsx`.
- Update installation docs to use `pnpm link`.
- Update Pi workflow/docs to use `pnpm` instead of `npm`.

## Tradeoffs and risks
- The exact OpenCode package config shape should be verified against current OpenCode behavior.
- `pnpm link --global` may need a consumer-side link step depending on how OpenCode resolves local packages.
- No schema changes are planned.

## Execution guidance
If execution deviates, update this plan file to reflect the latest approved plan and surface the deviation to the user.
