# Collector/server implementation gates

Status: Foundation complete; two regression tests red; runtime and schema migration pending.

See [PRD](PRD.md), [architecture proposal](../../docs/collector-server-architecture.md), [failure matrix](../../docs/collector-ingestion-tests.md), and [identity audit](IDENTITY-AUDIT.md).

## Gate 1: Prove source identities

Run synthetic fixtures through the real adapters. Compare exact logical identities, timestamps, attribution, token components, countability, and totals after repeated sync and fresh-DB reconstruction. Distinguish identical-source guarantees from changed-source scenarios. Preserve legitimate equal-token requests; suppress only proven copies.

Do not accept aggregate totals alone, generated goldens, skipped regression tests, or a simulated ingestion engine as evidence of production correctness. Genuine failures stay explicit until fixed with reviewed semantics. Identity/accounting fixes must move tests, README, and design together; storage changes require explicit approval.

## Gate 2: Approve storage and publication contract

Specify collector journal/cursors, server canonical entity identity, transactional batch receipts, and schema compatibility/migration. Review changed-payload and missing-native-ID policy. Obtain explicit approval before changing SQLite structures or cross-language schema contracts.

The canonical envelope must exclude local raw facts, transcript content, secrets, and source paths. Manual sync is the trigger. No retraction or reverse synchronization system is required.

## Gate 3: Implement durable collection/publication

Commit raw facts and source continuity safely; commit canonical publication changes with normalization. Save immutable batches before delivery; advance a destination cursor only after a committed receipt. Inject failures at each boundary, reopen actual databases, and verify retained work.

## Gate 4: Implement canonical server ingestion

Use one ingestion operation for local and remote compositions. Validate before committing; atomically apply canonical records and retry identity. A receipt means query-visible data. Run candidate failure traces against real SQLite/HTTP code, including fresh publisher streams, lost responses, concurrent replay, reference integrity, and incompatible schemas.

Bound admission and transactions. Keep parsing and harness source inspection in the collector. Do not substitute mock-only tests for storage crash/reopen coverage.

## Gate 5: Switch clients and CLI

Route manual sync through local collection and canonical publication; preserve useful per-stage failure reporting. Define viewer/refresh behavior and transport migration explicitly. Update CLI docs, API contracts, generated output, dashboard tests, and design together.

## Completion criteria

- Every guarantee and failure ID maps to runnable tests or an explicit pending implementation issue.
- Independent expected IDs/components/totals remain stable across reruns and fresh collector DBs.
- Server retries and recollection never add duplicate contributions.
- Failed transactions and lost responses do not lose queued facts or falsely advance delivery progress.
- Format, lint, focused tests, full tests, schema/API checks, and build results are recorded in [validation evidence](VALIDATION.md).

## Unresolved questions

- Missing-native-ID policy?
- Changed-payload precedence?
- Exact journal/receipt schema and migration?
- Viewer transport/refresh migration?
