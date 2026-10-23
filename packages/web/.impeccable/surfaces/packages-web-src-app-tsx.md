---
version: 1
slug: "packages-web-src-app-tsx"
primary_target: "src/App.tsx"
related_targets: ["src/styles.css","src/ui/tokens/palette.css","src/ui/tokens/semantic.css","src/ui/tokens/theme.css","src/ui/utilities.css","src/ui/index.ts","src/components/DashboardSidebar.tsx","src/components/SummaryCards.tsx","src/components/Filters.tsx","src/components/UsageChart.tsx","src/components/ResultsTable.tsx","src/components/DashboardHeader.tsx","src/components/PasswordLogin.tsx","src/components/SettingsPage.tsx","src/design/DesignCatalog.tsx","src/identity.tsx","src/account.ts","src/session.ts"]
---

# Browser dashboard — Quiet Blue / White First

Mode: Operate. Developers scan token usage, compare dimensions, narrow filters, and inspect exact rows. User chose White First and approved its neutral charcoal dark counterpart. Preserve existing canonical semantics, authenticated dataset isolation, URL state, recovery, server identity, keyboard access, and Go embedding.

## Direction contract

THESIS: A calm, mature analytics workspace gives navigation a stable home and lets usage data lead.

OWN-WORLD: White-first light surfaces, subtle neutral-gray fields and rules; neutral charcoal dark surfaces. Quiet blue belongs to interaction and chart ink. Locally bundled DM Sans, tabular sans numerals, restrained corners, generous spacing.

STORY: Choose an analytics view in the sidebar, scan the current-view header and dates, narrow horizontal filters, read shared totals, then compare the chart and exact table rows.

FIRST VIEWPORT: A 192px desktop sidebar with text wordmark and seven views; compact current-view/source/action header with a fixed-size local-only process/loading icon beside theme selection; status and local harness details on hover, focus, or tap; quick dates; wrapping filters; five readouts; 240px plot above the table. Small screens place wrapping navigation above content. Existing immediate filters and metric controls remain the signature interaction; focus is blue, sync rotation respects reduced motion.

FORM: User-selected Quiet Blue, refined and approved as White First in light and neutral charcoal in dark. Direction seed 09bee032; explicit user choice overrides assignment. Code-led implementation; approved conversation previews are critique references, not raster assets.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance.

## Implementation evidence

Desktop light/dark (1440×1100), mobile light/dark (390px), 200% text, model filter popover, Models and Repo captured from the production Go web command under `.impeccable/review/white-first/`. All captures opened and verified from document top, with reduced motion and complete loading. Data is synthetic conformance metadata from `packages/cli/testdata/conformance/sync-first-basic/source`, materialized by `pnpm dev:data`; no real user usage.

Initial independent review disposition: fix. Period labels now wrap without truncation at enlarged text sizes. Search fields keep the wrapper focus outline and suppress the nested input shadow. DESIGN.md and its sidecar match the single-weight brand and current primitives. Recaptures replace the same evidence paths. Final verdict disposition: ship; reviewer scored all three listed fixes resolved, with remaining clear. This verdict covers those fixes; the initial fidelity assessment stands.

Format, pinned-tool lint, 71 web unit tests, full Go/SQLite/PostgreSQL tests, production build, and 35 browser tests passed. Corrected build, lint, web unit tests, and all 35 browser tests pass. Pre-push verification is required before publication. Direct Go build and the real dashboard run with Node/npm/pnpm absent from PATH; the served font matches the committed WOFF2 bytes. Web shipping raster provenance scan reports zero missing; README screenshot embeds its synthetic source origin.

Header refinement: intro removed; current-view heading replaces the breadcrumb; fixed-size local icon opens compact process CPU/RAM, query/processing and harness details on hover, focus, or tap. Failure recovery stays beside saved usage. Production Go dashboard inspected at 1440px and 390px in light/dark with reduced motion; header accessibility audit found no violations. Browser checks verify stable local refresh geometry, keyboard/tap disclosure, and omission of local status in hosted mode.

Approved read API addition: optional local processUsage on status; hosted responses omit it. No storage, accounting, or authorization changes. Web composition owns the sampler; existing browser controller owns polling. New Brand and DashboardSidebar components own presentation only.

## Hosted identity extension

Scope: approved narrow extension of the existing Operate surface. Preserve White First, neutral charcoal, DM Sans, shared Button/Input primitives, fine section rules, and wrapping navigation. No new visual world or surface concept.

The hosted dashboard header adds a top-right profile icon labeled **Account settings**. It opens `/settings`, reusing the sidebar and compact header with **Return to usage**. Account, Password, and API keys form a single column of rule-separated sections. Forms have visible labels and bounded widths; key rows and actions wrap on small screens. Personal mode keeps automatic identity, omits account controls, and explains unavailability if `/settings` is opened directly.

The root App owns authentication recovery and the identity provider shared by usage and settings. Public authentication discovery supplies hosted password login and conditional signup. Login reuses the brand header and startup layout; signup adds username/password guidance. Submission disables fields and shows pending action text; failures retain entered username and offer retry. Session changes cancel and clear cached queries before another account's usage is shown. Account reads and mutations carry the expected dataset header; expired or changed sessions return to login.

Settings shows the username and sign-out action. Password change asks for current, new, and confirmed passwords, explains that all browser sessions end while API keys remain active, and returns to login with confirmation. Account/key loading failures expose retry actions; an empty key list explains the next action.

Key creation collects a name, sync or read-only access, and optional expiry. The successful form becomes a one-time secret view with **Copy key** and **Done**. Focus moves to the readonly secret; copy feedback is announced, with manual-copy recovery if clipboard access fails. **Done** removes the secret and returns focus to the key-name field. The list retains labels, access, creation/expiry dates, and explicit revoked/expired states; revoke actions show progress and disable revoked rows. These transitions reuse existing focus and status primitives rather than adding a modal or new card treatment.

Implementation evidence: source in the related identity targets; hosted login and settings captures at desktop/mobile widths, including the one-time-key view in dark mode, under ignored `.scratch/identity-preview/`. Captures are development evidence, not shipping assets. Final build recaptures, browser focus checks, and semantic assertions confirm focus on the revealed secret after creation and Key name after Done. Independent verdict: ship; the reviewer scored the listed focus fix resolved, with no visible regression. All 84 web unit tests and 40 real browser tests pass; mobile settings accessibility audit found no violations.

Unresolved questions: none.

## Design system consolidation

Scope: refinement of the existing surface; identity, copy and behavior preserved. Tokens are palette → semantic roles (`light-dark()`) → Tailwind mapping with default scales reset, all under `src/ui/tokens/`. `src/ui` primitives and patterns own appearance; features own layout. Unified states: selected is tint plus blue ink without border; focus is a 2px blue outline with 2px offset; headings are page 28, section 16, sub 14. Settings access uses the Radix Select. `/_design` is a development-only catalog; `src/ui/design-guard.test.ts` enforces the contract.

Evidence: the token commit captured pixel-identical to its base across 46 local and hosted captures (one 1/255 antialiasing difference). Later commits were reviewed side by side against that baseline. Captures live in ignored `packages/web/.scratch/visual/`.
