---
name: "TokenInsights — Quiet Blue / White First"
description: "White and neutral charcoal measurement workspace with DM Sans, restrained blue feedback, and clear navigation."
colors:
  background: "#ffffff"
  dark-background: "#181818"
  foreground: "#22252b"
  dark-foreground: "#efefef"
  card: "#ffffff"
  dark-card: "#181818"
  popover: "#ffffff"
  dark-popover: "#222222"
  primary: "#285bd4"
  dark-primary: "#82aaff"
  primary-foreground: "#ffffff"
  dark-primary-foreground: "#181818"
  secondary: "#f7f7f7"
  dark-secondary: "#222222"
  secondary-foreground: "#64666d"
  dark-secondary-foreground: "#b7b7b7"
  muted: "#f7f7f7"
  dark-muted: "#222222"
  muted-foreground: "#64666d"
  dark-muted-foreground: "#aaaaaa"
  accent: "#f1f1f2"
  dark-accent: "#292929"
  accent-foreground: "#285bd4"
  dark-accent-foreground: "#82aaff"
  destructive: "oklch(0.56 0.22 27)"
  dark-destructive: "oklch(0.68 0.19 23)"
  destructive-foreground: "oklch(0.985 0 0)"
  dark-destructive-foreground: "oklch(0.985 0 0)"
  border: "#e3e3e5"
  dark-border: "#343434"
  input: "#929299"
  dark-input: "#666666"
  ring: "#285bd4"
  dark-ring: "#82aaff"
  success: "oklch(0.49 0.14 155)"
  dark-success: "oklch(0.72 0.16 155)"
  warning: "oklch(0.55 0.14 75)"
  dark-warning: "oklch(0.78 0.15 80)"
  error-surface: "oklch(0.96 0.025 25)"
  dark-error-surface: "oklch(0.2 0.04 25)"
  chart-1: "#285bd4"
  dark-chart-1: "#82aaff"
  chart-2: "#b45c7f"
  dark-chart-2: "#e59aba"
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
  rail-surface: "#ffffff"
  dark-rail-surface: "#1b1b1b"
  field-surface: "#ffffff"
  dark-field-surface: "#222222"
  table-heading: "#fafafa"
  dark-table-heading: "#222222"
typography:
  readout:
    fontFamily: "'DM Sans', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.035em"
  headline:
    fontFamily: "'DM Sans', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1.75rem"
    fontWeight: 600
    lineHeight: 1.15
    letterSpacing: "-0.035em"
  title:
    fontFamily: "'DM Sans', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.5
    letterSpacing: "-0.015em"
  body:
    fontFamily: "'DM Sans', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "'DM Sans', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "0.8125rem"
    fontWeight: 500
    lineHeight: 1.5
  numeric:
    fontFamily: "'DM Sans', ui-sans-serif, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  sm: "0.25rem"
  md: "0.375rem"
  lg: "0.75rem"
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
    textColor: "#ffffff"
    rounded: "{rounded.md}"
    padding: "0.375rem 0.75rem"
    height: "2.25rem"
  input-text:
    backgroundColor: "{colors.field-surface}"
    textColor: "{colors.foreground}"
    rounded: "{rounded.md}"
    padding: "0.25rem 0.75rem"
    height: "2.25rem"
  navigation-selected:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-foreground}"
    rounded: "{rounded.md}"
    padding: "0.5rem 0.75rem"
  filter-chip:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.accent-foreground}"
    rounded: "{rounded.sm}"
    padding: "0.25rem 0.5rem"
    height: "2.25rem"
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

**Creative North Star: "Quiet Blue — White First"**

White First is a mature measurement workspace: white surfaces, neutral gray
supporting regions, precise typography, and restrained blue actions. Its dark
theme uses neutral charcoal and soft white ink with the same hierarchy.

Locally bundled DM Sans, tabular numerals, and generous section spacing support
repeated inspection. Preserve the TokenInsights logo, offline assets, explicit
states, and existing analytical behavior.

This browser contract derives from [tokens.css](packages/web/src/tokens.css),
shared styles, and UI primitives. Frontmatter records light roles and dark
counterparts; explicit and system themes use the same roles. Architecture remains
in [docs/design.md](docs/design.md); terminal styling in
[packages/cli/DESIGN.md](packages/cli/DESIGN.md).

**Key Characteristics:**

- White light surfaces and neutral charcoal dark surfaces.
- Blue actions, selection, focus, and principal chart series.
- Sidebar navigation, a visible dashboard title, and rule-separated analytics.
- Wrapping filters, scalable type, and visible keyboard focus.

## Colors

### Primary

Blue provides action and selection feedback. Primary controls use a filled blue
surface with contrasting text; selected navigation, quick periods, chart metrics,
and filter chips use blue ink on a neutral gray surface. Focus and the principal
chart line share the blue ink role.

### Secondary

Neutral gray distinguishes supporting regions, hover states, controls, and table
headings. The secondary chart series remains distinct from the principal blue line.

### Tertiary

Category colors distinguish chart marks and matching drill-down labels.
Success, warning, destructive, and error-surface roles retain status meanings;
pair them with text or icons.

### Neutral

- **White workspace / Charcoal workspace**: page backgrounds and analytics.
- **Navigation surface**: white in light mode, slightly raised charcoal in dark mode.
- **Supporting surface**: neutral gray for controls, headings, and selected regions.
- **Main, supporting, and muted ink**: values, labels, and metadata.
- **Fine rule / Field edge**: region separation and stronger control boundaries.

**The Neutral Surface Rule.** Keep large surfaces white or neutral gray in light
mode and neutral charcoal in dark mode. Blue belongs to feedback and plotted
data; readout values remain neutral.

## Typography

**Interface Font:** locally bundled variable DM Sans with system sans fallbacks.
**Numeric Font:** the same sans family with tabular numerals; monospace is reserved
for code and paths.

Readouts share one scale (24px at the default root size). Body and chart headings
use the body role (14px); labels and axes use label type (13px). Section headings
use the title role (16px). The visible dashboard title uses the headline role
(28px). The brand uses one weight (650) across the full TokenInsights name.

**The Readable Measurement Rule.** Use tabular numerals and right-aligned numeric
columns. Reflow controls before reducing type; retain the user's root font size
and complete timeline dates.

## Layout

A sticky desktop navigation sidebar (12rem) anchors the workspace. It holds the
brand and all existing view links. The main column keeps a wrapping, sticky header
(4rem minimum height) for machine identity, progress, theme selection, and local
Reload, followed by a visible
dashboard title, quick periods, and horizontal wrapping filters.

The spacing scale uses quarter-rem steps, with section gaps of 1.5rem and desktop
page gutters of 2rem. Below 75rem, gutters reduce to 1rem. Content caps at 100rem;
readouts, chart, and table align to shared gutters. Five static readouts form a
single rule-separated strip. At 45rem of readout-container width they become two
columns with the total spanning both. Readouts have no individual cards or
interactive selection treatment.

The plot is 15rem tall. Controls wrap; axes reserve room for complete date/category
labels and automatic Y-axis width. The context legend sits outside the SVG and
wraps. Chart geometry remains separate from global spacing tokens.

Below 55rem viewport width, the sidebar becomes a relative, wrapping top navigation
region. All views remain visible. At 38rem, the header becomes a relative flex
column; status and actions stay visible and wrap. Quick periods span their row,
date fields stack, and chart controls wrap.
Mobile/coarse-pointer controls grow to 2.75rem. Widths and type follow the root
font size; preserve reflow at 200% text.

Tables scroll within their own region, capped at 35rem, with sticky headings.
Padded rows use fine rules, neutral headings, and right-aligned numeric cells.
The independent result summary describes all filtered results. Headings and cells
keep shared horizontal gutters, including outer edges.

Daily dates sit beside small status icons and wrap when text grows; complete
labels and check times remain accessible and available on hover. Source coverage is a muted, collapsed
disclosure below results; diagnostic counts and dates appear only when expanded.
Completion icons stay blank while awaiting a check or when their saved check time
predates the current sync. Actual pending, updating, and failure states remain visible.

## Elevation & Depth

Surface tone and fine rules provide hierarchy. The readout strip, chart, and table
are flat workspace regions; ordinary sync containers retain subtle borders.
Overlays and chart tooltips share the theme-specific shadow in the sidecar.

**The Flat Workspace Rule.** Use fine rules and surface tone at rest. Reserve the
shared shadow for overlays and chart tooltips.

State colors transition over 150ms; loading/sync indicators rotate over 1.2s.
Overlays use the existing fade/scale treatment. Charts do not animate data.
Reduced-motion preferences disable transitions and animation.

## Shapes

Small corners belong to compact controls, chips, badges, and checkbox shapes;
medium corners to fields and regular controls; large corners to ordinary cards
and overlays. Analytics regions remain square and flat. Status dots are circular.
Rules use 0.0625rem; focus strokes and offsets use 0.125rem, inset inside clipped
navigation/table regions.

## Components

### Buttons

Primary, outline, secondary, ghost, and destructive variants share visible focus.
Regular and compact controls have a 2.25rem minimum height; large controls use
2.75rem. Frontmatter heights describe minima. Compact controls use small corners.
Primary/destructive hover reduces fill to 90%; secondary to 80%; outline and ghost
use the neutral accent surface. Disabled controls use 50% opacity. No hover movement.

### Inputs and overlays

Keep visible labels, field edges, native dates, and associated validation.
Date ranges wrap to preserve their full label. Search wrappers own their focus
outline, with no nested input shadow; multiselect rows retain checkbox labels and search.

Popovers target 22rem width, scroll within available height, and use 8px anchor
offset with 16px collision clearance. The header shows the local machine hostname
in single-process mode; its accessible label distinguishes viewer identity from
historical producers. The page origin remains available in the footer. Local-only
Reload refreshes reads; hosted retains automatic refresh and request retry.
Collection/submission and processing progress use compact status text beside saved
data. Connection failures retain retry controls; usage/filter errors identify the
failed request without marking the entire server unavailable.

### Navigation and chips

View links use muted text, with blue ink and a neutral supporting surface when
active. Hover uses a supporting surface; focus remains visible. Quick periods and
chart metrics expose pressed state. Active filter chips include dimension, value,
and remove action; long values truncate while retaining their accessible name.

### Readouts and charts

Five static text groups share the same informational treatment. All values use
the readout role; captions and details use label type. Usage qualifications remain
visible on narrow layouts. The excluded-usage review keeps its exclusion
qualification explicit.

Ordinary usage is the primary analytical surface. Matching excluded usage gets a
muted, wrapping disclosure below the readouts with a **Review excluded usage** action.
The separate review begins with its heading, a plain explanation and **Back to usage**;
its total, chart, table and session coverage retain explicit exclusion labels. Both
views preserve filters, URL state and browser history. Empty results explain their
scope and offer filter recovery. Review transitions announce the scope and move
keyboard focus to the result region without scrolling.

The timeline uses a fine blue line without an area fill. Category charts share
their colors with labeled drill-down actions; context comparisons include an HTML
legend. Model/provider/harness/repo shares appear above bars and in tooltips;
dimension drill-down labels also show shares. Long drill-down names wrap while
percentages remain visible at enlarged text sizes.

### Containers and tables

The shared Card primitive uses a subtle surface, fine border, and large corners.
Chart/table overrides create transparent, square, rule-separated analytics.
Tables retain sortable headings, column controls, row hover, independent scrolling,
and a wrapping full-result summary. Do not style static readouts as controls.

## Do's and Don'ts

### Do:

- **Do** use blue for actions, selection, focus, and the principal plotted series.
- **Do** keep both themes neutral, with readable supporting ink.
- **Do** use shared control sizes, visible focus, and enlarged touch targets.
- **Do** wrap filters, navigation, and chart controls; keep every view reachable.
- **Do** keep source identity, complete dates, and result summaries readable at enlarged text sizes.

### Don't:

- **Don't** tint large workspace surfaces blue.
- **Don't** add separate metric cards or selection borders to static readouts.
- **Don't** rely on color alone for selection, categories, or status.
- **Don't** add decorative chart animation, remote font dependencies, or arbitrary overlay shadows.
