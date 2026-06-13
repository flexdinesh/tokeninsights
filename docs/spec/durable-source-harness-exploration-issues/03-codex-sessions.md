# Explore Codex Session Durable Sources

Type: HITL

Blocked by: None

Status: Not started

## What to explore

Research Codex's local session Durable Sources and define parser requirements for `tokeninsights sync --harness codex`.

TokScale lead: Codex uses JSONL session data under `~/.codex/sessions/`.

Codex auth files, account stores, keychain data, remote usage/quota APIs, and token refresh flows are out of scope.

## Evidence packet

- [ ] List the Codex session roots and discovery rules to support.
- [ ] Document session file naming patterns without storing full paths.
- [ ] Document relevant JSONL event types using redacted minimal examples.
- [ ] Document where token usage lives, including `last_token_usage` and `total_token_usage` if present.
- [ ] Document stateful model and provider extraction.
- [ ] Document session identity and turn/message/fact identity.
- [ ] Document which events are countable and which are context, replay, or non-human input.
- [ ] Document fork, replay, resumed-session, cumulative-counter, and headless `exec` behavior.
- [ ] Identify stale snapshot, counter regression, and double-counting diagnostics.
- [ ] Identify private fields that must never enter raw storage.
- [ ] Propose minimal JSONL fixtures and expected raw/canonical/diagnostic outputs.

## Acceptance criteria

- [ ] The issue proves a metadata-only mapping from Codex JSONL events to `RawTokenFact`.
- [ ] The issue defines a stateful token interpretation model before implementation.
- [ ] The issue defines stable source IDs without storing full source paths.
- [ ] The issue defines stable raw and canonical semantic keys that do not depend on import order.
- [ ] The issue defines diagnostics for ambiguous state transitions or unsupported event patterns.
- [ ] The issue confirms whether current schema is sufficient or names follow-up schema work.
- [ ] No parser implementation or schema change is performed as part of exploration.

