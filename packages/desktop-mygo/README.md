# TokenInsights MyGo

Desktop shell around the shared web dashboard, using MyGo v0.7.4 and the system
webview (WKWebView, WebKitGTK). App name **TokenInsights MyGo**; bundle ID
`com.flexdinesh.tokeninsights.mygo`. Registers `tokeninsights` and
`tokeninsights-mygo`. Electron registers its separate `tokeninsights-electron`
scheme. Existing `tokeninsights://open?url=...` links remain supported.

There are no views here. The window loads the dashboard the backend serves, so
every feature comes from `packages/dashboard` and `packages/web`, and a change
there reaches browser, Electron and MyGo together. See ADR 0026.

`viewer.open` defaults to `auto`, which prefers MyGo and falls back to the browser. CLI `--open` overrides it with `mygo`,
`electron`, `browser`, `auto` or `false`. `app` remains a compatibility alias
for `mygo`; bare `--open` and `--open=true` select auto. Explicit unavailable
desktop targets return an error; `auto` tries MyGo and falls back to the browser.

`dashboard.go` owns backend attachment:

- A standalone launch holds a dashboard lease on the configured backend and
  requests capture.
- A linked URL attaches the matching local owner without capture or resolving
  default storage, and keeps the linked page. Hosted, external and tunneled
  dashboards without a local owner open in the browser.
- `--endpoint=ssh://...` holds the remote lease and serves its read-only
  dashboard through `dashboardproxy` on an available loopback port, as
  `web --endpoint` does.

`viewer.go` owns windows on the GUI thread, one per dashboard origin, each with
its own lease. Attaching and releasing stay off the GUI thread. Closing a window
releases its lease; reopening attaches a new one. A lost backend reports, closes
the window and leaves the app reopenable. Pages get browser capabilities only:
no Go bindings, permissions or downloads. Other http(s) origins open in the
browser and other schemes go nowhere. The bundled `tokeninsights-backend`
executable is adjacent to the native executable and requires no host JavaScript
runtime. Linux needs the system WebKitGTK 4.1.

Tests cover attachment against a real backend and the navigation policy.
`tools/build/src/smoke-mygo.ts` launches the installed app under Xvfb and waits
for the page's title, which proves WebKit loaded the dashboard. Change the
dashboard's UI in the web packages and verify it with `agent-browser`.
