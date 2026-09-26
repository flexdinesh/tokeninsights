---
version: 1
slug: "packages-web-src-app-tsx"
primary_target: "packages/web/src/App.tsx"
related_targets: ["packages/web/src/styles.css","packages/web/src/tokens.css","packages/web/src/components/SummaryCards.tsx","packages/web/src/components/Filters.tsx","packages/web/src/components/UsageChart.tsx"]
---

# Browser dashboard — Graphite & Lime

Mode: Operate. Individual developers scan local token usage, compare dimensions, then inspect exact rows. User approved the paired dark/light previews and implementation. Preserve canonical semantics, source identity, filters, sync recovery, offline Go embedding, and keyboard access.

## Direction contract

THESIS: A compact measurement workspace puts the chart and table together in the first viewport.

OWN-WORLD: Neutral graphite with crisp lime in dark mode; warm white with deep lime text and chart ink in light mode. Bright lime action fills use dark text in both themes. System typography, tabular numerals, small corners, and fine rules.

STORY: Scan the shared readouts, read the timeline, narrow horizontal filters, then compare exact table rows.

FIRST VIEWPORT: A roughly 48px source/status/action header; route tabs and quick dates on one row; wrapping horizontal filters; five compact static readouts; a 160px plot and compact toolbar; dense table rows. No tall title block or filter rail. Mobile wraps controls with accessible touch targets. Signature interaction: existing immediate filter/metric selection uses lime feedback; sync alone rotates, with reduced-motion support.

FORM: User-selected Graphite & Mint, refined and approved as Graphite & Lime with matching light mode. Direction seed d483b2c9; explicit user choice overrides the assignment. Code-led; approved conversation previews are critique references.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Implementation evidence

Desktop header 48px; plot 160px; Tokens table begins near 448px at 1440px width. Both themes, all seven routes, 320px mobile, tablet, filter popover, and 200% text captured under `.impeccable/review/graphite-lime/`.

Reviewer disposition: ship; source-label overlap and enlarged endpoint dates both resolved. Verdict scope is those two fixes; earlier fidelity assessment stands. Format, lint, full tests, build, and eight browser tests passed. Direct Go build serves byte-identical embedded assets and API with Node/npm/pnpm absent from PATH; project TUI verified in PTY. Updated README screenshot uses synthetic data and embeds its origin; shipping raster scan reports zero missing provenance.

Root DESIGN.md and `.impeccable/design.json` now document the implemented palette, compact scales, responsive behavior, and existing primitives. Documentation validation passed.

Unresolved questions: none.
