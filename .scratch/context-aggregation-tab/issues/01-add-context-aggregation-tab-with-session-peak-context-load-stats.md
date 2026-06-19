# Add Context Aggregation Tab With Session Peak Context Load Stats

Status: ready-for-agent
Type: AFK

## Parent

[Context Aggregation Tab PRD](../PRD.md)

## What to build

Add a `context` Aggregation Tab to `tokeninsights view` that groups countable canonical token facts by harness, provider, and model over the active Date Range Filter and Dimension Filters.

For each harness/provider/model row, compute one Session Peak Context Load per distinct canonical session in that row, then show `sessions`, `avg ctx`, `median ctx`, and `max ctx`. Session Peak Context Load is input tokens plus cache read tokens plus cache write tokens, excluding output and reasoning tokens. A session may contribute separately to multiple rows when it has in-range facts for multiple harness/provider/model combinations.

## Acceptance criteria

- [ ] The viewer includes a `context` Aggregation Tab.
- [ ] Context rows are grouped by exact harness, provider, and model combination.
- [ ] Context aggregation uses only countable canonical token rows.
- [ ] Active Date Range Filter is applied before per-session peak calculation.
- [ ] Active provider, model, and harness Dimension Filters apply to context rows.
- [ ] Each distinct canonical session contributes at most one peak value to each harness/provider/model row.
- [ ] A session can contribute to multiple rows when it uses multiple harness/provider/model combinations.
- [ ] Session Peak Context Load is computed as input plus cache read plus cache write tokens.
- [ ] Output and reasoning tokens do not affect Session Peak Context Load.
- [ ] Context columns are `harness`, `provider`, `model`, `sessions`, `avg ctx`, `median ctx`, and `max ctx`.
- [ ] `avg ctx` is the arithmetic mean of per-session peak values.
- [ ] `median ctx` is the median of per-session peak values.
- [ ] Even-count medians average the two middle peak values and render as integer token counts.
- [ ] `max ctx` is the highest per-session peak value in the row.
- [ ] Canonical `unknown` provider and model values are included as normal row values.
- [ ] Claude Code inferred provider rows continue to display `maybe-anthropic`.
- [ ] The default context sort is `avg ctx` descending.
- [ ] Focused query, row-loading, and rendering tests cover the externally visible behavior.
- [ ] No schema changes are made.

## Blocked by

None - can start immediately

