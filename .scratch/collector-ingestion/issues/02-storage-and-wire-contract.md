# Approve collector/server storage and canonical publication

Status: ready-for-agent
Implementation: Approved contract implemented; repository verification passed.

## Problem

Collector/server separation requires durable canonical publication and replay identity. New SQLite structures and cross-language storage contracts need explicit approval under AGENTS.md.

## Acceptance

- Specify local raw/canonical retention, publication journal, immutable batches, and per-destination delivery cursors.
- Specify stable public entity IDs independently of local row IDs and publisher streams.
- Specify server canonical records, batch replay identity, references, and compatible/incompatible schema handling.
- Show transaction boundaries and fresh role-checked database transition; preserve legacy storage.
- Explain changes and obtain explicit approval before modifying schema files or structures.
- A committed ingestion receipt means queryable canonical data.

## Unresolved questions

None blocking. Contract approved; user selected fresh databases and retained-source reconstruction.

## Comments

Explicit user approval resolved the contract gate. Collector journal/batches/cursors, canonical-only server schema, typed publication codec and REST contract are implemented. Candidate protocol traces remain design fixtures; schemas and OpenAPI own the actual contract. Unsupported changed payloads reject atomically; Claude revisions use native source timestamps. Legacy database is untouched, with no import or alias bridge.
