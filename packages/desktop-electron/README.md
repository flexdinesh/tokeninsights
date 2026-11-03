# TokenInsights Electron

The sandboxed Electron window loads the local Go backend's packaged browser
composition. That composition injects the HTTP client into
`@tokeninsights/dashboard`. The renderer has no Node integration, preload or IPC.

Use `mise run build:desktop` for the shared product packaging flow. It builds the
Go backend and Electron main process for the native target, then creates the
prebuilt product archives. The Electron bundle includes
Electron's runtime and the backend beside its resources. No host Node, npm, pnpm
or Go is needed. Generated output stays ignored.

The shell invokes the sibling resource as `tokeninsights-backend connect --json`.
Go owns configuration, database discovery and backend lifetime. The connector
prints one validated `{url, instanceId, protocolVersion}` JSON line; its stdin
stays open until the window closes. Unexpected connector exit closes the viewer.
Startup failures offer Retry or Quit. For development, install the pinned
Electron runtime with `pnpm --filter @tokeninsights/desktop-electron run install:runtime`.

Identity: `com.flexdinesh.tokeninsights.electron`; app: **TokenInsights Electron**;
executable/scheme: `tokeninsights-electron`. The generic `tokeninsights` scheme
belongs to the chosen default MyGo app. Explicit
`tokeninsights-electron://open?url=<encoded-http-url>` links and HTTP(S) arguments
open the system browser for hosted targets. Loopback targets are probed for
local/hosted mode; personal targets attach to a matching existing private owner.
URL handoffs request no capture, preserving the launcher's sync/source policy.
A tunneled personal dashboard without a local owner opens its original URL in
the browser. A launch without a URL uses Go-owned default settings and collects.
Renderer navigation stays on the acquired backend origin.
