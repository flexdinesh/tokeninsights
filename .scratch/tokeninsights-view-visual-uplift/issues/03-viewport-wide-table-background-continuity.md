# Viewport-Wide Table Background Continuity

Status: ready-for-agent
Type: AFK

## Parent

[TokenInsights View Visual Uplift PRD](../PRD.md)

## What to build

Ensure table row, header, separator, empty-state, and blank-row backgrounds continue across the full table viewport width, including space to the right of the last rendered column and space introduced by horizontal scrolling.

The table should pad each rendered line with the correct line style before ANSI-aware horizontal slicing, so sparse tabs and scrolled views do not show mismatched app backgrounds.

## Acceptance criteria

- [ ] Header backgrounds continue across the full table viewport width.
- [ ] Row banding continues across the full table viewport width, including empty horizontal space to the right of the last column.
- [ ] Separator and blank table lines render against the intended table/app surface.
- [ ] Empty states and loading states do not reveal mismatched terminal or app backgrounds.
- [ ] Horizontal viewport slicing remains ANSI-aware.
- [ ] Horizontal scrolling still works for tables wider than the viewport.
- [ ] Tables narrower than the viewport still paint the whole table viewport.
- [ ] Loading and loaded table heights remain stable.
- [ ] Existing renderer tests for ANSI clipping and padding continue to pass.
- [ ] No schema, SQL aggregation, token semantic, or product-surface changes are made.
- [ ] Repo-level test and build commands pass.

## Blocked by

- [01 - Semantic Table Styling And Sort Marker](01-semantic-table-styling-and-sort-marker.md)
- [02 - Cohesive Dark App Surface](02-cohesive-dark-app-surface.md)

