# Promote check-schema to a workspace package

## Summary
Rename `packages/scripts` to `packages/check-schema`, make it a pnpm workspace package named `check-schema`, and wire root scripts through pnpm filters.

## Key implementation changes
- Move `packages/scripts/check-schema.ts` to `packages/check-schema/check-schema.ts`.
- Add `packages/check-schema/package.json`:
  - `name: "check-schema"`
  - `private: true`
  - `type: "module"`
  - scripts:
    - `check-schema`: `node check-schema.ts`
    - `build`: `tsc --noEmit`
  - dev dependencies:
    - `typescript`
    - `@types/node`
- Do not add `packages/check-schema/tsconfig.json`; Node 24 runs `check-schema.ts` directly for the package script.
- Update `pnpm-workspace.yaml` to include `packages/check-schema`.
- Update root `package.json`:
  - `check-schema`: `pnpm --filter check-schema run check-schema`
  - leave root `build` focused on distributable plugin and CLI packages.
- Run `pnpm install` to update `pnpm-lock.yaml`.

## Tests / verification
Run:

```sh
pnpm install
pnpm run check-schema
pnpm run build
pnpm run test
```

## Decisions made
- Directory should be renamed to `packages/check-schema`.
- Workspace package name should be `check-schema`.
- Root build should not include `check-schema` typechecking because the package does not need a `tsconfig.json`.

## Tradeoffs and risks
- Without a package `tsconfig.json`, `check-schema.ts` is validated by executing `pnpm run check-schema` rather than by a package build step.

## Execution guidance
If execution deviates, update this plan file to reflect the latest approved plan and surface the deviation to the user.
