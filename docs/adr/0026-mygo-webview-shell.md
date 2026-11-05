# MyGo webview shell over the shared dashboard

Superseded where conflicting by [ADR 0027](0027-personal-backend-and-explicit-connections.md).
Current product: one personal SQLite backend, explicit local/SSH/temporary TCP
connections. Hosted/accounts/PostgreSQL/sync/plugins and conflicting listener,
opener or distribution choices below are historical. Applicable accounting,
privacy, identity, transaction and lifecycle principles remain.
Product, package, installer, archive and OS identity are now named `desktop`;
MyGo names below are historical. MyGo remains the framework.
The original decision below is preserved as history.

Status: Accepted. Date: 5 November 2026. User directed the change.

## Context

ADR 0024 made MyGo a native Go UI presentation with a small feature set. Bringing
it to parity with the web dashboard meant reimplementing the dashboard in a second
toolkit: charts, filters, tables, status, formatting and tokens, each needing its
own tests and each drifting from the web. The cost is ongoing, and every future
dashboard feature would pay it again.

## Decision

MyGo is a thin desktop shell. It loads the dashboard the backend serves in the
system webview. `packages/dashboard` and `packages/web` are the
only dashboard presentation; `packages/desktop-mygo` contains no views.

This supersedes, from ADR 0024, MyGo's native Go UI, and from ADR 0025, MyGo's
control lease. MyGo now holds a dashboard lease and uses the on-demand loopback
listener. It remains a separate shell with its own names, identifiers and schemes,
and remains the `auto` preference. This differs from the ADR 0023 thin viewer: MyGo
owns a backend lease and bundles its backend, so it starts without a CLI command;
hosted and external origins still open in the browser, with no trust prompts.

`--endpoint=ssh://...` keeps working. The shell holds the remote lease and serves
it through `dashboardproxy` on an available loopback port, as `web --endpoint`
does. Remote failures never select local storage.

Pages are untrusted: no Go bindings, permissions or downloads. Navigation stays on
the dashboard origin; other http(s) pages open in the browser.

## Consequences

- Dashboard features reach browser and MyGo together, and agents verify
  them with browser tooling.
- Rendering depends on system WebKit: WKWebView on macOS and WebKitGTK 4.1 on
  Linux. The dashboard's browser tests run in Chromium only, so WebKit differences
  surface only through the distribution smoke test and manual checks.
- MyGo opens a local TCP port while a window is open, as the browser dashboard does.
- No schema, wire protocol, read API or storage contract changes.

## Verification

Attachment tests run against a real backend. `smoke-mygo.ts` launches the installed
app under Xvfb in the distribution group and waits for the page's title.
