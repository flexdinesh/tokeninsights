# Multi-Line Summary Values

## Summary

Replace `N values` summaries in the viewer with the actual distinct values, rendered one per line inside the table row.

## Key Implementation Changes

- Change viewer aggregation in `packages/cli/internal/db/aggregate.go` so summary fields preserve all distinct values instead of compacting more than two values to `N values`.
- Keep schema unchanged.
- Update `packages/cli/internal/cli/render.go` so `models`, `providers`, and `harnesses` columns split summary values into multiple physical lines.
- Apply this anywhere those summary fields are rendered, including the `models`, `providers`, `harnesses`, and `sessions` tabs.
- Use blank continuation cells for non-list columns, so metrics and the primary row identity appear once on the first line.
- Update viewport height and scroll calculations in `packages/cli/internal/cli/table.go` to handle taller logical rows without broken scrolling.
- Update `README.md`, `packages/cli/README.md`, and `docs/design.md` to describe stacked summary values instead of `N values`.

## Tests / Verification

- Update aggregation tests that currently expect `N values`.
- Add render tests for multi-line values in dimension tabs and the sessions tab.
- Add or update scrolling/height tests so taller rows do not break viewport stability.
- Run `pnpm run test`.
- Run `pnpm run build`.

## Decisions Made

- Reveal all values instead of keeping `N values`.
- Apply wherever the viewer summary fields would previously show `N values`.
- Use blank continuation lines rather than repeating primary values or adding bullets.

## Tradeoffs / Risks

- Rows can become taller, so fewer logical rows fit on screen.
- Long value lists may still need per-item truncation to avoid horizontal overflow.
- Scroll math must account for logical rows with variable rendered height so navigation remains predictable.

## Execution Guidance

If implementation needs to deviate from this plan, update this plan file first and surface the deviation before continuing.
