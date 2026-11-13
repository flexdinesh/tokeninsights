---
name: "TokenInsights — Quiet Blue / White First"
description: "White and cool charcoal measurement workspace with Geist, restrained blue feedback, sharp corners and mild rules."
colors:
  surface: "#ffffff"
  dark-surface: "#121316"
  surface-raised: "#f7f7f7"
  dark-surface-raised: "#17181c"
  surface-overlay: "#ffffff"
  dark-surface-overlay: "#17181c"
  surface-field: "#ffffff"
  dark-surface-field: "#17181c"
  surface-rail: "#ffffff"
  dark-surface-rail: "#141519"
  surface-heading: "#ffffff"
  dark-surface-heading: "#17181c"
  surface-hover: "#f7f7f7"
  dark-surface-hover: "#17181c"
  ink: "#22252b"
  dark-ink: "#ecedf0"
  ink-muted: "#5f6269"
  dark-ink-muted: "#a2a5ad"
  ink-subtle: "#5f6269"
  dark-ink-subtle: "#a2a5ad"
  ink-hover: "#285bd4"
  dark-ink-hover: "#82aaff"
  line: "#e6e6e8"
  dark-line: "#2a2c31"
  line-strong: "#c9cacf"
  dark-line-strong: "#4a4c53"
  action: "#285bd4"
  dark-action: "#82aaff"
  action-ink: "#ffffff"
  dark-action-ink: "#121316"
  selected: "#eef2fc"
  dark-selected: "#1f2738"
  selected-ink: "#285bd4"
  dark-selected-ink: "#82aaff"
  focus: "#285bd4"
  dark-focus: "#82aaff"
  status-success: "#007e3b"
  dark-status-success: "#35c177"
  status-warning: "#9e6300"
  dark-status-warning: "#e9ab2b"
  status-danger: "#d8151e"
  dark-status-danger: "#f75c5e"
  status-danger-ink: "#ffffff"
  dark-status-danger-ink: "#121316"
  status-danger-surface: "#ffecea"
  dark-status-danger-surface: "#250e0d"
  chart-1: "#285bd4"
  dark-chart-1: "#82aaff"
  chart-2: "#b45c7f"
  dark-chart-2: "#e59aba"
  chart-3: "#d77b00"
  dark-chart-3: "#ed9e2f"
  chart-4: "#009664"
  dark-chart-4: "#2dc08e"
  chart-5: "#de3b3d"
  dark-chart-5: "#ff6c67"
  chart-6: "#bf56b9"
  dark-chart-6: "#e282da"
  chart-7: "#00868c"
  dark-chart-7: "#00bdc2"
  chart-8: "#739a00"
  dark-chart-8: "#9abd53"
  chart-9: "#a37d00"
  dark-chart-9: "#dab33a"
  chart-10: "#007ca4"
  dark-chart-10: "#00afe4"
  chart-11: "#bb4e7f"
  dark-chart-11: "#eb80ac"
  chart-12: "#a25834"
  dark-chart-12: "#d28a69"
typography:
  page:
    fontFamily: "'Geist', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.75rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.035em"
  readout:
    fontFamily: "'Geist', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.035em"
    fontFeature: "tnum"
  title:
    fontFamily: "'Geist', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.5
    letterSpacing: "-0.015em"
  body:
    fontFamily: "'Geist', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.4286
  label:
    fontFamily: "'Geist', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 400
    lineHeight: 1.3333
  mono:
    fontFamily: "ui-monospace, 'SFMono-Regular', Consolas, monospace"
    fontSize: "0.8125rem"
    fontWeight: 400
    lineHeight: 1.3333
rounded:
  sm: "0"
  md: "0"
  lg: "0"
spacing:
  "1": "0.25rem"
  "2": "0.5rem"
  "3": "0.75rem"
  "4": "1rem"
  "6": "1.5rem"
  "8": "2rem"
  "12": "3rem"
  "16": "4rem"
components:
  button-default:
    backgroundColor: "{colors.action}"
    textColor: "{colors.action-ink}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2rem"
  button-outline:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2rem"
  button-outline-hover:
    backgroundColor: "{colors.surface-hover}"
    textColor: "{colors.ink}"
  button-secondary:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2rem"
  button-quiet:
    backgroundColor: "transparent"
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2rem"
  button-link:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    padding: "0"
  button-destructive:
    backgroundColor: "{colors.status-danger}"
    textColor: "{colors.status-danger-ink}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2rem"
  button-compact:
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0 0.625rem"
    height: "1.875rem"
  control-selected:
    backgroundColor: "{colors.selected}"
    textColor: "{colors.selected-ink}"
  input-text:
    backgroundColor: "{colors.surface-field}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2rem"
  filter-chip:
    backgroundColor: "{colors.selected}"
    textColor: "{colors.selected-ink}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
    padding: "0 0.625rem"
    height: "1.875rem"
  navigation-item:
    backgroundColor: "transparent"
    textColor: "{colors.ink-muted}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2rem"
  navigation-item-selected:
    backgroundColor: "{colors.selected}"
    textColor: "{colors.selected-ink}"
  badge:
    backgroundColor: "{colors.surface-raised}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.label}"
    rounded: "{rounded.sm}"
  popover:
    backgroundColor: "{colors.surface-overlay}"
    textColor: "{colors.ink}"
    rounded: "{rounded.lg}"
    padding: "0.75rem"
    width: "22rem"
  table-heading:
    backgroundColor: "{colors.surface-heading}"
    textColor: "{colors.ink-muted}"
    typography: "{typography.label}"
    padding: "0 0.75rem"
  table-cell:
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    padding: "0.25rem 0.75rem"
    height: "2rem"
  total-readout:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    typography: "{typography.readout}"
    padding: "0"
---

# Design System: TokenInsights

## Overview

**Creative North Star: "Quiet Blue — White First"**

White First is a mature measurement workspace: full white surfaces, sharp corners,
mild full-width rules, precise typography, and restrained blue actions. Its dark
theme uses cool charcoal and soft white ink with the same hierarchy.

Locally bundled Geist, tabular numerals, and compact section spacing support
repeated inspection. Preserve the Split T mark and TokenInsights wordmark, offline
assets, explicit states, and existing analytical behavior.

This browser contract derives from the token layers and primitives in
[`packages/dashboard/src/ui/`](packages/dashboard/src/ui/). Frontmatter colors record each
semantic role's light value and its `dark-` counterpart, resolved through the
palette; the design guard test fails when they drift. Architecture remains in
[docs/design.md](docs/design.md); terminal styling in
[packages/cli/DESIGN.md](packages/cli/DESIGN.md).

**Key Characteristics:**

- Full white light surfaces and cool charcoal dark surfaces.
- Blue actions, selection, focus, and principal chart series.
- Sidebar navigation, a compact current-view heading, and analytics separated by full-width rules.
- Wrapping filters, scalable type, and visible keyboard focus.
- One component library owns appearance; features own layout.

### Token architecture

Three layers, each with one owner:

1. **Palette** (`ui/tokens/palette.css`): raw values only (`--palette-gray-600`,
   `--palette-blue-300`). Private; nothing outside the token files reads it.
2. **Semantic roles** (`ui/tokens/semantic.css`): the public contract. Each color
   role is written once as `light-dark(light, dark)`; `color-scheme` picks the
   side. System theme follows the OS; `data-theme` pins light or dark. Roles also
   cover space, strokes, focus, control heights, layout sizes, layers and motion.
3. **Tailwind mapping** (`ui/tokens/theme.css`): resets Tailwind's default color,
   type, weight, leading, tracking, radius, shadow, breakpoint and container
   scales, then exposes only TokenInsights tokens (`bg-surface`, `text-ink`,
   `border-line`, `text-label`, `rounded-md`, `max-compact:`).

Primitives (`ui/primitives/`) and patterns (`ui/patterns/`) are the only owners of
appearance and interaction state. Features import from `@/ui` and compose them
with Tailwind layout utilities. `styles.css` holds the font face, a layered
document base, and reduced motion only.

**The One Owner Rule.** A color, size or state lives in one token or one
primitive. Features never restyle a primitive's color, border or typography.

## Colors

Neutral surfaces carry the workspace; one quiet blue carries every interaction.

**The sRGB Rule.** Every color token is sRGB hex (`#rrggbb`), never `oklch()`
or another wide-gamut form. A token then looks the same on every screen, in the
browser and the native app, and the native app never switches a window to
wide-gamut frame buffers, which a color outside sRGB forces and which double
their memory. Tune hues inside sRGB by lightness, hue and chroma; both guards
reject any other form.

### Primary

- **Quiet Blue** (`#285bd4` light, `#82aaff` dark): `action`, `selected-ink`,
  `focus`, `ink-hover` and `chart-1`. Filled primary buttons use `action` with
  `action-ink`; selected controls use `selected-ink` on the `selected` tint;
  hovered link actions and table sort headings turn `ink-hover`.

### Tertiary

- **Category hues** (`chart-2`..`chart-12`): rose, orange, teal, scarlet,
  magenta, cyan, lime, yellow, azure, pink and brown, each with a lighter dark
  counterpart. They distinguish chart marks and matching drill-down swatches only.
- **Status** (`status-success`, `status-warning`, `status-danger`,
  `status-danger-surface`): retain their meanings; pair them with text or icons.

### Neutral

- **White workspace / Charcoal workspace** (`surface`): page backgrounds and analytics.
- **Raised and hover surfaces** (`surface-raised`, `surface-hover`): controls,
  row hover, skeletons and supporting regions.
- **Navigation rail** (`surface-rail`): white in light mode, slightly raised charcoal in dark mode.
- **Field and overlay surfaces** (`surface-field`, `surface-overlay`, `surface-heading`):
  inputs, popovers and table headings.
- **Main, muted and subtle ink** (`ink`, `ink-muted`, `ink-subtle`): values, labels, metadata.
- **Fine rule / Field edge** (`line`, `line-strong`): region separation and control boundaries.

**The Neutral Surface Rule.** Keep large surfaces white or neutral gray in light
mode and neutral charcoal in dark mode. Blue belongs to feedback and plotted
data; readout values remain neutral. The brand mark's single `action` token is the
only decorative exception.

**The Selection Rule.** Every selected state is the `selected` tint with
`selected-ink` and no border: navigation, segmented controls, date presets,
active filter triggers and chips. `aria-pressed`, `aria-current="page"` or the
Button `selected` prop drive it through one `selected:` variant, which keeps
its look while hovered. Hover is neutral (`surface-hover`, main ink) so it never
reads as selected.

## Typography

**Interface Font:** locally bundled variable Geist with system sans fallbacks.
**Numeric Font:** the same sans family with tabular numerals.
**Mono Font:** system monospace, reserved for code and paths.

### Hierarchy

- **Page** (600, 1.75rem, 1.15, -0.035em): page and startup headings (`Heading variant="page"`).
- **Readout** (600, 1.5rem, 1.15, -0.035em, tabular): the five usage readouts.
- **Section** (600, 1rem, 1.5, -0.015em): chart, table and settings section headings (`Heading` default).
- **Sub** (600, 0.875rem): empty states, alerts and catalog groups (`Heading variant="sub"`).
- **View** (600, 0.8125rem, subtle ink): the compact current-view heading in the header.
- **Body** (400, 0.875rem, 1.43): explanations, table cells and controls.
- **Label** (400, 0.8125rem, 1.33): captions, axes, compact controls and metadata.

The brand pairs the Split T mark (`Brand`; sources in `assets/brand/`) with one
weight (650) across the full TokenInsights name. Field
labels use body weight 500. Fields grow to the title size below 38rem so mobile
browsers do not zoom on focus.

**The Readable Measurement Rule.** Use tabular numerals and right-aligned numeric
columns. Reflow controls before reducing type; retain the user's root font size
and complete timeline dates.

## Layout

A sticky desktop navigation sidebar (12rem) anchors the workspace. It holds the
brand and all existing view links. The main column keeps a wrapping, sticky header
(2.75rem minimum height) for the current-view heading, machine identity, a fixed-size
status indicator beside theme selection. Horizontal wrapping filters and period
navigation share a toolbar immediately below the header. The date filter selects
Daily, Weekly, Monthly, Yearly, All time, or Custom. Previous/next arrows surround
a full date label; Next is disabled at the current period. All time and Custom
omit arrows. Only Custom reveals date inputs and Apply.

The spacing scale uses quarter-rem steps (`--spacing: 0.25rem`), with section gaps
of 0.75rem and desktop page gutters of 1rem. Below 38rem, gutters reduce to 0.75rem.
Content caps at 100rem; readouts, chart, and table align to shared gutters. Five
static readouts form a single rule-separated strip. At 45rem of readout-container
width they become two columns with the total spanning both. Readouts have no
individual cards or interactive selection treatment.

The plot is 12rem tall. Controls wrap; axes reserve room for complete date/category
labels and automatic Y-axis width. The context legend sits outside the SVG and
wraps. Chart geometry remains separate from global spacing tokens.

Breakpoints are named for behavior: `compact` (38rem), `nav` (55rem) and `wide`
(75rem), used as `max-compact:` and `max-nav:`. Below 55rem the sidebar becomes
a relative, wrapping top navigation region; all views remain visible. Below 38rem
the header becomes relative; the heading, truncated machine identity, status and
actions share a compact wrapping row. Period navigation spans its row, date fields
stack, and chart controls wrap. Mobile and coarse-pointer controls grow to 2.75rem.
Widths and type follow the root font size; preserve reflow at 200% text.

Tables scroll within their own region, capped at 35rem, with sticky headings.
Rows have a 2rem minimum height with 0.25rem vertical padding, fine rules, neutral
headings, and right-aligned numeric cells. The independent result summary describes
all filtered results. Headings and cells keep 0.75rem horizontal padding.

Named widths cap forms and pages: login 24rem, form 28rem, reading 42rem,
settings 62rem, content 100rem, prose 70ch.

## Elevation & Depth

Surface tone and fine rules provide hierarchy. The readout strip, chart, and table
are flat workspace regions.

### Shadow Vocabulary

- **Overlay** (`0 0.75rem 2rem`, black at 12% light / 45% dark): popovers, select
  menus and chart tooltips only.

**The Flat Workspace Rule.** Use fine rules and surface tone at rest. Reserve the
overlay shadow for floating layers.

State colors transition over 150ms; disclosures rotate over 180ms; loading
indicators rotate over 1.2s. Overlays use a fade/scale treatment. Charts do not
animate data. Reduced-motion preferences disable transitions and animation.

## Shapes

Every corner is square: controls, chips, badges, fields, overlays, alerts and
skeletons. The radius tokens stay as the single owner and resolve to 0. Status dots
are the one circular exception. Rules use 0.0625rem and stay low contrast.

**The Visible Focus Rule.** Every focusable element shows a 2px `focus` outline
with a 2px offset (`focus-ring`). Inside clipped regions (navigation, table
scroll, resize handles) the outline insets (`focus-ring-inset`); search wrappers
own their outline (`focus-ring-within`).

## Components

Refined and restrained: controls read as quiet tools until hovered or selected.

### Buttons

- **Variants:** `default` (filled `action`), `outline` (field edge on `surface`),
  `secondary` (`surface-raised`, muted ink), `ghost`, `quiet` (muted ink, no
  surface), `link` (main ink, blue underline on hover, no padding), `destructive`.
- **Sizes:** default 2rem body text; `compact` 1.875rem label text with small
  corners (filters, period arrows, chart metrics, chips, table controls); `lg`
  2.75rem; matching `icon`, `icon-compact` and `icon-lg` squares.
- **Hover / Focus:** outline, ghost and quiet take the neutral hover surface and
  main ink; link turns `ink-hover` blue with an underline; filled
  variants reduce to 90%; secondary to 80%. Disabled controls use 50% opacity.
  No hover movement. `type="button"` is the default.

### Segmented controls and chips

`SegmentedControl` renders a labelled group of `aria-pressed` buttons (chart
metrics, date presets, catalog theme). `Chip` shows an active filter's dimension,
truncated value and remove action, keeping the full value in its accessible name.

### Fields and overlays

`Field` pairs a visible label, optional description and associated error
(`aria-describedby`, `aria-invalid`, `role="alert"`). `Input` uses the field
surface and `line-strong` edge; invalid fields take the danger edge. `SearchField`
wraps an icon and text input and owns the focus outline. `Select` (Radix) and
`Checkbox`/`CheckboxOption` share the same edges and focus.

Popovers target 22rem width with 0.75rem padding, scroll within available
height, and use `PopoverHeader` for a title and optional link action. The header
shows the local machine hostname in single-process mode. A fixed-size status
icon (`StatusIcon`: ready, loading, attention) sits beside theme selection;
hover, keyboard focus, or tap reveals process CPU/RAM, query/processing status,
and capability-gated local harness details. Connection failures keep retry controls.

### Navigation

`Sidebar` and `SidebarItem` hold the brand, view links, workspace note and
actions. Links use muted ink, the selection tint with blue ink when current, and
the hover surface otherwise. `PageHeader` holds the current-view heading and
header actions separated by `HeaderDivider`.

### Readouts and charts

`ReadoutStrip` and `Readout` give five static text groups the same informational
treatment; captions and details use label type. Usage qualifications remain
visible on narrow layouts. Matching excluded usage gets a muted, wrapping notice
below the readouts with a ghost **Review excluded usage** action. The separate
review keeps explicit exclusion labels and **Back to usage**.

The timeline uses a fine blue line without an area fill. Category charts share
their colors with labeled drill-down link actions; context comparisons include an
HTML legend. Shares appear above bars, in tooltips and on drill-down labels. Long
drill-down names wrap while percentages remain visible at enlarged text sizes.

### Tables

`SectionHeader` titles a table with a count badge and actions (sort, direction,
columns). `TableScroll`, `Table`, `TableHead`/`TableCell` (`numeric` aligns and
tabulates), `TableSortButton` and `TableResizeHandle` provide sticky headings,
blue active sort, independent scrolling, keyboard resizing and row hover.
`EmptyState` explains empty results and offers recovery actions.

### Feedback

`Alert` (default or destructive) wraps title, description and an action.
`Skeleton` reserves layout while loading. `Spinner`, `StatusIcon` and `StatusDot`
communicate state alongside text.

### Catalog and guard

`/_design` renders every role, primitive and pattern in development only
(`pnpm --filter @tokeninsights/web dev`, then open `/_design?theme=dark`).
`design:capture` writes desktop and mobile captures in both themes to `.scratch/`.
`src/ui/design-guard.test.ts` fails on raw colors outside the palette, undefined
custom properties, arbitrary or reset-default Tailwind classes, Radix/cva imports
outside `src/ui`, catalog loading outside the DEV branch, frontmatter colors
that differ from the semantic roles, and color tokens that are not sRGB hex. The
native `internal/theme` tests check the same frontmatter and that every role is
inside sRGB.

## Do's and Don'ts

### Do:

- **Do** use blue for actions, selection, focus, and the principal plotted series.
- **Do** keep both themes neutral, with readable supporting ink.
- **Do** add a semantic role before adding a color, size or layer; reference it from a primitive.
- **Do** use shared control sizes, visible focus, and enlarged touch targets.
- **Do** wrap filters, navigation, and chart controls; keep every view reachable.
- **Do** keep source identity, complete dates, and result summaries readable at enlarged text sizes.

### Don't:

- **Don't** tint large workspace surfaces blue.
- **Don't** add separate metric cards or selection borders to static readouts.
- **Don't** rely on color alone for selection, categories, or status.
- **Don't** use palette variables, raw colors or arbitrary Tailwind values in features.
- **Don't** override a primitive's color, border or type from a feature; add a variant instead.
