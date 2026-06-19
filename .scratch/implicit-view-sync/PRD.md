# Implicit View Sync PRD

Status: ready-for-agent

## Problem Statement

TokenInsights currently makes users run a two-step workflow: first `sync` to refresh Durable Sources, then `view` to inspect canonical token facts. This is easy to forget, and a stale viewer can look like current usage even when newer local harness data is available.

Users want opening the dashboard to refresh data first, while still keeping an explicit escape hatch for read-only viewing of existing canonical facts.

## Solution

Make `tokeninsights view` perform Implicit View Sync before launching the interactive TUI. The implicit refresh behaves like `tokeninsights sync --all`: it discovers all supported Durable Sources, ingests raw facts, and runs normalization by default. After that write-before-read step completes successfully, the TUI opens over canonical token usage.

Add `--no-sync` to `view` so users can skip ingest and normalization and preserve the old read-only viewer behavior. Running `tokeninsights` without a subcommand remains a shortcut for `view`, so it should also sync implicitly unless `--no-sync` is provided.

## User Stories

1. As a TokenInsights user, I want `tokeninsights view` to refresh local Durable Sources before opening, so that the dashboard reflects current token usage.
2. As a TokenInsights user, I want `tokeninsights` with no subcommand to refresh before opening the viewer, so that the default command is useful without a two-step workflow.
3. As a TokenInsights user, I want `tokeninsights sync --all` to keep working as it does today, so that explicit refresh workflows remain available.
4. As a TokenInsights user, I want `tokeninsights view --no-sync` to skip all writes, so that I can inspect existing canonical facts without touching local harness data.
5. As a TokenInsights user, I want `tokeninsights --no-sync` to behave like `tokeninsights view --no-sync`, so that the no-subcommand shortcut has the same escape hatch.
6. As a TokenInsights user, I want Implicit View Sync to use all supported harnesses, so that opening the viewer refreshes the complete local token picture.
7. As a TokenInsights user, I want viewer `--harness` filters to remain display filters, so that filtering the dashboard does not accidentally narrow ingestion.
8. As a TokenInsights user, I want provider, model, and session filters to remain display constraints, so that Implicit View Sync does not mix source selection with viewer filtering.
9. As a TokenInsights user, I want Implicit View Sync to create the database when it is missing, so that first-run `view` behaves like first-run `sync`.
10. As a TokenInsights user, I want `view --no-sync` to fail on a missing database as the old read-only viewer did, so that read-only mode stays honest.
11. As a TokenInsights user, I want successful Implicit View Sync to avoid printing a pre-launch summary, so that the TUI opens cleanly.
12. As a TokenInsights user, I want failed Implicit View Sync to stop before launching the TUI, so that ingestion failures are not hidden behind stale or partial data.
13. As a TokenInsights user, I want failed Implicit View Sync to print the sync summary and underlying error, so that I can see what went wrong.
14. As a TokenInsights user, I want failed Implicit View Sync to recommend syncing individual harnesses directly, so that one failing harness does not block me from refreshing other harnesses manually.
15. As a TokenInsights user, I want the failure recommendation to mention opening `view --no-sync` after a manual sync, so that I have a clear recovery path.
16. As a TokenInsights user, I want `sync --harness <harness>` to remain the way to target a specific harness, so that source selection stays explicit.
17. As a TokenInsights user, I do not want `view` to grow `--source-dir`, so that fixture or custom source workflows remain explicit sync workflows.
18. As a TokenInsights maintainer, I want Implicit View Sync to reuse the existing sync pipeline, so that ingestion semantics do not fork.
19. As a TokenInsights maintainer, I want `--no-sync` to skip both raw ingest and normalization, so that the flag has a clear all-writes-off meaning.
20. As a TokenInsights maintainer, I want the docs to explain the new write-before-read behavior, so that the DB lifecycle contract remains clear.
21. As a TokenInsights maintainer, I want the design contract to preserve that the TUI queries canonical tables only, so that viewer aggregation semantics do not change.
22. As a TokenInsights maintainer, I want no schema change for this feature, so that no schema approval or migration work is required.
23. As an AFK implementation agent, I want the command behavior and docs captured in one PRD, so that the change can be implemented without reopening product decisions.

## Implementation Decisions

- Implicit View Sync means `view` performs the same refresh behavior as `sync --all` before opening the TUI.
- Plain `tokeninsights` remains equivalent to `tokeninsights view` and therefore also performs Implicit View Sync.
- Add a `--no-sync` flag to the viewer command surface. It skips both raw ingest and normalization.
- Viewer filters remain display-only constraints. In particular, `view --harness pi` syncs all supported harnesses first and then filters the viewer to Pi.
- Do not add sync-scoping flags to `view`; `--source-dir`, targeted harness refreshes, dry runs, and no-normalize workflows remain part of explicit `sync`.
- Implicit View Sync uses the existing sync pipeline and its DB lifecycle. A missing DB may be created during the implicit refresh.
- `view --no-sync` preserves the old read-only DB lifecycle and should fail on a missing or incompatible database.
- Successful Implicit View Sync should not print the normal one-line sync summary before the TUI launches.
- Failed Implicit View Sync should print the sync summary, return the sync error, and not launch the TUI.
- The failure output should recommend targeted manual sync with `tokeninsights sync --harness <harness>` for unaffected harnesses and then `tokeninsights view --no-sync`.
- The TUI remains interactive-only and continues to query canonical tables only after any optional pre-view sync.
- No schema changes are required.
- No ADR is required because this is reversible user-facing command behavior and is not a deep architectural trade-off.

## Testing Decisions

- Test at the highest existing seams: command-level CLI behavior, sync pipeline summary behavior, and DB lifecycle behavior.
- Good tests should assert external behavior: implicit sync occurs before viewer startup, missing DB handling matches sync when syncing is enabled, `--no-sync` preserves old read-only behavior, and failure output is actionable.
- Command tests should cover `tokeninsights view`, plain `tokeninsights`, `tokeninsights view --no-sync`, and `tokeninsights --no-sync`.
- Viewer flag parsing tests should verify `--no-sync` is accepted without changing existing date range, bucket, and Dimension Filter behavior.
- Failure-path tests should verify that the TUI is not launched when implicit sync returns an error and that the recommendation mentions targeted `sync --harness <harness>` plus `view --no-sync`.
- Tests should avoid relying on private helper implementation details. Prefer observable command output, returned errors, database rows, and whether the viewer startup path is invoked.
- Existing tests around sync harness selection, table option parsing, missing DB errors, and pipeline conformance are prior art.
- Documentation updates should be verified by the repo's normal test/build commands rather than a separate docs-only test seam.

## Out of Scope

- Schema changes.
- Changing `sync --all`, `sync --harness`, `sync --dry-run`, `sync --no-normalize`, or `sync --source-dir` behavior.
- Adding `--source-dir` or other sync-scoping flags to `view`.
- Making viewer Dimension Filters scope ingestion.
- Running sync concurrently with the TUI loading state.
- Opening the TUI after an implicit sync failure.
- Adding new viewer tabs, token semantics, or metric domains.
- Realtime plugins or checkpoint plugins.
- Cost tracking.

## Further Notes

The glossary defines Implicit View Sync as a viewer-opening behavior that refreshes all supported Durable Sources while keeping viewer filters as display constraints. The design docs currently state that `view` opens read-only; implementation should update that wording to distinguish the write-before-read command behavior from the read-only TUI query behavior.
