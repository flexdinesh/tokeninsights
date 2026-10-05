# Completion plugins

Completion hooks request `tokeninsights sync`. The collector discovers durable
source artifacts, normalizes locally, and publishes canonical data through the
same path as manual sync. Plugins never parse transcripts, write databases, or
send HTTP requests. Hook session IDs are trigger metadata, not fact identities.

This PR contains Codex/Claude native manifests and tested command wrappers.
Pi/OpenCode contain tested completion-routing source scaffolds only; package
entry points, bounded runners, host type checks and install artifacts remain
pending. No plugin is installed automatically. The collector/server migration
must land before these wrappers provide the proposed publication behavior.

| Harness | Completion boundary | Artifact | Current verification |
| --- | --- | --- | --- |
| Codex | `Stop` | `packages/plugin-codex/` | Isolated shell invocation |
| Claude Code | `Stop` | `packages/plugin-claude/` | Isolated shell invocation |
| Pi | `agent_settled` | `packages/plugin-pi/src/completion.ts` | Injected async runner |
| OpenCode V2 | `session.status`, idle | `packages/plugin-opencode/src/completion.ts` | Event selection |

Codex supports [bundled command hooks](https://developers.openai.com/codex/hooks).
Claude supports [plugin hooks](https://code.claude.com/docs/en/hooks) and its
native plugin installer. Pi loads [TypeScript extensions](https://github.com/badlogic/pi-mono/blob/main/packages/coding-agent/docs/extensions.md)
and exposes a final `agent_settled` event. OpenCode V2 exposes an
[abortable public event stream](https://opencode.ai/v2/docs/build/plugins).
OpenCode V1 and older Pi releases need separate compatibility decisions.

Codex/Claude wrappers run synchronously with a 60-second host deadline. They
discard hook stdin and CLI stdout/stderr, return empty decision JSON, and never
request continuation. Failures emit a fixed marker to hook stderr; enable host
debug logging and run `tokeninsights sync` manually for details. Host timeout or
teardown may cancel sync. Completed collector/server transactions remain
durable; a later manual sync retries pending delivery. There is no detached
worker, retry loop, or promise that hook scheduling means ingestion committed.

Set `TOKENINSIGHTS_BINARY` to the executable's absolute path when the harness
PATH lacks `tokeninsights`; quoting preserves spaces and literal shell
characters. The collector's own settings choose source roots, database and
destination. Plugins do not override them based on the current project or
transcript path.

Host completion is a collection opportunity, not proof that every source write
has flushed. Source truncation, unfinished trailing records, or delayed harness
writes must remain retryable on the next sync. Manual sync stays the primary
workflow and fallback. Real-host installation, timeout, teardown and durable
flush tests remain required before calling any adapter fully supported.
