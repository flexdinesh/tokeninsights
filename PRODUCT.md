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

TokenInsights reads durable local harness data, normalizes it into session-centric
SQLite facts, and presents the same canonical analytics in terminal and browser
dashboards. It does not require a separate authenticated API export or realtime
hooks to collect retained usage.

## Operating Context

- One native Go binary provides the CLI, TUI, and browser server. Production runs
  without Node, npm, or pnpm; browser assets are embedded and work offline.
- `tokeninsights` opens the terminal dashboard; `tokeninsights serve` hosts the
  browser dashboard on localhost by default.
- Opening either viewer refreshes supported local sources by default. `--no-sync`
  skips that refresh; browser users can explicitly request **Sync now**.
- Date ranges and dimension filters constrain displayed analytics, not which
  harnesses sync. Calendar grouping uses the serving machine's local time.
- The browser can select a reachable remote TokenInsights server. Each source is
  viewed separately; switching sources preserves filters and navigation and does
  not merge datasets. Remote access is intended for trusted networks; the current
  server has no authentication.

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
- Cost tracking is outside the active product. Realtime/checkpoint plugins are
  future concepts. Preserve TPS concepts for durable timing data without claiming
  currently unavailable measurements.
- Schema changes require explicit approval. UI changes must preserve canonical
  analytics semantics and sync behavior.

## Brand Commitments

The product name is **TokenInsights**; the public command is `tokeninsights`.
Use the established domain terminology and existing logo. The incumbent browser
identity is recorded separately in `DESIGN.md`.

## Evidence on Hand

- [README.md](README.md): current product description, commands, and screenshots.
- `assets/tokeninsights-web-light.png` and `assets/tokeninsights-view-models.png`:
  committed browser and terminal captures; verify freshness before visual work.
- `packages/web/public/tokeninsights-logo.png`: existing logo asset.
- `packages/cli/testdata/conformance/sync-first-basic/` and `packages/web/src/mocks/`:
  synthetic development/demo data, not customer evidence or usage benchmarks.

## Product Principles

1. Keep analytics grounded in durable source facts and explicit provenance.
2. Keep private conversation content out of analytics storage.
3. Give terminal and browser users consistent canonical answers.
4. Make filtering, session coverage, and unavailable data understandable.
5. Preserve a self-contained local runtime and deliberate remote-source selection.

## Accessibility & Inclusion

Preserve the browser's existing keyboard access, visible focus, accessible control
names, text alternatives for chart values, scalable type, reduced-motion support,
and light/dark themes. Narrow layouts must retain access to all views and readable
tables. No additional accessibility certification or target standard was specified.
