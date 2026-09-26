# Sync reliability and trustworthy coverage

Status: Historical research; implementation and schema changes approved and completed. See [implementation notes](IMPLEMENTATION.md).

User decision: status must survive restarts and be shared by TUI and web.

## Verified findings

- Read-only inspection found six failed Codex ingest runs, all with `bufio.Scanner: token too long`.
- A retained archived Codex JSONL record is 16,878,896 bytes, above the parser's 16 MiB Scanner limit. Its type is `response_item`, payload type `custom_tool_call_output`; it carries no usage counters. No transcript contents or paths were copied into this report.
- That source matches four recorded failures. The other failed source identity is not currently discoverable; its content was not inspected.
- Current Go code reproduced the error using `sync --harness codex --dry-run --full-refresh` against the offending source. Exit 1; `codex parse: bufio.Scanner: token too long`. No live database writes.
- Latest recorded Codex attempt completed 321 sources, then failed on source 322 of 574. Discovery sorts full paths, placing archives before ordinary sessions. The failure left 26 archived sources and all 226 sources under `sessions/` unattempted. This can directly prevent recent usage from being imported.
- `syncHarness` returns on the first source failure. Other harnesses continue, but later sources in the failed harness do not.
- Normalization runs after all harness ingestion. TUI reloads its saved snapshot at overall completion; web analytics keys change only at the final sync revision. Newly ingested raw usage therefore remains invisible for part of a healthy sync too.
- `LastCompletedSync` uses the newest completed source ingest, not a successful normalized all-harness job. A recent timestamp can coexist with omitted sources.
- Ingest start/completion timestamps both use the invocation's fixed `Now`. All 10,451 inspected completed/failed runs had identical start and completion timestamps. These cannot diagnose elapsed work reliably.
- This checkout has no overall sync deadline or `sync took too long` message. It has a 30-second writer-lock wait, 5-second SQLite busy waits, and 30-second HTTP analytics deadlines. An active session is unnecessary to reproduce the verified failure; separate timeout reports remain possible.
- Local Codex sources total roughly 975 MiB. Ordinary source reuse hashes content twice and scans location metadata; changed sources repeat fingerprinting after parsing. Codex forks/subagents always reparse. This is a performance concern, not a measured timeout cause.

## Recommended sequence

### 1. Reliability first

- Replace the Scanner-size failure path with Reader/streaming JSONL handling. Preserve usage-bearing records regardless of unrelated payload size. Do not blindly skip every oversized record or merely raise the cap. Share the record reader across parsing, discovery metadata, and location fingerprinting.
- Continue other sources after recoverable source errors; collect failures and return a nonzero result after useful work finishes. Cancellation, unavailable database writes, and lifecycle failures stop the job. Preserve per-source atomicity and dedupe state across failed transactions.
- Capture a bounded source snapshot: initial file extent, complete records, stable session identity. Append activity must not make a sync chase a moving end of file. Defer an unfinished final record and reread it next sync; never advance continuity past unprocessed data. Keep OpenCode's existing SQLite read snapshot.
- Distinguish waiting for another writer, parsing failure, source changed during read, cancellation, and database failure. Use actual stage durations; leave the injected clock available for deterministic tests.
- Normalize and publish after each harness initially, including successful sources from a partially failed harness. Smaller source batches can follow after dedupe/replay and transactional behavior are verified. Keep incomplete compatibility recovery hidden from analytics.

### 2. Durable shared job status

Add a small operational metadata model, subject to explicit schema approval:

- A sync job: job identity, source-scope fingerprint, owner, phase, real timestamps, heartbeat, outcome, progress counters, and canonical data revision.
- Source attempts within that job: hashed source identity, harness, discovered/reading/ingested/normalizing/ready/unchanged/failed state, failure code, captured extent, and publication status. Existing ingest runs remain source audit history.
- Source-to-day coverage only where it can be established from usage timestamps; maintain unknown coverage explicitly. Decide whether this needs a persisted sparse index during implementation design.

Use the existing database writer lock as authority. TUI, CLI, and server read one shared status; a contender reports waiting and can show the owner's progress. Cross-process joining requires compatible requested scopes. Process-local revisions cannot identify durable canonical changes reliably.

Mark abandoned jobs interrupted only after verifying their owner no longer holds the writer lock. Define how active job metadata survives compatibility reset/recovery. Store metadata only: no transcript content, full paths, or model/provider secrets.

Publish canonical data and its durable revision transactionally. Progress notifications follow commit. Both viewers then reload consistent canonical snapshots, with updates coalesced so frequent progress events cannot block ingestion or overwhelm queries.

### 3. Day visibility

Create calendar placeholder rows in the presentation layer for bounded ranges, beginning with this week, Monday through today. Metrics remain absent (`—`) until facts exist or a successful check establishes no matching usage. No synthetic canonical token facts or sessions.

| Day state | Meaning |
| --- | --- |
| Unverified | Coverage unknown; absence is not zero |
| Pending | Relevant source checks have not finished |
| Updating | Parsing or normalization still underway |
| Partial | Some usage is available; relevant work is pending or failed |
| Checked as of time | Relevant retained source snapshots checked and normalized |
| No usage found as of time | Successful coverage check; no matching facts |

Retain saved totals during refresh, visibly marked with their freshness and coverage. Failed refreshes preserve the previous successful check time and explain the affected harness. Day-level information belongs in time rows; other aggregation tabs use a selected-range coverage summary with a details panel.

A source can span multiple days. Assign dates from usage timestamps, never file modification times or session start alone. New, failed, or rewritten sources with unknown date coverage keep the range conservatively unverified. Absence of a harness installation is distinct from a successful empty check. Coverage uses server-local calendar boundaries and identifies its timezone. Viewer filters still never restrict all-harness ingestion.

“Checked” means checked retained local artifacts as of a snapshot, not provider-account lifetime completeness. Today's sources can gain new durable usage after that snapshot.

### 4. Honest progress

- Discovery: indeterminate; show sources found and elapsed time.
- Ingestion: `322 / 574 sources checked`, with ready, unchanged, and failed counts.
- Normalization: `N / M facts processed` when the batch denominator is known.
- Publication: canonical revision changes only after commit; mark newly visible usage partial until remaining relevant work finishes.
- Completion: distinguish success, partial failure, cancelled, and interrupted.

Source-count percentages measure work items, not time remaining or percentage of all usage collected. If a single overall percentage is added, count a source as ready only after its required normalization/publication, and separately show failed attempts. Do not report successful 100% coverage when sources failed. Unknown denominators stay indeterminate.

Example:

| Day | Tokens | Coverage |
| --- | ---: | --- |
| Monday | 1.2M | Checked 09:42 |
| Tuesday | 840K | Checked 09:42 |
| Today | — | Pending: Codex |

Later, Today might show `120K · Partial: Codex still updating` before the final snapshot is ready. Totals, charts, sessions, and exports consume facts only; placeholders must not manufacture zero-valued activity.

## Verification for implementation

- Usage before and after a synthetic tool-output record above 16 MiB survives import; genuine oversized usage is preserved too.
- A failed middle source leaves later sources imported and normalized; job ends partial/nonzero. Failed source writes do not leak dedupe state or observations.
- A concurrent append and partial final line produce a bounded first sync, then exactly-once usage on retry. Rewrites and truncation retain safe full-parse fallback.
- Committed usage appears during ongoing ordinary sync; compatibility recovery remains hidden until complete.
- Failed and interrupted jobs persist across server/TUI restarts and cannot claim a successful check time.
- Multi-day sessions, unknown source dates, filtered empty results, missing harnesses, DST, and week boundaries yield honest coverage.
- Placeholder rows never change token totals or distinct-session counts.
- Source scheduling or batching preserves Codex ancestry/replay and copied-transcript dedupe. Delay recent-first scheduling until this is proven.
- Shared TUI/API status transitions agree; progress delivery is bounded and cancellation-safe.
- Run repository format/lint, focused tests, full tests, schema/API generation checks as applicable, then build and native-runtime verification.

## Resolved decisions

- Operational schema additions approved and implemented in V13.
- Compact status and expandable web coverage details implemented.
- Durable source UTC bounds derive bounded daily coverage; publication runs per harness.

## Primary references

- [Go bufio](https://pkg.go.dev/bufio#Scanner): Scanner stops unrecoverably on oversized tokens; Reader is recommended for large tokens or more control.
- [SQLite isolation](https://www.sqlite.org/isolation.html): independent readers see committed transactions; refreshing a read transaction exposes later commits.
- [W3C range widgets](https://www.w3.org/WAI/ARIA/apg/practices/range-related-properties/): unknown progress must remain indeterminate.
