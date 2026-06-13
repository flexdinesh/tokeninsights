# TokenInsights

TokenInsights tracks local token usage across coding harnesses through durable local data that can be synced, normalized, and viewed over time.

## Language

**TokenInsights CLI**:
The user-facing command-line product for TokenInsights, invoked as `tokeninsights`.
_Avoid_: tokeninsights-cli as the public command name

**Durable Source**:
A local harness-owned file, database, or directory that persists usage-relevant session data after a harness run completes and can be batch-synced without realtime hooks.
_Avoid_: Harness source, local source, session file, harness local DB

**Date Range Filter**:
A viewer constraint that chooses which canonical facts are included based on local calendar time; supported presets are today, yesterday, this week, this month, this year, and all time.
_Avoid_: Time grouping, bucket

**Dimension Filter**:
A viewer constraint that chooses which canonical facts are included based on provider, model, or harness values.
_Avoid_: Aggregation tab, grouping

**Time Bucket**:
A viewer grouping interval that rolls included token facts into local calendar rows such as day, week, month, or year.
_Avoid_: Date range, period filter

**Aggregation Tab**:
A viewer mode that chooses the primary dimension used to summarize included token facts, such as tokens over time, model, provider, harness, or session.
_Avoid_: Metric tab, domain tab, filter

**Viewer Aggregation**:
A read-only summary of countable canonical token facts for one aggregation tab and the active date range and dimension filters.
_Avoid_: Pre-aggregated rollup, metric domain
