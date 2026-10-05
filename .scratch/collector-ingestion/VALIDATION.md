# Collector/ingestion validation

Date: 5 October 2026. Base: `e5ec2c7`. Branch: `codex/collector-ingestion-contract`.

Worktree: `/home/dee/workspace/tokeninsights/wt-codex-collector-ingestion-contract`.

Current status: parser regressions fixed; root format/lint/full tests/build pass. Storage/API approval, server ingestion, migration, and viewer wiring remain pending. Query-client evidence is recorded after its completion below. Foundation red results are retained as historical evidence.

## Scope

The foundation used four independent agents for architecture/PRD, harness fixtures/tests, failure traces, and identity audit. At that stage production Go, SQLite schema, and generated API contracts were unchanged. Subsequent parser/plugin/client implementation is recorded below. Collector/server ingestion remains unimplemented.

## Historical foundation evidence

Root ran `pnpm run format` and `pnpm run lint`: pass.

Root ran from `packages/cli`:

```sh
go test ./internal/pipeline -run TestCollectorRebuild -count=1 -v
```

Result: seven pass, two fail; approximately 1.44 seconds. Passing cases verify 12 independently specified canonical facts / 1102 tokens, exact components/native references, 100 forced full reparses, fresh databases at different clocks, raw-only renormalization without sources, relocated/copied artifacts, and missing-ID raw retention/diagnostics.

| Failure | Actual | Expected | Meaning |
| --- | --- | --- | --- |
| CFI-007 OpenCode equal-time native requests | 2 canonical facts | 3 canonical facts / 276 tokens | Equal-value copy heuristic suppresses an independent native request |
| CFI-008 Claude partial then completed sync | 2 canonical facts | 1 completed fact / 120 tokens | Timestamp-dependent canonical key retains both partial and completed contribution |

At foundation publication these tests remained unskipped and red, as permitted by AGENTS.md. Their accounting/identity fixes now pass; expected outputs were not changed to match these defects.

Independent review rejected an initial CFI-009 requirement for two canonical Pi facts without native message IDs. Different counters alone cannot establish independent requests versus revisions. The adversarial source remains; the executable test verifies raw preservation and missing-ID diagnostics. Canonical fallback policy remains unresolved and is not claimed tested.

## Historical foundation full verification

`pnpm run test`: fails on CFI-007/008 only. Build-tools typecheck and all 12 build-tools tests pass; web typecheck and all 42 browser unit tests pass; all other Go packages pass. The pipeline suite reports exactly the two documented regression failures. This branch is not a green-suite implementation signoff.

`pnpm run build`: passes schema-copy consistency, API contract consistency, frontend build, embedded asset staging, and Go build. Committed schema/API/assets are unchanged. Build was verified independently of the intentionally red regression suite.

Direct native build from `packages/cli`:

```sh
go build -o bin/tokeninsights-native ./cmd/tokeninsights
```

Runtime smoke check from repository root:

```sh
env PATH=/nonexistent ./packages/cli/bin/tokeninsights-native --help
```

Both pass. No Node/npm/pnpm exists on that runtime PATH. Help smoke test only; no live service/TUI or production database was used.

Focused fixture privacy suite (`node --test test/fixture-safety.test.ts` from `tools/build`): all five tests pass, including the two new rebuild source/stage guards. Root format/lint passed after the final safety-test edit. `git diff --check` passes.

## Candidate server evidence

F01–F14 are hand-authored planned failure traces. They cover replay, rebuilt streams, response loss, concurrency, references, compatibility, invalid counters, privacy, source absence, admission failure, and twelve persistence crash seams. They must later run against real collector/SQLite/HTTP code. JSON parsing, arithmetic checks, and current local pipeline tests do not prove these server guarantees.

Root validated all 17 new JSON files, 14 trace IDs/statuses, 12 golden harness facts with component sums, and local links across 13 Markdown files. Independently validated all 52 protocol snapshots for unique logical fact identities, fact counts, component sums, and totals. These are fixture-integrity checks, not executable server coverage.

## Remaining gates

- Resolve weak identity and changed-payload policies.
- Approve concrete storage/wire changes before schema implementation.
- Implement journal/receipts with real fault-injection tests.
- Preserve each CFI/F/G mapping through subsequent iterations.

## PR publication checks

`mise run check:push` passes formatting, lint, schema/API checks, build-tools tests, and web unit tests, then stops on CFI-007/008. Ran the remaining checks separately: `pnpm run check-web`, native CLI build, and all 19 browser E2E tests pass. `pnpm run test:race` reports the same two regression failures; no data races were reported.

Publishing this intentionally red foundation is explicitly requested after the failures were reported. AGENTS.md permits failing tests that expose genuine bugs. The push uses invocation-scoped `HUSKY=0` to bypass the known failing pre-push hook; hooks and tests remain unchanged. No failed assertion is removed or skipped. Trimmed trailing blank lines in candidate trace files before staging; JSON payloads unchanged.

## Implementation kickoff: parser fixes and parallel preparation

CFI001–009 now pass, including CFI007's three OpenCode facts / 276 tokens and CFI008's one completed Claude fact / 120 tokens. The full pipeline suite passes. New adjacent identity tests cover native request/session scoping, delimiter-safe hashing, decreasing snapshot components, reversed records, stale copied artifacts, retained-raw replay, and equal-time conflicts preserving saved facts.

Root `pnpm run format`, `pnpm run lint`, and `pnpm run test` pass after parser/plugin groundwork: 16 build-tool tests, 42 web tests, and all Go packages. This run preceded the new query-client implementation; verify it separately after that agent finishes. Four plugin tests prove isolated shell invocation and event routing; native host installation/teardown/flush remain pending. No schema/constants or generated API changes. No actual server ingestion coverage is claimed.

Execution, storage, failure, and plugin plans now specify ownership/dependencies and real persistence gates. Explicit storage/API approval was requested; migration and runtime contract edits remain pending. Parser fixes must not be released against legacy identities without the approved compatibility path.

The completed `internal/queryclient` suite exercises the existing real REST handler with 225 synthetic sessions over multiple pages, verifies all token/context components and summary/facets, and tests revision/instance/epoch churn, cancellation, inconsistent pagination, duplicate row identities, unsafe URLs, redirect rejection, and bounded/sanitized responses. It is reusable client code; TUI wiring is still pending.

Final kickoff `mise run check:push` passes: format/lint, unchanged schema/API checks, all unit tests including queryclient, full Go race suite (pipeline 101.9s; no races), embedded asset comparison, native build, and all 19 browser E2E tests (40.5s). Plugin artifacts are now included in root format/lint targets. Direct Go build and `env PATH=/nonexistent ./packages/cli/bin/tokeninsights-native --help` pass after these changes. Schema/constants/generated API/embedded assets remain unchanged. No hook bypass is needed for this green kickoff.
