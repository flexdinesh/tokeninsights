# Approve collector/server storage and canonical publication

Status: needs-info

## Problem

Collector/server separation requires durable canonical publication and replay identity. New SQLite structures and cross-language storage contracts need explicit approval under AGENTS.md.

## Acceptance

- Specify local raw/canonical retention, publication journal, immutable batches, and per-destination delivery cursors.
- Specify stable public entity IDs independently of local row IDs and publisher streams.
- Specify server canonical records, batch replay identity, references, and compatible/incompatible schema handling.
- Show transaction boundaries and schema-preserving migration behavior.
- Explain changes and obtain explicit approval before modifying schema files or structures.
- A committed ingestion receipt means queryable canonical data.

## Unresolved questions

- Exact tables/columns and wire fields?
- Changed-payload conflict response?
- Schema compatibility and migration scope?

## Comments

No schema or OpenAPI changes are authorized by this issue alone. Candidate protocol traces are design fixtures, not a generated wire contract.
