# Collector/ingestion validation

Date: 5 October 2026. Base: `e5ec2c7`. Branch: `codex/collector-ingestion-contract`.

Worktree: `/home/dee/workspace/tokeninsights/wt-codex-collector-ingestion-contract`.

Current status: review hardening R01–R08 complete; schema 16/2 and strict response contracts approved. Full repository gates pass. User has not run the PR and approved rebuilding all prior PR data. Historical results below remain evidence for their recorded heads.

## Scope

The foundation used four independent agents for architecture/PRD, harness fixtures/tests, failure traces, and identity audit. At that stage production Go, SQLite schema, and generated API contracts were unchanged. Subsequent parser/plugin/client implementation is recorded below. Collector/server ingestion is implemented in the current work; historical results are separated below.

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

Independent review rejected an initial CFI-009 requirement for two canonical Pi facts without native message IDs. Different counters alone cannot establish independent requests versus revisions. The adversarial source remains; the executable test verifies raw preservation and missing-ID diagnostics. Current publication quarantines missing native identity; historical raw retention evidence remains valid.

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

## Completed collector/server implementation

User approved the storage/wire split and selected fresh databases. Collector V15/data generation 6 and canonical-only server V1 use distinct application IDs. Legacy tokeninsights.sqlite is preserved; no legacy import or identity alias migration.

Parallel agents implemented publication, both stores, ingestion, local/remote composition, CLI, REST TUI, browser Reload, plugins, failure tests, and docs. Independent reviews found and fixed local delimiter collisions, filename-derived session publication, initialization crash windows, non-loopback auth bypass, local-view destination binding, stale facets, missing repo facet parameters, historical timezone formatting, optional location NULL semantics, and plugin descendant cleanup. Regressions execute production code.

Root verification:
- pnpm run format and pnpm run lint pass.
- pnpm run test passes plugin SDK/artifact checks, build-tools, 31 web unit tests, and every Go package.
- pnpm run build passes both schema contracts, OpenAPI generation consistency, committed embedded web assets, native Go build, and reproducible plugin artifacts.
- mise run check:push passes full Go race testing (pipeline 246.2s), schema/API/asset checks, native build, and all 19 browser E2E tests (38.5s).
- Focused plugin executable tests: 12 pass, including collector descendants surviving SIGTERM.
- Darwin amd64/arm64 serverstore cross-builds pass. Atomic publication tests preserve existing targets and SIGKILL a post-publication child; the resulting server database has one link and restarts.

Failure evidence: F01–F14 use real SQLite/HTTP and independently authored oracles. The native fixture has exactly 12 facts and token components [800,148,52,96,6], totaling 1,102. It survives 100 forced syncs and collector reconstruction. Twelve capture-to-acknowledgement barriers kill real subprocesses and then reopen/resume both roles. Boundary tests accept 1 MiB/256 entries/256-byte labels and reject +1. Unsafe counters, aggregate overflow, private fields, reference mismatch, duplicate/conflicting batches, independent owners/destinations, lost receipts, source absence, and SQLite busy/failure are explicit tests.

Native runtime verification from packages/cli:
```sh
go build -o bin/tokeninsights-native ./cmd/tokeninsights
```
The resulting binary runs with PATH=/nonexistent. A short isolated temporary XDG runtime directory avoids the documented Unix socket path limit. A production CLI smoke workflow verifies empty startup; eight fixture facts/896 tokens; collector database deletion and reconstruction with unchanged server totals; server-offline capture; manual retry adding one independently specified 10-token fact; repeated sync preserving nine facts/906 tokens. A legacy sentinel file remains byte-identical and the server contains no raw tables. The fixture service is stopped and temporary storage removed afterward.

Browser inspection used an isolated authenticated fixture and agent-browser. Desktop/mobile Reload controls and server timezone were checked; mobile document width equaled its 390px viewport. Native session labels, repo directory disclosure/focus, query recovery, pagination, filters, themes, and server identity are retained in E2E tests. The temporary browser/service was closed.

Normal pre-push verification stays enabled. Final publication reruns mandatory checks on the committed branch. No tests are skipped or weakened and no hook bypass is used for this implementation.

Remaining scope: remote provisioning/multi-tenancy; real harness installation/trust/event-flush smoke tests; future receipt/journal retention and new adapter revision policies. No physical media corruption or power-loss simulation is claimed. Unnamed system timezone fallback is an explicit fixed offset; historical DST requires an identifiable IANA reporting zone.

Final pre-push initially caught an asynchronous plugin descendant assertion: the test observed process state immediately after SIGKILL. The assertion now waits at most three seconds for disappearance or a non-running zombie, and cleans up on failure. Removing the production group SIGKILL still makes the test fail. All 27 build-tools tests then passed five consecutive runs with Git-local environment variables cleared; formatting, lint and typecheck pass. The normal push reruns the complete mandatory gate.

## CLI and location privacy follow-up

Canonical daily commands are `service`, `sync`, and `tui`; advanced normalization/reset commands are grouped under `collector`. Deprecated aliases remain. Tests verify both TUI names read saved REST data, grouped and legacy resets operate on actual collector databases, and help creates no storage.

Published directory/repository labels must be basenames on every host OS. Fourteen real HTTP cases reject Unix, drive, UNC, relative and home paths atomically without echoing or storing private labels; valid basename and optional SQL NULL behavior remain covered. OpenAPI, generated clients and embedded browser assets were regenerated together.

Root format, lint, full unit tests and build pass after this follow-up: 27 build-tools tests, 31 web tests, all Go packages, schema/API consistency and plugin artifacts. Direct Go build and the isolated native workflow pass again with JavaScript absent from PATH, including `tui`/`collector` help, database reconstruction and offline manual retry. Mandatory pre-push race/browser gates run again during publication.

## Removal of previous architecture compatibility

User requested removal rather than deprecation. Removed `view`, `serve`, top-level normalize/reset commands, `--db-path`, and `--no-sync`. Only grouped collector operations and role-specific path flags remain. Conventional help/version flags remain. Removed the unused source-aware application controller and pre-split schema recognition/metadata migration code; no schema/table structures changed.

`TestRemovedCommandsRejectWithoutStorageOrServiceSideEffects` and `TestRemovedFlagsRejectBeforeSideEffects` prove rejection before state creation or existing-file mutation. `TestLegacySchemasRejectWithoutMutation` covers versions 0–14, with either no role or the current collector role, across inspect/open/create/recovery/reset paths. Current-schema collector generation recovery/resume still passes; current sync, REST TUI and publication fault coverage remain active.

Root format, lint, full unit tests and build pass; focused CLI and DB race tests pass. Direct Go build/native smoke rejects twelve removed command/flag forms with usage exit 2 and no storage/service work, then verifies empty startup, collector deletion/replay and offline manual retry with unchanged facts and no JavaScript on PATH. Current docs and failure matrix were updated; historical ADRs and earlier evidence remain historical. Normal pre-push runs the full race/browser gates again.


## Review hardening R01–R08

User approved bounded canonical timestamps, required query response identities,
and collector/server schema versions 16/2. No earlier PR databases contain user
history; retained sources may rebuild fresh files. No compatibility aliases,
schema migration or receipt rewriting was introduced.

Four independent implementation lanes covered timestamp/storage validation,
harness normalization, service/tooling and query clients; an additional web lane
tightened consumers. Independent cross-reviews found no remaining actionable
introduced bugs. Exact test mappings are in docs/collector-ingestion-tests.md and
issues/04-review-hardening.md.

- R01: shared epoch-ms domain 0..253402214399999; validation before Claude merge,
  canonical envelopes and publication; matching SQL integer/range guards.
  Invalid raw time evidence remains inspectable. Actual ingestion/REST calendar
  buckets and native hourly queries pass across six isolated timezone processes.
  Valid prior collector15/server1 contracts reject with byte-identical files.
- R02/R03: weak Pi/Claude sessions raw-only; all 24 Claude record permutations,
  separate syncs and raw-only replay converge. Canonical projection avoids nil/0
  conflicts and repeated reasoning subtraction. Real native counter/attribution
  conflicts still roll back. Original CFI001 oracle remains unchanged.
- R04/R05: runtime credential comparison uses the owner-verified private socket,
  returns only a boolean and ignores edited persisted credentials. Real HTTP
  authorization rotates through restart while receipts remain exact. Verified
  stop works with missing/malformed storage; corrupt restart refuses unchanged;
  unknown/forged owners remain protected.
- R06/R08: required query envelopes distinguish missing revision from zero;
  unavailable metadata remains explicit. TUI generations discard old facets,
  status and snapshots; valid HTTP restart adopts the new instance with four
  GETs and no recursive reload. Web consumers reject invalid/stale tags and accept
  newer revisions. Initial facets wait for validated identity.
- R07: both real plugin builds/checks pass with spaces/percent/hash in workspace
  and temporary paths; formatter failure cleanup leaves no temporary artifacts.

Root verification passes: pnpm run format; pnpm run lint (0 issues);
pnpm run check-schema; pnpm run check-api; pnpm run test (30 build-tool, 50 web,
all Go suites); pnpm run build; mise run check:push (full Go race, embedded-asset
consistency, native build, all 19 browser E2E tests in 31.1s). The first full gate
exposed an E2E status mock missing required identity/readiness. That regression
now uses validated real server metadata while delaying an older revision.
Focused recovery test passes (7.5s), followed by the complete passing gate.
Go race pipeline ran 192.888s; no races reported.

Direct Go build from packages/cli and the built project binary --help both run
with PATH=/nonexistent. Isolated native CLI smoke passes empty startup, nine
removed command/flag rejections, the complete 12-fact/1102-token oracle, collector
deletion/recollection and repeated publication with unchanged full stable IDs,
payload hashes and server revision. Role versions are16/2 and the server has no
raw tables. Codex fixture message IDs are documented prefixes: the smoke checks
the prefix plus full 64-hex witness, then compares actual IDs across reconstruction.
An initial literal-prefix assertion was corrected in the temporary smoke harness;
repository fixtures/oracles were not altered. Fixture service/storage cleaned.

Remote provisioning, real harness install/trust/flush, retention and physical
media/power-loss simulation remain outside this PR. Normal push hooks stay enabled.
