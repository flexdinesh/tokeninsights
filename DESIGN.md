# TokenInsights web visual language

This is the visual contract for the React application in `packages/web`, including
all six Aggregation Tabs. Read it before changing UI. The architecture and token
usage semantics remain in [`docs/design.md`](docs/design.md).

**Source of truth:** [`packages/web/src/tokens.css`](packages/web/src/tokens.css).
Use shared rules in `styles.css` and existing React components before adding styles.
Token values below are reference sizes at the browser's default 16px root; preserve
`rem`/`em` sizing and user font preferences.

## Design direction

- Calm: green-tinted neutrals and generous separation between sections.
- Clear: summary first, visualization second, detailed evidence third.
- Precise: aligned numbers, explicit labels, predictable controls.
- Restrained: emerald has a purpose; surfaces carry little decoration.
- Approachable: system sans typography and modest corner rounding.
- Cohesive: every Aggregation Tab shares the same dashboard structure.

Avoid decorative gradients, glowing cards, excessive pills, nested card stacks,
decorative shadows, faint essential text, and page-specific palettes. Do not turn
the analytics dashboard into a marketing page or reduce usability for minimalism.

## Design principles

1. Consistency over novelty. Meaningful differences earn variants; incidental ones do not.
2. Hierarchy before decoration. Use placement, size, weight, and whitespace first.
3. Spacing communicates grouping. Related controls sit closer than separate sections.
4. Semantic colors over arbitrary colors. Components must work in both themes.
5. Reuse shared components and styles before creating another implementation.
6. Keep analytics dense but readable. Never squeeze labels or targets to fit more data.
7. Responsive layouts preserve order, meaning, and access to every view.
8. Styling must not change filtering, aggregation, token semantics, or sync behavior.

## Color

| Role | Token | Rule |
| --- | --- | --- |
| Application canvas | `--color-bg` | Page background; separates functional surfaces. |
| Primary surface | `--color-surface` | Header, panels, controls, and overlays. |
| Secondary surface | `--color-surface-raised` | Table headings/summary, neutral hover, segmented-control track, skeletons. “Raised” is a tone, not a shadow. |
| Primary text | `--color-text-primary` | Headings, values, entered text, important labels. |
| Secondary text | `--color-text-secondary` | Descriptions, navigation, supporting labels. |
| Muted text | `--color-text-muted` | Metadata and captions, still readable. Never reduce its opacity. |
| Divider | `--color-border` | Nonessential panel outlines, separators, table rows. |
| Control boundary | `--color-border-strong` | Inputs, selects, neutral buttons, selected neutral segments. |
| Primary accent | `--color-accent` | Primary action, focus ring, active-view underline, primary data series. |
| Accent interaction | `--color-accent-hover`, `--color-accent-active` | Primary-button hover and press. |
| Accent text | `--color-accent-text` | Selected labels, links, highlighted total. |
| Accent surface | `--color-accent-soft` | Selected controls and the single total-summary emphasis. |
| On accent | `--color-on-accent` | Text/icons on a solid accent button. |
| Status | `--color-success`, `--color-warning`, `--color-error` | Completed, caution, failed/destructive meanings. |
| Error surface | `--color-error-soft` | Error banners; pair with error text and a clear recovery action. |
| Brand | `--color-brand-bg`, `--color-brand-fg` | Existing logo mark only. |
| Comparison series | `--color-chart-secondary`, `--color-chart-tertiary` | Median and maximum context series; never general UI accents. |

Use one solid primary action per action group. Most surfaces remain neutral. Do not
color every metric or harness. The total-summary tint is deliberate emphasis, not
a template for every card. Selected controls combine accent treatment with a
border, underline, count, checkmark, or stronger weight.

Status must include text or an icon as well as color. A local-machine dot does not
prove successful sync; the explicit sync label/progress owns that information.
Warnings indicate caution, not failure. Errors must not use the warning palette.

Explicit light/dark and system theme use identical semantic roles. Keep both dark
definitions in `tokens.css` equivalent; never add component-local theme overrides.
Use primary/secondary text in tooltips and legends; series colors belong to marks.
The existing chart-area fade may communicate area under the line; do not reuse it
as decoration. Values and series names must also be available in text/table form.

## Typography

Use `--font-sans`, the local system sans stack; no remote font dependency. Use
`--font-mono` only for dense numeric table cells or identifiers that need it.

| Style | Size token | Weight / line height | Use |
| --- | --- | --- | --- |
| Page title | `--text-3xl` (40), mobile `--text-2xl` (32) | Semibold / tight | One `h1` per page. |
| Summary value | `--text-2xl` (32) | Semibold / tight | High-level metric values, not section headings. |
| Brand | `--text-lg` (18), mobile `--text-base` (16) | Bold + medium / normal | Existing wordmark only. |
| Section heading | `--text-base` (16) | Semibold / normal | `h2`, chart and table titles. |
| Subsection heading | `--text-sm` (14) | Semibold / normal | `h3` and local group titles. |
| Body / controls | `--text-sm` (14) | Regular or medium / normal | Dashboard prose, controls, table identities. |
| Reading / mobile entry | `--text-base` (16) | Regular / normal | Longer explanatory content and mobile text/date entry. |
| Label / metadata | `--text-xs` (12) | Medium / regular, normal | Filters, captions, axis labels, numeric cells, status metadata. |

Use `--leading-tight` only for titles and large values; everything else uses
`--leading-normal`. Use `--tracking-tight` for titles/values/brand,
`--tracking-heading` for section headings, and `--tracking-label` only for short
uppercase kickers or the LOCAL badge. Body and ordinary labels use normal tracking.
Weights come from `--weight-medium`, `--weight-semibold`, `--weight-bold`, or the
regular default. Do not rely on browser-default heading margins or weights.

Use tabular numerals for metrics, counters, and tables. Right-align numeric columns.
Do not shrink metadata below `--text-xs`. Long identities may truncate when their
full value remains available; supporting descriptions may wrap. Add a new type
style only for a distinct semantic role that weight, color, and spacing cannot express.

## Spacing

Use the 4px-based scale: `--space-1/2/3/4/6/8/12/16` → 4/8/12/16/24/32/48/64.

| Relationship | Rule |
| --- | --- |
| Label to input; heading to caption | `--space-1` |
| Icon to label; related actions; heading to body | `--space-2` |
| Filter rows, card/grid gaps, field groups | `--space-3` |
| Form/overlay inset; panel-heading vertical inset | `--space-4` |
| Between filters, chart, and table | `--space-6` |
| After page heading and summary group | `--space-8` (heading may use `--space-6` on mobile) |
| Empty-state vertical breathing room | `--space-12` |
| Card/panel horizontal inset | `--panel-padding`: 24 desktop, 16 at ≤75rem |

Align panel titles, metric controls, notes, and table edges to the same panel inset.
Use padding inside a component, gap between siblings, and margins at section
boundaries. Avoid stacking multiple margins to approximate the scale.

Exceptions are for geometry, not general spacing: one-pixel rules, focus offsets,
circles, measured table-column limits, chart-library pixel coordinates, and
Radix's 8px anchor offset and 16px viewport collision inset. Document an exception's purpose where introduced.
Do not mint a global token for every library coordinate or content-specific width.

## Layout

- Use the shared `.dashboard` container, capped by `--content-width` (100rem).
- Share `--page-gutter` between header and content: 32 desktop, 24 at ≤75rem,
  16 at ≤38rem. Align header contents with the capped dashboard on ultrawide screens.
- Use `--reading-width` (42rem) for prose-heavy content; analytics use available width.
- Header minimum height is `--header-height` (72px). Let it grow when content wraps.
  It is not a fixed overlay. No sidebar is part of the current visual system.
- Preserve page order: heading/status → filters → sync feedback → summaries →
  Aggregation Tabs → chart → table/summary → footer.
- Keep the source selector in the header beside source-aware status/actions. It must
  remain reachable when dashboard requests fail; never bury recovery inside failed content.
- Keep five summary cards in one desktop row; give total modest extra width.
  Use three columns at ≤55rem and two at ≤38rem, with total spanning two columns.
- Use flex wrapping for action groups, `minmax(0, 1fr)` for equal grid columns, and
  `min-width: 0` on shrinkable content. Never hide document overflow to conceal bugs.
- Scroll wide tables inside `.table-scroll`; preserve numeric column readability.
  Table headers are sticky inside that viewport; full-result summaries remain outside it.
- Use normal flow for page content. Reserve absolute positioning for genuine
  overlays/accessibility utilities. Use `--layer-sticky`, `--layer-popover`, and
  `--layer-skip-link`; do not escalate arbitrary z-index values.

## Borders, radii, and elevation

- `--radius-sm` (4px): badges, chips, segmented choices, checkbox-option hover.
- `--radius-md` (8px): buttons, fields, logo mark, tooltips.
- `--radius-lg` (12px): panels, summary cards, popovers, alerts.
- Circular status marks use 50%; do not turn ordinary controls into pills.
- Use `--border-width` for rules; `--focus-width` for focus/active-view indicators.
- Use `--shadow-overlay` only for overlays/tooltips. Cards and selected segments
  have no shadow. Distinguish sections with whitespace before adding a border/card.

## Components

### Buttons and density

Use `.button`, `.icon-button`, and `.filter-button` as the shared control foundation.
Primary, neutral, and subtle express action hierarchy; selected is a state, not
another product-specific variant. Use `.text-button` for low-emphasis local actions.

Standard controls have a 36px minimum height (`--control-height`) and 12px horizontal
padding. Icon buttons are at least square. Compact segments, chips, and text actions
use 32px (`--control-height-compact`); pagination uses the standard size. Control
icons use `--icon-size` (16px), independent of caption font size. Keep icon style
consistent with the existing Lucide outline set. Do not add per-page density modes.

### Forms and menus

Use visible labels, shared field borders/radii/heights, and explicit descriptions
for errors. Search wrappers own the focus ring; do not remove their visible focus.
Native selects and date inputs retain platform behavior. Checkbox labels provide
the full clickable row. Reuse `MultiSelect`, `DateFilter`, and Radix Popover for
filters; retain selected values, search, Escape dismissal, and focus return.

The source selector is the browser's persistent server switcher, not a data-merging
filter. Use a visible hostname as its primary label and the normalized URL as
disambiguating metadata. Its add form accepts HTTP(S) URLs or bare `host:port`, has
a visible label, describes normalization, associates validation/connection errors
with the input, and saves only after the source validates. Removing a source needs
a clear target and must not make the page-origin source removable. Preserve current
dashboard filters and navigation when selection changes.

### Cards and panels

Use summary cards for overview metrics; `.panel` for chart/table or sync regions.
Do not wrap every heading, toolbar, or paragraph in a card. Share panel padding.
Only total receives accent-surface emphasis. Avoid fixed content heights that clip
translated text, long metadata, or enlarged fonts.

### Navigation and tables

Aggregation Tabs use text and icons, an accent underline, stronger selected weight,
and `aria-current`. Quick date ranges and chart metrics use `aria-pressed`.
Use navigation semantics rather than adding partial ARIA tab behavior.
Keep table identities left-aligned and numbers right-aligned. Sort state includes
direction icons and `aria-sort`. Use subtle row dividers and hover, not alternating
near-white shades. Keep summaries independent of pagination and scrolling.

### Overlays and status

Popovers use `--popover-width`, at least 16px viewport clearance, available-height scrolling,
surface background, and overlay shadow. A future modal must use an accessible
dialog primitive with a name, focus containment/return, and deliberate dismissal;
do not build a second ad hoc overlay system. There is no modal component today.
Feedback includes a readable message and recovery action. Do not imply completion
through green alone or replace useful sync detail with a spinner.

## Interaction states

| State | Expectation |
| --- | --- |
| Hover | Neutral surface change or text emphasis; primary uses accent hover. Never move layout. |
| Focus | Visible 2px accent outline with 2px offset. Inset rings in clipped table/tab viewports. Keep a ring around search wrappers. |
| Active / pressed | Primary uses accent active; standard controls emphasize their boundary. No scale/bounce effects. |
| Selected | Persistent border/underline, weight/check/count, and appropriate ARIA state. Must survive hover. |
| Disabled | Native disabled behavior, default cursor, `--opacity-disabled`; no hover response. |
| Loading | Stable placeholders, `aria-busy`/status text, explicit syncing label; never relabel stale metrics as a new filter result. |
| Error | Error color plus readable message/icon, associated field descriptions, and a usable retry/correction path. |

When the active source is unavailable, keep its hostname selected, keep add/remove
and source-switch actions enabled, and show a readable retry/recovery path. Never
silently fall back to another source or present cached data from one source under
another hostname.

Use `--duration-fast` for color/border transitions. Continuous rotation is reserved
for active loading. Honor reduced motion; state labels must work without animation.

## Responsive design

Keep the existing content-driven thresholds: **75rem / 55rem / 38rem**. These are
literal media-query values because CSS custom properties cannot drive media queries.

- ≤75rem: reduce shared gutters/insets; hide redundant quick date shortcuts while
  keeping the full Date Range Filter.
- ≤55rem: collapse summary/sync columns; hide secondary header metadata; wrap table tools.
- ≤38rem: stack heading/status; use a two-column summary grid and a visible 3×2
  navigation grid so all six views remain discoverable. Stack date entry fields.
- At ≤38rem **or** with a coarse pointer, standard and compact targets are at least
  44px tall; icon-only controls are at least 44px wide. Checkbox labels own the target.
- Allow the header to wrap on very narrow screens. Keep Sync, Reload, and theme
  actions reachable. Keep the source selector reachable; its trigger may truncate
  the hostname when the full hostname and URL remain available in the popover.
  Hidden icon-button text still needs an accessible name.
- Preserve DOM/reading order. Reflow rather than CSS-ordering unrelated sections.
- Tables may scroll horizontally; the document must not. Long status/count text
  wraps. Never hide metrics or shrink typography just to force a desktop grid to fit.
- Check 320px and 390px widths, tablet, desktop, ultrawide alignment, and enlarged text.

## Accessibility

- Meet WCAG AA: at least 4.5:1 for ordinary text, 3:1 for large text, and 3:1 for
  necessary control boundaries, focus indicators, and meaningful chart marks.
  Decorative panel dividers need not carry control-level contrast.
- Check actual foreground/background pairs in light, dark, hover, and selected states.
- Keep keyboard navigation, skip link, focus visibility, Escape dismissal, and focus
  return intact. Never make a hover-only interaction the sole way to access data.
- Use visible labels where possible and accessible names for icon-only actions.
  Associate validation messages with fields; use `aria-invalid` for invalid inputs.
- Give the source selector a persistent accessible name, expose its selected source,
  announce connection validation/failure without moving focus, and return focus after
  its popover closes or a source is removed.
- Preserve browser zoom and scalable type. Mobile text/date entry uses readable
  16px-equivalent text. Do not disable zoom in viewport metadata.
- Desktop targets must exceed the 24px WCAG minimum; use the system's 32/36px sizes.
  Small-screen/coarse-pointer targets use 44px minimums.
- Communicate selection, sorting, loading, success, and error without color alone.
  Charts supplement the exact table data and keyboard-accessible filter labels.

## Rules for future UI work

> Do not introduce a new color, font size, spacing value, radius, shadow, or component
> variant unless the existing system cannot express the required design meaning.

Before introducing a visual value or pattern, ask:

1. Does a token already exist?
2. Does an existing component/shared style solve this?
3. Is this a new visual pattern?
4. Is the difference meaningful or incidental?
5. Does it remain coherent on mobile, with long data, and at enlarged text sizes?
6. Does it follow `DESIGN.md`, both themes, and keyboard interaction rules?

If an addition is necessary, explain the semantic role, update tokens and this
document together, and review every affected shared component. Do not patch the
generated assets directly; rebuild the embedded web application. Verify all six
views, selected filters, empty/loading/error states, popovers, and responsive layouts.

## Consolidation audit (September 2026)

The starting direction is retained: green neutrals, forest logo, emerald actions,
large summary numbers, and chart/table panels. Source audit covered `tokens.css`,
`styles.css`, `App.tsx`, and all four component files; rendered review covered the
fixture-backed light/dark dashboard and mobile layout.

| Original inconsistency | Consolidation |
| --- | --- |
| Light canvas `#f7f9f8`, surface `#fff`, hover `#f0f4f2`, table head `#f8faf9`, stripe `#fcfdfc` | Keep canvas/surface/secondary surface; remove separate header/stripe neutrals. |
| Accent soft `#edf8f1` vs total `#f1f9f4`; borders `#cce7d6` vs `#cde4d6` | One accent soft surface; selected boundaries use accent text. |
| One muted color `#687970` for all supporting content; faint control boundary `#b8c9c0` | Explicit secondary/muted text; stronger control border, equivalent dark roles. |
| Error palette was warning-orange; blue/purple had decorative names | Separate warning/error roles; semantic chart comparison colors. |
| Eight sizes: 11/12/14/16/18/28/34/40px; chart labels also 11.2px | Six sizes: 12/14/16/18/32/40px; chart axes and metadata share caption. |
| Optional undeclared Inter; several tracking values from −.045em to .13em | System font stack, three tracking roles, existing 500/600/700 weights and 1.15/1.5 line heights. |
| Gaps/padding around .3/.35/.4/.45/.55/.6/.65/.7/.8/.85/.9/1.1/1.2/1.3rem | Shared 4px scale; consistent action gaps and panel/table insets. |
| Page top 2.6rem; gutters 2.5/1.5/1rem; header 4.7rem | 32px top rhythm, shared 32/24/16 gutters, 72px growing header; retain 100rem content cap. |
| Radii .25/.3/.35/.4/.45/.55/.6/.7rem across badges, controls, tooltip, cards | Three roles: 4/8/12px. |
| Decorative total-card blur, selected-segment shadow, popover shadow | Remove decorative elevation; one overlay shadow. |
| Buttons 2.25rem, filters 2.2rem, pagination 1.75rem, selects sized by padding | Shared 36px standard / 32px compact / 44px touch targets. |
| Existing 75/55/38rem thresholds, clipped sixth mobile tab, hidden Reload label | Retain thresholds; visible mobile navigation and named icon-only Reload. |

Comparison bars share the existing 64px maximum width so sparse Context results do
not expand into oversized blocks. Chart plot coordinates, measured identity-column widths, the 17rem chart viewport,
35rem table scroll cap, and Radix anchor positioning remain local functional geometry.
Their values are not invitations to add new spacing or typography scales.
