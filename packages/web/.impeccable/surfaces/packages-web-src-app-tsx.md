---
version: 1
slug: "packages-web-src-app-tsx"
primary_target: "packages/web/src/App.tsx"
related_targets: ["packages/web/src/styles.css","packages/web/src/tokens.css","packages/web/src/components/SummaryCards.tsx","packages/web/src/components/Filters.tsx","packages/web/src/components/UsageChart.tsx"]
---

# Browser dashboard — Signal Studio

Mode: Operate. User-approved redesign. Primary task: usage at a glance; secondary: existing filters and comparison tabs. Sessions remain accessible but secondary. Preserve all canonical semantics, source switching, sync, themes, keyboard access, and offline Go embedding.

## Direction contract

THESIS: A studio for reading usage signals, replacing five disconnected cards with one coordinated meter bank and plot.

OWN-WORLD: Pale blue workspace, white instrument surface, navy text, sky selection and chart ink, neutral readouts, and slate secondary series. Dark mode uses ink-blue surfaces with soft sky and slate. Rounded controls, tabular numerals, sparse rules; no ornamental meters.

STORY: Read quantity and composition, see the usage pattern, then narrow the range or dimensions and compare views.

FIRST VIEWPORT: Product/source header; visible page title and quick dates; horizontal view navigation; a wide left meter-bank/chart/table region and a narrow right filter rail. Total has typographic emphasis. Readouts are compact static text groups with no click, hover, tooltip, or selection state. Chart controls exclusively select the timeline measure. Small screens place compact filters before the readout; tables scroll within their own region. Motion is brief selection-color feedback and real sync rotation, reduced-motion safe.

FORM: Signal Studio, grounded candidate 1, user-selected pick; seed 14d7bae9. Code-first build. No approved comp.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Completion evidence

Implemented in the React browser app and rebuilt into the Go binary. Independent
finish review requested two fixes: enlarged-text reflow/chart spacing and the
singular harness label. Both scored resolved; verdict: ship at the scope of those
fixes. Required captures: repository-root `.impeccable/review/web/` (desktop light
and dark, mobile, narrow dark, tablet, wide, mobile filter, all six views, 200% text).
Format, lint, full tests, build, and seven Go-backed browser tests pass. Direct Go
build serves embedded assets and API with `PATH=/nonexistent`. Shipping screenshot
and existing logo carry embedded provenance. Root `DESIGN.md` and
`.impeccable/design.json` document the finished browser system.
