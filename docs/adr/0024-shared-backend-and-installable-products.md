# Shared backend, removable presentations and release downloads

Status: Accepted. Date: 3 November 2026. User approved the execution plan and
the versioned local-client contract.

## Decision

TUI, browser, Electron and native MyGo are presentations of the same application.
A shared local Go backend owns capture, direct acceptance, durable processing,
queries, storage and shutdown. Hosted composition retains its independent
authenticated SQLite/PostgreSQL server and never captures host sources.

This supersedes command-owned local storage/lifetimes in ADRs 0009 and 0014,
the thin MyGo webview in ADR 0023, and source/tap publication in ADRs 0001, 0002,
0017 and 0019. Accounting, evidence, receipts, dataset isolation, persistence
contracts and hosted authorization remain unchanged. Database SQL and schema
versions do not change.

One root Go module contains the core and Go applications. The validation tool
retains its own module. `packages/core` owns domain/application behavior and
adapters; `packages/cli`, `packages/backend`, `packages/server` and
`packages/desktop-mygo` own executable composition. `packages/dashboard` owns
reusable React presentation; `packages/web` owns its browser HTTP adapter;
`packages/contracts` owns TypeScript contracts; `packages/desktop-electron`
owns the Electron shell. Shells cannot select storage adapters or import another
shell. Interface and dependency tests enforce these boundaries.

Application operations describe queries, status, progress, capture, maintenance
and viewer lifetime without HTTP request types. HTTP owns routing, encoding,
authorization and status mapping. Keep the existing hosted API shape and HTTP
as the initial process transport. Direct/HTTP conformance proves semantic parity;
no generic transport framework is introduced.

Local launchers serialize startup by canonical storage identity and discover or
start a bundled backend. A versioned, user-private Unix socket owns control.
Incompatible running owners and conflicting storage/listen settings reject;
they never trigger another backend or terminate an existing viewer. Public local
HTTP stays read-only except for each browser's own viewer lifetime.

Each viewer holds a live connection. Closing it releases its lease; connection
loss also releases it. Browser streams avoid reliance on tab-close callbacks or
background JavaScript timers. A short reconnect grace covers navigation/reload.
After the last viewer leaves, stop admission, cancel/join capture and processing,
close listeners/storage, then release ownership. A foreground `web` command is
itself a viewer; Ctrl+C releases it without terminating other viewers. Pending
accepted processing survives shutdown and resumes with the next owner.

Collect on launcher invocation, coalescing identical overlapping requests and
retaining bounded stronger follow-up work. No login service or periodic scan.
`--sync=false` skips launch capture while durable processing resumes. Browser
reload, filters and TUI reload query only. Local maintenance attaches to the
owner instead of reopening storage. Desktop URL handoffs attach without capture,
preserving the originating launcher's sync/source policy. Standalone desktop
launches collect. A tunneled personal dashboard without a matching local private
owner opens its original URL in the browser.

MyGo uses native Go UI for local totals, date/provider/model/harness filters,
results, progress and retry. Electron loads the composed reusable dashboard
through the local backend, with sandboxed renderers and no Node integration or
privileged page bindings. Hosted URLs open in the system browser. Distinct names,
identifiers and schemes allow both experiments to coexist. MyGo is the default;
`--open=mygo|electron|browser|auto|false` and `viewer.open` select presentation.

## Distribution

Stable releases use immutable source tags `vX.Y.Z`, with an explicit version
supplied to the manual release workflow. A rolling `main-build` GitHub prerelease
contains the latest main build that passed required checks. Both channels contain
prebuilt macOS/Linux amd64/arm64 archives and checksums. Product installers select
CLI, hosted server, MyGo or Electron; clients bundle their required backend.
Install to user-owned paths, verify checksums/layout and stage replacements before
publishing them. Stable/main replace that product; rerun the installer to update.

No compiled executables, app bundles, archives or generated dashboard assets are
committed to any branch or tag. Build generated assets locally/in CI and publish
compiled artifacts only as release attachments. Delete tap automation, generated
source commits, `edge` publication and supported `go install` delivery. Existing
published releases and tags remain untouched. No automatic updater or new hosted
installer domain is required.

Go applications require no host JavaScript runtime. Bundled Electron is the
explicit production exception; it includes its runtime and never requires host
Node/npm/pnpm. Go runtime code never launches JavaScript development tooling.

## Tradeoffs and verification

Shared ownership adds discovery, compatibility and disconnect handling, while
allowing every presentation to run together. The first live compatible backend
owns the configured stores; upgrades never force-stop its users. MyGo remains a
pinned experimental toolkit. Desktop packages are larger because each includes
its backend; their removal remains independent.

Prove startup races, rejection, disconnect/reconnect, final-viewer shutdown,
capture coalescing, maintenance and durable reopen with real processes/storage.
Preserve direct/HTTP parity and all accounting/tenant/receipt contracts. Verify
React client injection, native UI state, Electron isolation, archive hygiene,
installer failure atomicity and packaged runtime without host build tools.
Use existing local Dagger validation; native platform builds cover environments
unavailable on the developer host.

The user approved running installed Electron GUI smoke on the host through
`mise run check:desktop`, in addition to `mise run check:pr` on clean committed
HEAD. Dagger denies unprivileged user namespaces, which a user-owned Linux
installation needs for Chromium sandboxing without a privileged setuid helper.
Keep the renderer sandbox enabled. Packaging, installation, bundled-runtime,
native-window and backend HTTP checks remain isolated. No shared engine changes
or unsandboxed renderer fallback are introduced.

Unresolved questions: none.
