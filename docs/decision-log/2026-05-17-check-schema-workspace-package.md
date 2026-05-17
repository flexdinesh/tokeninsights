---
title: Check schema workspace package
description: Promote the schema checker into its own pnpm workspace package without TypeScript build config.
date: 2026-05-17
slug: check-schema-workspace-package
status: implemented
tags:
  - pnpm
  - workspace
  - schema
related_paths:
  - packages/check-schema/check-schema.ts
  - packages/check-schema/package.json
  - package.json
  - pnpm-workspace.yaml
---

## Why

`check-schema.ts` should be managed as a proper pnpm monorepo workspace package rather than as a loose root-invoked script.

## What

The package lives at `packages/check-schema` and is named `check-schema`. The root `check-schema` script invokes it with `pnpm --filter check-schema run check-schema`.

## How

Node 24 runs `check-schema.ts` directly, so the package intentionally does not include a `tsconfig.json`, `build` script, `typescript`, or `@types/node` dependencies just for this script. Validation for this package is execution-based via `pnpm run check-schema`.

## Tradeoffs

The checker is not typechecked as part of root `pnpm run build`; keeping it lightweight avoids unnecessary package config for a script Node can execute directly.
