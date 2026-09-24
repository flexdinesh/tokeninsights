---
version: 1
slug: "internal-cli-table-go"
primary_target: "internal/cli/table.go"
related_targets: ["internal/cli/render.go","internal/cli/theme.go"]
---

# TUI: Instrument desk

Mode: Operate. Target: packages/cli/internal/cli/table.go and the terminal renderer.
Confirmed: individual developers; complete visual redesign; full-screen priority;
Instrument desk selected; brief approved; code-first implementation.

Preserve canonical analytics, all six views, filters, session coverage, sync and
recovery behavior. No schema changes. Target 120×35 and larger; compact layouts
retain keyboard access, all views, and scrolling. Terminal font is user-owned.

## Direction contract

THESIS: A precision measurement desk for local usage. A spacious readout strip
and broad table make comparisons immediate; crowded chrome gives way to hierarchy.

OWN-WORLD: Terminal-owned transparent canvas and panels, sky blue focus, pink totals,
neutral data; inverse markers remain meaningful without color. Light
terminals receive an equivalent light palette. Sparse rules, consistent cell insets.

STORY: Choose a view and scope, read filtered totals, compare rows, adjust filters
without losing the visible dashboard. Coverage remains pinned below the table.

FIRST VIEWPORT: Brand and machine/sync status, compact six-view navigation,
scope controls, six token readouts, full-width table, coverage, concise shortcuts.
Context uses an explanatory readout instead of adding session peaks. Signature
interaction: right-side filter drawer, explicit Apply/Cancel, Escape restores
table focus. Terminal transitions are immediate; only real sync activity animates.

FORM: Instrument desk, grounded candidate 1, user-selected pick; seed 6f9d51fd.

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance

## Completion evidence

Implemented and documented in `packages/cli/DESIGN.md` and
`packages/cli/.impeccable/design.json`. Finish review: **ship**. The proposed
numeric-overflow finding was withdrawn after formatter verification; no unresolved
finish findings remain.

Reviewed captures: `.impeccable/review/{desktop-dark,desktop-light,compact-dark,wide-dark,filter-drawer,context,help,empty}.png`.
The repository README capture at `assets/tokeninsights-view-models.png` carries
embedded synthetic-fixture PTY provenance. Terminal styling is separate from the
unchanged browser contract in root `DESIGN.md`.

## Confirmed refinement

Terminal backgrounds must stay visible through ordinary surfaces; selection alone
uses a fill. Token readouts have equal top/bottom blank insets without reducing
table space. User selected sky + pink: sky blue marks interaction, pink marks totals, and
blue-tinted selection fills retain focus. Both light and dark terminals are supported.
