# Explore OpenCode Modern SQLite Durable Sources

Type: HITL

Blocked by: None

Status: Not started

## What to explore

Research OpenCode's modern SQLite Durable Sources and define the parser requirements needed for `tokeninsights sync --harness opencode` to ingest durable token usage safely and idempotently.

TokScale lead: OpenCode uses SQLite databases under `~/.local/share/opencode/`, including `opencode.db` and channel variants such as `opencode-stable.db`.

Legacy OpenCode JSON message storage under `~/.local/share/opencode/storage/message/` is explicitly out of scope.

## Evidence packet

- [ ] List the OpenCode database file names and discovery rules to support.
- [ ] Document the relevant table schemas and joins, using redacted schema output only.
- [ ] Document where assistant token usage lives.
- [ ] Document session identity and message/fact identity.
- [ ] Document provider and model extraction.
- [ ] Document timestamp and optional duration extraction.
- [ ] Identify dedupe risks, including forked or copied history rows.
- [ ] Identify unavailable or ambiguous data that should become diagnostics.
- [ ] Identify private fields or JSON paths that must never enter raw storage.
- [ ] Propose minimal SQLite fixture data and expected raw/canonical/diagnostic outputs.

## Acceptance criteria

- [ ] The issue proves a metadata-only mapping from OpenCode SQLite rows to `RawTokenFact`.
- [ ] The issue defines stable source IDs without storing full database paths.
- [ ] The issue defines stable raw and canonical semantic keys that do not depend on import order.
- [ ] The issue defines how repeated syncs avoid duplicate token facts.
- [ ] The issue confirms whether current schema is sufficient or names follow-up schema work.
- [ ] No parser implementation or schema change is performed as part of exploration.

