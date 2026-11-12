package views

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Results table texts, as the web's ResultsTable.
const (
	detailsSuffix        = " details"
	excludedPrefix       = "Excluded "
	sortLabel            = "Sort"
	sortByLabel          = "Sort by"
	sortAscendingLabel   = "Sort ascending"
	sortDescendingLabel  = "Sort descending"
	columnsLabel         = "Columns"
	visibleColumnsLabel  = "Visible columns"
	noMatches            = "No usage matches these filters"
	noExcludedMatches    = "No excluded usage matches these filters"
	includedScope        = "Your date range and filters apply to usage included in your total."
	excludedScope        = "Only usage excluded from your total appears here."
	widerRange           = " Try a wider date range"
	orClearFilters       = " or clear your filters"
	clearFiltersLabel    = "Clear filters"
	sessionsShown        = "Sessions"
	excludedSessions     = "Excluded sessions"
	shownSeparator       = "shown /"
	syncedSuffix         = "synced"
	acrossAllDates       = "across all dates"
	rowsSuffix           = "rows"
	totalLabel           = "Total"
	excludedTotalLabel   = "Excluded total"
	summaryDot           = "·"
	loadingRows          = "Loading more rows…"
	rowsFailedPrefix     = "More rows couldn’t load. "
	sessionsShownHint    = "Shown: distinct countable sessions matching all filters, across all pages. Synced: all countable sessions in this dataset, ignoring filters."
	excludedSessionsHint = "Shown: sessions with excluded usage matching all filters, across all pages. All dates: sessions with excluded usage in this dataset, ignoring filters."
)

// Table layout around its own scroll region.
const (
	// tablePrefetchRows is how near the end of the rows read the table's
	// view comes before it reads the next page.
	tablePrefetchRows = 50
	// tableMinRows keeps the heading and a row in view in a window too
	// short for more.
	tableMinRows = 2
	// tableSectionHeader and tableFooterHeight are the rows above and
	// below the table: the section header around its compact controls,
	// and the footer, as high as the web's around its pager.
	tableSectionHeader = theme.ControlCompact + 4*theme.Unit
	tableFooterHeight  = theme.ControlCompact + 2*theme.Unit
)

// tableState is a window's results table state, kept by its results
// region: each tab's table place and column widths, the hidden columns
// (per window, in memory, ADR 0032), the Columns popover, and the read
// whose rows the table last showed.
type tableState struct {
	lists       map[dashboardview.Tab]*ui.ListState
	hidden      []dashboardview.Sort
	columnsOpen bool
	shown       analytics.Query
}

func newTableState() tableState {
	return tableState{lists: map[dashboardview.Tab]*ui.ListState{}}
}

// list is tab's table state, kept while the window shows.
func (s *tableState) list(tab dashboardview.Tab) *ui.ListState {
	list, ok := s.lists[tab]
	if !ok {
		list = &ui.ListState{}
		s.lists[tab] = list
	}
	return list
}

// toggle shows a hidden column or hides a shown one, as the web's
// toggleColumn.
func (s *tableState) toggle(column dashboardview.Sort) {
	if i := slices.Index(s.hidden, column); i >= 0 {
		s.hidden = slices.Delete(slices.Clone(s.hidden), i, i+1)
		return
	}
	s.hidden = append(slices.Clone(s.hidden), column)
}

// visibleColumns are tab's columns not hidden, in order. At least one
// shows: Name always does, and a tab without Name, Context, shows its
// first column when the columns hidden on other tabs hide all of its own.
func visibleColumns(tab dashboardview.Tab, hidden []dashboardview.Sort) []dashboardview.Column {
	all := dashboardview.Columns(tab)
	visible := slices.DeleteFunc(slices.Clone(all), func(column dashboardview.Column) bool {
		return !dashboardview.ColumnVisible(column.ID, hidden)
	})
	if len(visible) == 0 && len(all) > 0 {
		return all[:1]
	}
	return visible
}

// resultsTable builds the table of the usage rows, the web's ResultsTable:
// the section header with the row count, sort and columns controls, the
// rows in a virtualized table reading more pages as its view nears their
// end, the empty state, and the footer of the result summary. The web's
// pager and rows per page select give way to the on-demand pages (ADR
// 0032).
func resultsTable(c *ui.Context, w *model.Window, s *tableState) {
	usage, _ := w.Usage()
	query, _ := w.Query()
	instance, _ := w.Instance()
	tab, estimated := usage.Tab, query.Quality == dashboardview.QualityEstimated
	label := dashboardview.TabLabel(tab) + detailsSuffix
	if estimated {
		label = excludedPrefix + label
	}
	ui.Column(c).Role(ui.RoleRegion).Label(label).Children(func() {
		kit.SectionHeader(c, kit.Section{Title: dashboardview.TableTitle(tab, query.Quality), Count: dashboardview.ExactCount(int64(usage.RowCount))}, func() {
			sortControls(c, w, query)
			columnsControl(c, s, tab)
		})
		columns := visibleColumns(tab, s.hidden)
		list := s.list(tab)
		if !usage.SameRows(s.shown) {
			// Other rows, or the same in another order: from their top.
			list.ScrollTo(0, ui.Start)
			s.shown = usage.Request
		}
		if _, last := list.Visible(); list.AtEnd() || last >= len(usage.Rows)-tablePrefetchRows {
			w.LoadMoreRows()
		}
		rowsTable(c, list, rowsView{rows: usage.Rows, tab: tab, columns: columns, query: query, timezone: instance.Timezone}, w.SortBy).
			MaxHeight(tableHeight(c))
		if usage.RowCount == 0 && len(usage.Rows) == 0 {
			emptyTable(c, w, query)
		}
		tableFooter(c, w, usage, query)
	})
}

// rowsView is what the table shows: the rows of tab in columns, sorted
// as query says, with dates in the server's timezone.
type rowsView struct {
	rows     []analytics.Row
	tab      dashboardview.Tab
	columns  []dashboardview.Column
	query    dashboardview.Query
	timezone string
}

// rowsTable builds the rows in a kit table whose headings sort them by
// sortBy, at their column widths, keyed by the rows' keys so the place
// and the pointer stay with the rows as pages arrive.
func rowsTable(c *ui.Context, list *ui.ListState, v rowsView, sortBy func(dashboardview.Sort)) ui.Element {
	list.Key = func(i int) any { return v.rows[i].Key }
	list.Label = func(i int) string { return v.rows[i].Name }
	columns := make([]kit.TableColumn, len(v.columns))
	for i, column := range v.columns {
		width := dashboardview.ColumnWidths(v.tab, column)
		columns[i] = kit.TableColumn{
			ID: string(column.ID), Label: column.Label, Numeric: column.Numeric,
			Width: float32(width.Default), MinWidth: float32(width.Min), MaxWidth: dashboardview.ColumnMaxWidth,
		}
		if dashboardview.HeaderSort(v.tab, column.ID) == v.query.Sort {
			columns[i].Sorted = ui.SortDescending
			if v.query.Direction == dashboardview.Ascending {
				columns[i].Sorted = ui.SortAscending
			}
		}
	}
	last := len(v.rows) - 1
	table := kit.Table(c, list, columns, len(v.rows), func(row, col int) {
		column := v.columns[col]
		kit.TableCell(c, kit.TableCellStyle{Numeric: column.Numeric, LastRow: row == last}, func() {
			resultsCell(c, v.rows[row], column, v.tab, v.timezone)
		})
	}, func(col int) { sortBy(dashboardview.HeaderSort(v.tab, v.columns[col].ID)) })
	return table.Label(dashboardview.TableTitle(v.tab, v.query.Quality))
}

// resultsCell builds one cell, as the web's ResultsCell: dates in the
// server's timezone, compact counts named by their exact values, the
// name with the dimensions the tab does not group by, and dimension
// summaries one per line.
func resultsCell(c *ui.Context, row analytics.Row, column dashboardview.Column, tab dashboardview.Tab, timezone string) {
	t := theme.Current(c)
	if column.ID == dashboardview.SortDate {
		date := theme.Body(ui.Text(c, dashboardview.FormatServerDate(row.Date, timezone))).SingleLine()
		kit.Tooltip(c, date, dashboardview.FormatServerDateTime(row.Date, timezone)+" · "+dashboardview.ServerZoneLabel(timezone))
		return
	}
	if value, ok := dashboardview.RowValue(row, column.ID); ok && column.Numeric {
		count := theme.Body(ui.Text(c, dashboardview.CompactCount(value))).FontFeatures("tnum").SingleLine()
		kit.Tooltip(c, count, dashboardview.ExactCount(value))
		return
	}
	text := dashboardview.RowText(row, column.ID)
	switch column.ID {
	case dashboardview.SortName:
		ui.Column(c).MinWidth(0).Gap(theme.Unit).Children(func() {
			name := theme.Medium(theme.Body(ui.Text(c, text)))
			if tab == dashboardview.TabTokens {
				name.FontFeatures("tnum")
			} else {
				name.SingleLine()
			}
			kit.Tooltip(c, name, text)
			if details := dashboardview.IdentityDetails(row, tab); len(details) > 0 {
				ui.Column(c).MinWidth(0).TextColor(t.InkSubtle).Children(func() {
					for _, detail := range details {
						theme.Label(ui.Text(c, detail))
					}
				})
			}
		})
	case dashboardview.SortHarness, dashboardview.SortProvider, dashboardview.SortModel:
		ui.Column(c).MinWidth(0).Children(func() {
			for _, line := range dashboardview.SummaryLines(text) {
				theme.Body(ui.Text(c, line))
			}
		})
	default:
		kit.Tooltip(c, theme.Body(ui.Text(c, text)), text)
	}
}

// tableHeight caps the table's own scroll region at DESIGN.md's 35rem
// and at the page's height under the header, less the section header and
// footer, so the whole region fits the window as the page scrolls to it.
func tableHeight(c *ui.Context) float32 {
	_, height := c.Size()
	room := height - theme.HeaderHeight - tableSectionHeader - tableFooterHeight
	return max(theme.TableRowHeight*tableMinRows, min(theme.TableMaxHeight, room))
}

// sortControls are the sort select and the direction toggle, as the
// web's: a new field sorts by it, the toggle flips the current one.
func sortControls(c *ui.Context, w *model.Window, query dashboardview.Query) {
	options := []kit.Option[dashboardview.Sort]{}
	for _, column := range dashboardview.SortOptions(query.Tab) {
		options = append(options, kit.Option[dashboardview.Sort]{Value: column.ID, Label: column.Label})
	}
	kit.ControlLabel(c, sortLabel, func() {
		selected := query.Sort
		if kit.Select(c.Key("sort"), &selected, options, kit.SizeCompact).Label(sortByLabel).Changed() && selected != query.Sort {
			w.SortBy(selected)
		}
	})
	direction, arrow := sortAscendingLabel, icons.ArrowDown()
	if query.Direction == dashboardview.Ascending {
		direction, arrow = sortDescendingLabel, icons.ArrowUp()
	}
	if kit.IconButton(c.Key("direction"), arrow, direction, kit.ButtonStyle{Variant: kit.Outline, Size: kit.SizeCompact}).Clicked() {
		w.SortBy(query.Sort)
	}
}

// columnsControl is the Columns button and its popover of the columns
// that can hide: every one but Name, the last one shown kept.
func columnsControl(c *ui.Context, s *tableState, tab dashboardview.Tab) {
	b := kit.ButtonContent(c.Key("columns"), kit.ButtonStyle{Variant: kit.Outline, Size: kit.SizeCompact}, func() {
		kit.Icon(c, icons.Columns3())
		ui.Text(c, columnsLabel).SingleLine()
	})
	if b.Clicked() {
		s.columnsOpen = !s.columnsOpen
	}
	panel := kit.PopoverWith(c, b, &s.columnsOpen, kit.PopoverOptions{End: true}, func() {
		kit.PopoverHeader(c, visibleColumnsLabel, nil)
		visible := visibleColumns(tab, s.hidden)
		for _, column := range dashboardview.Columns(tab) {
			if column.ID == dashboardview.SortName {
				continue
			}
			shown := slices.ContainsFunc(visible, func(v dashboardview.Column) bool { return v.ID == column.ID })
			checked := shown
			style := kit.CheckboxStyle{Disabled: shown && len(visible) == 1}
			if kit.CheckboxOptionWith(c.Key(column.ID), &checked, column.Label, style); checked != shown {
				s.toggle(column.ID)
			}
		}
	})
	panel.Label(visibleColumnsLabel)
}

// emptyTable explains a query without rows, as the web's: a wider date
// range, or clearing the filters it offers when set. Reviewing excluded
// usage adds its own action here with the excluded usage it reads.
func emptyTable(c *ui.Context, w *model.Window, query dashboardview.Query) {
	title, description := noMatches, includedScope
	if query.Quality == dashboardview.QualityEstimated {
		title, description = noExcludedMatches, excludedScope
	}
	description += widerRange
	hasFilters := query.HasFilters()
	if hasFilters {
		description += orClearFilters
	}
	var actions func()
	if hasFilters {
		actions = func() {
			if kit.Button(c, clearFiltersLabel, kit.ButtonStyle{Variant: kit.Outline}).Clicked() {
				w.ClearFilters()
			}
		}
	}
	kit.EmptyState(c, icons.Copy(), title, description+".", actions)
}

// tableFooter is the result summary under the table, as the web's footer:
// sessions shown of those synced, the row count and the total, then where
// the web has its pager, the next page's read or its failure.
func tableFooter(c *ui.Context, w *model.Window, usage model.Usage, query dashboardview.Query) {
	t := theme.Current(c)
	estimated := query.Quality == dashboardview.QualityEstimated
	sessions, synced, hint, total := sessionsShown, syncedSuffix, sessionsShownHint, totalLabel
	if estimated {
		sessions, synced, hint, total = excludedSessions, acrossAllDates, excludedSessionsHint, excludedTotalLabel
	}
	strong := func(text string) ui.Element {
		return theme.Strong(ui.Text(c, text)).TextColor(t.Ink).FontFeatures("tnum")
	}
	bar := ui.Row(c).Wrap().Justify(ui.SpaceBetween).GapX(theme.Unit*4).GapY(theme.Unit).MinHeight(tableFooterHeight).
		PaddingY(theme.Unit).BorderWidth(theme.Rule, 0, 0, 0).BorderColor(t.Line).TextColor(t.InkSubtle)
	theme.Label(bar).Children(func() {
		ui.Row(c).Wrap().Gap(theme.Unit * 2).MinWidth(0).Children(func() {
			// One group the screen reader reads whole, described as the
			// web's title describes it.
			counts := ui.Row(c).Gap(theme.Unit).Role(ui.RoleGroup).Children(func() {
				ui.Text(c, sessions)
				strong(dashboardview.ExactCount(usage.Summary.SessionCount))
				ui.Text(c, shownSeparator)
				strong(dashboardview.ExactCount(usage.Summary.SyncedSessions))
				ui.Text(c, synced)
			})
			kit.Tooltip(c, counts, hint)
			ui.Text(c, summaryDot)
			ui.Row(c).Gap(theme.Unit).Children(func() {
				strong(dashboardview.ExactCount(int64(usage.RowCount)))
				ui.Text(c, rowsSuffix)
			})
			if query.Tab != dashboardview.TabContext {
				ui.Text(c, summaryDot)
				ui.Row(c).Gap(theme.Unit).Children(func() {
					ui.Text(c, total)
					kit.Tooltip(c, strong(dashboardview.CompactCount(usage.Summary.TotalTokens)), dashboardview.ExactCount(usage.Summary.TotalTokens))
				})
			}
		})
		rowsStatus(c, w)
	})
}

// rowsStatus says that the next page of rows is being read, or why it
// failed, with Retry Request; the rows read stay.
func rowsStatus(c *ui.Context, w *model.Window) {
	t := theme.Current(c)
	switch err := w.RowsError(); {
	case err != nil:
		ui.Row(c).Gap(theme.Unit * 2).MarginStart(ui.Auto).TextColor(t.StatusDanger).Role(ui.RoleAlert).Children(func() {
			kit.Icon(c, icons.CircleAlert())
			ui.Text(c, rowsFailedPrefix+errorMessage(err))
			if kit.Button(c.Key("rows-retry"), retryRequest, kit.ButtonStyle{Variant: kit.Outline, Size: kit.SizeCompact}).Clicked() {
				w.RetryRows()
			}
		})
	case w.RowsLoading():
		ui.Row(c).Gap(theme.Unit * 2).MarginStart(ui.Auto).Role(ui.RoleStatus).Children(func() {
			kit.Spinner(c)
			ui.Text(c, loadingRows)
		})
	}
}
