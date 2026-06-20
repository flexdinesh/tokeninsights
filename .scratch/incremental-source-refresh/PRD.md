# Incremental Source Refresh PRD

Status: ready-for-agent

## Problem Statement

Opening TokenInsights currently feels heavier than it should because `tokeninsights view` runs Implicit View Sync before rendering the TUI, and that refresh currently reparses discovered Durable Sources and runs normalization over all matching raw token facts. The pipeline is idempotent, but the repeated work makes a normal dashboard open feel like it may be doing 100% of the historical parsing and canonicalization every time.

Users want the default dashboard workflow to keep data fresh without paying the full historical parsing and normalization cost when TokenInsights can prove the relevant source data has not changed.

## Solution

Implement source refresh optimization in phases.

Phase 1 introduces Recent Source Refresh. TokenInsights records Local-only Continuity Metadata for successful source refreshes and skips file-based Durable Sources whose local modification metadata is older than a conservative freshness window before the last successful source refresh. The initial freshness window is 48 hours. Sources inside the window are still fully parsed with existing parser behavior, and raw fact dedupe remains the correctness guard.

Phase 2 introduces an incremental normalization work queue. Newly inserted raw token facts enqueue token-usage normalization work. Ordinary normalization processes pending work instead of scanning all raw facts. Rebuild paths mark raw facts dirty and reuse the same mechanism.

Phase 3, true intra-source cursors, remains future work unless Phase 1 and Phase 2 are not enough. It may add JSONL byte-offset cursors and OpenCode SQLite row cursors later, but the first implementation should prefer the simpler freshness-window approach.

## User Stories

1. As a TokenInsights user, I want opening `tokeninsights view` to avoid reparsing old unchanged Durable Sources, so that the dashboard opens faster.
2. As a TokenInsights user, I want Implicit View Sync to keep refreshing recent and changed sources, so that the dashboard still reflects current token usage.
3. As a TokenInsights user, I want source refresh optimization to be best effort, so that missing or stale continuity metadata does not break sync.
4. As a TokenInsights user, I want TokenInsights to fall back to full parsing when source continuity cannot be trusted, so that correctness wins over speed.
5. As a TokenInsights user, I want a conservative freshness window, so that clock drift, delayed writes, and boundary timing do not cause recent data to be skipped.
6. As a TokenInsights user, I want old closed session files to be skipped when unchanged, so that historical local usage does not slow every dashboard open.
7. As a TokenInsights user, I want recent session files to be fully parsed even if some facts are duplicates, so that raw dedupe can preserve correctness.
8. As a TokenInsights user, I want unchanged Pi session files to be skipped after a successful refresh, so that repeated view opens are cheaper.
9. As a TokenInsights user, I want unchanged Codex session files to be skipped after a successful refresh, so that Codex history does not repeatedly reparse.
10. As a TokenInsights user, I want unchanged Claude Code transcript files to be skipped after a successful refresh, so that transcript history does not repeatedly reparse.
11. As a TokenInsights user, I want unchanged OpenCode SQLite databases to be skipped when local modification metadata proves they are old, so that OpenCode history does not repeatedly scan.
12. As a TokenInsights user, I want changed or recent OpenCode SQLite databases to keep using the existing full parser, so that row-cursor complexity is not required for the first performance win.
13. As a TokenInsights user, I want a `sync --full-refresh` escape hatch, so that I can force TokenInsights to ignore source refresh state without deleting the database.
14. As a TokenInsights user, I want `sync --dry-run` to preview the work a real sync would do, so that dry-run remains useful with source refresh state.
15. As a TokenInsights user, I want `sync --dry-run` to remain write-free, so that it never advances source refresh state or enqueues work.
16. As a TokenInsights user, I want `sync --no-normalize` to remember newly ingested raw facts still need canonicalization, so that a later normalize can finish the work.
17. As a TokenInsights user, I want ordinary `normalize` to process pending work only, so that it does not rescan the entire raw fact table.
18. As a TokenInsights user, I want `normalize --dry-run` to count pending work, so that it predicts ordinary normalization accurately.
19. As a TokenInsights user, I want `reset-canonical` to rebuild canonical facts from existing raw facts, so that it still means "rebuild derived canonical data."
20. As a TokenInsights user, I want `reset-all` to clear source refresh state, so that the next sync starts from Durable Sources from scratch.
21. As a TokenInsights user, I want `view --no-sync` to remain read-only, so that it does not process pending normalization work.
22. As a TokenInsights user, I want Implicit View Sync to process existing pending normalization work before opening the dashboard, so that canonical data is not stale after a prior `sync --no-normalize`.
23. As a TokenInsights user, I want sources that are up to date to be reported as successfully checked rather than absent, so that sync status remains understandable.
24. As a TokenInsights maintainer, I want source refresh state to be Local-only Continuity Metadata, so that it cannot become viewer analytics or future cloud export data.
25. As a TokenInsights maintainer, I want Syncable Analytics Data to remain metadata-only, so that the privacy contract stays intact.
26. As a TokenInsights maintainer, I want future cloud export to be canonical-first by default, so that raw ingestion mechanics do not leak into cloud sync.
27. As a TokenInsights maintainer, I want source refresh optimization to keep raw fact dedupe, so that duplicate parsing does not duplicate analytics.
28. As a TokenInsights maintainer, I want normalization work to be keyed by raw fact and canonical domain, so that future TPS, request, or tool domains can use the same queue shape.
29. As a TokenInsights maintainer, I want missing-session diagnostics to complete normalization work, so that unresolvable facts are not retried forever.
30. As a TokenInsights maintainer, I want failed normalization transactions to leave work pending, so that the next run can retry safely.
31. As a TokenInsights maintainer, I want source refresh state advancement to be transactional with successful ingest, so that a crash cannot skip unpersisted facts.
32. As a TokenInsights maintainer, I want Phase 3 true cursors to remain optional future work, so that the first implementation avoids unnecessary adapter complexity.
33. As an AFK implementation agent, I want the schema, docs, tests, and command behavior specified together, so that the change can be implemented without reopening the product design.

## Implementation Decisions

- Implement the plan in three phases: Recent Source Refresh, incremental normalization work queue, and optional future true intra-source cursors.
- Recent Source Refresh is a best-effort optimization based on local source modification metadata, not source event timestamps or local calendar dates.
- The initial freshness cutoff is `last_successful_source_refresh_at_ms - 48h`.
- Sources older than the freshness cutoff may be treated as up to date and skipped.
- Sources at or after the freshness cutoff are parsed from the beginning with the existing parser behavior.
- Raw fact dedupe remains mandatory and remains the correctness guard for repeated parsing inside the freshness window.
- Source refresh state is Local-only Continuity Metadata. It must not feed viewer tables or future cloud export.
- Syncable Analytics Data remains metadata-only. Future cloud export should be canonical-first by default.
- Avoid raw JSONL lines and full source paths in source refresh state unless a specific adapter cannot maintain continuity without them.
- Add durable source refresh state after explicit schema approval. The state should be keyed by harness, source kind, and adapter-provided source state key.
- Source refresh state should record the relevant parser and collector provenance, last successful source refresh time, and observed modification metadata needed for freshness checks.
- Add durable pending normalization work after explicit schema approval. Work should be keyed by raw fact and canonical domain, initially `token_usage`.
- `sync --no-normalize` still enqueues normalization work for newly inserted raw facts.
- Ordinary `normalize` processes pending work by default rather than scanning all raw facts.
- A rebuild path can mark existing raw facts dirty and then process the same work queue.
- `reset-canonical` deletes canonical facts and diagnostics and marks all existing raw facts dirty.
- `reset-all` clears raw facts, canonical facts, pending work, and source refresh state by recreating the database.
- `view --no-sync` remains read-only and does not process pending work.
- Implicit View Sync should normalize existing pending work even when no new source facts were parsed.
- Add `sync --full-refresh` as a user-facing escape hatch that ignores source refresh state for the requested sync scope without requeueing all existing raw facts.
- `sync --dry-run` uses source refresh state to preview real sync behavior but remains write-free.
- Up-to-date sources should create lightweight completed ingest audit rows with zero raw facts and zero observations.
- Phase 3 true intra-source cursors are intentionally deferred until Phase 1 and Phase 2 have been evaluated.
- Schema, embedded schema, generated constants, README, CLI README, design docs, and tests must move together for implementation.

## Testing Decisions

- Use the existing pipeline-level integration seam as the primary test seam: run sync and normalize against temp SQLite databases and synthetic Durable Sources, then assert external database state and command summaries.
- Good tests assert observable behavior, not private helper details. They should verify skipped sources, parsed sources, pending work, canonical facts, diagnostics, ingest runs, and reset behavior.
- Pipeline conformance tests and existing sync/normalize tests are the closest prior art.
- Add tests for normalization work queue behavior: newly inserted raw facts enqueue work, ordinary normalize drains work, `sync --no-normalize` leaves work pending, and missing-session diagnostics complete work.
- Add tests for reset behavior: `reset-canonical` requeues raw facts and `reset-all` clears source refresh state and pending work.
- Add tests for Recent Source Refresh using synthetic file-based sources: old unchanged sources are skipped, recent sources are fully parsed, touched historical sources are parsed, and raw dedupe prevents duplicate analytics.
- Add command-level tests for `sync --full-refresh`, `sync --dry-run`, `sync --no-normalize`, `normalize --dry-run`, `reset-canonical`, and `reset-all`.
- Add implicit view sync tests proving pending normalization work is processed before loading the dashboard, while `view --no-sync` remains read-only.
- Schema changes must be verified with the existing schema check command.
- Full verification should use the repo's normal test and build commands.

## Out of Scope

- True JSONL byte-offset cursors in the first source refresh implementation.
- OpenCode SQLite row cursors in the first source refresh implementation.
- Cloud export implementation.
- Realtime plugins or checkpoint plugins.
- Changing token semantics, provider/model fallback rules, countable semantics, or viewer aggregation behavior.
- Making viewer filters scope source refresh.
- Storing prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, or full source paths in Syncable Analytics Data.
- Cost tracking.

## Further Notes

This PRD is based on the accepted design in the glossary, design guide, and ADR for incremental source refresh and local-only continuity state. Implementation requires schema changes and therefore must follow the repository rule requiring explicit approval before modifying schema files or table structures.
