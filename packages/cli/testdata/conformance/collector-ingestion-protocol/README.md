# Candidate canonical ingestion failure fixtures

Status: planned behavioral traces. There is no canonical ingestion endpoint yet. These files are synthetic review inputs and golden outcomes, not executable ingestion coverage or an approved cross-language/schema contract.

See [failure matrix](../../../../../docs/collector-ingestion-tests.md) for Fxx/Gxx ownership, existing tests, and implementation gates.

- `canonical-inputs.json`: independently specified canonical facts and deliberate invalid variants.
- `F01`–`F14`: operations, faults, and expected durable outcomes. Each trace is independent unless it explicitly states a prior step.
- Identity labels (`usage-A`, `session-A`) represent future source-derived IDs. No hash algorithm, production ID, revision format, request field, response code, table, or limit is established here.
- Owner labels are trusted test configuration; submitted owner claims do not establish authority.
- Snapshot totals sum input, output, reasoning, cache read, and cache write tokens. Two equal-valued facts from different requests must both count.
- Revision evidence labels are assumptions exercised by candidate tests. F04/F06 need an approved precedence policy before implementation.

A future production test should resolve input labels to fixture payloads, drive the real collector/HTTP server, inject the specified fault, reopen storage, and compare exact identities, payloads, receipts, and REST totals. Loading JSON or writing a matching fake state machine does not prove these guarantees.

All values are hand-authored and synthetic. Prohibited-field variants use harmless markers solely to verify rejection without persistence or body logging. Never replace them with real content. Decimal strings in counter-edge variants express exact large integers for review; they do not specify the eventual wire encoding.
