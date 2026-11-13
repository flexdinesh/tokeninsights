# Native presentation boundaries and shared fixtures

Status: Accepted. Date: 12 November 2026. User directed porting the web
dashboard to TokenInsights Native element by element.

## Context

ADR 0030 added TokenInsights Native beside the webview desktop and named drift
as its main cost. Porting the dashboard means a second implementation of rules
the web keeps in TypeScript: number and date formatting, calendar ranges, query
transitions, status priority, columns, chart titles and filter ordering. Copies
of those rules drift silently unless both sides prove the same outputs.

## Decision

- **`packages/core/dashboardview`** is the pure Go presentation model: the
  semantic twin of the web rules above, mapping query state to the existing
  `analytics.Query` and facet requests. It imports core value types only, never
  MyGo, a product shell, a transport client or storage. A dependency guard
  enforces this. URL encoding stays web-only.
- **Shared fixtures** in `packages/contracts/fixtures/dashboard-view/*.json`
  pin both sides. Web tests and `dashboardview` tests read the same files; a
  rule change updates the fixture, both implementations and their tests
  together. Rules without a pure TypeScript function (status priority, columns,
  chart titles) are tested in Go against the web behavior.
- **Native UI** lives in `packages/desktop-native/internal`: `kit` (DESIGN.md
  primitives on MyGo widgets, reading theme tokens only), `icons` (embedded
  Lucide SVGs), `model` (per-window query, data and sync; I/O in workers) and
  `views` (one file per dashboard element).
- **Review**: a tester gallery renders kit and view states to PNGs on demand for
  manual parity review against web captures. No pixel goldens run in validation.
- **Preferences**: theme persists app-wide in MyGo `PathUserData`
  `preferences.json` (versioned, written atomically). A missing, unreadable or
  unrecognized file starts with the system theme; an unrecognized file is never
  replaced or deleted. The header's theme cycle and the View menu radios set
  one choice, which native window chrome follows too. Hidden columns and chart
  metric are per window, in memory.

Accepted native differences: no backend CPU/RAM in the status popover; native
dialogs for startup instead of the web connection screen; a virtualized table
fetching 200-row pages on demand instead of a pager; native menu and shortcuts;
the MyGo inactive-window accent fade; the sidebar stays a rail at every
supported window width; the footer omits the web's static harness list, which
the backend instance does not report; a failed instance load shows the
connection alert, whose Retry Request reads the instance again, and reports
"Connection unavailable" in the header status; closing a window cancels its in-flight refresh. Each window reads
status and collector progress every 5 seconds only while it shows (MyGo
`WindowHidden`), reading status at once as it shows again, and does not
reread its instance on the first status read, which the web's bootstrap
invalidation does. Its model owns the poll timer and redraws only when what
the window shows changes (or the server's calendar day turns), so an idle
window draws nothing, where the web re-renders on every poll. A failed usage
read shows its alert at once rather than after the web's single automatic retry; only usage answered from another
snapshot than the status is read once more, after status is read again.
Usage of earlier filters is not kept, so returning to them shows the
loading skeleton until their read answers.
The results table is one virtualized MyGo table rather than the web's
pager and rows-per-page select: its query keeps Page 1 and a PageSize of
200 (the backend's maximum), and as the table's view nears the end of the
rows read it reads the next 200-row page of the same request and snapshot,
one at a time. A new query or revision reads the first page again; a page
of another revision is dropped and status is read again; a failed page keeps
the rows read and shows Retry Request in the table footer. The table scrolls
within its own region, capped at DESIGN.md's 35rem and at the window's page
height under its section header and footer, so the whole region fits as the
page scrolls to it. Its last column takes the room the others leave instead
of the web's proportional stretch. Columns resize by dragging (a double click
fits a column to its cells instead of resetting it) and move by dragging
their headings, as in Finder; their edges take no keyboard resizing. A tab
without Name (Context) keeps its last shown column.
The custom date bounds are picked from MyGo's calendar below each field
rather than typed: an empty bound shows Beginning or Now, as the date
filter's label reads it, and the calendar's Clear and Today buttons empty
a bound or choose the server's date, as a browser's date picker. The
calendar starts its weeks as the user's region does, while weekly periods
stay Monday to Sunday. Opening the date filter leaves the focus on its
trigger, from which Tab moves into the popover, rather than moving it to
the popover's first control.
The filter values a window offers stay while the facets of a changed query
or revision of the same snapshot are read, where the web offers only the
selected values until they answer; a snapshot identity change still drops
them. A failed facets read, as a failed usage read, shows its banner at once,
and facets answered from another snapshot are read once more after status.
A usage filter's options are a virtualized list, as the backend offers up
to 1,000 values per filter; long option names wrap instead of truncating with a
hover title. Facets are keyed on the facet request, so a sort change does not
reread them.
The usage chart is drawn with MyGo `ui/chart`. Its Y axis rounds to steps
of 1, 2 or 5 times a power of ten (0–300K where the web's five even ticks
read 0–260K). Hovering, or the arrows, Home and End once the plot has the
keyboard focus, select one row: a timeline shows a rule and point, bars
the hover surface behind their category, and the row's tooltip sits at
the plot's top beside it rather than following the pointer, its values
reading "Total: 257,000" rather than "Total : 257,000". X axis labels
that would overlap are dropped from the start, keeping the last, by the
width the window gives the plot, and a shortened category label shows its
whole name on hover.
Switching between usage and its excluded usage review gives the results
the focus and announces what they show, rather than keeping the web's
hidden status line beside them.
Every web tab is ported, Repo last. Returning to Repo keeps the grouping
the window last chose there (`dashboardview.Query.WithTab`), where the web's
address, which carries the grouping only on Repo, starts it at Repository
again; location filters are dropped on leaving Repo in both. An unknown
location's directories open from a MyGo disclosure, which assistive
technology reads as one rather than as a button, and grow into view with
their table row rather than appearing at once.

## Consequences

- Fixtures cover observable outputs, not layout. Visual parity remains manual.
- Go matches Intl output for advertised timezones (canonical IANA names and
  `UTC±HH:MM`). It does not accept Intl's case-insensitive or bare-offset names.
  Filter ordering uses CLDR collation from `golang.org/x/text`, which can trail
  the browser's ICU for rare scripts.
- No schema, storage, wire protocol, read API or accounting changes.
