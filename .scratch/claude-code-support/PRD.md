# Claude Code Harness Support PRD

Status: ready-for-agent

## Problem Statement

TokenInsights can sync, normalize, and view token usage for OpenCode, Pi, and Codex, but users who use Claude Code cannot include those local token facts in the same session-centric viewer.

Claude Code stores modern durable transcript data as local JSONL files. TokenInsights should ingest the token usage available from those Durable Sources while preserving the existing privacy boundary, canonical session model, and idempotent sync behavior.

## Solution

Add Claude Code as a first-class harness with persisted harness ID `claude-code`. `tokeninsights sync --harness claude-code` and `tokeninsights sync --all` will discover modern Claude Code JSONL transcripts, parse assistant token usage metadata, normalize it into canonical token usage, and make it available in the existing viewer tabs and filters.

Claude Code support will be token-only and metadata-only. It will not parse Cursor in this work, will not call authenticated APIs, and will not store prompt text, assistant text, tool arguments, tool output, raw transcript payloads, full source paths, or secrets.

## User Stories

1. As a Claude Code user, I want TokenInsights to sync my Claude Code token usage, so that my local usage view includes the harness I use day to day.
2. As a TokenInsights user, I want `sync --all` to include Claude Code, so that I do not need a separate workflow for this harness.
3. As a TokenInsights user, I want `sync --harness claude-code` to work directly, so that I can refresh Claude Code data without touching other harnesses.
4. As a TokenInsights user, I want Claude Code token facts to appear in the harness aggregation tab, so that I can compare Claude Code with OpenCode, Pi, and Codex.
5. As a TokenInsights user, I want Claude Code token facts to appear in the model aggregation tab, so that I can see which Claude Code models produced usage.
6. As a TokenInsights user, I want Claude Code token facts to appear in the provider aggregation tab as `maybe-anthropic` when the source did not provide a provider, so that the viewer reflects the Claude Code artifact source without claiming an explicit Anthropic provider fact.
7. As a TokenInsights user, I want provider provenance recorded for canonical token facts, so that explicit provider values can be distinguished from inferred Claude Code provider values.
8. As a TokenInsights user, I want Claude Code subagent token usage counted under the parent session when Claude Code identifies the parent, so that session counts reflect user-visible coding sessions rather than subordinate agent runs.
9. As a TokenInsights user, I want Claude Code sidechain token usage included, so that parent sessions are not undercounted when work is delegated to subagents.
10. As a TokenInsights user, I want repeated `sync` runs to avoid duplicate token facts, so that my tables do not grow with duplicate usage rows.
11. As a TokenInsights user, I want repeated `normalize` runs to converge on the same canonical facts, so that viewer totals remain stable.
12. As a TokenInsights user, I want streaming duplicate Claude Code rows merged safely, so that partial token snapshots do not overcount usage.
13. As a TokenInsights user, I want Claude Code cache read tokens captured, so that cached input usage remains visible in the existing token columns.
14. As a TokenInsights user, I want Claude Code cache creation tokens captured as cache write tokens, so that cache creation usage remains visible in the existing token columns.
15. As a TokenInsights user, I want raw Claude Code totals to preserve source absence, so that derived totals are clearly a canonical concern.
16. As a TokenInsights user, I want Claude Code token facts marked as derived, so that the product does not claim billing-grade exactness for transcript-derived usage.
17. As a TokenInsights user, I want diagnostics when Claude Code data is skipped, suppressed, or limited, so that I can understand why expected facts may not appear.
18. As a TokenInsights user, I want malformed Claude Code rows without stable session identity skipped before raw ingestion, so that bad artifacts do not fill raw tables with weakly identified facts.
19. As a TokenInsights user, I want copied Claude Code transcript facts suppressed by logical dedupe, so that the same session copied across directories is not counted twice.
20. As a TokenInsights user, I want `--source-dir` fixture behavior to match other harnesses, so that tests and ad hoc imports are easy to run.
21. As a TokenInsights maintainer, I want schema, CLI, docs, and tests updated together, so that the schema contract stays coherent.
22. As a TokenInsights maintainer, I want the schema version bumped for the new persisted harness constraint, so that old databases fail clearly with the existing reset instruction.
23. As a TokenInsights maintainer, I want the Claude Code adapter behind the existing harness adapter interface, so that harness-specific parsing does not leak into normalization or viewer code.
24. As a TokenInsights maintainer, I want fixture-style conformance coverage for Claude Code, so that future parser changes can be validated against raw, canonical, observation, and diagnostic expectations.
25. As a privacy-conscious user, I want TokenInsights to avoid storing transcript content from Claude Code files, so that local code and conversation content remain outside the database.
26. As a privacy-conscious user, I want TokenInsights to avoid authenticated Cursor exports in this work, so that sync remains limited to local Durable Sources not behind API auth.
27. As an AFK implementation agent, I want the Claude Code decisions captured in a PRD and issues, so that implementation can proceed without reopening settled scope.

## Implementation Decisions

- Persist the Claude Code harness as `claude-code`, not `claude`, because the harness is distinct from the Anthropic provider and Claude model family.
- Cursor is out of scope. The known tokscale Cursor path depends on an authenticated API export cache, which does not meet the clarified Durable Source boundary for this work.
- The schema contract will add `claude-code` to every persisted harness constraint and bump the SQLite schema version. Existing older databases should continue to be rejected by the current reset-all instruction rather than migrated in place.
- Modern Claude Code discovery uses the configured Claude Code root and the `projects` transcript layout documented by Claude Code. The current configuration environment variable is `CLAUDE_CONFIG_DIR`; when absent, the default root is the user's `.claude` directory.
- Single-harness `--source-dir` scans the provided directory directly for JSONL fixtures. `sync --all --source-dir <root>` scans the `claude-code` subdirectory, consistent with existing harness behavior.
- The adapter parses modern JSONL transcript files only. Older or alternate JSON formats are out of scope unless current Claude Code docs identify them as active storage.
- The adapter captures token data and the metadata necessary for current TokenInsights workflows and patterns only.
- Main transcript session identity comes from the file stem. Subagent transcript token facts use the parent session identity when the source line provides it. If no parent identity exists, the subagent file stem is the fallback session identity.
- Token facts without stable session identity are skipped at parse time with diagnostics instead of being inserted as null-session raw facts.
- Assistant usage rows are the token source. Prompt text, assistant text, tool arguments, tool output, raw JSON lines, request headers, secrets, full source paths, and raw provider payloads must not be stored.
- Streaming duplicate rows are merged within a source file by `message.id + requestId` when both are present, or by `message.id` when request ID is absent. Merging keeps the maximum value seen per token component.
- Raw message identity uses `message.id` when present. Request ID may participate in dedupe, but it is not the canonical message identity.
- Claude Code `cache_read_input_tokens` maps to cache read tokens. Claude Code `cache_creation_input_tokens` maps to cache write tokens.
- Raw total tokens remain null when Claude Code does not provide a total. Existing canonical logic derives totals from token components.
- Claude Code explicit provider values are preserved as source-provided facts. When Claude Code artifacts omit provider metadata, canonical rows use provider `maybe-anthropic` with provider source `inferred`.
- Claude Code model values are stored when present. Missing model canonicalizes to `unknown`.
- Claude Code token facts are marked `derived`, not `exact`, because transcript rows may be streaming-derived snapshots rather than final billing records.
- A source-level info diagnostic should explain that Claude Code JSONL usage is derived from transcript data. This diagnostic should be emitted once per source file that produces token facts.
- Duplicate copied transcript facts should be suppressed by logical fact dedupe, independent of full source path.
- The viewer should keep using existing token aggregation tabs and dimension filters. No new tabs or metric domains are introduced.
- The design docs and README need to move with schema, harness, CLI, and token-semantics changes.

## Testing Decisions

- Test at the highest existing seams: sync plus normalization over fixture Durable Sources, schema contract validation, and CLI harness validation.
- Fixture-style conformance tests should cover Claude Code raw facts, observations, canonical token usage, and diagnostics.
- Focused parser tests should cover modern transcript rows, cache token mapping, missing provider behavior, streaming duplicate merge behavior, subagent parent session attribution, copied transcript dedupe, and malformed rows with no stable session identity.
- Schema tests should verify the schema version and embedded schema copy stay aligned with the schema source.
- CLI tests should verify `claude-code` is accepted anywhere harness filters are validated.
- Viewer behavior should be tested through existing aggregation/filter seams rather than adding Claude Code-specific viewer logic.
- Good tests assert external behavior: synced counts, stored rows, canonical totals, diagnostics, and CLI acceptance. They should not assert private helper implementation details.
- Existing OpenCode, Pi, and Codex conformance and adapter tests are prior art for the new Claude Code tests.

## Out of Scope

- Cursor support.
- Cursor API authentication, credential storage, export fetching, or tokscale cache ingestion.
- Claude Code cost tracking.
- Tool, request, TPS, duration, or agent metric domains.
- New viewer tabs.
- Storing prompt text, assistant text, tool arguments, tool output, raw transcript payloads, full source paths, request headers, secrets, or raw provider payloads.
- Supporting older Claude Code storage formats not identified as the current durable transcript format.
- In-place migration of older SQLite databases.
- Realtime plugins or checkpoint plugins.

## Further Notes

The schema version bump has already been accepted as the preferred compatibility behavior for expanding harness constraints. The decision is recorded in the ADR set because the logical change is additive, but existing SQLite tables physically reject the new harness value.

The glossary now clarifies the Durable Source boundary, the Claude Code Harness term, Parent Session Token Attribution, and Explicit Provider Attribution. Implementers should use those terms in docs and issue comments.
