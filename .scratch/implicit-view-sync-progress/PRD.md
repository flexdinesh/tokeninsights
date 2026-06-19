# Implicit View Sync Progress PRD

Status: ready-for-agent

## Problem Statement

TokenInsights now refreshes Durable Sources automatically when users open `view`, but the current implementation performs that sync before the interactive TUI starts. If sync takes noticeable time, the user sees no dashboard state explaining what is happening.

Users want the viewer to open immediately into a clear loading experience while Implicit View Sync runs, with enough high-level harness progress to understand where the refresh is spending time.

## Solution

Move Implicit View Sync into the TUI lifecycle. When a user runs `tokeninsights view` or plain `tokeninsights`, the interactive terminal UI should launch immediately and render a full-screen sync progress state. The sync progress state should show one row per supported harness in the existing supported harness order and update each harness sequentially as sync advances.

The sync pipeline should expose optional progress events or callbacks that the TUI can consume. Explicit `tokeninsights sync` should keep its existing one-line summary output and should not render live progress in this work.

After sync and normalization finish successfully, the TUI should transition to the existing canonical table loading flow, then render the normal Aggregation Tab table. If implicit sync fails, the TUI should exit and the command should print the same terminal sync summary, error, and targeted harness recovery guidance already expected for Implicit View Sync failure.

`view --no-sync` should skip the sync progress state entirely and go straight to the existing canonical table loading behavior over existing canonical facts.

## User Stories

1. As a TokenInsights user, I want `tokeninsights view` to open the TUI immediately, so that I see visible feedback while Durable Sources are being refreshed.
2. As a TokenInsights user, I want plain `tokeninsights` to show the same sync progress state, so that the default command behaves consistently with `view`.
3. As a TokenInsights user, I want a full-screen sync progress view before the table renders, so that I know the dashboard is refreshing rather than hanging.
4. As a TokenInsights user, I want the sync progress view to show each supported harness, so that I can see which harness is currently being refreshed.
5. As a TokenInsights user, I want high-level harness statuses, so that I can understand progress without seeing private source details.
6. As a TokenInsights user, I want harness progress to update sequentially in the existing supported harness order, so that progress matches `sync --all` behavior.
7. As a TokenInsights user, I want each harness to start as `pending`, so that I can see the full planned refresh scope.
8. As a TokenInsights user, I want a harness to show `discovering` while TokenInsights looks for Durable Sources, so that I know discovery has started.
9. As a TokenInsights user, I want a harness to show `syncing` while discovered Durable Sources are being ingested, so that I know ingestion is active.
10. As a TokenInsights user, I want a harness to show `skipped` when no Durable Sources are found, so that absence of data is visible and not mistaken for failure.
11. As a TokenInsights user, I want a harness to show `synced` when raw ingest completed, so that completed harnesses are clear.
12. As a TokenInsights user, I want a harness to show `failed` when that harness fails, so that the failing harness is identifiable.
13. As a TokenInsights user, I want the loader to show `normalizing` after successful harness scopes finish, so that I know canonical facts are being prepared.
14. As a TokenInsights user, I want the loader to show `loading dashboard` after sync and normalization finish, so that the transition to canonical queries is clear.
15. As a TokenInsights user, I do not want source paths, project names, or source IDs in the progress view, so that screenshots do not leak local context.
16. As a TokenInsights user, I want the normal table to render only after successful implicit sync and canonical loading, so that stale or partial tables are not shown after failure.
17. As a TokenInsights user, I want implicit sync failure to exit the TUI and print the terminal summary, error, and recovery guidance, so that failure handling stays simple and script-friendly.
18. As a TokenInsights user, I want `view --no-sync` to skip harness progress entirely, so that read-only viewing remains fast and semantically clear.
19. As a TokenInsights user, I want `view --no-sync` to keep the existing canonical table loading state, so that there is still feedback while rows are queried.
20. As a TokenInsights user, I want explicit `tokeninsights sync --all` output to stay unchanged, so that existing terminal and script workflows do not change.
21. As a TokenInsights maintainer, I want sync progress exposed as an optional pipeline contract, so that the TUI can consume it without forking sync behavior.
22. As a TokenInsights maintainer, I want the sync pipeline to remain sequential, so that DB writes, failure timing, and summary semantics remain stable.
23. As a TokenInsights maintainer, I want progress events to avoid source-level private metadata, so that the privacy boundary remains intact.
24. As a TokenInsights maintainer, I want the TUI to reuse existing canonical table loading after sync completes, so that viewer query behavior remains unchanged.
25. As a TokenInsights maintainer, I want docs to describe the TUI sync progress state, so that the design contract matches product behavior.
26. As an AFK implementation agent, I want the progress states and failure behavior captured clearly, so that implementation can proceed without reopening product decisions.

## Implementation Decisions

- Implicit View Sync should run inside the TUI lifecycle rather than before the TUI process starts.
- `tokeninsights view` and plain `tokeninsights` should launch into a full-screen sync progress state unless `--no-sync` is present.
- `view --no-sync` should skip the sync progress state and go directly to existing canonical table loading.
- Sync should remain sequential in the supported harness order.
- The sync pipeline should expose optional progress events or callbacks for TUI consumers.
- Explicit `tokeninsights sync` should keep its current one-line summary output.
- The progress contract should be high-level and harness-scoped only. It should not include individual source paths, source IDs, project names, or raw source details.
- The progress state vocabulary is: `pending`, `discovering`, `syncing`, `skipped`, `synced`, `failed`, `normalizing`, and `loading dashboard`.
- All supported harnesses should appear in the progress view from the start as `pending`.
- A harness should become `discovering` before adapter discovery starts.
- A harness should become `skipped` when discovery finds no Durable Sources.
- A harness should become `syncing` once discovered Durable Sources are being ingested.
- A harness should become `synced` when raw ingest for that harness completes successfully.
- A harness should become `failed` when that harness fails.
- The global progress state should show `normalizing` while canonical facts are being rebuilt after successful harness scopes.
- The global progress state should show `loading dashboard` after sync and normalization finish and before the normal table is rendered.
- If implicit sync fails, the TUI should exit and the command should print the same summary, error, and recovery guidance used by the existing implicit sync failure path.
- No schema changes are required.
- No ADR is required because the change is reversible UI and pipeline-contract behavior, not a hard-to-reverse architecture decision.

## Testing Decisions

- Test at the highest existing seams: sync pipeline behavior with optional progress events, command-level `view` behavior, TUI model state transitions, and rendered view output.
- Good tests should verify externally observable behavior: progress statuses emitted, progress rows rendered, `--no-sync` skipping progress, successful transition to table loading, and failure exiting with terminal guidance.
- Pipeline tests should verify progress event order and harness-scoped statuses without asserting private helper structure.
- TUI tests should verify the initial sync progress screen and transitions through sync completion into existing canonical table loading.
- Command tests should verify `view` and plain `tokeninsights` launch into progress-enabled TUI behavior, while `view --no-sync` and `tokeninsights --no-sync` skip sync progress.
- Failure-path tests should verify that implicit sync failure exits the TUI and surfaces the existing summary/error/recovery behavior.
- Rendering tests should verify that the progress screen does not include source paths or source IDs.
- Existing table loading tests are prior art for stable loading-state rendering and should remain valid.
- Existing sync and pipeline conformance tests are prior art for ensuring sync semantics remain unchanged.
- Documentation updates should be verified through the normal project checks.

## Out of Scope

- Source-level progress, source paths, source IDs, project names, or file counts derived from private local paths.
- Concurrent harness sync.
- Changing explicit `tokeninsights sync` output.
- Allowing the table to render after failed implicit sync.
- In-TUI failure recovery actions.
- New sync-scoping flags for `view`.
- Schema changes.
- Token semantic changes.
- New Aggregation Tabs or metric domains.
- Realtime plugins or checkpoint plugins.
- Cost tracking.

## Further Notes

The existing viewer already has a canonical table loading state for query reloads after the TUI starts. This feature adds a distinct pre-table sync progress state for Implicit View Sync and should preserve the existing table loading behavior for `--no-sync`, tab switches, filters, and dashboard query reloads.
