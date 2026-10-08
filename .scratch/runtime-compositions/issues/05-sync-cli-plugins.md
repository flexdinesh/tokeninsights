# Background CLI contract and minimal plugin input

Status: complete
Dependencies: 04

Distributed sync defaults to background. --wait runs foreground until acceptance;
--debug is foreground compact terminal UI with stages/counters and receipt-specific
processing observation. Non-TTY uses plain progress. Debug requires read+ingest;
ordinary submit may use ingest-only. Cancellation preserves pending delivery.

--print submits and emits exactly canonical remote URL plus newline on stdout after
successful job startup. Job ID/errors/progress use stderr. Combined --wait/--debug
retain URL-only stdout; local --print rejects before work. Reject contradictory
options explicitly. sync status reads latest job state without networking/spawning.
Regular background success means started, not accepted/queryable.

Plugins pass only supported --harness value and explicit foreground --wait so their
existing subprocess completion/deadline supervision remains meaningful. Hooks may
launch that finite child asynchronously; retain bounded lifetime and sanitized error.
Where host callbacks await completion, ensure harness UI does not wait unnecessarily.
Coalesce concurrent same-harness events with a follow-up pass when events arrive after
scan start. No transcript/event JSON/native counter payload accepted in this stage.
Update OpenCode/Pi TypeScript and Codex/Claude hook assets together with generated
plugin bundles and CLI help. Never add TS any/assertions.

Tests: URL stdout plus actual submission, mode/flag errors, ingest-only vs debug
permissions, non-TTY fallback, status terminal errors, finite retry, plugin harness
selection/coalescing/follow-up/disposal and package build checks.

Implementation: implemented in this worktree. See PRD validation record.

## Unresolved questions

None.
