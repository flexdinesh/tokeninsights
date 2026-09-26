---
name: "TokenInsights — Graphite & Lime"
description: "Compact measurement workspace with graphite, lime feedback, and warm light surfaces."
colors:
  background: "#f5f6f1"
  dark-background: "#17191b"
  foreground: "#272d24"
  dark-foreground: "#edf0ee"
  card: "#fdfefb"
  dark-card: "#202326"
  popover: "#fdfefb"
  dark-popover: "#282c2e"
  primary: "#c5e85d"
  dark-primary: "#c5ee62"
  primary-foreground: "#25300f"
  dark-primary-foreground: "#1c260e"
  secondary: "#ecefe6"
  dark-secondary: "#292e2a"
  secondary-foreground: "#46513e"
  dark-secondary-foreground: "#c2c9be"
  muted: "#ecefe6"
  dark-muted: "#292e2a"
  muted-foreground: "#5d6657"
  dark-muted-foreground: "#afb6ad"
  accent: "#e7eecf"
  dark-accent: "#303b21"
  accent-foreground: "#507000"
  dark-accent-foreground: "#c5ee62"
  destructive: "oklch(0.56 0.22 27)"
  dark-destructive: "oklch(0.68 0.19 23)"
  destructive-foreground: "oklch(0.985 0 0)"
  dark-destructive-foreground: "oklch(0.985 0 0)"
  border: "#dce1d5"
  dark-border: "#393e39"
  input: "#828f75"
  dark-input: "#69745f"
  ring: "#507000"
  dark-ring: "#c5ee62"
  success: "oklch(0.49 0.14 155)"
  dark-success: "oklch(0.72 0.16 155)"
  warning: "oklch(0.55 0.14 75)"
  dark-warning: "oklch(0.78 0.15 80)"
  error-surface: "oklch(0.96 0.025 25)"
  dark-error-surface: "oklch(0.2 0.04 25)"
  chart-1: "#507000"
  dark-chart-1: "#c5ee62"
  chart-2: "#748068"
  dark-chart-2: "#9da991"
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
typography:
  readout:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.035em"
  headline:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "2.25rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.035em"
  title:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.0625rem"
    fontWeight: 600
    lineHeight: 1.5
    letterSpacing: "-0.015em"
  body:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 500
    lineHeight: 1.5
  numeric:
    fontFamily: "ui-monospace, 'SFMono-Regular', Consolas, monospace"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  sm: "0.25rem"
  md: "0.375rem"
  lg: "0.5rem"
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
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.primary-foreground}"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2rem"
  button-outline:
    backgroundColor: "{colors.background}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2rem"
  button-secondary:
    backgroundColor: "{colors.secondary}"
    textColor: "{colors.secondary-foreground}"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2rem"
  button-ghost:
    backgroundColor: "transparent"
    textColor: "{colors.foreground}"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2rem"
  button-destructive:
    backgroundColor: "{colors.destructive}"
    textColor: "#ffffff"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2rem"
  input-text:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2rem"
  navigation-selected:
    backgroundColor: "transparent"
    textColor: "{colors.accent-foreground}"
    rounded: "0"
    padding: "0.5rem 0.75rem"
  filter-chip:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-foreground}"
    rounded: "{rounded.sm}"
    padding: "0.25rem 0.5rem"
    height: "1.875rem"
  card:
    backgroundColor: "{colors.card}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.lg}"
  total-readout:
    backgroundColor: "transparent"
    textColor: "{colors.foreground}"
    typography: "{typography.readout}"
    padding: "0"
---

# Design System: TokenInsights

## Overview

**Creative North Star: "Graphite & Lime"**

Graphite & Lime is a compact measurement workspace: neutral graphite in dark mode, warm white in light mode, and crisp lime feedback. Fine rules, static readouts, and dense rows keep measurements close together.

System typography, tabular numerals, restrained motion, and explicit states support repeated inspection. Preserve the TokenInsights logo and offline assets; no remote fonts are needed.

This browser contract derives from [tokens.css](packages/web/src/tokens.css),
shared styles, and UI primitives. Frontmatter records light roles and dark
counterparts; explicit and system themes use the same roles.
Architecture remains in [docs/design.md](docs/design.md); terminal styling in
[packages/cli/DESIGN.md](packages/cli/DESIGN.md).

**Key Characteristics:**

- Graphite dark surfaces and warm light surfaces.
- Lime actions and selection; neutral readout values.
- Flat, rule-separated analytics with compact controls.
- Wrapping filters, scalable type, and visible keyboard focus.

## Colors

Warm, lightly green neutrals contrast with graphite and one lime action accent.

### Primary

- **Action lime**: primary fills with dark text in both themes.
- **Lime ink**: deeper light-theme selection, focus, and principal chart line;
  crisp lime in dark mode.
- **Lime tint**: selected quick dates, chart metrics, and filters.

### Secondary

- **Sage chart neutral**: the second chart series, distinct from action feedback.

### Tertiary

Category colors distinguish chart marks and matching drill-down labels.
Success, warning, destructive, and error-surface roles retain status meanings;
pair them with text or icons.

### Neutral

- **Warm workspace / Graphite workspace**: page backgrounds.
- **Warm white / Graphite surface**: header, fields, and ordinary containers.
- **Raised surface**: hover regions and overlays.
- **Main, supporting, and muted ink**: values, labels, and metadata.
- **Fine rule / Field edge**: region separation and stronger control boundaries.

**The Lime Roles Rule.** Use lime for actions, selection, focus, and the principal plotted series. Readout values remain neutral; category colors follow their corresponding labels.

## Typography

**Interface Font:** local system sans stack.
**Numeric Font:** local system monospace stack for dense numeric cells.

Readouts share one scale (24px at the default root size). Body and chart headings
use the body role (14px); labels and axes use label type (13px). Section headings
use the title role (17px). Large headlines belong to connection/error screens;
the dashboard heading remains accessible but visually hidden.
The brand combines weight 650 with a lighter second word at 500.

**The Readable Measurement Rule.** Use tabular numerals and right-aligned numeric columns. Reflow controls before reducing type; retain the user's root font size and complete timeline dates.

## Layout

The dashboard caps at 100rem with 1rem gutters and shared header alignment.
The desktop header has a 3rem minimum height and sticks to the top while remaining
in flow. View tabs and quick dates share a wrapping row; horizontal filters wrap
below. Readouts, plot, and exact rows follow without a side rail or tall title.

Five readouts use a 1.25fr total and four equal columns, with 1rem gaps and
0.75rem vertical padding. At 45rem of readout-container width, they become two
columns with total spanning both. This reflow responds to enlarged root text.
Readouts have no individual surface or padding.

The plot is 10rem tall. Controls wrap; axes reserve 48px for date/category
labels, use automatic Y-axis width, and keep timeline endpoints complete.
The context legend sits outside the SVG and wraps. These are chart geometry,
not global spacing tokens.

At 55rem viewport width, tabs take the full row and sync progress uses two
columns. At 38rem, the header becomes a relative two-row grid: brand/theme/reload
above source/sync. Source name and URL stack, retaining width for the name.
Quick dates span the row, tabs scroll horizontally, date fields stack, and
chart controls wrap. Mobile/coarse-pointer controls grow to 2.75rem.
Widths and type follow the root font size; preserve reflow at 200% text.

Tables scroll within their own region, capped at 35rem, with sticky headings.
Dense rows use 0.25rem vertical padding and fine rules. Numeric cells align
right; the independent result summary describes all filtered results.

## Elevation & Depth

Surface tone and fine rules provide hierarchy. The readout bank, chart, and table
are flat workspace regions; ordinary sync containers retain subtle borders.
Overlays and chart tooltips share the theme-specific shadow in the sidecar.

**The Flat Workspace Rule.** Use fine rules and surface tone at rest. Reserve the shared shadow for overlays and chart tooltips.

State colors transition over 150ms; loading/sync indicators rotate over 1.2s.
Overlays use the existing fade/scale treatment. Charts do not animate data.
Reduced-motion preferences disable transitions and animation.

## Shapes

Small corners belong to compact controls, chips, badges, and checkbox shapes;
medium corners to fields and regular controls; large corners to ordinary cards
and overlays. Analytics regions and underline navigation remain square and flat.
Status dots are circular. Rules use 0.0625rem; focus strokes and offsets use
0.125rem, inset inside clipped navigation/table regions.

## Components

### Buttons

Primary, outline, secondary, ghost, and destructive variants share visible focus.
Regular minimum height is 2rem; compact is 1.875rem; large is 2.25rem.
Frontmatter heights describe minima. Compact controls use small corners.
Primary/destructive hover reduces fill to 90%; secondary to 80%; outline and ghost
use the accent tint. Disabled controls use 50% opacity. No hover movement.

### Inputs and overlays

Keep visible labels, field edges, native dates, and associated validation.
Date ranges wrap to preserve their full label. Search wrappers own their focus
outline; multiselect rows retain checkbox labels and search.
Popovers target 22rem width, scroll within available height, and use 8px anchor
offset with 16px collision clearance. Source selection retains hostname/URL
disambiguation and add/remove/recovery controls.

### Navigation and chips

View links use muted text and a lime underline plus stronger weight when active.
Hover uses a raised surface; focus remains visible in the scrolling row.
Quick dates and chart metrics expose pressed state with a lime tint.
Active filter chips include dimension, value, and remove action; long values
truncate within the chip while retaining their accessible name.

### Readouts and charts

Five static text groups share the same informational treatment. All values use
the readout role; captions and details use label type. Total detail is hidden on
narrow layouts while its accessible quantity remains.
The timeline uses a fine lime line and tinted area. Category charts share their
colors with labeled drill-down actions; context comparisons include an HTML legend.
Model/provider/harness shares appear above bars and in drill-down labels. Long
drill-down names wrap while their percentages remain visible at enlarged text sizes.

### Containers and tables

The shared Card primitive uses a subtle surface, fine border, and large corners.
Chart/table overrides create transparent, square, rule-separated analytics.
Tables retain sortable headings, column controls, row hover, independent scrolling,
and a wrapping full-result summary. Do not style static readouts as controls.

## Do's and Don'ts

### Do:

- **Do** preserve lime feedback in both themes, using the deeper light-theme ink for readable text and chart lines.
- **Do** use shared control sizes, visible focus, and enlarged touch targets.
- **Do** wrap filters and chart controls; keep every view reachable on narrow screens.
- **Do** keep source identity, complete timeline dates, and full-result summaries readable at enlarged text sizes.

### Don't:

- **Don't** add separate metric cards or selection borders to static readouts.
- **Don't** introduce a tall dashboard title block or side filter rail into this compact surface.
- **Don't** rely on color alone for selection, categories, or status.
- **Don't** add decorative chart animation, remote fonts, or arbitrary overlay shadows.
