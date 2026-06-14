# TokenInsights View Visual Uplift PRD

Status: ready-for-agent

## Problem Statement

TokenInsights `view` is functionally useful, but its table can be hard to scan compared with denser terminal dashboards such as Tokscale. In light terminal themes, the viewer also inherits unpainted terminal whitespace, causing the TUI to look partially unstyled and visually inconsistent.

Users need the read-only Viewer Aggregation surface to feel legible, colorful, and deliberate without changing what canonical token facts are queried, how Aggregation Tabs work, or what TokenInsights tracks.

## Solution

Polish the existing `view` TUI rendering layer so the table is easier to scan and the application owns its visual surface across light and dark terminal themes.

The uplift should keep the current TokenInsights viewer model: Aggregation Tabs, Date Range Filters, Time Buckets, Dimension Filters, sorting, footer totals, and read-only canonical queries all remain unchanged. The work is a visual-only improvement over the active token viewer.

The table should use stable semantic color for token categories, subtle row banding, an active sort marker in the table header, muted framing, and a cohesive dark app surface that paints the full TUI area, including empty table space and horizontal padding.

## User Stories

1. As a TokenInsights user, I want token table rows to be easier to scan, so that I can compare usage across buckets and dimensions quickly.
2. As a TokenInsights user, I want token categories to have consistent colors, so that input, output, reasoning, cache read, cache write, and total values are visually distinct.
3. As a TokenInsights user, I want the same token color semantics across all Aggregation Tabs, so that switching tabs does not require relearning the table.
4. As a TokenInsights user, I want input token counts to read as a positive green metric, so that prompt-side usage is easy to identify.
5. As a TokenInsights user, I want output token counts to use a warm contrasting color, so that response-side usage is easy to identify.
6. As a TokenInsights user, I want reasoning token counts to use their own accent color, so that reasoning usage does not disappear into generic numeric styling.
7. As a TokenInsights user, I want cache read token counts to use a cool blue accent, so that cached input reuse is visually distinct.
8. As a TokenInsights user, I want cache write token counts to use a separate amber-style accent, so that new cache writes are visually distinct from cache reads.
9. As a TokenInsights user, I want total token counts to be visually emphasized, so that the primary comparison metric is easy to find.
10. As a TokenInsights user, I want model, provider, harness, bucket, latest, and session identity columns to use a dimension accent, so that row identities are easy to track.
11. As a TokenInsights user, I want companion dimension columns to be visually quieter, so that secondary context does not compete with the primary row identity.
12. As a TokenInsights user, I want subtle row banding, so that I can follow a row horizontally across wide token tables.
13. As a TokenInsights user, I want row banding to continue across empty horizontal table space, so that the table looks intentionally painted rather than clipped.
14. As a TokenInsights user, I want the active sort column marked in the header, so that I can see the current ordering without reading only the footer.
15. As a TokenInsights user, I want the active sort marker to be minimal, so that it helps orientation without adding visual noise.
16. As a TokenInsights user, I want table borders and separators to be muted, so that the data carries more visual weight than the frame.
17. As a TokenInsights user, I want the app to render correctly in light terminal themes, so that unpainted white terminal background does not break the visual design.
18. As a TokenInsights user, I want the viewer to own a cohesive dark surface, so that the whole TUI reads as one application instead of mixed terminal defaults.
19. As a TokenInsights user, I want empty table space to share the app surface, so that sparse tabs still look complete.
20. As a TokenInsights user, I want the footer panel to match the app theme, so that controls and totals feel connected to the table.
21. As a TokenInsights user, I want popups to match the app theme, so that filter, bucket, date, and sort menus do not look visually detached.
22. As a TokenInsights user, I want horizontal scrolling to keep working with styled content, so that narrow terminals remain usable.
23. As a TokenInsights user, I want ANSI-styled output to preserve alignment, so that colored cells do not break column widths.
24. As a TokenInsights user, I want loading and loaded table widths to remain stable, so that the view does not jump when data arrives.
25. As a TokenInsights user, I want visible-row scrolling to preserve column widths, so that long offscreen values do not resize the table while scrolling.
26. As a TokenInsights user, I want this polish to avoid new product concepts, so that the viewer remains focused on local token usage.
27. As a TokenInsights user, I do not want cost columns or cost sorting added, so that TokenInsights stays within its documented product boundary.
28. As a TokenInsights user, I do not want schema or token semantic changes for a visual polish pass, so that existing data remains untouched.
29. As a TokenInsights developer, I want the visual palette centralized, so that future theme tuning is localized and maintainable.
30. As a TokenInsights developer, I want table line padding to be style-aware, so that row backgrounds and header backgrounds paint the full viewport.
31. As a TokenInsights developer, I want full-screen surface painting centralized, so that future view branches do not accidentally inherit terminal defaults.
32. As a TokenInsights developer, I want renderer tests to remain behavior-oriented, so that tests protect alignment and content without snapshotting brittle ANSI output.
33. As a TokenInsights developer, I want repo-level verification to include the schema check, so that visual work does not accidentally touch schema contracts.

## Implementation Decisions

- This is a visual-only `view` TUI uplift.
- The active viewer surface remains the existing token-focused Viewer Aggregation model.
- Aggregation Tabs remain `tokens`, `models`, `providers`, `harnesses`, and `sessions`.
- Date Range Filters, Time Buckets, Dimension Filters, sorting behavior, footer totals, and query semantics do not change.
- Cost tracking remains out of scope and must not appear in columns, totals, sort labels, or docs.
- No schema changes are allowed for this work.
- No storage, normalization, SQL aggregation, metric-name, table-column, or token semantic changes are part of this work.
- Token-category color is stable across all tabs:
  - input uses green;
  - output uses a warm red/coral accent;
  - reasoning uses a violet/magenta accent;
  - cache read uses blue;
  - cache write uses amber;
  - total uses a brighter emphasized neutral or mint.
- Row identity dimensions use a cyan/teal accent.
- Secondary companion dimensions use muted text.
- Empty zero token values continue to render as empty cells rather than noisy zeros.
- Row banding is subtle and must improve horizontal scanning without overwhelming the table.
- The active sort column receives a minimal down-arrow header marker.
- Sort markers affect header width calculations and horizontal scrolling bounds.
- Borders and section frames are kept but visually dimmed.
- The app owns a cohesive dark surface that works in light terminal themes.
- The full TUI rectangle should be painted, not only the characters rendered by labels and cells.
- Table viewport lines should be padded with the correct line style before horizontal slicing so row bands and header backgrounds continue across the full viewport.
- Horizontal viewport padding must remain ANSI-aware.
- The implementation should keep styling concerns in the rendering layer and avoid leaking theme concerns into canonical query code.
- Palette values should be centralized as named constants rather than scattered color literals.
- Tests should continue to assert rendered content, layout stability, and ANSI-safe widths rather than brittle full-screen snapshots.

## Testing Decisions

- Use existing renderer tests as the primary seam for ANSI-aware clipping, padding, and table content.
- Use existing TUI model/view tests as the seam for loading/loaded height stability, column width stability, horizontal scrolling, and visible-row window stability.
- Add or update tests only where they assert externally visible behavior such as header markers, table content, alignment width, and non-jumping layout.
- Do not test private style helper implementation details directly unless behavior cannot be covered at a higher seam.
- Continue using `ansi.Strip` where tests need to assert text content without depending on exact color codes.
- Continue using ANSI-aware width checks where tests need to assert layout width.
- Run focused CLI tests during iteration.
- Run the repo-level test script before considering the visual uplift complete.
- Run the repo-level build script, which includes schema contract validation.

## Out of Scope

- Cost tracking, cost columns, cost totals, cost-per-token calculations, or cost sorting.
- Any schema change.
- Any change to canonical token usage semantics.
- Any change to raw storage or normalization.
- Any change to Viewer Aggregation query behavior.
- Any change to Date Range Filter, Time Bucket, Dimension Filter, or sort semantics.
- Any new chart, sparkline, row-details screen, or drilldown interaction.
- Auto-refresh, reload behavior, sync behavior, or polling from inside `view`.
- Theme configuration, user-selectable themes, or terminal theme detection.
- Documentation updates to `docs/design.md` or README unless a future visual decision changes the documented product contract.

## Further Notes

- This PRD follows the existing viewer contract in `docs/design.md` and ADR 0001, which states that viewer tabs are token aggregation modes.
- The comparison target from the session was Tokscale's legible, colorful, dense table treatment, not Tokscale's product concepts.
- The discovered light-terminal issue was caused by unpainted terminal whitespace and later by style padding occurring after horizontal slicing.
- The desired result is a cohesive TokenInsights visual language, not a copy of Tokscale's exact palette or cost-oriented table.
