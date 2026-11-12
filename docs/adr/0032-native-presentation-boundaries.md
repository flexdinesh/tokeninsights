# Native presentation boundaries and shared fixtures

Status: Accepted. Date: 12 November 2026. User directed porting the web
dashboard to TokenInsights Native element by element.

## Context

ADR 0030 added TokenInsights Native beside the webview desktop and named drift
as its main cost. Porting the dashboard means a second implementation of rules
the web keeps in TypeScript: number and date formatting, calendar ranges, query
transitions, status priority, columns, chart titles and filter ordering. Copies
of those rules drift silently unless both sides prove the same outputs.

## Decision

- **`packages/core/dashboardview`** is the pure Go presentation model: the
  semantic twin of the web rules above, mapping query state to the existing
  `analytics.Query` and facet requests. It imports core value types only, never
  MyGo, a product shell, a transport client or storage. A dependency guard
  enforces this. URL encoding stays web-only.
- **Shared fixtures** in `packages/contracts/fixtures/dashboard-view/*.json`
  pin both sides. Web tests and `dashboardview` tests read the same files; a
  rule change updates the fixture, both implementations and their tests
  together. Rules without a pure TypeScript function (status priority, columns,
  chart titles) are tested in Go against the web behavior.
- **Native UI** lives in `packages/desktop-native/internal`: `kit` (DESIGN.md
  primitives on MyGo widgets, reading theme tokens only), `icons` (embedded
  Lucide SVGs), `model` (per-window query, data and sync; I/O in workers) and
  `views` (one file per dashboard element).
- **Review**: a tester gallery renders kit and view states to PNGs on demand for
  manual parity review against web captures. No pixel goldens run in validation.
- **Preferences**: theme persists app-wide in MyGo `PathUserData`
  `preferences.json`. Hidden columns and chart metric are per window, in memory.

Accepted native differences: no backend CPU/RAM in the status popover; native
dialogs for startup instead of the web connection screen; a virtualized table
fetching 200-row pages on demand instead of a pager; native menu and shortcuts;
the MyGo inactive-window accent fade.

## Consequences

- Fixtures cover observable outputs, not layout. Visual parity remains manual.
- Go matches Intl output for advertised timezones (canonical IANA names and
  `UTC±HH:MM`). It does not accept Intl's case-insensitive or bare-offset names.
  Filter ordering uses CLDR collation from `golang.org/x/text`, which can trail
  the browser's ICU for rare scripts.
- No schema, storage, wire protocol, read API or accounting changes.
