# OpenCode Deploy Build

## Summary

Build OpenCode plugin to `dist`, then vendor it with a separate `pnpm deploy` script into `deploy`.

## Changes

- Add `injectWorkspacePackages: true` to `pnpm-workspace.yaml`.
- Update `packages/plugins/opencode-server/package.json` scripts:
  - run check
  - run `tsup`
  - copy build extras via `scripts/build-dist.mjs`
  - add `vendor` to run `pnpm --filter @tokeninsights/opencode-server --prod deploy ./deploy`
- Update root `build:plugins` to build all plugins, then run OpenCode vendor.
- Update package entrypoints/files so deployed package runs `./dist/index.js`.
- Clean generated `deploy` before vendoring and stale `dist/node_modules` before deploy packaging.
- Gitignore `packages/plugins/opencode-server/deploy/`.
- Update README symlink from `opencode-server/dist` to `opencode-server/deploy`.

## Verification

- `pnpm install`
- `pnpm run build:plugins`
- Inspect `packages/plugins/opencode-server/deploy/package.json`.
- Confirm `deploy/node_modules` present.
- Confirm README uses `deploy`.

## Decisions

- Use `injectWorkspacePackages: true`.
- Do not use `--legacy`.
- Use `--prod`.
- Use explicit pnpm filter because `pnpm deploy` selected no project inside the package script.
- Clean generated artifact dirs because stale `dist/node_modules` can be copied into `deploy/dist`.
- Split build and vendor scripts per user request.

## Risks

- Workspace install behavior changes; requires `pnpm install`.

## Open Questions

- None.

## Execution Guidance

If execution deviates, update this plan to reflect the latest approved plan and surface the deviation to the user.
