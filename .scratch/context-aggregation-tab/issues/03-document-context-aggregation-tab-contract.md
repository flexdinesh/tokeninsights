# Document Context Aggregation Tab Contract

Status: ready-for-agent
Type: AFK

## Parent

[Context Aggregation Tab PRD](../PRD.md)

## What to build

Document the `context` Aggregation Tab in the user-facing README and architecture design document. The docs should explain that the tab groups by harness, provider, and model; computes Session Peak Context Load per session; summarizes per-session peaks with `sessions`, `avg ctx`, `median ctx`, and `max ctx`; respects existing filters; and requires no schema change.

## Acceptance criteria

- [ ] README mentions the `context` Aggregation Tab and its purpose.
- [ ] README explains `avg ctx`, `median ctx`, and `max ctx` at user-facing depth.
- [ ] The architecture design document lists `context` as an active viewer Aggregation Tab.
- [ ] The architecture design document defines the grouping and statistics for context rows.
- [ ] The architecture design document states that Date Range Filters and Dimension Filters apply before context aggregation.
- [ ] The architecture design document states that context uses only countable canonical token rows.
- [ ] The architecture design document states that Session Peak Context Load is input plus cache read plus cache write tokens, excluding output and reasoning.
- [ ] The architecture design document states that no schema change is required for the context tab.
- [ ] Documentation avoids introducing cost tracking, realtime behavior, context window percentages, or new metric domains.
- [ ] Documentation remains consistent with the TokenInsights glossary term `Session Peak Context Load`.

## Blocked by

- [01 - Add Context Aggregation Tab With Session Peak Context Load Stats](01-add-context-aggregation-tab-with-session-peak-context-load-stats.md)
- [02 - Wire Context Tab Sorting, Navigation, And Filter Semantics](02-wire-context-tab-sorting-navigation-and-filter-semantics.md)

