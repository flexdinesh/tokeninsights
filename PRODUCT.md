# TokenInsights

<!-- impeccable:product-schema 1 -->

Shared product context for CLI, TUI, browser and native desktop.
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

TokenInsights collects durable local harness data, processes it in the local backend,
and publishes session-centric facts to a canonical SQLite server. Every presentation consumes the same semantic application operations through transport adapters. Manual collection does not
require a separate authenticated harness export or realtime hooks.

## Operating Context

- One shared Go backend owns local storage, capture, processing and queries. CLI/TUI, web and native desktop attach as independent viewers. All can run together.
- Launching a viewer requests collection; overlapping requests coalesce. No periodic scanning or login service. Saved data stays usable during collection and processing. The backend stops after the final viewer disconnects and a reconnect grace.
- Go products run without host JavaScript tools. Browser assets are embedded in the backend/server. Prebuilt GitHub Release/prerelease products install through shell scripts; compiled outputs never enter Git.
- Native desktop initially supports local totals, date/dimension filters, table, progress and retry. Hosted URLs open in the system browser.
- `tokeninsights sync` retains its finite hosted submission workflow. --sync=false skips local launch capture, while processing resumes. Filters and TUI reload query only; browser status refreshes automatically. Errors preserve access to saved data.
- Local hostname labels identify the machine running the viewer, including before
  ingestion. They do not attribute imported history to that machine. Loading,
  connection/storage failures, and individual usage/filter errors remain distinct.
- Date ranges and dimension filters constrain displayed analytics, not which
  harnesses sync. Calendar grouping uses the serving machine's local time.
- The browser queries the server serving its page. Local `web` has no
  authentication, including non-loopback binds; hosted servers use password login
  and API keys. SSH users reach a loopback dashboard through a tunnel.
- Fresh `collector.sqlite` and `server.sqlite` have distinct roles. The old
  `tokeninsights.sqlite` stays untouched; previous commands/schema migrations
  are not supported. Local maintenance uses `data reprocess|wait` through the private owner contract.

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
- Schema changes require explicit approval. UI changes must preserve canonical
  analytics semantics and sync behavior.

## Brand Commitments

The product name is **TokenInsights**; the public command is `tokeninsights`.
Use the established domain terminology, the Split T mark and the TokenInsights
wordmark. Brand sources live in `assets/brand/`; the terminal stays text-only. The
incumbent browser identity is recorded separately in `DESIGN.md`.

## Evidence on Hand

- [README.md](README.md): current product description, commands, and screenshots.
- `assets/tokeninsights-web.png` and `assets/tokeninsights-tui.png`:
  committed browser and terminal captures; verify freshness before visual work.
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
