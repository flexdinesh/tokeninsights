# Desktop thin viewer

Status: Accepted. Date: 3 November 2026. User approved the plan.

## Context

`web` and `browse` open dashboards in the system browser. Users asked for a
dedicated app window on machines with a display, and an easy path from an SSH
session to an app on their own machine. MyGo builds desktop apps in pure Go
(`CGO_ENABLED=0`) on the system webview (WKWebView, WebKitGTK), with URL scheme
registration and a single-instance lock. It is pre-1.0 and changing quickly.

## Decision

Add an optional thin viewer as the separate Go module `packages/desktop`, pinned to
an exact MyGo version. It shows dashboards served elsewhere and owns no usage
storage, collection, ingestion or analytics. The CLI stays the owner of local
storage; "one viewer owns local storage" is unchanged. The CLI module never imports
MyGo or the desktop module.

- Link contract: `tokeninsights://open?url=<escaped absolute http(s) URL>`.
  The CLI writes it; the app accepts nothing else.
- CLI policy: `--open=auto|app|browser|false` on `web` and `browse`, config
  `viewer.open`, default `auto`. Auto-detect the registered app (macOS `open -b
  com.flexdinesh.tokeninsights`, Linux `xdg-mime` default handler), else open the
  browser. SSH sessions open nothing; `web` prints the URL, app link and an
  `ssh -L` tunnel hint.
- App policy: one window per origin, pages untrusted (no Go bindings), loopback
  origins open directly, other origins require confirmation once per origin,
  cross-origin navigation goes to the system browser, empty launch shows help.
- Platforms: macOS and Linux, amd64 and arm64. No Windows.
- Distribution: unversioned `tokeninsights-desktop_<os>_<arch>.tar.gz` archives in
  the existing CLI release and `checksums.txt`, installed by the raw `main`
  `install.sh` from `releases/latest/download`. No Homebrew cask, no auto-update,
  no Developer ID signing or notarization.

This changes the release workflow: native release jobs also build the app, and
macOS runners are required to seal the bundle with an ad-hoc signature. This is
the documented reason for departing from "release workflows remain unchanged".
No schema, wire protocol, read API or storage contract changes.

## Consequences

- Archives downloaded by a browser carry quarantine and Gatekeeper blocks them;
  `curl | sh` does not set quarantine.
- The raw `main` script may differ from the latest release's; it depends only on
  stable archive names and checksums.
- Hosted logins inside the app use the webview's own cookie store.
- Detection can miss a registered app (Linux without `xdg-utils`); auto falls back
  to the browser and never fails the command.
- Local `web` still has no authentication on non-loopback binds; SSH tunnels are
  the documented remote path. Authentication is separate work.
- MyGo upgrades are deliberate version bumps verified on macOS and Linux.

## Verification

Pure tests cover link parsing, origin normalization, loopback classification and
the trusted-origin store. CLI tests cover opener selection per platform, SSH,
mode parsing, config precedence and printed output. Tooling tests cover archive
packaging, the Homebrew formula ignoring desktop archives and `install.sh` against
a `file://` release with checksum, layout and host failures. GUI behavior
requires manual verification on a machine with a display.

Unresolved questions: none.
