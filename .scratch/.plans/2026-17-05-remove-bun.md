# Remove Bun APIs and Bun build tooling from OpenCode plugins

## Summary

Replace Bun-specific runtime and build usage with Node-native APIs and package dependencies. Server `.ts` files rely on Node native TypeScript support. TUI `.tsx` is compiled with `esbuild`.

## Key implementation changes

- Set Node minimum/runtime target to Node >=24.
- Replace `bun:sqlite` with `better-sqlite3`.
- Replace `Bun.file(...).text()` with `readFile(..., "utf8")` from `node:fs/promises`.
- Replace Bun/Web Worker style writer with Node worker threads:
  - `node:worker_threads` `Worker` in `writer-client.ts`
  - `parentPort` in `oc-tokeninsights-writer.ts`
- Keep OpenCode server package exporting native `.ts`.
- Compile only `plugins/opencode-tui/index.tsx` to `plugins/opencode-tui/dist/index.js`.
- Replace all `bun build` scripts with Node/package tooling:
  - TUI build via `esbuild`
  - server/writer smoke validation via Node-native TypeScript syntax checks/import checks where practical.
- Add package dependencies as needed:
  - `better-sqlite3`
  - `@types/better-sqlite3`
  - `esbuild`
- Avoid lockfile changes unless required.
- Update docs to remove Bun references and document Node worker thread + `better-sqlite3`.

## Tests / verification

Run:

```sh
pnpm run check-schema
pnpm run smoke:plugins
pnpm run test:go
pnpm run build:cli
```

No schema changes expected.

## Decisions made by user

- Full Bun removal, including build tooling.
- Use Node worker threads.
- Use Node native TypeScript for `.ts`.
- Compile `.tsx` only.
- Minimum Node version: Node 24+.
- Avoid lockfile changes unless required.
- Use `esbuild` for TSX build.

## Tradeoffs / risks discussed

- Native Node TypeScript only supports erasable TS syntax; implementation must avoid unsupported TS runtime constructs.
- `better-sqlite3` is a native dependency, so users need an install step for OpenCode plugin packages.
- TUI package export changes may require docs updates for local linked install.
- Worker path resolution must be tested carefully when loaded through linked OpenCode package.

## Execution guidance

If implementation deviates from this plan, update this file to reflect the latest approved approach and surface the deviation to the user.
