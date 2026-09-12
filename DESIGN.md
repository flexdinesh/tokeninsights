# TokenInsights web visual language

This is the visual contract for the React application in `packages/web`, including
all six Aggregation Tabs. Read it before changing UI. The architecture and token
usage semantics remain in [`docs/design.md`](docs/design.md).

**Source of truth:** semantic values live in
[`packages/web/src/tokens.css`](packages/web/src/tokens.css), Tailwind exposes them as
utilities, and local shadcn primitives live in `packages/web/src/components/ui`.
Use those primitives and existing feature styles before adding a visual value.
Token values below are reference sizes at the browser's default 16px root; preserve
`rem`/`em` sizing and user font preferences.

## Design direction

- Calm: neutral surfaces and generous separation between sections.
- Clear: summary first, visualization second, detailed evidence third.
- Precise: aligned numbers, explicit labels, predictable controls.
- Restrained: emerald has a purpose; surfaces carry little decoration.
- Approachable: system sans typography and crisp corner rounding.
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
| Application canvas | `--background` | Page background; separates functional surfaces. |
| Primary surface | `--card`, `--popover` | Panels and floating layers. |
| Primary text | `--foreground` | Headings, values, entered text, important labels. |
| Primary action | `--primary`, `--primary-foreground` | One dominant action per group. |
| Secondary control | `--secondary`, `--secondary-foreground` | Quiet controls and grouped choices. |
| Supporting content | `--muted`, `--muted-foreground` | Metadata, captions, table headings, skeletons. |
| Selection / hover | `--accent`, `--accent-foreground` | Neutral persistent or interactive emphasis. |
| Divider / input | `--border`, `--input` | Hairlines and stronger control boundaries. |
| Focus | `--ring` | Two-pixel keyboard focus indicator. |
| Failure | `--destructive`, `--destructive-foreground`, `--error-surface` | Failed/destructive meaning and error banners. |
| Status | `--success`, `--warning` | Completed and caution meanings. |
| Data series | `--chart-1` through `--chart-12` | Chart marks only; independent from actions. |

Use one solid primary action per action group. Most surfaces remain neutral. Do not
color every metric or harness. Summary cards have equal visual weight. Selected controls combine accent treatment with a
border, underline, count, checkmark, or stronger weight.

Status must include text or an icon as well as color. A local-machine dot does not
prove successful sync; the explicit sync label/progress owns that information.
Warnings indicate caution, not failure. Errors must not use the warning palette.

Explicit light/dark and system theme use identical semantic roles. Keep both dark
definitions in `tokens.css` equivalent; never add component-local theme overrides.
Use primary/secondary text in tooltips and legends; series colors belong to marks.
Categorical model, provider, and harness bars use distinct palette colors, repeated
in their drill-down labels. Category identity remains available as text.
The existing chart-area fade may communicate area under the line; do not reuse it
as decoration. Values and series names must also be available in text/table form.

## Typography

Use `--font-sans`, the local system sans stack; no remote font dependency. Use
`--font-mono` only for dense numeric table cells or identifiers that need it.

| Style | Size token | Weight / line height | Use |
| --- | --- | --- | --- |
| Page title | `--text-3xl` (38), mobile `--text-2xl` (30) | Semibold / tight | One `h1` per page. |
| Summary value | `--text-2xl` (30) | Semibold / tight | High-level metric values, not section headings. |
| Brand | `--text-lg` (19), mobile `--text-base` (17) | Bold + medium / normal | Existing wordmark only. |
| Section heading | `--text-base` (17) | Semibold / normal | `h2`, chart and table titles. |
| Subsection heading | `--text-sm` (15) | Semibold / normal | `h3` and local group titles. |
| Body / controls | `--text-sm` (15) | Regular or medium / normal | Dashboard prose, controls, table identities. |
| Reading / mobile entry | `--text-base` (17) | Regular / normal | Longer explanatory content and mobile text/date entry. |
| Label / metadata | `--text-xs` (13) | Medium / regular, normal | Filters, captions, axis labels, numeric cells, status metadata. |

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

- Use the shared `.dashboard` container, capped by `--content-width` (90rem).
- Share `--page-gutter` between header and content: 24 desktop,
  16 at ≤38rem. Align header contents with the capped dashboard on ultrawide screens.
- Use `--reading-width` (42rem) for prose-heavy content; analytics use available width.
- Header minimum height is `--header-height` (64px). Let it grow when content wraps.
  It may remain sticky, but must stay in document flow. No sidebar is part of the current visual system.
- Preserve page order: heading/status → Aggregation Tabs and quick date ranges →
  filters → sync feedback → summaries → chart → table/summary → footer.
- Keep the source selector in the header beside source-aware status/actions. It must
  remain reachable when dashboard requests fail; never bury recovery inside failed content.
- Keep five equal-weight summary cards in one desktop row. Use three columns at
  ≤55rem and two at ≤38rem.
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

Use the local shadcn `Button` as the shared control foundation. Its CVA variants
express primary, outline, secondary, ghost, and destructive hierarchy; selected is
a state, not another product-specific variant. Feature classes may control layout,
not recreate the primitive.

Control sizes are shared across Button and Select: `sm` is 32px
(`--control-height-sm`), default is 36px (`--control-height-md`), and `lg` is 40px
(`--control-height-lg`). Icon variants use the same square dimensions. Use `sm` for
filter, chart, and segmented toolbars; default for header, form, table, and pagination
controls. Control icons use `--icon-size` (16px), independent of caption font size.
Keep icon style consistent with the existing Lucide outline set. Do not add per-page
density modes.

### Forms and menus

Use visible labels, shared field borders/radii/heights, and explicit descriptions
for errors. Search wrappers own the focus ring; do not remove their visible focus.
Date inputs retain platform behavior. Use local shadcn `Input`, `Select`, `Checkbox`,
and `Popover` primitives for other form controls. Checkbox labels provide the full
clickable row. Reuse `MultiSelect` and `DateFilter`; retain selected values, search,
Escape dismissal, and focus return.

The source selector is the browser's persistent server switcher, not a data-merging
filter. Use a visible hostname as its primary label and the normalized URL as
disambiguating metadata. Its add form accepts HTTP(S) URLs or bare `host:port`, has
a visible label, describes normalization, associates validation/connection errors
with the input, and saves only after the source validates. Removing a source needs
a clear target and must not make the page-origin source removable. Preserve current
dashboard filters and navigation when selection changes.

### Cards and panels

Use the local shadcn `Card` for overview metrics and chart/table/sync regions.
Feature classes may add layout and chart geometry without rebuilding the surface.
Do not wrap every heading, toolbar, or paragraph in a card. Share panel padding.
Only total receives accent-surface emphasis. Avoid fixed content heights that clip
translated text, long metadata, or enlarged fonts.

### Navigation and tables

Aggregation Tabs use text and icons, an accent underline, stronger selected weight,
and `aria-current`. The underline has square ends with no corner radius. Quick date
ranges and chart metrics use `aria-pressed`.
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
- ≤38rem: stack heading/status; use a two-column summary grid and a horizontally
  scrollable navigation rail so all six views remain discoverable. Stack date entry fields.
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

## Geist-inspired refresh (September 2026)

Geist is research input, not a dependency. TokenInsights owns its neutral palette,
system fonts, local shadcn source, and Tailwind theme. The refresh applies these
decisions:

- canonical shadcn semantic color roles in OKLCH;
- flat neutral surfaces, crisp hairlines, and overlay-only shadow;
- black/inverse primary actions with independent blue/violet/amber chart series;
- compact 32px desktop controls and 44px touch targets;
- equal-weight summary cards and a narrower 90rem dashboard measure;
- compact utility copy, noun-only analytics tabs, and a scrollable mobile tab rail;
- local `Button`, `Badge`, `Card`, `Input`, `Skeleton`, `Popover`, `Select`,
  `Checkbox`, `Alert`, and `Table` primitives using Tailwind utilities and Radix
  behavior where appropriate.

Comparison bars share the existing 64px maximum width so sparse Context results do
not expand into oversized blocks. Chart plot coordinates, measured identity-column widths, the 17rem chart viewport,
35rem table scroll cap, and Radix anchor positioning remain local functional geometry.
Their values are not invitations to add new spacing or typography scales.
