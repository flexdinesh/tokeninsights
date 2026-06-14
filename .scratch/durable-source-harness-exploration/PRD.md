# Durable Source Harness Exploration PRD

## Problem Statement

TokenInsights has a sync-first pipeline, canonical normalization, and adapters for OpenCode, Pi, and Codex, but the current adapter implementation is still a generic JSONL/NDJSON scaffold. It does not yet prove that `tokeninsights sync` is making the most of each harness's real Durable Sources.

The next work should explore each harness one at a time, document the local data shape, and define implementation-ready parser requirements without overloading a single session with every harness format.

## Goals

- Identify the best Durable Sources for OpenCode, Pi, and Codex.
- Document how each source represents token usage, session identity, message or turn identity, provider, model, timing, and dedupe-relevant metadata.
- Preserve TokenInsights invariants: metadata-only raw storage, session-centric canonical facts, missing provider/model as `unknown`, idempotent sync, and diagnostics for unavailable or rejected data.
- Produce one implementation-ready issue document per harness.
- Use TokScale as an inspiration and lead source, not as the acceptance oracle.

## Non-Goals

- Do not implement parser changes as part of exploration.
- Do not modify the SQLite schema during exploration.
- Do not ingest or store prompt text, assistant text, tool arguments, tool output, request headers, secrets, raw provider payloads, or full source paths.
- Do not add realtime or checkpoint plugin behavior.
- Do not add cloud sync, account APIs, billing APIs, quota APIs, or cost tracking.

## Source Leads

TokScale identifies likely Durable Sources for the harnesses in scope:

- OpenCode: modern SQLite database under `~/.local/share/opencode/`, including channel database variants such as `opencode.db` and `opencode-stable.db`.
- Pi: JSONL session data under `~/.pi/agent/sessions/`.
- Codex: JSONL session data under `~/.codex/sessions/`.

These paths are starting points. Each exploration issue must verify the source shape directly and document any version or channel differences it discovers.

## Explicit Scope Boundaries

OpenCode exploration targets modern SQLite databases only. Legacy OpenCode JSON message storage under `~/.local/share/opencode/storage/message/` is out of scope.

Pi exploration targets only Pi under `~/.pi/agent/sessions/`. Oh My Pi data under `~/.omp/agent/sessions/` is out of scope.

Codex exploration targets local session logs only. Codex auth files, account stores, keychain data, remote usage/quota APIs, and token refresh flows are out of scope.

## Evidence Packet

Each harness exploration issue must produce a read-only evidence packet before implementation begins:

- Durable Source locations and discovery rules.
- Source kind and format, such as SQLite tables or JSONL files.
- Redacted table schemas or minimal record shapes.
- Token fields and token semantics.
- Stable session identity strategy.
- Stable message, turn, or fact identity strategy when available.
- Provider and model extraction rules.
- Timestamp and timing extraction rules.
- Dedupe risks, including copied history, forks, resumed sessions, or cumulative counters.
- Privacy review naming fields that must never enter raw storage.
- Diagnostics to emit when expected data is missing, unsupported, rejected, or ambiguous.
- Proposed source fixtures and expected raw, canonical, and diagnostic outputs.

Evidence packets must not include private transcript content. If source examples are needed, use redacted minimal records containing only metadata fields required to prove parsing behavior.

## Exploration Method

1. Load this PRD, `docs/design.md`, the existing sync-first PRD, and the current adapter implementation.
2. Load only the harness-specific issue document being worked.
3. Inspect TokScale's matching parser as a lead source for source locations and edge cases.
4. Inspect the harness's local data shape read-only.
5. Map source fields to TokenInsights `RawTokenFact` and normalization expectations.
6. Identify any data that cannot be represented safely under the current schema.
7. Write or update the harness evidence packet and fixture proposal.
8. Stop before parser or schema implementation unless a follow-up implementation issue explicitly begins.

## Acceptance Criteria

- Each harness has a separate issue document that can be executed in its own session.
- Each issue starts from a narrow Durable Source scope.
- Each issue requires a read-only evidence packet.
- Each issue requires fixture-ready examples before parser implementation.
- OpenCode excludes legacy JSON message storage.
- Pi excludes Oh My Pi.
- Codex requires stateful token interpretation analysis.
- Any schema limitation is documented as a follow-up proposal, not silently changed.
