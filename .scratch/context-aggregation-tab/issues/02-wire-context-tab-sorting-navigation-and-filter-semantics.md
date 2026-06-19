# Wire Context Tab Sorting, Navigation, And Filter Semantics

Status: ready-for-agent
Type: AFK

## Parent

[Context Aggregation Tab PRD](../PRD.md)

## What to build

Complete the interactive behavior for the `context` Aggregation Tab so it behaves like a first-class viewer tab. Users should be able to navigate to it through tab cycling and number-key shortcuts, sort it by all approved context fields, and rely on existing Date Range Filter and Dimension Filter semantics.

This slice should preserve the existing viewer interaction model while extending it to include `context`.

## Acceptance criteria

- [ ] Tab and shift-tab navigation include the `context` Aggregation Tab.
- [ ] Number-key tab navigation includes the `context` Aggregation Tab in a predictable order after the existing tabs.
- [ ] The sort popup exposes context sort options for `avg ctx`, `median ctx`, `max ctx`, `sessions`, `harness`, `provider`, and `model`.
- [ ] Context defaults to `avg ctx` descending when no explicit sort is selected.
- [ ] Sorting by `median ctx` ranks rows by median Session Peak Context Load.
- [ ] Sorting by `max ctx` ranks rows by maximum Session Peak Context Load.
- [ ] Sorting by `sessions` ranks rows by contributing session count.
- [ ] Sorting by `harness`, `provider`, or `model` sorts alphabetically by that row identity field.
- [ ] Provider, model, and harness filter popups continue to work when the active tab is `context`.
- [ ] Date range popup changes reload context rows using the selected fact scope.
- [ ] Empty state behavior on `context` matches the rest of the viewer.
- [ ] Horizontal scrolling and viewport-aware rendering continue to work for context rows.
- [ ] Focused model tests cover tab navigation, number-key navigation, sorting, filter reload behavior, and empty state behavior.
- [ ] No schema changes are made.

## Blocked by

- [01 - Add Context Aggregation Tab With Session Peak Context Load Stats](01-add-context-aggregation-tab-with-session-peak-context-load-stats.md)

