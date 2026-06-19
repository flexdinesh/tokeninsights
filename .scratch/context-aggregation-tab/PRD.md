# Context Aggregation Tab PRD

Status: ready-for-agent

## Problem Statement

TokenInsights users can see total token usage by time, model, provider, harness, and session, and the sessions Aggregation Tab exposes a per-session `ctx used` value. However, users cannot compare Session Peak Context Load across harness, provider, and model combinations. This makes it hard to understand which local coding setup tends to consume the most prompt-side context across sessions.

The current viewer can answer "how many tokens did this model use?" but not "which harness/provider/model combinations are filling the context most often across sessions?" Users need a focused viewer aggregation that summarizes context pressure without introducing schema changes, cost tracking, or non-token metric domains.

## Solution

Add a new `context` Aggregation Tab to `tokeninsights view`.

The tab groups countable canonical token facts by `harness + provider + model` over the active Date Range Filter and active Dimension Filters. Within each row, TokenInsights first computes the Session Peak Context Load for each distinct canonical session in that row, using only facts inside the active Date Range Filter. It then summarizes those per-session peaks with session count, average context, median context, and maximum context.

The tab should use these columns in order: `harness`, `provider`, `model`, `sessions`, `avg ctx`, `median ctx`, and `max ctx`.

The default sort for `context` is `avg ctx` descending. Sort options are `avg ctx`, `median ctx`, `max ctx`, `sessions`, `harness`, `provider`, and `model`.

No schema change is required. The feature is a read-only Viewer Aggregation over existing canonical token usage facts.

## User Stories

1. As a TokenInsights user, I want a `context` Aggregation Tab, so that I can compare prompt-side context usage across local coding setups.
2. As a TokenInsights user, I want context rows grouped by harness, provider, and model, so that I can see each concrete usage combination separately.
3. As a TokenInsights user, I want each context row to include the harness, so that I can distinguish Codex, OpenCode, Pi, and Claude Code usage.
4. As a TokenInsights user, I want each context row to include the provider, so that I can compare provider-specific context behavior.
5. As a TokenInsights user, I want each context row to include the model, so that I can compare model-specific context behavior.
6. As a TokenInsights user, I want rows to include `unknown` providers and models, so that missing source metadata does not hide real usage.
7. As a TokenInsights user, I want Claude Code inferred provider rows to display `maybe-anthropic`, so that inferred provider attribution remains visible and honest.
8. As a TokenInsights user, I want context statistics to be based on Session Peak Context Load, so that the tab describes peak prompt-side load rather than total token volume.
9. As a TokenInsights user, I want Session Peak Context Load to count input tokens, cache read tokens, and cache write tokens, so that cached prompt-side context is included.
10. As a TokenInsights user, I want Session Peak Context Load to exclude output and reasoning tokens, so that the value represents prompt-side context pressure.
11. As a TokenInsights user, I want each session to contribute one peak value per harness/provider/model row, so that repeated facts within a session do not overrepresent that session.
12. As a TokenInsights user, I want a session that uses multiple models to contribute separately to each matching row, so that each combination reflects its own observed context pressure.
13. As a TokenInsights user, I want the active Date Range Filter to apply before context aggregation, so that the tab respects the same selected fact scope as other viewer tabs.
14. As a TokenInsights user, I want a cross-boundary session to use only in-range facts for its row peak, so that a prior period does not leak into the selected date range.
15. As a TokenInsights user, I want active provider, model, and harness Dimension Filters to affect the context tab, so that I can narrow context analysis to the same selected scope as other tabs.
16. As a TokenInsights user, I want the context tab to include only countable canonical token rows, so that fallback-like scopes do not inflate context statistics.
17. As a TokenInsights user, I want a `sessions` column, so that I can judge whether a row's statistics are based on many sessions or only one.
18. As a TokenInsights user, I want an `avg ctx` column, so that I can compare typical Session Peak Context Load across combinations.
19. As a TokenInsights user, I want a `median ctx` column, so that outlier-heavy rows are easier to interpret.
20. As a TokenInsights user, I want a `max ctx` column, so that I can spot the largest observed Session Peak Context Load for a combination.
21. As a TokenInsights user, I want even-count medians to use the average of the two middle session peaks, so that median behavior is mathematically clear.
22. As a TokenInsights user, I want median values rendered as integer token counts, so that context columns match the rest of the TUI's token display.
23. As a TokenInsights user, I want the default context sort to be `avg ctx` descending, so that high average context pressure appears first.
24. As a TokenInsights user, I want to sort by `median ctx`, so that I can rank combinations by robust central tendency.
25. As a TokenInsights user, I want to sort by `max ctx`, so that I can find extreme context usage.
26. As a TokenInsights user, I want to sort by `sessions`, so that I can find combinations with broadest observed usage.
27. As a TokenInsights user, I want to sort by harness, provider, or model, so that I can scan rows alphabetically by dimension.
28. As a TokenInsights user, I want the `context` tab to appear alongside the existing Aggregation Tabs, so that tab navigation remains the way I discover viewer modes.
29. As a TokenInsights user, I want number-key tab navigation to include the new context tab, so that direct tab access remains predictable.
30. As a TokenInsights user, I want horizontal scrolling and viewport-aware rendering to work for the context tab, so that the row identity columns remain usable in narrow terminals.
31. As a TokenInsights user, I want the context tab empty state to match existing viewer behavior, so that empty canonical data and restrictive filters are understandable.
32. As a TokenInsights user, I want README documentation to mention the context tab, so that I can discover what `avg ctx`, `median ctx`, and `max ctx` mean.
33. As a TokenInsights maintainer, I want `docs/design.md` to define the new Aggregation Tab, so that the architecture contract stays aligned with the viewer.
34. As a TokenInsights maintainer, I want no schema changes for this feature, so that the feature remains a read-only aggregation over canonical facts.
35. As a TokenInsights maintainer, I want query tests for context aggregation behavior, so that the statistics remain stable as the viewer evolves.
36. As a TokenInsights maintainer, I want model and rendering tests for the new tab, so that sorting, navigation, and columns are protected at visible seams.
37. As a TokenInsights maintainer, I want docs and tests to move with the CLI change, so that future agents do not drift from the product contract.

## Implementation Decisions

- The new viewer mode is an Aggregation Tab named `context`.
- The tab is part of the active token viewer surface, not a new metric domain.
- Rows are grouped by the exact combination of harness, provider, and model.
- The active Date Range Filter is applied to canonical facts before computing per-session peaks.
- Active provider, model, and harness Dimension Filters apply to the tab.
- The query uses only countable canonical token usage rows.
- Each distinct canonical session contributes at most one peak value to a given harness/provider/model row.
- A session may contribute to multiple rows when it has in-range facts for multiple harness/provider/model combinations.
- Session Peak Context Load is computed as input tokens plus cache read tokens plus cache write tokens.
- Output tokens and reasoning tokens are excluded from Session Peak Context Load.
- Row statistics are computed across the per-session peak values for that row.
- `avg ctx` is the arithmetic mean of per-session peak values.
- `median ctx` is the median of per-session peak values.
- For an even number of sessions, `median ctx` is the average of the two middle peak values, rendered as an integer token count.
- `max ctx` is the largest per-session peak value in the row.
- The tab columns are `harness`, `provider`, `model`, `sessions`, `avg ctx`, `median ctx`, and `max ctx`.
- The default sort is `avg ctx` descending.
- Sort options are `avg ctx`, `median ctx`, `max ctx`, `sessions`, `harness`, `provider`, and `model`.
- Canonical `unknown` provider and model values are included as normal row values.
- Claude Code artifact-derived inferred provider rows continue to display `maybe-anthropic`.
- No average and mean duplicate columns are added. `avg ctx` is the arithmetic mean.
- No Time Bucket column is added to the context tab in this pass. Date range acts as the temporal scope.
- No schema change is required or approved for this feature.
- No ADR is needed because this is a reversible viewer aggregation over existing canonical facts and is covered by the design document.
- The implementation should update the viewer aggregation query layer, CLI table model, sort selection, renderer, README, and design document together.

## Testing Decisions

- Tests should assert externally visible behavior at the highest practical seam rather than private implementation details.
- Query-level tests should verify that context aggregation computes per-session peaks before row-level average, median, and maximum statistics.
- Query-level tests should cover multiple sessions within one harness/provider/model row.
- Query-level tests should cover one session contributing to multiple harness/provider/model rows.
- Query-level tests should cover multiple facts within one session and use only the peak value for that session and row.
- Query-level tests should cover Date Range Filter behavior, including a session with facts both inside and outside the selected range.
- Query-level tests should cover provider, model, and harness Dimension Filters on the context tab.
- Query-level tests should cover `unknown` and `maybe-anthropic` row values.
- Query-level tests should cover countable-only behavior and ensure non-countable facts do not affect context statistics.
- Query-level tests should cover even-count median behavior and integer rendering expectations.
- CLI row-loading tests should verify that the context tab loads rows with the requested columns and default sort semantics.
- CLI model tests should verify tab cycling and number-key navigation after adding the context tab.
- Sort tests should verify all context sort modes and the default `avg ctx` ordering.
- Rendering tests should verify context column labels, numeric alignment, empty state behavior, and horizontal scrolling compatibility.
- Documentation should be manually reviewed to ensure README and design language match the implemented semantics.
- Existing viewer aggregation, table model, and renderer tests are the prior art and should be extended rather than bypassed.

## Out of Scope

- Schema changes.
- New raw or canonical tables.
- Realtime context tracking.
- Context window size percentages.
- Provider-specific maximum context window metadata.
- Cost tracking.
- A time-bucketed context trend tab.
- Charts or sparklines.
- Row details or drilldown screens.
- Non-interactive export or summary commands.
- Changes to sync, normalize, raw storage, or canonical token semantics beyond the read-only aggregation needed by this tab.
- Prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, or full source paths.

## Further Notes

- This PRD follows the existing decision that viewer tabs are token Aggregation Tabs.
- `Session Peak Context Load` is captured in the TokenInsights glossary.
- The implementation must keep CLI, docs, and tests aligned because this changes viewer aggregation behavior and user-facing columns.
