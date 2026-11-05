# TokenInsights Electron

Sandboxed Electron windows load the shared dashboard through per-window loopback
bridges. Renderers have no Node integration, preload or privileged IPC. Each
window has an isolated browser session and owns one native Go connector.

`tokeninsights --open=electron` launches local data. `tokeninsights connect URI
--open=electron` targets local, SSH or temporary pinned TCP explicitly. Each
existing-app handoff opens a new window with its exact target and capture policy.
No browser or local-data fallback.

The main process starts bundled `tokeninsights-backend connect --json`, optionally
with `--launch` and an opaque rendezvous link. Go receives credentials over a
private Unix socket, holds the requested session, starts the shared bridge and
prints only `{url, instanceId, protocolVersion}`. After loading the bridge,
Electron sends `ready` through stdin. Go acknowledges the private launcher only
then. Keeping stdin open retains the window lease. Closing releases both session
and bridge. Standalone launch captures locally. Handoffs capture exactly once
according to the originating CLI policy.

Identity: `com.flexdinesh.tokeninsights.electron`; scheme:
`tokeninsights-electron`. The OS URI carries only a random rendezvous ID.
Connection secrets never enter renderer configuration, argv or files.
Navigation stays on the bridge origin; other HTTP(S) links open the browser.

`mise run build:desktop` packages the bundled Electron runtime and Go backend.
No host Node/npm/pnpm or Go is needed. `mise run check:desktop` validates the
installed app with Chromium sandboxing enabled. Generated products stay ignored.
