---
name: "TokenInsights — Signal Studio"
description: "Browser measurement workspace with sky selection and neutral readouts."
colors:
  background: "#eef4f9"
  dark-background: "#101b2c"
  foreground: "#1c3048"
  dark-foreground: "#e5eff8"
  card: "#ffffff"
  dark-card: "#17263a"
  popover: "#ffffff"
  dark-popover: "#1c2e45"
  primary: "#12628f"
  dark-primary: "#83cefa"
  primary-foreground: "#ffffff"
  dark-primary-foreground: "#10273b"
  secondary: "#e7f0f8"
  dark-secondary: "#20344c"
  secondary-foreground: "#304c67"
  dark-secondary-foreground: "#d4e3f0"
  muted: "#e7f0f8"
  dark-muted: "#20344c"
  muted-foreground: "#55687e"
  dark-muted-foreground: "#adbdd0"
  accent: "#ddeffc"
  dark-accent: "#263f59"
  accent-foreground: "#105d8b"
  dark-accent-foreground: "#9bdafd"
  destructive: "oklch(0.56 0.22 27)"
  dark-destructive: "oklch(0.68 0.19 23)"
  destructive-foreground: "oklch(0.985 0 0)"
  dark-destructive-foreground: "oklch(0.985 0 0)"
  border: "#d9e3ed"
  dark-border: "#31455d"
  input: "#8298ae"
  dark-input: "#68819c"
  ring: "#12628f"
  dark-ring: "#83cefa"
  success: "oklch(0.49 0.14 155)"
  dark-success: "oklch(0.72 0.16 155)"
  warning: "oklch(0.55 0.14 75)"
  dark-warning: "oklch(0.78 0.15 80)"
  error-surface: "oklch(0.96 0.025 25)"
  dark-error-surface: "oklch(0.2 0.04 25)"
  chart-1: "#147cab"
  dark-chart-1: "#66c6f1"
  chart-2: "#677d99"
  dark-chart-2: "#9aacc4"
  chart-3: "oklch(0.67 0.16 65)"
  dark-chart-3: "oklch(0.76 0.15 70)"
  chart-4: "oklch(0.58 0.16 165)"
  dark-chart-4: "oklch(0.72 0.14 165)"
  chart-5: "oklch(0.6 0.2 25)"
  dark-chart-5: "oklch(0.72 0.18 25)"
  chart-6: "oklch(0.62 0.18 330)"
  dark-chart-6: "oklch(0.74 0.16 330)"
  chart-7: "oklch(0.55 0.14 200)"
  dark-chart-7: "oklch(0.72 0.13 200)"
  chart-8: "oklch(0.63 0.16 125)"
  dark-chart-8: "oklch(0.75 0.14 125)"
  chart-9: "oklch(0.61 0.15 90)"
  dark-chart-9: "oklch(0.78 0.14 90)"
  chart-10: "oklch(0.54 0.16 225)"
  dark-chart-10: "oklch(0.7 0.15 225)"
  chart-11: "oklch(0.58 0.15 355)"
  dark-chart-11: "oklch(0.73 0.14 355)"
  chart-12: "oklch(0.54 0.11 45)"
  dark-chart-12: "oklch(0.7 0.1 45)"
  rail-surface: "#e5eef6"
  dark-rail-surface: "#152238"
typography:
  readout:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.875rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.035em"
  headline:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "2.25rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.035em"
  metric:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.035em"
  title:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.1875rem"
    fontWeight: 600
    lineHeight: 1.5
    letterSpacing: "-0.015em"
  section:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.0625rem"
    fontWeight: 600
    lineHeight: 1.5
    letterSpacing: "-0.015em"
  body:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "0.9375rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 500
    lineHeight: 1.5
  numeric:
    fontFamily: "ui-monospace, 'SFMono-Regular', Consolas, monospace"
    fontSize: "0.9375rem"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  sm: "0.25rem"
  md: "0.5rem"
  lg: "1rem"
spacing:
  1: "0.25rem"
  2: "0.5rem"
  3: "0.75rem"
  4: "1rem"
  6: "1.5rem"
  8: "2rem"
  12: "3rem"
  16: "4rem"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.primary-foreground}"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2.25rem"
  button-outline:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2.25rem"
  button-secondary:
    backgroundColor: "{colors.secondary}"
    textColor: "{colors.secondary-foreground}"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2.25rem"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.foreground}"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2.25rem"
  button-destructive:
    backgroundColor: "{colors.destructive}"
    textColor: "{colors.destructive-foreground}"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2.25rem"
  input-text:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2.25rem"
  navigation-selected:
    backgroundColor: "{colors.card}"
    textColor: "{colors.accent-foreground}"
    rounded: "{rounded.sm}"
    padding: "0.5rem 1rem"
  filter-chip:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-foreground}"
    rounded: "{rounded.sm}"
    padding: "0.25rem 0.5rem"
    height: "2rem"
  card:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.lg}"
  total-readout:
    backgroundColor: "transparent"
    textColor: "{colors.foreground}"
    padding: "0"
---

# Design System: TokenInsights

## Overview

**Creative North Star: "Signal Studio"**

Signal Studio treats usage as a coordinated bank of readouts and a plot. Pale blue surroundings, a white instrument surface, navy readouts and sky controls make dense measurements approachable. Dark mode preserves those roles on ink-blue surfaces.

This is an Operate interface: readable local system typography, tabular numerals, restrained motion, and explicit control states support repeated inspection. Use the existing TokenInsights logo and embedded assets; the browser must work offline without remote fonts.

This contract covers `packages/web`. Architecture and token semantics remain in
[docs/design.md](docs/design.md); terminal styling remains in
[packages/cli/DESIGN.md](packages/cli/DESIGN.md). The normative implementation is
[packages/web/src/tokens.css](packages/web/src/tokens.css), shared styles, and local
UI primitives. Frontmatter records light roles plus `dark-` counterparts; code
switches the same CSS roles for explicit or system dark mode.

**Key Characteristics:**

- One shared instrument surface for readouts and chart.
- Sky selection; neutral readouts; quiet blue surfaces.
- Readable tables, scalable type, and visible keyboard focus.
- Flat surfaces with rounded edges; depth reserved for overlays.

## Colors

Cool blue neutrals support clear sky interactions and calm, consistent readouts.

### Primary

- **Sky ink** (`primary`, `accent-foreground`, `ring`): primary buttons,
  selected controls, sort emphasis, and keyboard focus.
- **Pale sky** (`accent`): chart controls and active filters.
- **Plot sky** (`chart-1`): the principal timeline series, separate from action ink.

### Secondary

- **Slate blue** (`chart-2`): the secondary data series. Readout values use
  the foreground color; total emphasis comes from type size, not a separate hue.

### Tertiary

- `chart-3` through `chart-12` distinguish categories and context comparisons.
  Preserve text identities and matching drill-down marks.
- `success`, `warning`, `destructive`, and `error-surface` retain status
  meanings. Pair status color with a label or icon.

### Neutral

- **Blue workspace** (`background`) surrounds the **instrument surface**
  (`card`); `rail-surface` groups navigation and filters.
- **Navy text** (`foreground`) carries main values and labels;
  `secondary-foreground` and `muted-foreground` carry supporting information.
- `border` separates regions; `input` provides stronger control boundaries.
  `popover` supplies floating surfaces, including the raised dark treatment.

**The Signal Roles Rule.** Sky identifies controls and the principal plotted series. Category colors identify chart marks and their corresponding text labels, not unrelated controls.

## Typography

**Interface Font:** the local `font-sans` system stack.
**Numeric Font:** `font-mono` for dense numeric table cells.

System UI typography is intentional for this Operate surface. No remote font or
decorative display face is required. The frontmatter records the observed role
ramp; use the corresponding `text-*`, weight, leading, and tracking tokens.

- **Readout:** total quantity; other metrics use the metric role.
- **Headline:** page title, reduced to the metric size on narrow screens.
- **Title:** chart title and brand scale; the brand uses weight 650 with its
  secondary word at 500.
- **Section:** table and ordinary section headings; also readable mobile entry.
- **Body:** controls, descriptions, table identities.
- **Label:** metadata, filters, axes, and supporting readout explanations.

**The Readable Measurement Rule.** Use tabular numerals for readouts and right-aligned numeric columns. Reflow the workspace before reducing type; retain the user's root font size.

## Layout

The dashboard caps at 100rem with shared header/content alignment. Page gutters
are 2rem, reducing to 1.5rem at 75rem viewport width and 1rem at 38rem.
Panel inset is 1.5rem, reducing to 1rem at 75rem. Use the frontmatter spacing
scale for grouping; local chart and table geometry may remain component-specific.

Desktop composition: source/action header, page heading and quick dates, view
navigation, then results left and filters right. The results contain a shared
readout/chart surface followed by the table. The filter rail is 17rem, reducing
to 15rem at 75rem. The gap is 1.5rem, then 1rem.

The named dashboard container stacks filters above results at 65rem of available
container width. Its disclosure becomes visible there. The React disclosure's
initial open state and resize defaults use viewport `matchMedia('(min-width:
65rem)')`; this is distinct from the CSS container threshold. Preserve manual
disclosure interaction between threshold crossings.

The readout bank starts with a 1.25fr total and four equal remaining columns.
At 45rem of readout-container width it becomes two columns with total spanning
both. Container queries respond to enlarged root text as well as narrow space;
keep this reflow at 200% root font size.

At 55rem viewport width, navigation expands and sync cards become two columns.
At 38rem, header controls use two rows; quick dates span the width; navigation
scrolls horizontally; filter dimensions form two columns; date fields stack.
Touch/coarse-pointer control sizes rise to 2.75rem. Preserve every view and action.

The desktop header is sticky and remains in flow; the narrow header is relative.
Tables scroll within their own region (35rem maximum height), with sticky headings
and an independent full-result summary. The chart viewport is 17rem; automatic
Y-axis width, a 48px category-axis height, wrapping controls, and an HTML
context legend outside the SVG preserve enlarged-text readability. Do not turn
these plotting dimensions into global spacing tokens.

## Elevation & Depth

Surface tone, whitespace, and fine rules provide hierarchy. Readouts and chart
share one flat instrument surface. Navigation selection uses a surface and border;
filters use a quiet tinted rail. Overlays and chart tooltips alone use
`shadow-overlay`; its theme-specific values live in the sidecar.

**The Flat Instrument Rule.** Resting surfaces use tone and hairlines. Reserve the shared shadow for overlays and chart tooltips.

State colors transition over 150ms; active sync rotates over 1.2s. Popovers use the
existing primitive's fade/scale entry and exit. Charts do not animate their data.
Reduced-motion preferences disable animation and transitions.

## Shapes

Small corners belong to compact controls, chips, badges, and navigation items;
medium corners to fields and regular controls; large corners to instrument,
table, filter, and overlay surfaces. Readouts are plain text groups without
individual surfaces or selection borders. Status dots remain circular.

Rules are 0.0625rem; focus strokes are 0.125rem. Focus
offset is 0.125rem, inset where a clipped viewport requires it. Avoid arbitrary
pills, new shadow styles, or nested card borders without a functional reason.

## Components

### Buttons

Use the shared Button variants: primary, outline, secondary, ghost, destructive.
The regular control minimum height is 2.25rem; compact is 2rem; large is 2.5rem.
Frontmatter component heights describe these minima, not clipping heights.
Compact controls have small corners; regular controls have medium corners.
Primary/destructive hover reduces fill opacity to 90%; secondary uses 80%;
outline and ghost use the sky accent surface. Keyboard focus uses sky rings;
disabled controls use native disabled behavior and 50% opacity. No hover movement.

### Inputs and overlays

Keep visible labels, shared field borders, native date behavior, and associated
validation messages. Search wrappers own their focus outline. Multiselect rows
include checkboxes with full labels; maintain search, Escape dismissal, and focus
return. Popovers use a 22rem target width, available-height scrolling, 8px anchor
offset, and 16px collision clearance. Source selection remains in the header with
hostname and URL disambiguation, plus reachable add/remove/recovery controls.

### Navigation and chips

View links sit in a rounded tinted track; active links combine a white or dark
card surface, border, sky text, stronger weight, and `aria-current`.
Quick dates and chart metrics expose `aria-pressed`. Filter chips use sky ink,
tint, a border, a clear dimension/value label, and a remove action. Category
identity and selection must remain understandable without color.

### Instrument readouts

Five static readouts belong to one compact bank above the plot. Total uses
1.875rem type and the other values use 1.5rem. Each text group has a 0.25rem gap
and no internal padding; the bank supplies shared spacing. All views use the
same informational treatment, with no click, hover, tooltip, or selection state.
Chart metric selection belongs exclusively to the chart toolbar.

Readout values use tabular numerals, abbreviated display, and exact accessible
values. Supporting token composition and full synced-session coverage may wrap.
Narrow screens hide the total's short explanatory detail but retain the value and
label. Do not make absent metrics look interactive.

### Charts and tables

Keep timeline sky ink and its quantity-bearing area fade. Categorical marks use
distinct series colors repeated in drill-down labels. Context comparisons retain
their HTML legend below the SVG. Axis and tooltip text use readable semantic text
colors; charts supplement exact tables.

Tables use left-aligned identities, right-aligned numeric values, subtle dividers,
and a quiet hover surface. Sort state includes direction and `aria-sort`.
Wrapping controls, column selection, pagination, and full-filter summaries remain
accessible independently of horizontal scrolling.

### Feedback

Preserve explicit loading, empty, sync, and error text with usable recovery.
Skeletons retain region structure; errors use error surfaces and messages.
An unavailable source remains selected until the user changes it; never label
another source's data with that hostname.

## Do's and Don'ts

### Do:

- Do reuse semantic tokens and local shadcn primitives in both themes.
- Do keep the source selector and recovery actions reachable when requests fail.
- Do preserve readable type, wrapping labels, visible focus, and 44px-equivalent touch targets.
- Do keep exact values, session coverage, and chart meaning available as text.
- Do retain the existing logo and offline font/assets behavior.

### Don't:

- Don't reintroduce equal-weight standalone summary cards or a black/inverse primary palette.
- Don't make summary readouts interactive; chart metric selection belongs to the chart toolbar.
- Don't shrink typography or conceal document overflow to fit a desktop grid.
- Don't use decorative meters, glowing cards, or ornamental gradients; the chart-area fade represents plotted quantity.
- Don't let styling change token semantics, filtering, sync scope, or source identity.
