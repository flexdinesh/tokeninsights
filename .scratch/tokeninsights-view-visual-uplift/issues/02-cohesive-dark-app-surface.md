# Cohesive Dark App Surface

Status: ready-for-agent
Type: AFK

## Parent

[TokenInsights View Visual Uplift PRD](../PRD.md)

## What to build

Make `view` own a cohesive dark TokenInsights visual surface so the TUI renders consistently in light and dark terminal themes. The title, tab bar, table area, footer, popups, borders, and full-screen whitespace should all use a deliberate dark palette rather than inheriting terminal defaults.

The work should remain visual-only and should not change viewer data, command behavior, or product scope.

## Acceptance criteria

- [ ] The full TUI rectangle is painted with a cohesive TokenInsights dark surface when terminal dimensions are known.
- [ ] Title, tabs, table area, footer, and popups use compatible background shades.
- [ ] Light terminal themes do not show unpainted white or default terminal whitespace inside the TUI.
- [ ] Popup placement uses the same app surface treatment as the normal view.
- [ ] The palette is centralized as named values or an equivalent maintainable structure.
- [ ] Borders and panel backgrounds remain visually distinct without creating harsh blocks.
- [ ] The visual treatment preserves existing tabs, filters, buckets, sort behavior, and footer content.
- [ ] No theme settings, terminal theme detection, or user-selectable theme configuration is added.
- [ ] No schema, SQL aggregation, token semantic, or product-surface changes are made.
- [ ] Focused CLI view/model tests pass.
- [ ] Repo-level test and build commands pass.

## Blocked by

None - can start immediately

