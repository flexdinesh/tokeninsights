# TokenInsights

TokenInsights tracks local token usage across coding harnesses through host collection and normalization, canonical-only ingestion, and server queries over retained history.

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

**Syncable Analytics Data**:
Normalized usage facts and their stable session/message/location references governed by the publication privacy contract. Raw facts, parsing diagnostics, and continuity metadata remain local to the collector.
_Avoid_: Local cursor data, source continuity state

**Local-only Continuity Metadata**:
Operational source refresh state used only to resume local Durable Source parsing and never sent to the canonical server or used for server analytics.
_Avoid_: Analytics data, canonical data, syncable data

**Session Peak Context Load**:
The largest prompt-side context load observed within a session, counted as input tokens plus cache read tokens plus cache write tokens and excluding output and reasoning tokens.
_Avoid_: Context used when it could mean total tokens, output tokens, or context window size

**Collector**:
The host-side Go workflow that discovers Durable Sources, captures metadata-only raw facts, normalizes usage in collector SQLite, and publishes its durable canonical journal.
_Avoid_: Server-side source parsing, raw ingestion into server

**Canonical Server**:
The shared local/remote Go server core that transactionally ingests normalized facts, dedupes stable IDs, persists receipts, and serves REST analytics plus embedded web assets.
_Avoid_: Collector service, server-side normalization

**Read-only View**:
`tokeninsights tui` and the browser read committed server data through the same REST API. Reload requests queries only. Explicit `tui --sync` performs caller-side collection/publication first; viewer filters remain display constraints.
_Avoid_: Implicit View Sync, dashboard source refresh

**Durable Publication**:
Immutable normalized journal entries and saved upload batches in collector SQLite, acknowledged per destination only after server commit. Later manual sync retries pending delivery.
_Avoid_: Best-effort upload marker, insert-only export

**Stable Fact Identity**:
A reproducible source-native session/message/request tuple independent of collector installation, SQLite row IDs, delivery batches, and capture time.
_Avoid_: Payload hash as request identity, equal token counts as dedupe evidence

**Incremental Source Refresh**:
A best-effort source refresh that reads newly available usage facts from a Durable Source when prior source continuity can be trusted, while preserving a full-refresh fallback when continuity cannot be trusted.
_Avoid_: Filtered sync, partial view sync, partial normalize

**Recent Source Refresh**:
A best-effort source refresh that skips Durable Sources whose local modification metadata shows they have not changed since a conservative freshness window before the last successful source refresh.
_Avoid_: Date-filtered sync, event-time sync

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
