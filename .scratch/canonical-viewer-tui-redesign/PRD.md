# Canonical Viewer TUI Redesign PRD

## Problem Statement

TokenInsights `view` currently exposes stale metric-domain tabs and grouping modes from an earlier UI model. TPS, request, tool-call, and tool-breakdown tabs render empty placeholder domains, while useful token questions are split across keyboard grouping shortcuts that are easy to confuse with filters. The result is a TUI that reads canonical data, but does not make the active sync-first product model clear.

The viewer needs to become an honest, dense terminal dashboard over countable canonical token facts. It should use tabs for Viewer Aggregations, shortcuts for Date Range Filters, Time Buckets, Dimension Filters, and sorting, and it should remove unavailable metric domains from the active surface.

## Solution

Redesign `tokeninsights-cli view` around token aggregation tabs:

1. `Tokens`: token totals over time using the active Time Bucket.
2. `Models`: token totals grouped by model.
3. `Providers`: token totals grouped by provider.
4. `Harnesses`: token totals grouped by harness.
5. `Sessions`: token totals grouped by canonical session.

The TUI remains interactive-only and read-only. It opens on the `Tokens` tab with a default Date Range Filter of `this month` and a default Time Bucket of `day`. It removes active TPS, request, tool-call, tool-breakdown, hourly, and legacy session grouping surfaces. Session viewing is retained as a first-class aggregation tab.

The visual redesign should take spacing and density inspiration from tokscale screenshots without copying its palette. Use a dark graphite terminal UI with muted dividers, cyan and seafoam accents, amber and coral numeric accents, zebra rows, and a compact top bar, main table panel, and footer. Do not add charts.

## User Stories

1. As a TokenInsights user, I want the viewer to open on token usage for this month, so that the default screen is immediately useful.
2. As a TokenInsights user, I want token usage shown by local day by default, so that I can scan recent usage over time.
3. As a TokenInsights user, I want to switch token time buckets between day, week, month, and year, so that I can change the time-series granularity without changing the selected facts.
4. As a TokenInsights user, I want week buckets to start on Monday, so that weekly rows match the existing local calendar week behavior.
5. As a TokenInsights user, I want day buckets labeled `YYYY-MM-DD`, so that daily rows are unambiguous.
6. As a TokenInsights user, I want week buckets labeled by Monday start date, so that the row identifies the covered local week.
7. As a TokenInsights user, I want month buckets labeled `YYYY-MM`, so that monthly usage is compact.
8. As a TokenInsights user, I want year buckets labeled `YYYY`, so that yearly usage is compact.
9. As a TokenInsights user, I want current partial buckets included, so that today and the current month show usage so far.
10. As a TokenInsights user, I want tabs for Tokens, Models, Providers, Harnesses, and Sessions, so that each tab clearly represents a Viewer Aggregation.
11. As a TokenInsights user, I want model rows aggregated by model only, so that the same model is not split into provider and harness combinations.
12. As a TokenInsights user, I want provider rows aggregated by provider only, so that provider totals are easy to compare.
13. As a TokenInsights user, I want harness rows aggregated by harness only, so that OpenCode, Pi, and Codex usage are easy to compare.
14. As a TokenInsights user, I want session rows aggregated by canonical session only, so that each session appears once even when it spans multiple models or providers.
15. As a TokenInsights user, I want companion columns to summarize other dimensions, so that aggregated rows still show whether multiple providers, models, or harnesses contributed.
16. As a TokenInsights user, I want companion columns to show up to two literal values and then a count, so that rows stay readable.
17. As a TokenInsights user, I want `unknown` provider and model values to display normally, so that missing source data is still visible and counted.
18. As a TokenInsights user, I want session counts in non-session tabs, so that I can see how many canonical sessions contributed to a bucket or dimension.
19. As a TokenInsights user, I want compact but collision-safe session labels, so that session rows are readable without hiding identity collisions.
20. As a TokenInsights user, I want Dimensions Filters for provider, model, and harness, so that I can narrow which canonical facts are included.
21. As a TokenInsights user, I want direct filter shortcuts for provider, model, and harness, so that I do not need a generic filter-type menu.
22. As a TokenInsights user, I want Dimension Filters to support multi-select, so that I can include more than one provider, model, or harness.
23. As a TokenInsights user, I want Dimension Filter changes to be staged until I press enter, so that I can select multiple values before the table is re-queried.
24. As a TokenInsights user, I want escape to cancel staged filter changes, so that accidental changes do not affect the view.
25. As a TokenInsights user, I want `c` in a filter popup to stage clearing that dimension, so that I can remove a filter without selecting every value.
26. As a TokenInsights user, I want filter value lists to respect other active filters and the active Date Range Filter, so that popup choices remain relevant.
27. As a TokenInsights user, I want filter value lists to ignore their own dimension's active filter, so that I can change or expand the current selection.
28. As a TokenInsights user, I want date ranges available through a popup, so that I can change the included calendar scope inside the TUI.
29. As a TokenInsights user, I want date range presets for today, yesterday, this week, this month, this year, and all time, so that common calendar scopes are fast.
30. As a TokenInsights user, I want startup flags for all date range presets, so that I can open the TUI directly on the desired scope.
31. As a TokenInsights user, I want custom startup day bounds to still work, so that precise ranges remain available.
32. As a TokenInsights user, I want custom day bounds to override preset date ranges, so that the effective Date Range Filter is not a hidden intersection.
33. As a TokenInsights user, I want a startup Time Bucket flag, so that I can open the Tokens tab at a non-default bucket.
34. As a TokenInsights user, I want legacy grouping flags removed, so that grouping is not confused with tabs, filters, or Time Buckets.
35. As a TokenInsights user, I want the TUI to always open on the Tokens tab, so that the default entry point is predictable.
36. As a TokenInsights user, I want tab and shift-tab to move between aggregation tabs, so that keyboard navigation is simple.
37. As a TokenInsights user, I want number keys 1 through 5 to jump to aggregation tabs, so that direct tab navigation is fast.
38. As a TokenInsights user, I want sorting available through a popup, so that I can choose date, token, component, or name ordering without memorizing many keys.
39. As a TokenInsights user, I want Tokens to sort newest bucket first by default, so that recent usage is first.
40. As a TokenInsights user, I want Models, Providers, and Harnesses to sort by highest total tokens by default, so that the largest contributors are first.
41. As a TokenInsights user, I want Sessions to sort by newest session activity by default, so that recent sessions are first.
42. As a TokenInsights user, I want `name` sorting to sort by the tab's primary dimension, so that alphabetical scans are possible.
43. As a TokenInsights user, I want token columns in semantic order, so that input, output, reasoning, cache read, cache write, and total remain familiar.
44. As a TokenInsights user, I want horizontal scrolling to keep working, so that wide tables remain usable in narrower terminal panes.
45. As a TokenInsights user, I want arrow keys and Vim-style keys for scrolling, so that terminal navigation feels natural.
46. As a TokenInsights user, I want home and end to jump horizontally, so that wide tables are manageable.
47. As a TokenInsights user, I want no row details screen in this pass, so that the redesign remains focused on the main dashboard.
48. As a TokenInsights user, I want no selected-row highlight when rows have no action, so that the UI does not imply unavailable interactions.
49. As a TokenInsights user, I want a concise empty state when there are no rows, so that empty canonical data or restrictive filters are understandable.
50. As a TokenInsights user, I want the footer to show selected-scope totals and row counts, so that I can read the current aggregate without mental addition.
51. As a TokenInsights user, I want the footer to show global last completed sync time, so that I know how fresh the local database is.
52. As a TokenInsights user, I want no reload key and no auto-refresh, so that the read-only viewer does not imply it can sync or poll for changes.
53. As a TokenInsights user, I want no cost columns, totals, sorting, or labels, so that TokenInsights stays focused on local token usage.
54. As a TokenInsights user, I want unavailable TPS, request, and tool domains hidden from the active tab bar, so that the UI does not show empty placeholder product surfaces.
55. As a TokenInsights developer, I want Viewer Aggregation queries to replace legacy group-by and empty metric-domain queries, so that the query layer matches the product language.
56. As a TokenInsights developer, I want the viewer to avoid schema changes unless absolutely necessary, so that this remains a TUI and query redesign.
57. As a TokenInsights developer, I want tests at query, model, rendering, and command-flag seams, so that the behavior is stable without testing private implementation details.
58. As a TokenInsights developer, I want docs updated with the new viewer model, so that future agents do not reintroduce the removed placeholder tabs or hourly grouping.

## Implementation Decisions

- The active viewer surface uses Aggregation Tabs, not metric-domain tabs.
- The active Aggregation Tabs are `Tokens`, `Models`, `Providers`, `Harnesses`, and `Sessions`.
- TPS, request, and tool domains remain future-compatible data domains, but are not active empty tabs.
- Cost tracking is not part of TokenInsights and must not appear in the viewer, docs, tests, or sort labels for this redesign.
- The TUI opens on `Tokens`.
- The default Date Range Filter is `this month`.
- Supported Date Range Filter presets are `today`, `yesterday`, `this week`, `this month`, `this year`, and `all time`.
- Startup date range flags should include all presets: `--today`, `--yesterday`, `--week`, `--month`, `--year`, and `--all-time`.
- Existing custom day-bound flags remain startup-only and render as a custom Date Range Filter.
- Custom day bounds override preset period filtering for the effective query.
- The `Tokens` tab uses a Time Bucket.
- Supported Time Buckets are `day`, `week`, `month`, and `year`.
- The default Time Bucket is `day`.
- A startup `--bucket=day|week|month|year` flag sets the initial Time Bucket.
- The legacy `--group-by` flag is removed from `view`.
- No startup `--tab` flag is added.
- Provider, model, harness, and session startup filter flags remain for compatibility and testing, but the primary TUI workflow uses direct filter popups.
- Interactive Dimension Filters exist for provider, model, and harness only.
- No interactive session filter is included in this pass.
- Filter shortcuts are `p` for provider, `m` for model, and `h` for harness.
- Date range shortcut is `d`.
- Time Bucket shortcut is `g`.
- Sort shortcut is `s`.
- Tab navigation uses tab, shift-tab, and number keys 1 through 5.
- Vertical scrolling uses arrow keys and `j`/`k`.
- Horizontal scrolling uses left/right arrow keys; home and end jump horizontally. `h` is reserved for the harness Dimension Filter shortcut.
- There is no reload key and no auto-refresh.
- Filter popups are multi-select. `space` toggles a value, `c` stages clearing the dimension, `enter` applies, and `esc` cancels.
- Single-choice date, bucket, and sort popups apply when the user selects with enter or space.
- `Tokens` columns are `bucket`, `sessions`, `input`, `output`, `reasoning`, `cache read`, `cache write`, and `total`.
- `Models` columns are `model`, `provider`, `harnesses`, `sessions`, and token columns.
- `Providers` columns are `provider`, `models`, `harnesses`, `sessions`, and token columns.
- `Harnesses` columns are `harness`, `providers`, `models`, `sessions`, and token columns.
- `Sessions` columns are `latest`, `session`, `harness`, `providers`, `models`, and token columns.
- Token columns remain in input-first semantic order with total last.
- Companion summary columns show one value literally, two values literally, and three or more values as a count.
- `unknown` is displayed and counted as a normal provider or model value.
- Session labels are compact but collision-safe among visible rows.
- The footer shows selected-scope total tokens, tab-specific row count, active Date Range Filter, active Time Bucket on Tokens, active Dimension Filters, and global last completed sync time.
- Global last sync is `MAX(completed_at_ms)` for completed ingest runs and is independent of active filters.
- Empty states distinguish no canonical token facts from filters/date ranges that match no rows.
- The TUI should use Bubble Tea, Lip Gloss, and Bubbles where useful.
- Add Bubbles for viewport and key binding support, but do not force `bubbles/table` if custom table rendering better fits the dense layout.
- Replace nested bordered table rendering with a custom dense table renderer.
- Keep horizontal scrolling as fallback for narrow terminals.
- Use a dark graphite visual direction with cyan/seafoam accents, muted borders, amber/coral numeric accents, and subtle zebra rows.
- Do not add a chart.
- Do not add row details or enter behavior in this pass.
- Prefer no schema changes. Revisit only if implementation proves impossible without a schema change, and then follow the explicit schema approval rule.
- Replace the legacy viewer query API with dedicated Viewer Aggregation query functions for token buckets, models, providers, harnesses, sessions, filter values, and last completed sync.
- Documentation must align the PRD, architecture design document, CLI README, root README, and ADR.

## Testing Decisions

- Tests should assert external viewer behavior at the highest practical seam.
- Command flag tests should cover the new default month range, `--yesterday`, `--year`, `--bucket`, removed `--group-by`, custom day-bound override behavior, and retained startup filters.
- Query tests should cover each Viewer Aggregation using countable canonical token facts only.
- Query tests should cover session counts, companion dimension summaries, `unknown` values, filters, custom date ranges, Time Buckets, Monday-start weeks, and global last completed sync.
- TUI model tests should cover tab cycling, number-key tab jumps, date popup selection, bucket popup selection, sort popup selection, direct dimension filter popups, staged filter application, staged clear, escape cancellation, and absence of reload behavior.
- Rendering tests should cover tab labels, footer totals, last sync labels, empty states, color/style-safe table output, horizontal clipping, compact summary rendering, and collision-safe session labels.
- Documentation tests are not required, but docs should be manually reviewed for removed cost, TPS placeholder, request/tool placeholder, hourly grouping, and refresh/reload references.
- Existing CLI table, render, DB aggregate, and flag tests provide prior art and should be updated rather than bypassed.
- Full verification for implementation should include focused Go tests during iteration and the repo-level test/build commands before completion.

## Out of Scope

- Cost tracking of any kind.
- Charts or sparklines.
- Row details screens or modal drilldowns.
- Interactive session search or session filtering.
- Auto-refresh, polling, reload, or sync from inside `view`.
- Non-interactive summaries or export commands.
- Schema changes unless implementation proves they are absolutely necessary and explicit approval is obtained.
- TPS, request, tool-call, or tool-breakdown active viewer tabs.
- Realtime or checkpoint plugin behavior.
- Cloud sync or vendor dashboard integration.
- Prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, or full source paths.

## Further Notes

- This PRD follows [ADR 0001](../../docs/adr/0001-viewer-tabs-are-token-aggregation-modes.md).
- `docs/design.md` is the architecture reference and should be updated to describe the new active Viewer surface.
- `.scratch/sync-first-canonical-redesign-followups/issues/03-keep-non-token-tabs-empty.md` is superseded by ADR 0001 for the active viewer surface.
- The implementation should remain minimally scoped to viewer query, TUI behavior, rendering, docs, and tests.
