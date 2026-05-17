---
title: Package OpenCode plugins for pnpm link
description: OpenCode plugins are self-contained package directories instead of file symlinks with a shared folder.
date: 2026-05-17
slug: package-opencode-plugins
status: implemented
tags:
  - opencode
  - plugins
  - packaging
  - pnpm
related_paths:
  - plugins/opencode-server/package.json
  - plugins/opencode-server/index.ts
  - plugins/opencode-tui/package.json
  - plugins/opencode-tui/index.tsx
  - plugins/pi/package.json
  - README.md
  - docs/design.md
---

## Why

The previous OpenCode setup symlinked individual plugin files and depended on `plugins/shared/` for shared TypeScript modules. That made local installation less package-like and required special handling for internal worker/shared files.

## What

OpenCode plugins are now package-shaped directories, matching the Pi extension pattern:

- `plugins/opencode-server/` is package `@tokeninsights/opencode-server`.
- `plugins/opencode-tui/` is package `@tokeninsights/opencode-tui`.
- Entrypoints are package-standard `index.ts` and `index.tsx`.
- `plugins/shared/` is removed.
- Each OpenCode plugin owns only the files it needs.
- Plugin installation docs use `pnpm link --global` and package names instead of symlinked file paths.
- Pi dependency workflow uses pnpm instead of npm.

## How

The server package owns the shared implementation required for durable OpenCode writes:

- `types.ts`
- `schema-migrate.ts`
- `writer-client.ts`
- `oc-tokeninsights-writer.ts`

The TUI package only keeps a minimal local `types.ts` for its read-only/live display types. Schema validation reads TS row types from `plugins/opencode-server/types.ts`.

## Tradeoffs and gotchas

Duplicating only needed files avoids dead code in the TUI package, but shared concepts are no longer centralized in a single folder. Future schema/type updates should treat `plugins/opencode-server/types.ts` as the OpenCode TypeScript row-type source for schema validation.

OpenCode package resolution should be checked when updating install docs or changing package names, because package-link behavior depends on how OpenCode resolves plugins from config.
