# Attribute Claude Code subagent and duplicate transcript token usage safely

Type: AFK

Status: ready-for-agent

## Parent

Claude Code Harness Support PRD

## What to build

Extend Claude Code sync so subagent transcript token usage is attributed to the parent session when Claude Code provides parent identity, and copied duplicate transcript facts are suppressed by logical token fact identity. This slice completes the Claude Code session-centric behavior needed for realistic local transcript trees.

The implementation should continue to capture only token data and metadata required by current TokenInsights workflows.

## Acceptance criteria

- [ ] Nested Claude Code subagent JSONL files are discovered under the modern projects transcript layout.
- [ ] Subagent token facts use the parent session identity when the JSONL line provides it.
- [ ] Subagent token facts fall back to the subagent file stem only when parent identity is unavailable.
- [ ] Token rows with no stable session identity are skipped at parse time with a diagnostic instead of being inserted as null-session raw facts.
- [ ] Subagent tokens appear in canonical viewer totals under the parent Claude Code session.
- [ ] Subagent token usage does not inflate session counts when parent identity is available.
- [ ] Duplicate copied transcript facts are suppressed by a logical dedupe key that does not include full source paths.
- [ ] Duplicate suppression emits a constrained diagnostic without private content.
- [ ] Repeated sync runs do not duplicate raw token facts or canonical token facts.
- [ ] Fixture-style conformance tests cover parent-session attribution, fallback behavior, skipped unstable-session rows, copied transcript suppression, and repeat-sync idempotence.
- [ ] README and design documentation describe Parent Session Token Attribution for Claude Code.

## Blocked by

- Issue 2
