# TokenInsights

TokenInsights tracks local token usage across coding harnesses through durable local data that can be synced, normalized, and viewed over time.

## Language

**TokenInsights CLI**:
The user-facing command-line product for TokenInsights, invoked as `tokeninsights`.
_Avoid_: tokeninsights-cli as the public command name

**Durable Source**:
A local harness-owned file, database, or directory that persists usage-relevant session data after a harness run completes and can be batch-synced without realtime hooks or separate authenticated API export.
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

**Session Peak Context Load**:
The largest prompt-side context load observed within a session, counted as input tokens plus cache read tokens plus cache write tokens and excluding output and reasoning tokens.
_Avoid_: Context used when it could mean total tokens, output tokens, or context window size

**Implicit View Sync**:
The product behavior where opening the viewer first refreshes all supported Durable Sources so the dashboard reflects newly available canonical token facts. Viewer filters remain display constraints, not source selection.
_Avoid_: Filtered sync, view-only sync

**Claude Code Harness**:
The Claude Code command-line coding harness as a token usage source, distinct from the Anthropic provider and Claude model family.
_Avoid_: Claude as a harness ID

**Parent Session Token Attribution**:
The rule that token usage produced by a subordinate agent run belongs to the parent user-visible coding session when the source identifies that parent session.
_Avoid_: Counting subordinate agent runs as separate sessions when parent identity is available

**Provider Attribution Source**:
The canonical provenance marker that distinguishes provider values copied from source artifacts (`explicit`), derived from harness-level knowledge (`inferred`), or unavailable (`unknown`).
_Avoid_: Treating inferred provider values as source-provided facts

**Claude Code Inferred Provider**:
The rule that Claude Code artifact-derived token facts without explicit provider metadata canonicalize to provider `maybe-anthropic` with provider source `inferred`, because the artifact source is Claude Code but the provider was not explicitly present.
_Avoid_: Canonicalizing these rows as actual `anthropic` provider facts
