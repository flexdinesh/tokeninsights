---
title: Rename Pi package to @tokeninsights/pi
description: Pi extension package uses the scoped TokenInsights package name everywhere.
date: 2026-05-17
slug: pi-package-name
status: implemented
tags:
  - pi
  - packaging
  - naming
related_paths:
  - packages/plugins/pi/package.json
  - packages/plugins/pi/src/index.ts
  - package.json
  - README.md
  - docs/design.md
---

## Why

The Pi extension package should follow the same scoped TokenInsights package naming convention as the OpenCode packages. Using `@tokeninsights/pi` makes package configuration and monorepo scripts consistent across supported harness integrations.

## What

The Pi package name is `@tokeninsights/pi`.

All active references should use this package name, including:

- Pi settings package entries
- root pnpm build filters
- design docs and README install instructions
- runtime log prefixes from the Pi extension

## How

The package manifest was renamed, the root `build:pi` filter was updated, docs were updated, and the Pi extension log prefix now uses `@tokeninsights/pi`.

## Tradeoffs

Historical plans and decision logs were also updated to keep repository search results clean. This makes those archival notes anachronistic, but matches the project preference for consistent current naming across docs.
