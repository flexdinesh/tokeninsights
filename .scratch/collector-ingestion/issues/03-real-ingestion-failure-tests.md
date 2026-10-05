# Execute publication failure traces against real ingestion

Status: needs-info

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

- Fault-injection seams after approved contract?

## Comments

Protocol traces currently specify future behavior. Fixture syntax/arithmetic checks do not count as production ingestion verification.
