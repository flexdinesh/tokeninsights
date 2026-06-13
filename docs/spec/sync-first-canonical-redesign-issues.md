# Sync-First Canonical Redesign Issue Breakdown

This document breaks the sync-first canonical redesign PRD into independently grabbable implementation slices. The slices are ordered by dependency and are intended to be copied into the project issue tracker when ready.

Last audited: 2026-06-12.

## Implementation Status Summary

| Issue | Status | Notes |
|-------|--------|-------|
| 1. Greenfield Schema And DB Reset Contract | Complete | Schema V3, DB creation, incompatible DB rejection, reset commands, and schema validation are implemented. |
| 2. Go CLI Command Surface For Sync/View Pipeline | Complete | `sync`, `normalize`, `reset-canonical`, `reset-all`, and `view` exist with the planned flags and thin command handlers. |
| 3. Raw Fact, Observation, And Ingest Run Pipeline | Complete | Raw ingest, observation recording, dedupe, dry-run isolation, and partial-success behavior are implemented and tested. |
| 4. Canonical Normalization And Diagnostics | Partial | Core token normalization and diagnostics exist. Real conflict/precedence handling and richer diagnostic categories remain. |
| 5. OpenCode, Pi, And Codex Sync Adapters | Partial | All three harnesses are wired through the adapter interface, but real harness-specific source research/parsing remains open. |
| 6. Canonical TUI Reader | Complete | Token viewing reads canonical token usage and tolerates sparse non-token domains. Independent partial-domain error isolation remains a future hardening item. |
| 7. Fixture Conformance Suite | Partial | Inline fixture-style pipeline tests exist. A language-neutral fixture contract with explicit expected fact files remains open. |
| 8. Remove Legacy Plugin Product Path | Complete | Legacy plugin/logger packages, workspace metadata, build scripts, stale docs, and old proposal file were removed. |

## 1. Greenfield Schema And DB Reset Contract

Type: HITL

Blocked by: None

Status: Complete

### What to build

Create the greenfield local SQLite contract for the sync-first architecture. The system should create new compatible databases, reject incompatible old databases clearly, and provide explicit reset behavior for destructive local data changes.

This slice must establish the raw, observation, canonical, ingest run, and diagnostic table concepts at the schema-contract level without preserving the old plugin-written table families.

### Acceptance criteria

- [x] The new schema has a fresh compatibility version.
- [x] The CLI can distinguish a missing DB, a compatible DB, and an incompatible old DB.
- [x] Missing DBs are created from the schema source of truth.
- [x] Incompatible DBs are rejected with a clear reset instruction.
- [x] `reset-all --confirm` recreates the DB file and SQLite sidecars.
- [x] `reset-canonical --confirm` removes canonical facts and normalization diagnostics without removing raw facts.
- [x] Reset commands without `--confirm` explain what would be deleted and make no changes.
- [x] The schema includes concepts for ingest runs, raw facts, raw observations, canonical token usage, canonical sessions, canonical messages or turns, and normalization diagnostics.
- [x] The schema includes enough metadata for semantic keys, quality, countability, provenance, harness identity, source identity, and timestamps.
- [x] Schema validation/check tooling is updated to treat the SQL schema as the source of truth.
- [x] Explicit user approval is obtained before changing the schema contract.

## 2. Go CLI Command Surface For Sync/View Pipeline

Type: AFK

Blocked by: Issue 1

Status: Complete

### What to build

Reshape the Go CLI around the new command surface while keeping `view` as the interactive TUI entry point. Commands should parse flags, validate combinations, call service boundaries, and print concise summaries without embedding adapter, ingest, normalization, or rendering logic in command handlers.

### Acceptance criteria

- [x] The CLI exposes `sync`, `normalize`, `reset-canonical`, `reset-all`, and `view`.
- [x] `sync` supports `--harness <harness>`.
- [x] `sync` supports `--all`.
- [x] `sync` supports `--dry-run`.
- [x] `sync` supports `--no-normalize`.
- [x] `normalize` supports `--dry-run`.
- [x] Reset commands support `--confirm`.
- [x] Invalid command and flag combinations produce clear errors.
- [x] `view` remains interactive-only.
- [x] No export or non-interactive summary command is introduced.
- [x] Command handlers are thin orchestration code and delegate DB lifecycle, ingest, normalization, and querying to isolated packages or services.

## 3. Raw Fact, Observation, And Ingest Run Pipeline

Type: AFK

Blocked by: Issues 1 and 2

Status: Complete

### What to build

Implement the raw ingest pipeline used by `tokeninsights sync`. The pipeline should record immutable ingest runs, deduplicate raw facts where safely possible, record raw observations for repeated sightings, and handle partial success when syncing multiple harnesses.

### Acceptance criteria

- [x] A sync creates an ingest run with harness, collector, parser, source, status, and timestamp metadata.
- [x] Completed ingest runs are not rewritten except for active-run completion fields.
- [x] Raw facts are deduplicated by stable source identity or deterministic non-private fingerprints where possible.
- [x] Raw observations record each ingest-run sighting of a raw fact.
- [x] Re-running sync on the same source does not duplicate raw facts.
- [x] Re-running sync on the same source can record a new observation.
- [x] Raw rows preserve missing source values as null rather than `unknown`.
- [x] Full source paths are not stored.
- [x] Prompt text, assistant text, tool arguments, tool output, headers, secrets, and raw provider payloads are not stored.
- [x] `sync --dry-run` discovers and parses candidate raw facts but writes no ingest runs, raw facts, observations, canonical facts, or diagnostics.
- [x] `sync --all` attempts all requested harnesses when possible.
- [x] `sync --all` normalizes successful scopes when enabled even if one harness fails.
- [x] `sync --all` exits non-zero if any requested harness fails.
- [x] The command summary reports synced, skipped, failed, raw fact, observation, normalized, and diagnostic counts where applicable.

## 4. Canonical Normalization And Diagnostics

Type: AFK

Blocked by: Issue 3

Status: Partial

### What to build

Implement the normalization pipeline from raw facts to canonical facts. The pipeline should be idempotent, semantic-key driven, precedence-aware, sparse-data tolerant, and structured around diagnostics for skipped, rejected, conflicting, and suppressed facts.

### Acceptance criteria

- [x] Normalization can run automatically after sync.
- [x] Normalization can run explicitly through `tokeninsights normalize`.
- [ ] Normalization is incremental by default.
  - Current state: normalization is idempotent but scans and upserts all matching raw token facts.
- [x] Normalization can rebuild canonical facts from existing raw facts after `reset-canonical`.
- [x] `normalize --dry-run` computes candidate changes and diagnostics without writing them.
- [x] Canonical facts use stable semantic keys.
- [x] Re-running normalization does not duplicate canonical facts.
- [x] Canonical token usage includes time, harness, session, optional message, provider, model, token counts, usage scope, quality, countability, semantic key, and primary provenance.
- [x] Missing provider and model values are normalized to `unknown`.
- [x] Canonical facts require stable canonical session identity.
- [x] Facts without a stable explicit or derived session identity are skipped with diagnostics.
- [x] Multiple token usage scopes can coexist without double-counting default analytics.
- [x] Default token analytics use only countable canonical token rows.
- [ ] Conflict handling is deterministic and precedence-driven.
  - Current state: upserts are deterministic by semantic key, but there is no explicit conflict model or precedence table yet.
- [ ] Structured diagnostics are written for skipped, rejected, conflicting, and suppressed facts.
  - Current state: skipped missing-session facts and parser warnings are diagnostic-backed; rejected/conflicting/suppressed categories remain open.
- [x] Diagnostics contain metadata only and do not include full paths or private content.

## 5. OpenCode, Pi, And Codex Sync Adapters

Type: HITL

Blocked by: Issue 4

Status: Partial

### What to build

Implement OpenCode, Pi, and Codex sync adapters against the same harness adapter interface. The goal is to exercise the common pipeline across all three harnesses early, even if metric coverage is uneven.

Harness-specific source format research belongs with this slice, but the architecture should remain driven by the shared raw-to-canonical pipeline rather than by one harness.

### Acceptance criteria

- [x] All three harnesses are represented by concrete adapters.
  - Current state: the three adapters are configured instances of the shared JSONL/NDJSON adapter scaffold.
- [x] All adapters use the same adapter interface.
- [ ] Each adapter can discover at least one durable local source.
  - Current state: adapters scan configured default directories and `--source-dir` for JSONL/NDJSON, but real durable harness source discovery has not been researched or implemented.
- [x] Each adapter can create an ingest run.
- [x] Each adapter can write deduplicated raw facts and raw observations.
- [x] Each adapter can normalize at least sessions plus one useful canonical domain.
- [x] Token usage is normalized where durable token usage can be derived.
- [x] Uneven metric coverage across harnesses does not fail sync, normalization, or view.
- [ ] Unavailable or rejected data is represented through structured diagnostics.
  - Current state: parser warnings and normalization skips are diagnostic-backed; unavailable harness source coverage is not yet represented as structured diagnostics.
- [x] Repeated syncs for each adapter do not duplicate canonical rows.
- [x] `sync --all` uses the same partial-success behavior across all adapters.

## 6. Canonical TUI Reader

Type: AFK

Blocked by: Issue 4

Status: Complete

### What to build

Move the interactive TUI to read canonical tables only. The viewer should answer token usage questions from countable canonical facts while tolerating empty databases, sparse canonical domains, uneven harness coverage, and missing optional dimensions.

### Acceptance criteria

- [x] TUI token queries read `canonical_token_usage` or its canonical query abstraction.
- [x] TUI token totals use only countable canonical token rows.
- [x] The viewer can group token usage by hour.
- [x] The viewer can group token usage by day.
- [x] The viewer can group token usage by session.
- [x] The viewer can show or filter by harness.
- [x] The viewer can show or filter by provider and model.
- [x] Missing provider and model render as `unknown`.
- [x] Empty canonical tables produce a clean empty state rather than a crash.
- [x] Missing TPS, request, or tool-call data does not break token viewing.
- [x] Filters derive values from available canonical data.
- [ ] Query failures in one domain do not make unrelated available views unusable when partial display is possible.
  - Current state: non-token domains are empty placeholders, but independent partial-domain error isolation has not been exercised because token viewing is currently the only implemented canonical query domain.
- [x] The TUI remains interactive-only.

## 7. Fixture Conformance Suite

Type: AFK

Blocked by: Issues 4, 5, and 6

Status: Partial

### What to build

Add fixture-driven conformance tests for the sync-first pipeline. Fixtures should make behavior explicit from source discovery through raw ingest, normalization, diagnostics, duplicate protection, and viewer-facing canonical queries.

These fixtures are the future parity contract for TypeScript checkpoint plugins.

### Acceptance criteria

- [x] Fixtures exist for OpenCode, Pi, and Codex sources.
  - Current state: fixtures are inline JSONL test data generated by Go tests, not standalone fixture files.
- [ ] Fixtures define expected raw facts.
  - Current state: tests assert counts and selected fields, not complete expected raw fact records.
- [x] Fixtures define expected raw observations.
  - Current state: observation counts are asserted, including repeat-sync behavior.
- [ ] Fixtures define expected canonical facts.
  - Current state: tests assert counts and selected fields, not complete expected canonical fact records.
- [x] Fixtures define expected diagnostics.
  - Current state: missing-session diagnostic counts and codes are asserted.
- [x] Tests cover repeated sync idempotency.
- [x] Tests cover repeated normalization idempotency.
- [x] Tests cover sparse data and missing optional dimensions.
- [x] Tests cover missing provider/model normalization to `unknown`.
- [x] Tests cover missing session identity rejection.
- [x] Tests cover dry-run write isolation.
- [x] Tests cover partial success behavior for multi-harness sync.
- [ ] Fixture format is language-neutral enough for future TypeScript checkpoint plugin conformance.
  - Current state: fixtures are Go test literals; they should be promoted to shared source and expected-output files.

## 8. Remove Legacy Plugin Product Path

Type: AFK

Blocked by: Issue 1

Status: Complete

### What to build

Remove the current plugin packages and old direct-write product path from the active repository. The redesign should not leave old plugin schema assumptions in scripts, docs, tests, or package metadata.

### Acceptance criteria

- [x] Existing OpenCode plugin package code is removed.
- [x] Existing Pi plugin package code is removed.
- [x] Root workspace/package metadata no longer references deleted plugin packages.
- [x] Build scripts no longer reference deleted plugin packages.
- [x] Docs no longer describe plugin direct writes as the active architecture.
- [x] Tests no longer rely on old `oc_*` or `pi_*` plugin-written table families.
- [x] Shared packages are retained only if still used by the new CLI or schema tooling.
- [x] The active docs describe sync-first ingest and canonical viewing as the product path.

## Remaining Work

1. Replace the generic JSONL/NDJSON adapter scaffold with researched OpenCode, Pi, and Codex durable source adapters.
2. Add an explicit normalization conflict and precedence model, plus diagnostics for rejected, conflicting, and suppressed facts.
3. Promote inline Go fixture literals into language-neutral fixture directories with source files and expected raw/canonical/diagnostic outputs.
4. Add independent canonical query domains for TPS, requests, and tools when durable source data exists, then harden partial-domain error isolation in the TUI.
