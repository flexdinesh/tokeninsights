---
name: TokenInsights Terminal
description: Instrument desk for local token usky blue.
colors:
  inverse-light: "#FFFFFF"
  inverse-dark: "#1C1C24"
  brand-light: "#292B32"
  brand-dark: "#E7E7EE"
  accent-light: "#17669D"
  accent-dark: "#80CFFF"
  muted-light: "#60636F"
  muted-dark: "#AAAAB7"
  faint-light: "#A0A2AD"
  faint-dark: "#525460"
  text-light: "#292B32"
  text-dark: "#E7E7EE"
  total-light: "#A2306C"
  total-dark: "#F49AC2"
  danger-light: "#9C3530"
  danger-dark: "#F0A39B"
  selected-light: "#DFEAF3"
  selected-dark: "#273D4D"
typography:
  body:
    fontWeight: 400
  emphasis:
    fontWeight: 700
spacing:
  outer-inset: "1 cell"
  column-gap: "2 cells"
  panel-inset: "2 cells"
  control-gap: "3 cells"
  section-gap: "1 row"
components:
  canvas-dark:
    textColor: "{colors.text-dark}"
  canvas-light:
    textColor: "{colors.text-light}"
  active-tab-dark:
    backgroundColor: "{colors.accent-dark}"
    textColor: "{colors.inverse-dark}"
    typography: "{typography.emphasis}"
    padding: "0 rows 1 cell"
  active-tab-light:
    backgroundColor: "{colors.accent-light}"
    textColor: "{colors.inverse-light}"
    typography: "{typography.emphasis}"
    padding: "0 rows 1 cell"
  readout-dark:
    textColor: "{colors.text-dark}"
    typography: "{typography.emphasis}"
    height: "4 rows"
  readout-light:
    textColor: "{colors.text-light}"
    typography: "{typography.emphasis}"
    height: "4 rows"
  selected-row-dark:
    backgroundColor: "{colors.selected-dark}"
    textColor: "{colors.text-dark}"
  selected-row-light:
    backgroundColor: "{colors.selected-light}"
    textColor: "{colors.text-light}"
  drawer-dark:
    textColor: "{colors.text-dark}"
    width: "min(44 cells, terminal width)"
  drawer-light:
    textColor: "{colors.text-light}"
    width: "min(44 cells, terminal width)"
---

# Design System: TokenInsights Terminal

## Overview

**Creative North Star: "Instrument desk"**

A precision measurement desk for local usky blue. Quiet canvas, clear readouts and a
broad table make comparisons immediate. Sky blue marks interaction; pink marks
totals. The terminal owns font choice, font size and cell geometry.

This contract covers the Go terminal renderer under `internal/cli/`. The
repository-root `DESIGN.md` governs the separate browser interface. Tokens here
describe terminal cells, rows and ANSI emphasis, not browser CSS dimensions.
The light and dark palettes follow terminal theme detection.

**Key Characteristics:**

- Full-screen measurement surface with restrained rules.
- Spacious readouts above a dense, aligned table.
- Explicit keyboard focus and persistent scope and coverage.
- Flat panels; immediate transitions.

Source evidence: `internal/cli/theme.go`, `desk.go`, `render.go` and
`table.go`. Reviewed captures live under `.impeccable/review/`: desktop dark
and light, compact, wide, filter drawer, Context, help and empty. Direction and
approval history remain in `.impeccable/surfaces/internal-cli-table-go.md`.

## Colors

The terminal supplies every ordinary background. Neutral foregrounds adapt to
light and dark terminals; only active navigation and focused selections use fills. Frontmatter contains the normative values for both themes.

### Primary

- **Sky blue accent** (`accent-*`): active navigation, table headers, shortcut keys
  and busy sync indicators. Active navigation uses the contrasting `inverse-*` foreground.

### Secondary

- **Pink total** (`total-*`): total token readout, total column, summary total
  and completed sync status.
- **Warning red** (`danger-*`): failed sync status.

### Neutral

- **Inverse** (`inverse-*`): contrasting foreground on the active tab fill.
- **Background:** terminal-owned for the canvas, readouts, and drawers; no color token.
- **Brand** (`brand-*`): product name and messky blue titles.
- **Text** (`text-*`): dimensions, ordinary metrics and drawer content.
- **Muted** (`muted-*`): scope, readout labels, supporting dimensions, coverage
  and shortcut guidance.
- **Faint** (`faint-*`): dividers and separators.
- **Selected** (`selected-*`): focused table row and current drawer item.

**The Redundant Focus Rule.** Pair selection color with text markers: brackets
around the active tab, `>` on the focused row, and explicit drawer selection marks.

## Typography

Font family, pixel size, line height and glyph appearance belong to the user's
terminal. There is one cell-based type size. Frontmatter weights describe regular
and ANSI bold roles; they do not request a new terminal font.

- **Emphasis:** product name, active tab, table headers, readout values, context
  values, total values, drawer title, focused drawer item and shortcut keys.
- **Body:** ordinary cells, inactive tabs, descriptions and support text.
- **Hierarchy:** placement, whitespace, weight and semantic color establish
  prominence. Numbers align right; dimensions align left.

**The Terminal Type Rule.** Preserve the user's cell grid and font settings;
establish hierarchy through emphasis and alignment.

## Layout

The application fills the terminal. The dashboard reserves one cell on either
side; the leading cell also holds the focused-row marker. Table content width is
terminal width minus two cells. Columns use the shared two-cell gap.

The vertical sequence is status, navigation, scope controls, active filters,
readouts, table, coverage and shortcuts. The footer reserves four rows: spacer,
coverage, horizontal divider and shortcuts. Remaining height belongs to the
table, whose column header consumes one row. Multiline dimension values consume
their actual line count; the viewport clips oversized rows to retain footer space.

- At **30 rows or more**, add a blank row after status and navigation.
- Readouts appear at **24 rows or more** and **72 content cells or more**.
- Below **96 content cells**, shorten readout labels to Total, Cache R and Cache W.
- Below **72 terminal cells**, abbreviate tab labels to three characters.
- Readouts divide available content width into six cells of equal floor width;
  the final readout takes the remainder. Each has a two-cell left inset.
- The drawer is at most **44 cells** wide, anchored right, starting at row index
  **2**, with height equal to terminal height minus two rows. Its left rule
  consumes one cell; content uses two-cell insets.
- Shortcuts collapse when the full hint plus position cannot fit. Horizontal
  table scrolling retains access to columns beyond the viewport.

The chosen full-screen reference is 120 × 35 cells; this is a design target,
not an enforced minimum. Width thresholds distinguish terminal width from
content width.

## Elevation & Depth

Whitespace distinguishes readouts; rules distinguish drawers. These surfaces
remain transparent to the terminal background. A single
vertical rule separates a drawer; a single horizontal rule precedes shortcuts.
There are no shadows. Drawers overlay the right side of the current dashboard,
preserving visible context on the left.

Transitions are immediate. Only real sync activity animates: pending dots and
the ASCII spinner update every 120 ms. Loading data uses a textual table state.

**The Flat Desk Rule.** Use whitespace and sparse rules to separate surfaces;
keep the table free of decorative containers.

## Shapes

Rectangular terminal regions follow the cell grid. Navigation uses padded text;
the active tab gains brackets and an inverse fill. There are no rounded corners,
card silhouettes or decorative frames. Table rows share the canvas except the
focused row. Ellipses indicate truncated content; the table can scroll sideways.

## Components

### Status and navigation

The status line places the bold product name beside muted host and sync
information. Six numbered views remain available: Tokens, Models, Providers,
Harnesses, Sessions and Context. Inactive tabs use text on the canvas with one
cell of horizontal padding; the active tab uses bold inverse sky blue and brackets.
Use 1–6 directly or Tab / Shift+Tab to cycle.

### Scope controls

Inline sky blue shortcut keys precede the date, optional time bucket, sort and
Filters controls. Controls have three-cell gaps. The following muted row shows
active filters or the all-providers, all-models and all-harnesses scope.
Direct keys are d, g, s, f and p / m / h for the individual dimensions.

### Readout strip

One transparent four-row strip holds a blank row, labels, bold values, and a
blank row. Equal vertical insets center the content. Its leading blank replaces
the former external spacer, preserving table height.
Its six measurements are Total tokens, Input, Output, Reasoning, Cache read and
Cache write. Total receives pink emphasis. Values summarize the entire filtered
result, independently of visible rows. Loading or failed queries display an em
dash instead of a potentially misleading readout.

Context replaces the token strip with a three-line explanation of Session Peak
Context Load. It names average, median and maximum per session, states that
peaks are not additive, and identifies included and excluded components.

### Table and coverage

Sky blue bold headers identify columns and the sorted header carries a direction
marker. Numeric cells align right; ordinary token zeros render blank. Totals use
bold pink; context values use bold neutral text. Multi-value dimensions may span
lines. Column sizing considers the full filtered result to avoid jumping while
scrolling. Dimension columns shrink within bounds before horizontal overflow.

The focused row has selected fill and a leading `>`. Up/Down or j/k move focus;
PgUp/PgDn move a page. Left/Right scroll columns; Home/End reach the first/last
column. The footer exposes row position and horizontal position when relevant.

Coverage stays below the table and distinguishes sessions shown from sessions
synced. The summary total belongs to the filtered result. Context coverage does
not add session peaks.

### Drawers and help

Filters, individual filter values, date range, bucket, sort and keyboard help
share the same transparent right-side panel. Titles are bold; descriptions muted. List
capacity adapts to height; long lists show visible range and scroll guidance.

A focused item uses bold selected fill with `>`. Single-choice drawers mark the
current option with `*`; multi-select values use `[ ]` and `[x]`. Space toggles,
c clears selection, Enter applies, and Escape cancels draft changes and restores
table interaction. No selected values means all values. The filter menu uses
Enter Edit; help uses Escape Close. Help remains reachable with ? when compact
shortcuts omit individual commands.

### Loading, empty, error and sync states

Loading preserves table headers and shows Loading data. An empty filtered result
offers date/filter recovery; a database with no synced sessions directs the user
to sync and reopen. Query failure shows Could not load usky blue, error detail and
r Retry / q Quit; stale coverage is hidden. Filter-value failures appear within
the drawer with cancellation and reopen guidance.

Initial sync centers progress on the same canvas. Status text accompanies
pending dots, busy spinner, completed check, skipped dash and failed cross.
These are functional terminal status marks, not decorative glyph icons.

## Do's and Don'ts

### Do:

- **Do** preserve the terminal background and use adaptive foreground colors.
- **Do** preserve text markers alongside semantic color.
- **Do** keep filtered readouts, visible rows and session coverage distinct.
- **Do** retain keyboard access and scrolling when space contracts.
- **Do** show recovery actions within empty and failed states.

### Don't:

- **Don't** sum Session Peak Context Load into a total.
- **Don't** derive totals from abbreviated cell text or the visible viewport.
- **Don't** impose browser type scales, pixel breakpoints or font choices.
- **Don't** add decorative shadows, rounded cards or animation to this flat desk.

