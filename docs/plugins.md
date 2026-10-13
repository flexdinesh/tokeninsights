# Completion plugins

Completion adapters trigger `tokeninsights sync --harness HARNESS` in the background.
The CLI durably enqueues a finite worker; hooks never wait for remote ingestion. The Go collector discovers retained artifacts and submits sanitized evidence for shared processing, over authenticated remote HTTP only. Plugins contain no parser, database writer or HTTP client. Hook session fields never become fact identity or source-selection arguments.

Native manifests and packaged Pi/OpenCode entry points are included. Install manually using the native host mechanism; this repository never edits user harness registrations automatically. Real-host installation/trust, event delivery and durable-write timing remain deferred verification. Package/type/fake-executable checks do not establish those host behaviors.

| Harness | Completion boundary | Artifact | Verification boundary |
| --- | --- | --- | --- |
| Codex | `Stop` | `packages/plugin-codex/` native plugin manifest and hooks | Isolated shell wrapper tests; host deadline enforcement deferred |
| Claude Code | `Stop` | `packages/plugin-claude/` native plugin manifest and hooks | Isolated shell wrapper tests; host deadline enforcement deferred |
| Pi | `agent_settled`; cleanup on `session_shutdown` | `packages/plugin-pi/`, committed `dist/index.js` and extension package metadata | Pinned SDK 1.0.0; lifecycle/fake-executable checks |
| OpenCode V2 | `session.status` idle; cleanup aborts event subscription | `packages/plugin-opencode/`, committed `dist/index.js` and root loader | Pinned SDK 2.0.22; event/lifecycle/fake-executable checks |

Pinned SDK versions describe the declarations used to build adapters. They are not a claim that every host release or installation path passed a smoke test. OpenCode V1 and older Pi interfaces are outside this initial adapter contract.

Codex uses [bundled command hooks](https://developers.openai.com/codex/hooks); Claude uses [native plugin hooks](https://code.claude.com/docs/en/hooks). Pi loads [extensions](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md). OpenCode V2 exposes the [public event stream](https://opencode.ai/v2/docs/build/plugins/#events).

Codex/Claude command hooks spawn the CLI in the shell background, discard stdin
and collector stdout/stderr, and immediately return empty decision JSON. They
declare a ten-second host deadline and never ask the agent to continue. Background
startup/upload failures are diagnosed with manual sync and `sync status`; hook
output contains no credentials or event contents. Actual host cancellation
semantics remain a smoke-test item.

Pi/OpenCode runners spawn an argument array with `shell: false`, observe only the short enqueue child, bound its lifetime to 60 seconds and coalesce overlapping callbacks with a follow-up pass for completions during a scan. Pi returns control to the harness while the finite subprocess runs. Cleanup terminates an active enqueue child; the separately detached Go worker
survives plugin cleanup after successful startup. Unix process groups receive termination followed by bounded kill escalation. The plugins delegate durable queuing and bounded background retries to the CLI;
they keep no separate persistent trigger queue. Missing executables, nonzero exit and timeout expose only a generic host diagnostic, never event or collector output.

Set `TOKENINSIGHTS_BINARY` to the executable path when the harness PATH lacks `tokeninsights`. Literal spaces/metacharacters stay in one executable argument. Collector configuration selects its fresh database files, source roots and destination; completion payloads do not override them. Committed standalone JavaScript artifacts use the harness runtime; the Go product still requires no host JavaScript runtime.

A completion event is a collection opportunity, not proof that every harness write is flushed. Interrupted/truncated or delayed records remain collectible by later sync. Background enqueue may fail or be interrupted; already committed collector journal and server receipts survive, and manual sync resumes delivery. Manual sync and `sync status` remain troubleshooting commands. Background work
is finite, not an always-on retry service; later sync replays retained evidence. Configure the distributed URL/token before installing hooks. Plugins never refresh an in-process viewer; restart tui/web to recollect locally.

Executable adapter and packaging checks live in `tools/build/test/plugin-adapters.test.ts` and `tools/build/test/plugin-build.test.ts`. The [development guide](development.md) defines full verification; real-host installation and event delivery remain deferred as described above.

## Installation and contract scope

Use the matching native registration artifact, with the CLI installed on PATH (or
`TOKENINSIGHTS_BINARY` set) and distributed preferences configured for the harness
process's HOME/config environment:

- Codex: register `.agents/plugins/marketplace.json`, then install its TokenInsights
  entry, which resolves `packages/plugin-codex`.
- Claude Code: register `.claude-plugin/marketplace.json`, then install its
  TokenInsights entry, which resolves `packages/plugin-claude`.
- Pi: install/load `packages/plugin-pi`; package metadata registers `dist/index.js`.
- OpenCode V2: register `packages/plugin-opencode/index.js` through its plugin loader.

Use user-wide registration to cover multiple repositories. Hosts own plugin trust,
installation and lifecycle. No test invokes an installed harness or edits host
registrations. Fake host APIs and executables prove event selection, immediate
callback/hook return during blocked enqueue, safe executable arguments, empty
stdin, output isolation, overlapping-event coalescing and cleanup. Packaged-artifact
checks prove loading outside the workspace. Real CLI/container fixtures prove
queue startup, ingestion, replay and restart independently of host assumptions.
These contracts assume the documented event is delivered and durable artifacts
become readable; real-host timing and registration remain manual verification.
