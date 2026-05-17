---
title: Move plugin source into src directories
description: Plugin packages keep implementation code under src and are installed through package mechanisms.
date: 2026-05-17
slug: plugin-src-layout
status: implemented
tags:
  - plugins
  - packaging
  - opencode
  - pi
related_paths:
  - plugins/opencode-server/src/index.ts
  - plugins/opencode-tui/src/index.tsx
  - plugins/pi/src/index.ts
  - plugins/opencode-server/package.json
  - plugins/opencode-tui/package.json
  - plugins/pi/package.json
  - README.md
  - docs/design.md
---

## Why

Plugin package roots were mixing package metadata with source files. Moving implementation code into `src/` makes all plugin packages follow a conventional package layout and keeps entrypoint paths explicit.

## What

All source code for the OpenCode server plugin, OpenCode TUI plugin, and Pi extension lives under each package's `src/` directory.

OpenCode packages are configured by package name after `pnpm link --global`:

- `@tokeninsights/opencode-server`
- `@tokeninsights/opencode-tui`

The Pi extension is configured as a Pi package after `pnpm link --global` using the package name:

- `pi-tokeninsights`

No Pi root `index.ts` shim is kept, and Pi extension-directory symlinks are no longer part of the documented workflow.

## How

Package manifests point at `src` entrypoints where runtime supports TypeScript directly. The TUI package still exports compiled `dist/index.js`, but builds from `src/index.tsx`.

The Pi package declares:

```json
{
  "pi": {
    "extensions": ["./src/index.ts"]
  }
}
```

Schema validation reads OpenCode row types from `plugins/opencode-server/src/types.ts`.

## Tradeoffs and gotchas

Pi must load `pi-tokeninsights` through the package mechanism, not auto-discover it from `~/.pi/agent/extensions/*/index.ts`.

The OpenCode TUI package still requires a build step before linking or use, because runtime export remains `dist/index.js`.
