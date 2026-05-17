# Move Plugin Source into src/ and Use Package Installs

## Summary

Restructure every plugin under `plugins/` so implementation code lives in `src/`, update package entrypoints/build scripts/docs, and install all plugins through package mechanisms using `pnpm link --global` plus package-name config entries.

## Key implementation changes

- Move source files:
  - `plugins/opencode-server/*.ts` -> `plugins/opencode-server/src/*.ts`
  - `plugins/opencode-tui/index.tsx`, `plugins/opencode-tui/types.ts` -> `plugins/opencode-tui/src/`
  - `plugins/pi/index.ts` -> `plugins/pi/src/index.ts`
- Update package manifests:
  - OpenCode server:
    - `main` / `exports`: `./src/index.ts`
    - scripts use `src/index.ts` and `src/oc-tokeninsights-writer.ts`
  - OpenCode TUI:
    - build input: `src/index.tsx`
    - output remains `dist/index.js`
    - `main` / `exports` remain `./dist/index.js`
  - Pi:
    - `main` / `exports`: `./src/index.ts`
    - add `pi.extensions: ["./src/index.ts"]`
    - typecheck script targets `src/index.ts`
- Update root scripts in `package.json`.
- Update `scripts/check-schema.ts` to read `plugins/opencode-server/src/types.ts`.
- Update schema file path inside server worker from `../../schema/schema.sql` to `../../../schema/schema.sql`.
- Update docs:
  - README code organization/path references.
  - README install instructions for `pnpm link --global` and package-name config entries.
  - `docs/design.md` path table and references.
  - `docs/rename-migration.md` stale install/path references where applicable.
- Rebuild tracked TUI bundle at `plugins/opencode-tui/dist/index.js`.

## Installation decisions

- OpenCode uses global package links and package names in `opencode.json`:
  - `@tokeninsights/opencode-server`
  - `@tokeninsights/opencode-tui`
- Pi uses global package links and package names in Pi `settings.json` `packages`:
  - `@tokeninsights/pi`
- No Pi extension-directory symlink.
- No root `plugins/pi/index.ts` shim.

## Tests or verification

Run:

```sh
pnpm run check-schema
pnpm run smoke:plugins
```

## Tradeoffs and risks

- Pi must load the extension as a package via its package mechanism, not extension-directory auto-discovery.
- OpenCode server runs TypeScript from `src/index.ts`, matching the current native TypeScript runtime assumption.
- OpenCode TUI still needs compiled `dist/index.js`.

## Execution guidance

If implementation needs to deviate from this plan, update this plan file first and surface the deviation before continuing.
