# Canonical Viewer TUI Redesign Issue Breakdown

This document breaks the canonical viewer TUI redesign PRD into independently grabbable implementation slices. The slices are ordered by dependency and are intended to be executed from the local markdown issue tracker.

Parent PRD: [`PRD.md`](../PRD.md)

Triage label: `ready-for-agent`

Last audited: 2026-06-13.

## Implementation Status Summary

| Issue | Type | Status | Blocked by | Notes |
|-------|------|--------|------------|-------|
| 1. Tokens Tab Uses Date Range Filters And Time Buckets | AFK | Ready | None | Establishes the new active viewer foundation. |
| 2. Models, Providers, And Harnesses Are Aggregation Tabs | AFK | Ready | Issue 1 | Adds dimension-comparison tabs over the same token facts. |
| 3. Sessions Is A First-Class Aggregation Tab | AFK | Ready | Issue 1 | Adds session-level viewing without bringing back legacy grouping. |
| 4. Direct Popups Drive Date, Bucket, Sort, And Dimension Filters | AFK | Ready | Issues 1, 2, and 3 | Adds the new keyboard interaction model. |
| 5. Dense Graphite Renderer Replaces Nested Table Styling | AFK | Ready | Issues 1, 2, and 3 | Redesigns layout, footer, empty states, and scrolling. |
| 6. Documentation And Legacy Surface Cleanup | AFK | Ready | Issues 1 through 5 | Aligns docs and removes stale viewer references. |

## 1. Tokens Tab Uses Date Range Filters And Time Buckets

Type: AFK

Blocked by: None - can start immediately

User stories covered: 1-9, 28-35, 43-44, 50-54, 55-58

### What to build

Replace the legacy token grouping model with the new `Tokens` Aggregation Tab. The viewer should open on token usage for `this month`, grouped into local day buckets by default, with support for day, week, month, and year Time Buckets. Date range startup flags should include every preset, custom day bounds should override preset periods, and the legacy grouping flag should be removed.

This slice should make the default viewer surface honest even before the other new tabs exist: no active TPS, request, tool-call, tool-breakdown, hourly, or legacy session grouping tabs should remain reachable.

### Acceptance criteria

- [ ] `view` opens on the `Tokens` tab by default.
- [ ] The default Date Range Filter is `this month`.
- [ ] The default Time Bucket is `day`.
- [ ] `Tokens` rows aggregate countable canonical token facts by local day, week, month, or year.
- [ ] Week buckets start on Monday and are labeled by their Monday start date.
- [ ] Day, week, month, and year labels follow the PRD formats.
- [ ] Current partial buckets are included.
- [ ] Startup date flags include `--today`, `--yesterday`, `--week`, `--month`, `--year`, and `--all-time`.
- [ ] `--bucket=day|week|month|year` sets the initial Time Bucket.
- [ ] `--group-by` is no longer accepted for `view`.
- [ ] Custom day-bound flags still work and override preset period filtering in the effective query.
- [ ] Token columns remain in input-first semantic order with total last.
- [ ] `Tokens` includes a distinct canonical session count column.
- [ ] Non-token metric-domain tabs are removed from active tab navigation.
- [ ] Query, flag, TUI model, and rendering tests cover the new Tokens behavior.
- [ ] No schema changes are made.

## 2. Models, Providers, And Harnesses Are Aggregation Tabs

Type: AFK

Blocked by:

- Issue 1

User stories covered: 10-18, 20-27, 36-46, 50, 53, 55-58

### What to build

Add `Models`, `Providers`, and `Harnesses` as Aggregation Tabs over the active Date Range Filter and Dimension Filters. Each tab should aggregate by its primary dimension only, not by all provider/model/harness combinations. Companion dimension columns should summarize the other dimensions without exploding rows.

Startup provider, model, harness, and session filters should continue to work for compatibility, but the active UI should frame provider, model, and harness filtering as Dimension Filters rather than tab grouping.

### Acceptance criteria

- [ ] `Models` groups rows by model only.
- [ ] `Providers` groups rows by provider only.
- [ ] `Harnesses` groups rows by harness only.
- [ ] Each dimension tab uses countable canonical token facts only.
- [ ] Each dimension tab respects startup provider, model, harness, session, date range, and custom day-bound filters.
- [ ] Companion columns summarize other dimensions using one literal value, two literal values, or a count for three or more values.
- [ ] `unknown` provider and model values display and count normally.
- [ ] Each non-session tab includes a distinct canonical session count.
- [ ] Default sort is highest total tokens first for Models, Providers, and Harnesses.
- [ ] `name` sorting sorts by the tab's primary dimension.
- [ ] Tab and shift-tab navigation includes Tokens, Models, Providers, and Harnesses in order.
- [ ] Number keys jump directly to the available aggregation tabs.
- [ ] Query, model, and rendering tests cover aggregation without splitting by companion dimensions.
- [ ] No cost columns, labels, or sorting are introduced.
- [ ] No schema changes are made.

## 3. Sessions Is A First-Class Aggregation Tab

Type: AFK

Blocked by:

- Issue 1

User stories covered: 10, 14, 19, 35-46, 50, 53, 55-58

### What to build

Add `Sessions` as a first-class Aggregation Tab. Session rows should aggregate by canonical session, respect applicable filters, and show latest activity plus companion provider/model summaries. This replaces the old session grouping shortcut with a clearer tab-based session view.

### Acceptance criteria

- [ ] `Sessions` appears as an aggregation tab after Harnesses.
- [ ] `Sessions` groups rows by canonical session only.
- [ ] A session with multiple providers or models remains one row.
- [ ] Session rows include latest activity, compact session label, harness, provider summary, model summary, and token columns.
- [ ] Session labels are compact and collision-safe among visible rows.
- [ ] `Sessions` respects startup provider, model, harness, session, date range, and custom day-bound filters.
- [ ] Default sort is newest session activity first.
- [ ] `name` sorting sorts by session identity.
- [ ] There is no interactive session filter in this slice.
- [ ] There is no row details or enter behavior.
- [ ] Query, model, and rendering tests cover multi-model or multi-provider sessions.
- [ ] No schema changes are made.

## 4. Direct Popups Drive Date, Bucket, Sort, And Dimension Filters

Type: AFK

Blocked by:

- Issue 1
- Issue 2
- Issue 3

User stories covered: 20-29, 36-42, 45-46, 52, 57

### What to build

Replace the legacy generic grouping/filter popup flow with direct popups for Date Range Filters, Time Buckets, sorting, and Dimension Filters. The interaction model should make tabs, filters, buckets, and sort choices visually and behaviorally distinct.

### Acceptance criteria

- [ ] `d` opens a Date Range Filter popup with today, yesterday, this week, this month, this year, and all time.
- [ ] `g` opens a Time Bucket popup for day, week, month, and year.
- [ ] `s` opens a sort popup with date, tokens, input, output, cache read, and name where applicable.
- [ ] `p` opens provider filter values directly.
- [ ] `m` opens model filter values directly.
- [ ] `h` opens harness filter values directly.
- [ ] Provider, model, and harness filter popups are multi-select lists.
- [ ] Filter value toggles are staged until enter is pressed.
- [ ] `c` stages clearing the active filter dimension and requires enter to apply.
- [ ] `esc` cancels staged filter changes.
- [ ] Filter value lists respect the active date range and other Dimension Filters while ignoring their own current dimension filter.
- [ ] Date range, bucket, and sort popups apply on enter or space selection.
- [ ] There is no generic `f` filter-type menu.
- [ ] There is no reload shortcut and no auto-refresh behavior.
- [ ] Model tests cover key handling and popup state transitions.
- [ ] No schema changes are made.

## 5. Dense Graphite Renderer Replaces Nested Table Styling

Type: AFK

Blocked by:

- Issue 1
- Issue 2
- Issue 3

User stories covered: 43-54, 57-58

### What to build

Redesign the TUI rendering into a dense terminal dashboard with a top bar, main table panel, and footer. Replace nested table borders with a custom table renderer, use Bubbles where it helps, keep horizontal scrolling as a fallback, and show selected-scope totals plus global last sync time.

### Acceptance criteria

- [ ] The top-left product label is `TokenInsights`.
- [ ] The top bar shows aggregation tabs and active context without nested table borders.
- [ ] The main panel uses dense table rows, subtle dividers, and zebra row styling.
- [ ] The footer shows selected-scope total tokens and tab-specific row count.
- [ ] The footer shows global last completed sync time from completed ingest runs, or `never`.
- [ ] The footer shows active date range, active Time Bucket where relevant, active Dimension Filters, and compact key hints.
- [ ] Empty canonical data and filter/date-range misses render concise empty states inside the main panel.
- [ ] There is no selected row highlight unless an actual row action is introduced, which is out of scope for this PRD.
- [ ] Horizontal scrolling works for wide tables.
- [ ] Vertical scrolling works with arrow keys and `j`/`k`.
- [ ] Horizontal scrolling works with left/right arrow keys; `h` is reserved for the harness Dimension Filter shortcut.
- [ ] Home and end jump horizontally.
- [ ] The visual palette follows the dark graphite, cyan/seafoam, amber/coral direction without copying tokscale colors.
- [ ] Bubbles is added and used where useful for viewport or key binding support.
- [ ] `bubbles/table` is not required if custom rendering better fits the layout.
- [ ] Rendering tests cover footer totals, last sync, empty states, horizontal clipping, summary values, and session label collision behavior.
- [ ] No chart is added.
- [ ] No cost display is added.
- [ ] No schema changes are made.

## 6. Documentation And Legacy Surface Cleanup

Type: AFK

Blocked by:

- Issue 1
- Issue 2
- Issue 3
- Issue 4
- Issue 5

User stories covered: 53-58

### What to build

Align project documentation with the redesigned active viewer surface. The sync-first architecture remains intact, but docs should stop describing placeholder metric tabs, hourly grouping, cost sorting, refresh behavior, or the old grouping model as active UI behavior.

### Acceptance criteria

- [ ] The architecture design document describes Aggregation Tabs, Date Range Filters, Time Buckets, Dimension Filters, global last sync, and hidden future metric domains.
- [ ] The root README and CLI README show current `view` examples and flags.
- [ ] Documentation no longer presents TPS, request, tool-call, or tool-breakdown tabs as active viewer tabs.
- [ ] Documentation no longer presents hourly grouping or `--group-by` as active viewer behavior.
- [ ] Documentation does not include cost examples, cost sort hints, cost totals, or cost columns.
- [ ] Documentation does not advertise reload or auto-refresh.
- [ ] ADR 0001 remains the decision record for the tab model.
- [ ] The superseded sparse non-token tab follow-up remains marked as superseded.
- [ ] Verification notes point implementers to focused Go tests and repo-level test/build commands.
- [ ] No schema changes are made.
