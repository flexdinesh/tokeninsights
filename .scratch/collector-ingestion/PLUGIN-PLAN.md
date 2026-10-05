# Completion plugin implementation

Status: native command adapters started; Pi/OpenCode routing scaffolds started.
Scope: same PR as collector/server ingestion. Manual sync remains primary.

## Responsibilities

- Collector owns parsing, normalization, SQLite, outbox and delivery.
- Thin host adapters request the same `tokeninsights sync`; no direct HTTP or
  database writes, no hook transcript fields forwarded.
- Server dedupe handles repeated completion triggers. Hook execution itself
  supplies no exactly-once guarantee and no usage identity.
- Hook failure must not ask the coding agent to continue. Users retain manual
  sync and host debug logs for diagnosis.

## Prior art and evidence

Read `/home/dee/workspace/servediff/main/docs/plugins.md`,
`docs/agent-plugins-plan.md` and all four native packages. Servediff schedules a
detached Go worker; TokenInsights initially performs bounded finite sync instead
of copying that separate worker/state architecture.

Checked official host docs on 2026-10-05: [Codex hooks](https://developers.openai.com/codex/hooks),
[Claude hooks](https://code.claude.com/docs/en/hooks),
[Pi extensions](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md),
[OpenCode V2 plugins](https://opencode.ai/v2/docs/build/plugins).
Local installed Pi declaration confirms `agent_settled`. No supported interface
guarantees every durable source write has flushed before our trigger.

## Implementation gates

1. Command wrappers: native Stop manifests; quoted runtime binary override;
   stdin discarded; output suppressed; empty decision JSON; fixed failure
   marker; synchronous 60-second host deadline.
2. Pi runner: await finite Go CLI process from `agent_settled`; terminate on
   deadline; surface generic host UI error without event content. Register no
   model tool, agent continuation, timer or process during extension factory.
3. OpenCode V2 runner: consume idle `session.status` events, ignore legacy idle
   duplicates, busy/retry and malformed events; bounded/coalesced triggering;
   cleanup aborts stream and active child; host-owned logs only.
4. Native host type checks and standalone install artifacts. Add narrowly
   scoped plugin workspace/build checks with integration owner; no bundled
   Node runtime or JavaScript dependency from Go production code.
5. Isolated real-host smoke tests for installation/trust, event delivery,
   late/truncated source writes, failure reporting, timeout and teardown.
   Preserve existing host registrations. Install to temporary host roots only.

## Traceable tests

`tools/build/test/plugin-adapters.test.ts` currently verifies native Stop
selection/deadline, literal binary paths, PATH fallback, zero forwarded stdin,
exact `sync` arguments, hidden CLI output, nonzero/missing-binary isolation,
Pi settled routing/waiting and OpenCode V2 idle filtering. Fake binaries and
synthetic markers supply all inputs. These tests do not prove native host
timeout enforcement or that source writes flush before hook dispatch.

| ID | Invariant | Coverage |
| --- | --- | --- |
| PL01 | Stop invokes only the shared sync command | Codex/Claude executable tests |
| PL02 | Hook content never reaches collector stdin or output | Codex/Claude executable tests |
| PL03 | Literal runtime binary paths preserve argument boundaries | Codex/Claude executable tests |
| PL04 | Missing/nonzero CLI cannot request agent continuation | Codex/Claude executable tests |
| PL05 | Settled callbacks await collection rather than acknowledge scheduling | Pi routing test |
| PL06 | Idle only; legacy, busy, retry and malformed events ignored | OpenCode event test |
| PL07 | Deadline/teardown terminates work without leaked children | Real-host tests pending |
| PL08 | Late durable source writes remain collectable on later sync | Collector integration pending |

Required next cases: host lifecycle teardown; bounded child kill; overlapping
triggers converge through collector writer/admission; source trailing record
finishes after hook; unavailable destination leaves durable retry; lost ack
then repeated hook dedupes; no conversation content in local diagnostics or
published payload; package works outside workspace without build tools.

## Unresolved questions

- Supported host version baselines?
- Keep synchronous hook wait, or later detached Go worker?

Defaults: recent native interfaces only; finite synchronous command hooks now.
