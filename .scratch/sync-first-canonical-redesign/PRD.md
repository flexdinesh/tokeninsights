# Sync-First Canonical Redesign PRD

Viewer surface note: the sync-first pipeline decisions in this PRD remain active, but the detailed `view` TUI behavior has been superseded by [`canonical-viewer-tui-redesign/PRD.md`](../canonical-viewer-tui-redesign/PRD.md) and [ADR 0001](../../docs/adr/0001-viewer-tabs-are-token-aggregation-modes.md).

## Problem Statement

TokenInsights currently depends on harness plugins writing directly into harness-specific SQLite tables. That makes local usage data hard to rebuild, hard to deduplicate across multiple ingest paths, and hard to extend consistently across OpenCode, Pi, and Codex.

The redesigned system needs to ingest durable local harness data, preserve enough raw facts for rebuilds, normalize trustworthy canonical facts, and let the TUI answer token usage questions without assuming every harness exposes the same data.

## Solution

Redesign TokenInsights around a local-first Go CLI with two primary workflows:

1. `tokeninsights sync` batch-ingests local harness data into raw tables and normalizes affected data into canonical facts.
2. `tokeninsights view` opens the interactive TUI over canonical data only.

V1 focuses on local SQLite, token usage, rebuildability, duplicate resistance, diagnostics, and sparse-data tolerance. Existing plugins are removed from the active product. Future checkpoint and realtime plugins should be possible, but they are not part of the V1 implementation.

## User Stories

1. As a TokenInsights user, I want to run `tokeninsights sync --harness opencode`, so that OpenCode local usage data is ingested.
2. As a TokenInsights user, I want to run `tokeninsights sync --harness pi`, so that Pi local usage data is ingested.
3. As a TokenInsights user, I want to run `tokeninsights sync --harness codex`, so that Codex local usage data is ingested.
4. As a TokenInsights user, I want to run `tokeninsights sync --all`, so that all supported harnesses are synced with one command.
5. As a TokenInsights user, I want `sync --all` to partially succeed, so that one failing harness does not prevent other harnesses from syncing.
6. As a TokenInsights user, I want repeated syncs to avoid duplicate raw and canonical facts, so that token totals stay stable.
7. As a TokenInsights user, I want `sync --dry-run`, so that I can preview discovered data without writing to the database.
8. As a TokenInsights user, I want `sync --no-normalize`, so that raw ingest can be inspected separately from canonical normalization.
9. As a TokenInsights user, I want `tokeninsights normalize`, so that existing raw data can be reprocessed without re-syncing harness sources.
10. As a TokenInsights user, I want `normalize --dry-run`, so that parser and normalization changes can be evaluated without mutating canonical data.
11. As a TokenInsights user, I want `reset-canonical --confirm`, so that canonical facts and diagnostics can be cleared explicitly.
12. As a TokenInsights user, I want `reset-all --confirm`, so that the local TokenInsights database can be recreated explicitly when the schema is incompatible or during development.
13. As a TokenInsights user, I want old incompatible databases to fail clearly, so that stale schema data is not silently misread.
14. As a TokenInsights user, I want to view token usage grouped by hour, so that I can understand when token usage happened.
15. As a TokenInsights user, I want to view token usage grouped by day, so that I can understand usage across a week.
16. As a TokenInsights user, I want to filter or group token usage by harness, so that I can compare OpenCode, Pi, and Codex usage.
17. As a TokenInsights user, I want to filter or group token usage by provider and model, so that I can answer questions like how many tokens Pi used with a GPT model over a week.
18. As a TokenInsights user, I want sparse harness data to render correctly, so that missing metrics do not break the TUI.
19. As a TokenInsights user, I want missing provider or model data to appear as `unknown`, so that valid token facts are still counted.
20. As a TokenInsights user, I want missing session identity to be rejected from canonical analytics, so that durable token rows remain session-centric.
21. As a TokenInsights developer, I want raw harness facts separated from canonical facts, so that canonical analytics can be rebuilt.
22. As a TokenInsights developer, I want raw observations separated from raw facts, so that repeated sync sightings can be audited without bloating raw fact tables.
23. As a TokenInsights developer, I want semantic keys in canonical tables, so that rebuilds and repeated syncs converge without duplicates.
24. As a TokenInsights developer, I want structured normalization diagnostics, so that skipped, rejected, and conflicting data is explainable.
25. As a TokenInsights developer, I want fixture-based conformance tests, so that future TypeScript checkpoint plugins can match Go sync behavior.
26. As a TokenInsights developer, I want clear package and file boundaries, so that future rewrites and refactors remain manageable.
27. As a future plugin author, I want the V1 data contract to support checkpoint plugins later, so that plugins can reuse the same raw and canonical concepts even if they are not implemented now.

## Implementation Decisions

### Product Scope

- V1 is sync-first. Batch sync is the primary ingest path.
- Realtime metrics and realtime plugins are not implementation priorities for V1.
- Checkpoint plugins are documented as a future-compatible ingest path, but not implemented in V1.
- Existing plugin packages and direct-write plugin assumptions are removed.
- The product remains local-only. Cloud sync is out of scope.
- Cost tracking is out of scope.

### Command Surface

- The CLI remains implemented in Go.
- The user-facing commands are `sync`, `normalize`, `reset-canonical`, `reset-all`, and `view`.
- `tokeninsights sync --harness <harness>` ingests one harness.
- `tokeninsights sync --all` attempts all supported harness adapters.
- `sync` normalizes affected data by default.
- `sync --no-normalize` writes raw data and observations without canonical normalization.
- `sync --dry-run` discovers and parses candidate data, reports counts and diagnostics, and writes nothing.
- `normalize` reprocesses existing raw data without re-syncing source data.
- `normalize --dry-run` computes candidate canonical changes and diagnostics without writing them.
- `reset-canonical --confirm` deletes canonical facts and normalization diagnostics only.
- `reset-all --confirm` recreates the database file and SQLite sidecar files.
- Reset commands do one thing. They do not automatically sync or normalize after reset.
- Reset commands without `--confirm` explain what would be deleted and exit without changes.

### Database And Schema

- V1 uses a greenfield schema.
- Backwards compatibility with the current `oc_*` and `pi_*` tables is not required.
- Incompatible existing databases must fail clearly and require an explicit reset.
- `packages/schema/schema.sql` remains the schema source of truth.
- Schema changes still require explicit approval before implementation.
- `PRAGMA user_version` gates schema compatibility.
- The default DB path remains `~/.local/share/tokeninsights/tokeninsights.sqlite`.
- `TOKENINSIGHTS_DB_PATH` remains the DB path override.
- Retention and pruning behavior are out of scope for V1.
- `TOKENINSIGHTS_RETENTION_DAYS` should be removed or ignored during the redesign.

### Raw Layer

- Raw storage is harness-owned but structurally similar across harnesses where practical.
- Every raw domain table uses a shared envelope.
- Common raw concepts should use common column names when available: session ID, message or turn ID, provider, model, token counts, timing, tool identity, and status.
- Raw preserves source absence as null or missing values.
- Raw tables may contain typed harness-specific columns when useful.
- `raw_payload_json` is not required. It may be used only for tightly constrained, redacted, metadata-only fields that are useful during parser iteration.
- Raw storage must not dump full source records.
- Raw storage must not contain prompt text, assistant text, tool arguments, tool output, request headers, secrets, or raw provider payloads.
- Full source file or database paths are not stored in V1.
- Source identity should use stable logical IDs or hashed locators where needed.
- Raw facts are deduplicated wherever safely possible.
- Raw dedupe should use stable harness/source identities when available and deterministic non-private fingerprints when not.
- Raw dedupe must not discard meaningfully different observations from different collector or parser versions.
- Raw facts and raw observations are separate.
- Raw fact tables store deduplicated harness facts.
- Raw observations record ingest-run sightings of raw facts.

### Ingest Runs

- Ingest runs are immutable audit records after completion.
- An active ingest run may update status, error, and completion fields.
- Completed ingest identity and provenance are not rewritten.
- Re-syncing the same source creates a new ingest run and records observations without duplicating raw facts where dedupe keys match.
- `sync --all` may partially succeed.
- Failed harnesses record failure status and errors where applicable.
- Successful harness scopes are still normalized when possible.
- If any requested harness fails, the command exits non-zero after attempting the requested work.

### Canonical Layer

- Canonical data is a cleaned, normalized, duplicate-resistant time series.
- Canonical tables are not pre-aggregated rollups.
- The TUI and future local viewers query canonical tables by default.
- The primary V1 canonical fact is `canonical_token_usage`.
- `canonical_token_usage` is one fact table for all token categories, not separate tables by time period or harness.
- Canonical token rows carry enough dimensions for flexible grouping: time, harness, session, optional message, provider, model, token counts, semantic key, provenance, quality, and countability.
- Canonical facts use stable semantic keys in addition to local integer IDs.
- Canonical identity must not depend on raw row IDs or import order.
- Repeated syncs and rebuilds must converge on the same canonical identities for the same underlying facts.
- Canonical facts include simple quality metadata, such as `exact`, `derived`, or `estimated`.
- Canonical token usage supports a `usage_scope` or equivalent field so different source grains can coexist.
- Canonical token analytics query only `is_countable = true` rows by default.
- Normalization must avoid double-counting when both fine-grained and fallback token facts exist for the same semantic usage.
- Canonical provider and model values use `unknown` when missing.
- Every canonical row must resolve to a stable canonical session.
- If a source lacks an explicit harness session ID, the normalizer may derive a stable session key from non-private source metadata.
- If no stable session identity can be derived, the fact is skipped and a diagnostic is written.
- `canonical_messages` or canonical turns should exist as a first-class concept, but fact tables must tolerate missing message identity.

### Normalization And Diagnostics

- Normalization is a separate idempotent pass invoked after ingest by default.
- Normalization can also be run explicitly.
- Normalization is incremental by default.
- Full canonical rebuild from raw data must be available through explicit commands.
- Canonical conflict handling is deterministic and precedence-driven.
- Prefer exact durable harness data over realtime estimates.
- Prefer checkpoint or sync parsed final session data for token totals.
- Realtime data is reserved for facts that are unavailable after completion, such as live timing, when implemented later.
- Prefer newer parser output only when parser rules explicitly supersede older output.
- Canonical rows should include a primary provenance pointer to the winning raw fact or ingest run.
- Many-to-many canonical provenance is out of scope for V1.
- Conflicts, skipped facts, rejected alternatives, and suppressed duplicates are recorded in structured normalization diagnostics.
- Diagnostics are database rows, not just logs.
- Dry-runs compute diagnostics for command output but do not persist diagnostics.
- No diagnostics TUI is included in V1.

### Viewer Behavior

- `tokeninsights view` is interactive-only.
- Export and non-interactive summaries are out of scope.
- The TUI reads canonical tables only.
- The TUI must not assume any canonical domain has rows.
- The TUI must tolerate sparse data and uneven harness coverage.
- Missing metrics are valid states.
- Filters and selectable values derive from available canonical data.
- Query failures for one domain should not make the entire TUI unusable where partial results can still be shown.
- Token analytics should answer questions such as tokens by hour, day, session, harness, provider, and model.

### Harness Adapter Scope

- V1 starts OpenCode, Pi, and Codex adapters together to reveal schema and pipeline rough edges early.
- All adapters use the same interface.
- Uneven metric coverage is acceptable.
- The minimum success bar for each adapter is:
  - discover at least one durable local source;
  - create an ingest run;
  - write deduplicated raw facts and raw observations;
  - normalize at least sessions plus one useful domain, preferably token usage when available;
  - record diagnostics for unavailable or rejected data;
  - run repeatedly without duplicating canonical rows;
  - make produced canonical facts visible in the TUI.

### Code Boundary Recommendation

Implementation should keep clear boundaries and small, isolated units. CLI command handlers should orchestrate behavior, not own parsing, ingestion, normalization, diagnostics, and rendering logic directly.

Recommended boundaries:

- command orchestration: parse flags, call services, print summaries;
- DB lifecycle: open, create, schema version checks, reset behavior;
- harness adapters: discover and parse harness sources;
- raw ingest service: ingest runs, raw facts, observations, raw dedupe;
- normalization service: semantic keys, precedence, quality, canonical writes;
- diagnostics service: structured warnings and errors;
- canonical query layer: read models for the TUI;
- TUI model and rendering: interactive display only;
- fixture conformance: source input to expected raw, canonical, and diagnostic output.

This boundary discipline is part of the design, not optional cleanup. The redesign should be easier to understand, rewrite, refactor, and extend with future plugin parity.

### Cross-Language Parity

- The V1 sync pipeline is Go.
- Future checkpoint plugins may be TypeScript and may implement equivalent ingest behavior.
- Cross-language parity should be guarded by language-neutral fixture conformance tests.
- The same fixture source should have expected raw facts, canonical facts, and diagnostics.
- Future TypeScript plugin implementations must match those expectations.
- SQL schema remains the shared contract.
- Schema constants and types should be generated or mechanically validated across Go and TypeScript rather than manually trusted.

## Testing Decisions

- Prefer high-level command and pipeline tests over implementation-detail tests.
- Test the sync command seam with fixture sources and assert raw facts, observations, canonical facts, and diagnostics.
- Test the normalize seam from existing raw facts and assert idempotent canonical output.
- Test reset commands with and without `--confirm`.
- Test viewer query/model behavior against canonical tables only.
- Test empty and sparse canonical datasets.
- Test duplicate protection by running sync and normalize repeatedly.
- Test incompatible schema handling.
- Test partial success behavior for `sync --all`.
- Test dry-runs write no ingest runs, raw facts, observations, canonical facts, or diagnostics.
- Add fixture conformance tests that can be reused later by TypeScript checkpoint plugins.

## Out Of Scope

- Cost tracking.
- Cloud sync.
- Export commands.
- Non-interactive summaries.
- Automated pruning or retention.
- Realtime plugin implementation.
- Checkpoint plugin implementation.
- Diagnostics TUI.
- Full source file or database paths.
- Prompt text.
- Assistant text.
- Tool arguments.
- Tool output.
- Request headers.
- Secrets.
- Raw provider payloads.
- Migration from existing `oc_*` and `pi_*` schema.

## Further Notes

- TPS remains a supported project domain, but realtime metrics do not drive V1.
- If durable sources expose timing or TPS data, adapters may normalize it.
- If timing or TPS data is unavailable, it should be absent rather than blocking sync, normalization, or viewing.
- Token usage is the primary V1 acceptance fact because it supports the north-star questions around local usage over time.
