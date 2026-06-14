# OpenCode Bun SQLite Migration

## Summary

Migrate only `packages/plugins/opencode-server` from `node:sqlite` to `bun:sqlite`; leave Pi on `node:sqlite`.

## Key Implementation Changes

- Update `packages/plugins/opencode-server/src/index.ts` to import `Database` from `bun:sqlite`.
- Replace direct request DB open with Bun SQLite using `{ create: true, strict: true }`.
- Update `packages/plugins/opencode-server/src/oc-tokeninsights-writer.ts` to import `Database` from `bun:sqlite`.
- Update writer DB type references and remove `node:sqlite` transaction state check.
- Keep existing named bind objects unchanged via Bun SQLite `strict: true`.
- Remove `engines.node` from `packages/plugins/opencode-server/package.json`.
- Mark `bun:sqlite` external in `packages/plugins/opencode-server/tsup.config.mjs` if build needs it.
- Update `docs/design.md` and `README.md` to document OpenCode on Bun SQLite and Pi on Node SQLite.
- Do not change Pi plugin source.
- Do not change schema.

## Verification

- Run `pnpm run build:opencode-server`.
- Run `pnpm run build:pi` to verify Pi remains valid.
- Run `pnpm run build:plugins` if targeted builds pass.
- If Bun CLI exists, run a smoke test for `bun:sqlite` named params with `strict: true`.

## Decisions

- Remove `opencode-server` `engines.node`.
- Keep Pi on `node:sqlite`.
- No schema changes.

## Tradeoffs And Risks

- `bun:sqlite` is runtime-specific, so the OpenCode server plugin will no longer run under plain Node.
- Build/type tooling may need Bun-specific handling if it rejects `bun:sqlite`.
- Existing `pnpm run build:plugins` can fail at vendor step when `packages/plugins/opencode-server/deploy` is non-empty.

## Execution Guidance

If execution deviates from this plan, update this file to reflect the latest approved plan and surface the deviation to the user.

## Open Questions

- none
