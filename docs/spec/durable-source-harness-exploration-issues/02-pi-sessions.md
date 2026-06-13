# Explore Pi Session Durable Sources

Type: HITL

Blocked by: None

Status: Not started

## What to explore

Research Pi's local session Durable Sources and define parser requirements for `tokeninsights sync --harness pi`.

TokScale lead: Pi uses JSONL session data under `~/.pi/agent/sessions/`.

Oh My Pi data under `~/.omp/agent/sessions/` is explicitly out of scope.

## Evidence packet

- [ ] List the Pi session root and discovery rules to support.
- [ ] Document session directory and file naming patterns without storing full paths.
- [ ] Document the JSONL header shape and message entry shape using redacted minimal examples.
- [ ] Document where assistant token usage lives.
- [ ] Document session identity and message/fact identity.
- [ ] Document provider and model extraction.
- [ ] Document timestamp fallback behavior when entry timestamps are missing or invalid.
- [ ] Identify malformed-line and missing-field diagnostics.
- [ ] Identify private fields that must never enter raw storage.
- [ ] Propose minimal JSONL fixtures and expected raw/canonical/diagnostic outputs.

## Acceptance criteria

- [ ] The issue proves a metadata-only mapping from Pi JSONL entries to `RawTokenFact`.
- [ ] The issue defines stable source IDs without storing full source paths.
- [ ] The issue defines stable raw and canonical semantic keys that do not depend on import order.
- [ ] The issue defines whether missing provider/model should preserve null in raw and normalize to `unknown`.
- [ ] The issue confirms whether current schema is sufficient or names follow-up schema work.
- [ ] No parser implementation or schema change is performed as part of exploration.

