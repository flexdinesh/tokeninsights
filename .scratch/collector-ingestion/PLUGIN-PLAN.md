# Completion plugin implementation

Status: Native manifests, lifecycle runners and standalone package artifacts implemented; package builds, SDK typechecks, reproducible artifact checks and repository gates pass. Isolated real-host installation remains deferred.
Scope: PR #53 collector/server ingestion. Manual sync remains primary.

## Responsibilities

The collector owns parsing, normalization, SQLite, publication and delivery. Thin adapters invoke `tokeninsights sync` directly and pass no hook payload, transcript path or conversation content. Fact/batch dedupe makes repeated collection converge; completion dispatch itself supplies no exactly-once guarantee. A later manual sync collects late retained records and retries durable publication.

## Implemented artifacts

- Codex/Claude native manifests and Stop hooks: finite synchronous command, quoted binary override, discarded stdin/output, empty decision JSON and a 60-second host deadline.
- Pi extension entry and package metadata: `agent_settled` awaits collection; `session_shutdown` closes active runner. SDK declarations pinned to `@earendil-works/pi-coding-agent` 1.0.0.
- OpenCode V2 plugin entry/root loader: abortable event subscription selects idle `session.status`; cleanup aborts subscription and child. SDK declarations pinned to `@opencode/plugin` 2.0.22.
- Shared bounded spawn runner bundled into committed standalone package output. Argument arrays and `shell: false`; overlapping callbacks coalesce; deadline/cleanup terminate the child/process group with bounded escalation. No retry daemon or durable trigger scheduler.

Pinned declarations and buildable artifacts establish an implementation target, not verified universal host compatibility. No user plugin registration is installed or modified automatically. Go production code never invokes JavaScript tooling.

## Traceable test requirements

| ID | Invariant | Verification |
| --- | --- | --- |
| PL01 | Stop invokes shared sync only | Codex/Claude isolated executable tests |
| PL02 | Hook content/output never crosses into collection arguments/stdin/logs | Synthetic private sentinels and discarded streams |
| PL03 | Literal executable paths preserve argument boundaries | Paths with spaces/metacharacters and PATH fallback |
| PL04 | Missing/nonzero executable never requests continuation | Empty decision JSON and fixed failure marker |
| PL05 | Pi settled handler waits; shutdown closes work | Native lifecycle entry with fake runner/executable |
| PL06 | OpenCode idle only; unrelated/malformed events ignored | Routing and abortable subscription tests |
| PL07 | Deadline/teardown/coalescing contain child lifetime | Bounded fake-child process tests; native host deadline enforcement remains deferred |
| PL08 | Late source writes and delivery failure remain recoverable | Real collector source/retry tests; real host flush timing deferred |
| PL09 | Package works outside workspace | Standalone committed artifacts; native host installation/trust deferred |

Existing `tools/build/test/plugin-adapters.test.ts` covers wrapper privacy, argument boundaries, failures and completion selection. Lifecycle/process/package tests must execute the actual committed runner/entry path. Record completed commands in [VALIDATION.md](VALIDATION.md); this checklist alone does not claim every case passed.

## Deferred host verification

Use isolated temporary host roots when later verifying installation/trust, native event delivery, host-enforced deadlines, teardown and late durable writes. Preserve existing registrations. Completion may arrive before final source data is durable; no adapter promises complete turn accounting. This deferred host exercise is distinct from deterministic fake-executable and production collector tests.

Prior art: servediff native manifests and completion adapters informed registration, while TokenInsights uses direct finite sync rather than a detached worker/state architecture. References: [Codex hooks](https://developers.openai.com/codex/hooks), [Claude hooks](https://code.claude.com/docs/en/hooks), [Pi lifecycle](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md), [OpenCode V2 events](https://opencode.ai/v2/docs/build/plugins/#events).

## Unresolved questions

None blocking finite completion adapters. Later: isolated host support verification and remote setup/authentication.
