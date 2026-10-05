# Collector ingestion: traceable architecture and tests

Status: Runtime implementation complete in PR #53; repository verification passed.

## Problem

A remote server cannot inspect producer artifacts. Moving parsing and normalization to the host creates a delivery boundary where database deletion, replay, and partial failure must preserve accurate usage. Failures require reproducible fixtures and transaction evidence.

## Approved direction

The [architecture](../../docs/collector-server-architecture.md) owns G01–G11; the [failure matrix](../../docs/collector-ingestion-tests.md) owns F01–F14. The user approved implementation and chose fresh `collector.sqlite` and `server.sqlite`, rebuilt from retained sources. Existing `tokeninsights.sqlite` remains untouched; no legacy canonical import or identity alias bridge is implemented.

Collector SQLite owns metadata-only raw facts, canonical facts, source continuity, an atomic publication journal, immutable batches, and destination progress. Manual `sync` defaults to all harnesses and collects, normalizes, then publishes. `--publish-only` delivers retained normalized work. Source failures still permit publication of previously committed journal work, while the command reports failure separately.

One canonical-only ingestion/query core serves local and remote compositions. Stable native identities survive collector deletion; delivery stream/batch IDs do not identify facts. Receipts acknowledge committed queryable state. Equal payloads are no-ops; Claude source-timed revisions update one contribution; unsupported changed values conflict. Ambiguous native identity stays local with publication diagnostics.

Local remains default. `service` manages the local server, `sync` collects and publishes, and `tui` queries committed data through REST without implicit collection. Advanced normalize/reset operations belong under `collector`; former names remain deprecated aliases. Explicit remote URL skips local startup. Browser Reload never triggers a producer. Thin completion adapters invoke the same finite `sync` command. Remote provisioning, multi-tenant authorization, autonomous retry, retention, retractions, and backflow remain future work.

## Implemented deliverables

- Separate role-checked collector/server schemas, independent versions and fresh defaults.
- Native identity fixes, synthetic raw harness fixtures, independently reviewed 12-fact/1102-token oracle.
- Atomic normalization+journal writes, immutable batches, per-destination cursors and validated receipts.
- Atomic server fact/reference/receipt commits, bounded admission, strict privacy/numeric/version validation.
- Manual CLI workflow, host-only normalization/resets, REST-backed TUI and browser Reload.
- Production SQLite/HTTP tests for dedupe, reconstruction, conflicts, lost responses, destination isolation and process kills at 12 barriers.
- Native plugin packaging and adapter tests tracked in [plugin plan](PLUGIN-PLAN.md); host installation remains deferred.

## Acceptance evidence

| Requirement | Executable evidence |
| --- | --- |
| Database-independent identity | CFI001–009, publication identity vectors and collector reconstruction tests |
| Distinct equal-valued requests | OpenCode CFI007 and server fresh-stream contract tests |
| Streaming replacement | Claude CFI008, source-revision ordering and conflict tests |
| No replay amplification | 100 full reparses, fresh collector streams, immutable request replay and concurrent server ingestion |
| Durable publication | Collector-store rollback/ack tests and 12 subprocess kill barriers |
| Atomic server ingestion | Real SQLite trigger failures, whole-batch validation and receipt assertions |
| History preservation | Source absence/subset uploads, collector resets, local replacement and role rejection |
| Privacy | Metadata-only source fixtures, strict envelope decoder and publication sentinel checks |

Tests assert identities, every token component, references, receipts and progress; total-only assertions are insufficient. Candidate JSON traces remain design inputs rather than the authoritative wire contract. Actual commands/results belong in [VALIDATION.md](VALIDATION.md); implementation presence does not imply all repository gates passed.

## Remaining work

Finish integration review, schema/API/generated-asset checks, formatting/lint, full tests, native build/runtime, race and browser gates. Keep completed implementation and verified evidence distinct. Plugin installation/trust and source-flush behavior need later isolated real-host verification; no user harness configuration is edited automatically.

## Unresolved questions

None required for the approved local implementation. Future: remote provisioning/authentication, retention and custom source namespaces.
