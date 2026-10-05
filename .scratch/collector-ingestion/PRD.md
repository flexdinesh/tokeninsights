# Collector ingestion: traceable architecture and tests

Status: Proposed. Test/fixture groundwork only; collector/server runtime and schema changes await contract review and explicit schema approval.

## Problem

Today's local service owns both source processing and queries. A future remote server cannot inspect producer files. Moving normalization to the host introduces a delivery boundary where replay, database loss, and partial failure must not duplicate or silently lose usage. Failures need reproducible evidence, not merely an unchanged grand total.

## Direction

The [architecture proposal](../../docs/collector-server-architecture.md) owns guarantees G01–G11. The [failure contract](../../docs/collector-ingestion-tests.md) owns F01–F14 and coverage status. Host SQLite retains metadata-only raw facts, canonical facts, parsing continuity, and a durable canonical change journal. Manual `sync` collects, normalizes, and publishes. Shared local/remote server core accepts canonical entities only; successful transactional ingestion is immediately queryable. Stable source-derived entity identity survives collector database deletion; batch identity survives delivery retries.

Local remains default. Plugins, remote deployment/setup, autonomous retries, retractions, and producer backflow are outside the initial scope. Views show available committed facts without promising complete historical collection.

## Deliverables for this groundwork

- Proposed responsibility/process changes and explicit persistence boundaries.
- Independent adapter identity audit against actual source formats and current code.
- Synthetic raw harness fixtures committed with hand-reviewed expected canonical facts and totals.
- Rebuild/replay/incremental pipeline regressions where existing interfaces support them.
- Canonical publication/failure examples clearly labeled illustrative, not an approved production wire schema.
- Failure matrix distinguishing runnable current tests, known gaps, and future ingestion tests.
- Reviewable unresolved contract questions and implementation gates.

No source transcript copies, real user paths, table changes, generated transport-contract changes, or claims that the proposed endpoint already exists.

## Acceptance evidence

| Requirement | Evidence needed |
| --- | --- |
| Database-independent identity | Compare explicit identity sets and expected values across fresh databases, full reparses, and supported incremental refresh |
| No accidental collapse | Distinct requests with equal counters remain separate facts |
| No replay amplification | Repeated collection yields equal fact sets/totals; future server replay retains the same query results |
| Safe local continuity | Fault/rewrite fixtures prove failed capture cannot skip source data |
| Safe publication | Future real persistence tests reopen databases after faults and verify journal/progress/receipts |
| Atomic server ingestion | Future failed-batch tests prove no partial facts, references, receipt, or publication revision |
| Debuggable failures | Fixture ID + Fxx + Gxx references, safe error reason, and before/after state |
| Privacy | Synthetic metadata only; excluded content never enters canonical publication |

Expected outputs are reviewed independently of parser output. Never regenerate expectations merely to make tests green. A failing semantic test can document a real identity/accounting defect; keep its status explicit. A design matrix or transport stub is not server-ingestion verification.

## Implementation sequence

1. Land fixture/test evidence and architecture proposal. Record existing defects and unsupported cases.
2. Resolve identity rules, publication entities, changed-fact behavior, receipts, limits, and compatibility. Request approval for concrete proposed SQLite and wire contracts.
3. Implement collector publication storage with atomic normalization/journal writes and crash tests.
4. Implement server canonical validation/dedupe and atomic receipts with real database integration tests.
5. Compose local default workflow and API viewers; migrate existing compatible data explicitly.
6. Add remote foreground composition and, later, thin harness plugins using the same tested path.

For code changes, run root format and lint, focused tests, full tests, then build. Retain direct Go build/runtime checks when composition or assets change. Record actual commands/results separately from planned coverage.

## Unresolved questions

- Harness fallback identity and namespace?
- Published entities and payload equality?
- Changed-value precedence and conflict cursors?
- Compatibility/identity migration rules?
- Limits, DB paths, viewer transition, timezone?
