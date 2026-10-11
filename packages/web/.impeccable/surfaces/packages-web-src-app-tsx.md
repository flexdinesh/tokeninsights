---
version: 1
slug: "packages-web-src-app-tsx"
primary_target: "packages/web/src/App.tsx"
related_targets: ["packages/web/src/styles.css","packages/web/src/tokens.css","packages/web/src/components/DashboardSidebar.tsx","packages/web/src/components/SummaryCards.tsx","packages/web/src/components/Filters.tsx","packages/web/src/components/UsageChart.tsx"]
---

# Browser dashboard — Quiet Blue / White First

Mode: Operate. Developers scan token usage, compare dimensions, narrow filters, and inspect exact rows. User chose White First and approved its neutral charcoal dark counterpart. Preserve existing canonical semantics, authenticated dataset isolation, URL state, recovery, server identity, keyboard access, and Go embedding.

## Direction contract

THESIS: A calm, mature analytics workspace gives navigation a stable home and lets usage data lead.

OWN-WORLD: White-first light surfaces, subtle neutral-gray fields and rules; neutral charcoal dark surfaces. Quiet blue belongs to interaction and chart ink. Locally bundled DM Sans, tabular sans numerals, restrained corners, generous spacing.

STORY: Choose an analytics view in the sidebar, scan the page heading and dates, narrow horizontal filters, read shared totals, then compare the chart and exact table rows.

FIRST VIEWPORT: A 192px desktop sidebar with existing logo and seven views; 64px source/status/action header; visible Token usage heading; quick dates; wrapping filters; five readouts; 240px plot above the table. Small screens place wrapping navigation above content. Existing immediate filters and metric controls remain the signature interaction; focus is blue, sync rotation respects reduced motion.

FORM: User-selected Quiet Blue, refined and approved as White First in light and neutral charcoal in dark. Direction seed 09bee032; explicit user choice overrides assignment. Code-led implementation; approved conversation previews are critique references, not raster assets.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance.

## Implementation evidence

Desktop light/dark (1440×1100), mobile light/dark (390px), 200% text, model filter popover, Models and Repo captured from the production Go web command under `.impeccable/review/white-first/`. All captures opened and verified from document top, with reduced motion and complete loading. Data is synthetic conformance metadata from `packages/cli/testdata/conformance/sync-first-basic/source`, materialized by `pnpm dev:data`; no real user usage.

Initial independent review disposition: fix. Period labels now wrap without truncation at enlarged text sizes. Search fields keep the wrapper focus outline and suppress the nested input shadow. DESIGN.md and its sidecar match the single-weight brand and current primitives. Recaptures replace the same evidence paths. Final verdict disposition: ship; reviewer scored all three listed fixes resolved, with remaining clear. This verdict covers those fixes; the initial fidelity assessment stands.

Format, pinned-tool lint, 71 web unit tests, full Go/SQLite/PostgreSQL tests, production build, and 35 browser tests passed. Corrected build, lint, web unit tests, and all 35 browser tests pass. Pre-push verification is required before publication. Direct Go build and the real dashboard run with Node/npm/pnpm absent from PATH; the served font matches the committed WOFF2 bytes. Web shipping raster provenance scan reports zero missing; README screenshot embeds its synthetic source origin.

No schema, API, storage, accounting, authorization, or state ownership changes. New Brand and DashboardSidebar components own presentation only.

Unresolved questions: none.
