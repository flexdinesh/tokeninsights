# Experimental native desktop on the edge channel

[ADR 0031](0031-local-source-installs.md) retires the `edge` channel and
`edge-build` prerelease; `install-dev-native.sh` builds the native app locally.

Status: Accepted. Date: 11 November 2026. User directed a second, fully native
desktop app installed beside the current desktop until it can replace it.

## Context

ADR 0026 made desktop a MyGo webview shell over the shared dashboard because
a second toolkit duplicated charts, filters, tables, status and formatting, and
drifted. The webview depends on system WebKit, which browser tests (Chromium)
do not cover. The user wants a native app with the same visual polish and native
behavior, evaluated side by side before any replacement decision.

## Decision

Add **TokenInsights Native** (`packages/desktop-native`, binary
`tokeninsights-native`, id `com.flexdinesh.tokeninsights.native`, scheme
`tokeninsights-native`). It renders with MyGo native UI (`mygo/ui`), with no
webview and no loopback bridge. Each window holds its own `localclient` session
through `desktoplaunch.Connect` and queries the same `application` reads,
progress and capture as the TUI. TokenInsights Desktop stays the default and
is unchanged.

This supersedes, for this experiment only:

- ADR 0026: the shared web dashboard is no longer the only dashboard
  presentation. The native app is a second presentation of the same semantic
  operations; it owns no storage, accounting or query policy.
- ADR 0027: desktop is no longer the only desktop product. `tokeninsights
  [connect URI] --app=native` selects the native app explicitly; `--app`
  defaults to `desktop`. The handoff is addressed to the selected app's scheme
  and only that app's acknowledgement satisfies it. An unavailable native app
  fails with guidance and never falls back to desktop, browser or local data.
- ADR 0024: an `edge` channel returns, for `desktop-native` only. The rolling
  `edge-build` prerelease is rebuilt on each main push and can be published
  from any ref by manual dispatch. Edge installs never own `~/.local/bin/tokeninsights`.

The native app ships in vertical slices (shell, filters, then tabs). Tabs not
yet ported stay visible but disabled. Accepted native-first differences: a
native menu and shortcuts, a virtualized table that fetches pages on demand
instead of a pager, and a desktop-only layout (minimum 720×520). It keeps the
DESIGN.md roles, square corners and bundled Geist.

Replacing desktop with the native app is a separate future decision.

## Consequences

- Drift is the main cost. DESIGN.md stays the visual contract: a Go guard test
  fails when native color roles differ from its frontmatter. Shared formatting
  rules are pinned by fixtures both TypeScript and Go tests consume.
- Native rendering needs GTK 3 and OpenGL on Linux (CPU fallback in VMs), not
  WebKitGTK. macOS uses Metal.
- Two desktop shells must stay independently removable; a dependency guard
  forbids either importing the other.
- No schema, storage, wire protocol, read API or accounting changes.

## Verification

Theme guard, headless `ui.Tester` views, real-backend session tests, CLI and
`desktoplaunch` selection/acknowledgement tests, and an installed-app smoke
under Xvfb in the distribution group. Visual parity is reviewed manually from
native and web captures; no pixel goldens run in validation.
