# Semantic Table Styling And Sort Marker

Status: ready-for-agent
Type: AFK

## Parent

[TokenInsights View Visual Uplift PRD](../PRD.md)

## What to build

Improve the `view` table's visual hierarchy without changing canonical query behavior. Token categories should use stable semantic colors across all Aggregation Tabs, row identity columns should be visually distinct, companion dimensions should be quieter, totals should be emphasized, rows should be subtly banded, separators should be muted, and the active sort column should be marked in the header.

The result should be a denser, more legible token table while preserving existing tabs, filters, buckets, sorting semantics, token semantics, and horizontal scrolling.

## Acceptance criteria

- [ ] Input, output, reasoning, cache read, cache write, and total columns use stable distinct colors across all Aggregation Tabs.
- [ ] Total token cells are visually emphasized compared with component token cells.
- [ ] Primary row identity columns use a consistent dimension accent.
- [ ] Companion dimension columns use quieter styling than primary row identity columns.
- [ ] Rows use subtle banding that improves horizontal scanning.
- [ ] Borders and separators are muted so table data carries the main visual weight.
- [ ] The active sort column is marked in the header with a minimal indicator.
- [ ] Header width calculations and horizontal scroll bounds account for the sort indicator.
- [ ] ANSI-styled output preserves numeric alignment and table column widths.
- [ ] Loading and loaded table widths remain stable.
- [ ] Visible-row scrolling does not resize columns when wider offscreen values exist.
- [ ] No schema, SQL aggregation, token semantic, or product-surface changes are made.
- [ ] Cost columns, cost totals, and cost sorting are not introduced.
- [ ] Focused CLI renderer/model tests cover externally visible behavior.
- [ ] Repo-level test and build commands pass.

## Blocked by

None - can start immediately

