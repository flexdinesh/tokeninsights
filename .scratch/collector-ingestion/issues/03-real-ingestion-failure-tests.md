# Execute publication failure traces against real ingestion

Status: ready-for-agent
Implementation: Production-path failure tests implemented; repository verification passed.

## Dependencies

[Stable identities](01-stable-harness-identities.md); [approved storage/wire contract](02-storage-and-wire-contract.md).

## Acceptance

- Implement one canonical ingestion operation shared by local/remote composition.
- Run protocol failure traces against actual SQLite transactions and HTTP delivery.
- Inject failures before/after normalization journal commit, immutable batch persistence, server commit, response delivery, and cursor advancement.
- Reopen databases after failures; check exact identities, components, references, receipts, cursor state, and totals.
- Cover same-batch replay, new-batch recollection, fresh publisher streams, concurrent replay, whole-batch rollback, incompatible input, and privacy.
- Preserve server history when a rebuilt collector has fewer retained source facts.
- Keep every guarantee mapped to a runnable test and stable failure ID.

## Unresolved questions

None blocking. Twelve deterministic subprocess kill barriers cover capture through acknowledgement.

## Comments

Real file-backed SQLite/HTTP tests now cover native fixture reconstruction, 100 reparses, exact lost-response retries, concurrent duplicates, whole-batch failures, privacy/numeric/version rejection, source absence and independent destinations. Process-kill tests inspect durable intermediate state at twelve named barriers, reopen databases without deleting WAL, and resume actual collector delivery.

Focused suites passed during implementation. Full PR gates remain recorded separately in [VALIDATION.md](../VALIDATION.md). Candidate trace syntax/arithmetic checks do not replace executable production-path assertions. Controlled mutation experiments and deferred multi-tenant/real-host behaviors are not claimed.
