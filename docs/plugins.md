# Completion plugins

Completion adapters invoke the same finite `tokeninsights sync` command as manual collection. The Go collector discovers retained artifacts, normalizes locally and publishes canonical facts. Plugins contain no parser, database writer or HTTP client. Hook session fields never become fact identity or source-selection arguments.

Native manifests and packaged Pi/OpenCode entry points are included. Install manually using the native host mechanism; this repository never edits user harness registrations automatically. Real-host installation/trust, event delivery and durable-write timing remain deferred verification. Package/type/fake-executable checks do not establish those host behaviors.

| Harness | Completion boundary | Artifact | Verification boundary |
| --- | --- | --- | --- |
| Codex | `Stop` | `packages/plugin-codex/` native plugin manifest and hooks | Isolated shell wrapper tests; host deadline enforcement deferred |
| Claude Code | `Stop` | `packages/plugin-claude/` native plugin manifest and hooks | Isolated shell wrapper tests; host deadline enforcement deferred |
| Pi | `agent_settled`; cleanup on `session_shutdown` | `packages/plugin-pi/`, committed `dist/index.js` and extension package metadata | Pinned SDK 1.0.0; lifecycle/fake-executable checks |
| OpenCode V2 | `session.status` idle; cleanup aborts event subscription | `packages/plugin-opencode/`, committed `dist/index.js` and root loader | Pinned SDK 2.0.22; event/lifecycle/fake-executable checks |

Pinned SDK versions describe the declarations used to build adapters. They are not a claim that every host release or installation path passed a smoke test. OpenCode V1 and older Pi interfaces are outside this initial adapter contract.

Codex uses [bundled command hooks](https://developers.openai.com/codex/hooks); Claude uses [native plugin hooks](https://code.claude.com/docs/en/hooks). Pi loads [extensions](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md). OpenCode V2 exposes the [public event stream](https://opencode.ai/v2/docs/build/plugins/#events).

Codex/Claude command hooks wait synchronously, declare a 60-second host deadline, discard stdin and collector stdout/stderr, and return empty decision JSON. Failure prints a fixed hook-log marker and never asks the agent to continue. Actual host cancellation semantics remain a smoke-test item.

Pi/OpenCode runners spawn an argument array with `shell: false`, wait for the finite child, bound its lifetime to 60 seconds and coalesce overlapping callbacks. Cleanup terminates active work; Unix process groups receive termination followed by bounded kill escalation. The plugins do not start a detached retry worker or persist another trigger queue. Missing executables, nonzero exit and timeout expose only a generic host diagnostic, never event or collector output.

Set `TOKENINSIGHTS_BINARY` to the executable path when the harness PATH lacks `tokeninsights`. Literal spaces/metacharacters stay in one executable argument. Collector configuration selects its fresh database files, source roots and destination; completion payloads do not override them. Committed standalone JavaScript artifacts use the harness runtime; the Go product still requires no host JavaScript runtime.

A completion event is a collection opportunity, not proof that every harness write is flushed. Interrupted/truncated or delayed records remain collectible by later sync. Hooks may time out before all collection finishes; already committed collector journal and server receipts survive, and manual sync resumes delivery. Manual sync remains the primary workflow and troubleshooting command.

See the [plugin implementation plan](../.scratch/collector-ingestion/PLUGIN-PLAN.md) for test IDs and deferred real-host verification. Full build/test outcomes are recorded separately in [validation](../.scratch/collector-ingestion/VALIDATION.md).
