# TokenInsights MyGo

Thin MyGo webview shell over the shared dashboard. MyGo is the only desktop.
`tokeninsights` opens local data; `tokeninsights connect URI` opens
the explicit local, SSH or temporary TCP target. No transport or browser fallback.

Every window holds its own native backend session and loopback dashboard bridge.
Standalone launch captures locally. CLI handoffs carry the selected target and
capture policy through a private Unix rendezvous; the OS URI contains only a
random ID. The CLI returns after the window acknowledges its live session.
An existing app opens a new window for each request. No credential files.

Pages have no Go bindings, permissions or downloads. Navigation stays on the
window's bridge origin; other HTTP(S) navigation opens the browser. Closing the
window releases bridge and session. The adjacent bundled backend requires no
host JavaScript runtime. Linux requires system WebKitGTK 4.1.

Identity: `com.flexdinesh.tokeninsights.mygo`; scheme: `tokeninsights-mygo`.
The shared dashboard owns all visual presentation. See ADR 0027.
