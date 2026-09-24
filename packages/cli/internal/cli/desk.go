package cli

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	deskRoomyHeight         = 30
	deskMetricMinHeight     = 24
	deskMetricMinWidth      = 72
	deskCompactReadoutWidth = 96
	deskFooterRows          = 4 // Spacer, coverage, divider, shortcuts.
	deskDrawerWidth         = 44
	deskDrawerTop           = 2
	deskDrawerChrome        = 10 // Title, description, spacing, position, and actions.
)

var (
	deskPanelStyle = lipgloss.NewStyle().Foreground(themeText)
	deskLabelStyle = lipgloss.NewStyle().Foreground(themeMuted)
	deskValueStyle = lipgloss.NewStyle().Foreground(themeText).Bold(true)
	deskTotalStyle = lipgloss.NewStyle().Foreground(themeTotal).Bold(true)
	deskKeyStyle   = lipgloss.NewStyle().Foreground(themeAccent).Bold(true)
	deskScopeItems = []string{"Date range", "Provider", "Model", "Harness"}
	deskHelpItems  = []string{
		"1–6         Choose view", "Tab / Shift Tab   Next / previous",
		"↑↓ or j/k   Move through rows", "PgUp/PgDn   Move one page",
		"←→          Scroll columns", "Home / End  First / last column",
		"d           Date range", "g           Time bucket", "s           Sort",
		"f           Filters", "p / m / h   Provider/model/harness",
		"r           Reload usage", "q / Ctrl+C  Quit",
	}
)

type deskMetric struct {
	label string
	value int64
}

func (m interactiveModel) handleDeskMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	count := len(deskScopeItems)
	if m.popup == popupHelp {
		count = len(deskHelpItems)
	}
	switch msg.String() {
	case "esc", "q", "?":
		m.popup = popupNone
	case "up", "k":
		m.popupCursor = movePopupCursor(m.popupCursor, count, -1)
	case "down", "j":
		m.popupCursor = movePopupCursor(m.popupCursor, count, 1)
	case "enter", " ":
		if m.popup == popupHelp {
			m.popup = popupNone
			break
		}
		switch m.popupCursor {
		case 0:
			m.popup, m.popupCursor = popupDateRange, indexOfPeriod(m.options.period)
		case 1:
			return m.openFilterValues(filterProvider)
		case 2:
			return m.openFilterValues(filterModel)
		case 3:
			return m.openFilterValues(filterHarness)
		}
	}
	return m, nil
}

// These are exact component sums from the entire filtered result, never from
// formatted K/M labels or the visible viewport. Context peaks are not additive.
func tokenReadouts(rows []renderRow) []deskMetric {
	metrics := []deskMetric{{label: "Total tokens"}, {label: "Input"}, {label: "Output"},
		{label: "Reasoning"}, {label: "Cache read"}, {label: "Cache write"}}
	for _, row := range rows {
		metrics[0].value += row.totalValue
		metrics[1].value += row.inputValue
		metrics[2].value += row.outputValue
		metrics[3].value += row.reasoningValue
		metrics[4].value += row.cacheReadValue
		metrics[5].value += row.cacheWriteValue
	}
	return metrics
}

func (m interactiveModel) deskHeader() []string {
	width := m.tableViewportWidth()
	lines := []string{m.renderStatusline()}
	roomy := m.height >= deskRoomyHeight
	if roomy {
		lines = append(lines, "")
	}
	lines = append(lines, m.deskTabs())
	if roomy {
		lines = append(lines, "")
	}
	controls := []string{deskControl("d", "Date", statuslineDateRangeLabel(m.options))}
	if m.activeTab == tabTokens {
		controls = append(controls, deskControl("g", "Bucket", string(m.options.bucket)))
	}
	controls = append(controls, deskControl("s", "Sort", string(activeSort(m.activeTab, m.options.sort))), deskControl("f", "Filters", ""))
	lines = append(lines, strings.Join(controls, "   "))
	filters := activeFiltersLabel(m.options.filters)
	if filters == "" {
		filters = "All providers · All models · All harnesses"
	}
	lines = append(lines, hintStyle.Render(filters))
	if m.height >= deskMetricMinHeight && width >= deskMetricMinWidth {
		if m.activeTab == tabContext {
			lines = append(lines, "")
		}
		lines = append(lines, m.deskReadouts(width)...)
	}
	lines = append(lines, "")
	return lines
}

func deskControl(key, label, value string) string {
	return deskKeyStyle.Render(key) + " " + textCellStyle.Render(strings.TrimSpace(label+" "+value))
}

func (m interactiveModel) deskTabs() string {
	var tabs []string
	for i, tab := range aggregationTabs {
		label := fmt.Sprintf("%d %s", i+1, tab.String())
		if m.width < deskMetricMinWidth {
			label = fmt.Sprintf("%d %.3s", i+1, tab.String())
		}
		style := inactiveTabStyle
		if tab == m.activeTab {
			style = activeTabStyle
			label = "[" + label + "]"
		}
		tabs = append(tabs, style.Render(label))
	}
	return strings.Join(tabs, " ")
}

func (m interactiveModel) deskReadouts(width int) []string {
	if m.activeTab == tabContext {
		return []string{
			deskPanelStyle.Width(width).Render("  Session peak context load"),
			deskLabelStyle.Width(width).Render("  Compare average, median and maximum per session. Peaks are not additive."),
			deskLabelStyle.Width(width).Render("  Input + cache read + cache write; excludes output and reasoning."),
		}
	}
	metrics := tokenReadouts(m.rows)
	cellWidth := width / len(metrics)
	labels, values := make([]string, len(metrics)), make([]string, len(metrics))
	for i, metric := range metrics {
		w := cellWidth
		if i == len(metrics)-1 {
			w = width - i*cellWidth
		}
		label := metric.label
		if width < deskCompactReadoutWidth {
			switch i {
			case 0:
				label = "Total"
			case 4:
				label = "Cache R"
			case 5:
				label = "Cache W"
			}
		}
		labels[i] = deskLabelStyle.Width(w).Render(ansi.Truncate("  "+label, w, "…"))
		value := formatTableSummaryTokens(metric.value)
		if m.loading || m.err != nil {
			value = "—"
		}
		style := deskValueStyle
		if i == 0 {
			style = deskTotalStyle
		}
		values[i] = style.Width(w).Render("  " + value)
	}
	padding := deskPanelStyle.Width(width).Render("")
	return []string{padding, strings.Join(labels, ""), strings.Join(values, ""), padding}
}

func (m interactiveModel) renderDesk() string {
	width := m.tableViewportWidth()
	lines := m.deskHeader()
	rowStart := len(lines) + 1 // Skip the table's column header.
	visible := m.maxVisibleRows()
	rows := m.visibleRows()
	selectedSort := activeSort(m.activeTab, m.options.sort)
	var table string
	switch {
	case m.err != nil:
		table = m.deskMessage("Could not load usage", m.err.Error(), "r Retry · q Quit", visible+1)
	case m.loading:
		table = renderLoadingTableViewportWithSort(m.groupBy, m.activeTab, selectedSort, width, visible)
	case len(m.rows) == 0:
		message, recovery := "No rows match the current scope.", "d Change date range · f Adjust filters"
		if m.sessionCounts.Synced == 0 {
			message, recovery = "No synced usage yet.", "Quit and run tokeninsights sync --all, then reopen."
		}
		table = m.deskMessage(message, "", recovery, visible+1)
	default:
		focus := m.cursor - m.scrollOffset
		table = renderTableViewportWithSortAndFocus(rows, m.rows, m.groupBy, m.activeTab, selectedSort, width, m.horizontalOffset, visible, focus)
	}
	tableLines := strings.Split(strings.TrimSuffix(table, "\n"), "\n")
	// A single multi-value row can be taller than the viewport. Keep coverage
	// and shortcuts pinned even when that row has to be clipped.
	lines = append(lines, tableLines[:min(len(tableLines), visible+1)]...)
	summary := m.renderTableSummary()
	if m.err != nil {
		summary = ""
	}
	lines = append(lines, "", summary, dividerStyle.Render(strings.Repeat("─", width)), m.deskFooter())
	focusLine := -1
	if !m.loading && m.err == nil && len(rows) > 0 {
		focusLine = rowStart
		for i := 0; i < m.cursor-m.scrollOffset && i < len(rows); i++ {
			focusLine += renderRowLineCount(rows[i], columnsForModeAndTab(m.groupBy, m.activeTab))
		}
	}
	for i, line := range lines {
		prefix := " "
		if i == focusLine {
			prefix = deskKeyStyle.Render(">")
		}
		lines[i] = prefix + ansi.Truncate(line, width, "…") + " "
	}
	return renderOnAppSurface(strings.Join(lines, "\n"), m.width, m.height)
}

func (m interactiveModel) deskMessage(title, detail, action string, height int) string {
	width := m.tableViewportWidth()
	lines := make([]string, max(1, height))
	lines[0] = titleStyle.Render(title)
	if len(lines) > 2 {
		lines[2] = ansi.Truncate(strings.ReplaceAll(detail, "\n", " "), width, "…")
	}
	if len(lines) > 4 {
		lines[4] = hintStyle.Render(action)
	}
	return strings.Join(lines, "\n") + "\n"
}

func (m interactiveModel) deskFooter() string {
	keys := "tab Views   ↑↓ Rows   ←→ Columns   f Filters   ? Help   q Quit"
	position := ""
	if m.loading {
		position = "loading"
	} else if m.err != nil {
		position = "r Retry"
	} else if len(m.rows) > 0 {
		position = fmt.Sprintf("%d–%d / %d", m.scrollOffset+1, m.scrollOffset+len(m.visibleRows()), len(m.rows))
	}
	if maxOffset := m.maxHorizontalOffset(m.rows); maxOffset > 0 {
		position += fmt.Sprintf(" · x %d/%d", m.horizontalOffset+1, maxOffset+1)
	}
	if ansi.StringWidth(keys)+ansi.StringWidth(position)+ansi.StringWidth(tableSummarySeparator) > m.tableViewportWidth() {
		keys = "f Filters   ? Help   q Quit"
	}
	return renderFooterLine(keys, position, m.tableViewportWidth())
}

func (m interactiveModel) drawerSize() (int, int) {
	return min(deskDrawerWidth, m.width), max(1, m.height-deskDrawerTop)
}

func (m interactiveModel) renderDeskDrawer(background string) string {
	width, height := m.drawerSize()
	panel := m.renderDrawerContent(width, height)
	base := strings.Split(background, "\n")
	for i, line := range strings.Split(panel, "\n") {
		y := i + deskDrawerTop
		if y >= len(base) {
			break
		}
		base[y] = ansi.Cut(base[y], 0, m.width-width) + line
	}
	return strings.Join(base, "\n")
}

func (m interactiveModel) renderDrawerContent(width, height int) string {
	const inset = 2
	contentWidth := max(1, width-2*inset-1)
	title, description := "Filters", "Choose a scope to edit."
	items := deskScopeItems
	checked := -1
	multi := false
	switch m.popup {
	case popupDateRange:
		title, description, items = "Date range", "Choose the period to compare.", nil
		checked = indexOfPeriod(m.options.period)
		for _, opt := range dateRangeOptions {
			items = append(items, periodLabel(opt))
		}
	case popupBucket:
		title, description, items = "Time bucket", "Group usage by calendar interval.", nil
		checked = indexOfBucket(m.options.bucket)
		for _, opt := range bucketOptions {
			items = append(items, string(opt))
		}
	case popupSort:
		title, description, items = "Sort", "Choose the table's sort order.", nil
		checked = indexOfSort(activeSort(m.activeTab, m.options.sort), m.activeTab)
		for _, opt := range sortOptionsForTab(m.activeTab) {
			items = append(items, string(opt))
		}
	case popupFilterValues:
		title, description = "Filter "+filterDimensionLabel(m.filterDimension), "Select values; none means all."
		items, multi = m.filterValues, true
		if m.filterLoading {
			items = []string{"Loading values..."}
		}
		if m.filterErr != nil {
			items = []string{"Could not load values.", m.filterErr.Error(), "Esc Cancel; reopen to retry."}
		}
		if len(items) == 0 {
			items = []string{"No values available"}
		}
	case popupHelp:
		title, description = "Keyboard guide", "Everything stays within reach."
		items = deskHelpItems
	}
	lines := []string{"", popupTitleStyle.Render(title), "", hintStyle.Render(description), ""}
	capacity := max(1, height-deskDrawerChrome)
	start := max(0, min(m.popupCursor-capacity+1, len(items)-capacity))
	end := min(len(items), start+capacity)
	for i := start; i < end; i++ {
		prefix := "  "
		style := popupItemStyle
		if i == m.popupCursor && m.popup != popupHelp {
			prefix, style = "> ", selectedRowStyle.Bold(true)
		}
		mark := ""
		if multi && !m.filterLoading && m.filterErr == nil && len(m.filterValues) > 0 {
			mark = "[ ] "
			if m.filterSelections[items[i]] {
				mark = "[x] "
			}
		} else if i == checked {
			mark = "* "
		}
		lines = append(lines, style.Width(contentWidth).Render(ansi.Truncate(prefix+mark+items[i], contentWidth, "…")))
	}
	for len(lines) < height-4 {
		lines = append(lines, "")
	}
	position := ""
	if len(items) > capacity {
		position = fmt.Sprintf("%d–%d of %d · ↑↓ scroll", start+1, end, len(items))
	}
	lines = append(lines, hintStyle.Render(position))
	actions := "Enter Apply   Esc Cancel"
	if m.popup == popupFilters {
		actions = "Enter Edit   Esc Close"
	}
	if m.popup == popupHelp {
		actions = "Esc Close"
	}
	if multi {
		lines = append(lines, hintStyle.Render("Space Toggle   c Clear selection"))
	} else {
		lines = append(lines, "")
	}
	lines = append(lines, deskKeyStyle.Render(actions), "")
	for i, line := range lines {
		lines[i] = dividerStyle.Render("│") + paintSurfaceLine(strings.Repeat(" ", inset)+ansi.Truncate(line, contentWidth, "…"), width-1, deskPanelStyle)
	}
	return strings.Join(lines[:min(height, len(lines))], "\n")
}
