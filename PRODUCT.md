# TokenInsights

<!-- impeccable:product-schema 1 -->

Shared product context for the CLI, terminal dashboard, and browser dashboard.
Architecture and analytics semantics live in [docs/design.md](docs/design.md);
terminology lives in [CONTEXT.md](CONTEXT.md). The browser visual contract is
[DESIGN.md](DESIGN.md).

## Platform

web

## Users

Individual developers inspecting their own token usage across coding harnesses.
They want to understand usage over time, compare model and harness usage, and
inspect sessions and their context load using locally retained data.

## Product Purpose

Make local token usage across OpenCode, Pi, Codex, and Claude Code understandable
in one tool, without relying on vendor dashboards. Success means developers can
find trustworthy usage totals and trace them to relevant sessions, models,
providers, harnesses, and dates.

## Positioning

TokenInsights collects durable local harness data, normalizes it in host SQLite,
and publishes session-centric facts to a canonical SQLite server. Terminal and
browser dashboards query the same REST analytics. Manual collection does not
require a separate authenticated harness export or realtime hooks.

## Operating Context

- One native Go binary provides the CLI, TUI, and browser server. Production runs
  without Node, npm, or pnpm; browser assets are embedded and work offline.
- `tokeninsights service` manages the local server, `tokeninsights sync` collects
  and publishes, and `tokeninsights tui` opens the terminal dashboard.
- Viewers read committed usage through shared direct/HTTP query adapters. TUI
  startup collects and publishes within a progress screen; `tui --sync=false`
  skips collection. Local Web opens after storage initialization and shows
  collection/processing progress alongside saved data. Dashboard `r` and the
  local-only browser **Reload** only reload queries. TUI startup errors offer
  Retry, View saved data, and Quit; Web collection errors preserve saved-data access.
- Local hostname labels identify the machine running the viewer, including before
  ingestion. They do not attribute imported history to that machine. Loading,
  connection/storage failures, and individual usage/filter errors remain distinct.
- Date ranges and dimension filters constrain displayed analytics, not which
  harnesses sync. Calendar grouping uses the serving machine's local time.
- The browser queries the server serving its page. Explicit CLI `--server-url`
  selects another server and skips local startup. Every non-loopback bind
  requires a token; clients use bearer auth and browser Basic auth. Remote TLS
  deployment, provisioning, and multi-account administration remain later work.
- Fresh `collector.sqlite` and `server.sqlite` have distinct roles. The old
  `tokeninsights.sqlite` stays untouched; previous commands/schema migrations
  are not supported. Advanced host maintenance uses the `collector` namespace.

## Capabilities and Constraints

- Summarize tokens over time and by model, provider, harness, and session; compare
  Session Peak Context Load. Support filtering, sorting, and browser pagination.
- Store usage metadata only. Exclude prompts, responses, tool arguments/output,
  request headers, secrets, raw provider payloads, and full source paths.
- Canonical usage must resolve to a stable session. Preserve deduplication,
  countability, component semantics, and provider provenance across every view.
- Missing model/provider information must remain visible through the documented
  fallback values; inferred attribution must not masquerade as explicit data.
- Session Peak Context Load measures prompt-side input plus cache reads/writes;
  it excludes output/reasoning and is not an additive token-total metric.
- Totals and session coverage describe the full filtered result, independently
  of table pagination. Distinguish sessions shown from all synced sessions.
- Cost tracking is outside the active product. Thin completion plugins invoke
  the same collector sync; realtime/checkpoint accounting remains future work.
  Preserve TPS concepts without claiming unavailable timing measurements.
- Schema changes require explicit approval. UI changes must preserve canonical
  analytics semantics and sync behavior.

## Brand Commitments

The product name is **TokenInsights**; the public command is `tokeninsights`.
Use the established domain terminology and existing logo. The incumbent browser
identity is recorded separately in `DESIGN.md`.

## Evidence on Hand

- [README.md](README.md): current product description, commands, and screenshots.
- `assets/tokeninsights-web.png` and `assets/tokeninsights-tui.png`:
  committed browser and terminal captures; verify freshness before visual work.
- `packages/web/public/tokeninsights-logo.png`: existing logo asset.
- `packages/cli/testdata/conformance/sync-first-basic/` and `packages/web/src/mocks/`:
  synthetic development/demo data, not customer evidence or usage benchmarks.

## Product Principles

1. Keep analytics grounded in durable source facts and explicit provenance.
2. Keep private conversation content out of analytics storage.
3. Give terminal and browser users consistent canonical answers.
4. Make filtering, session coverage, and unavailable data understandable.
5. Preserve a self-contained local runtime and direct network access.

## Accessibility & Inclusion

Preserve the browser's existing keyboard access, visible focus, accessible control
names, text alternatives for chart values, scalable type, reduced-motion support,
and light/dark themes. Narrow layouts must retain access to all views and readable
tables. No additional accessibility certification or target standard was specified.
