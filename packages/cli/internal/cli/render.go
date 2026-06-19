package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type tabMode int

const (
	tabTokens tabMode = iota
	tabModels
	tabProviders
	tabHarnesses
	tabSessions
)

func (t tabMode) String() string {
	switch t {
	case tabTokens:
		return "tokens"
	case tabModels:
		return "models"
	case tabProviders:
		return "providers"
	case tabHarnesses:
		return "harnesses"
	case tabSessions:
		return "sessions"
	default:
		return ""
	}
}

type column struct {
	name    string
	field   string
	numeric bool
}

const sortIndicator = " ↓"

const (
	tableColumnGapWidth = 2
	truncationMarker    = "..."

	bucketColumnWidth  = 10
	latestColumnWidth  = 16
	sessionColumnWidth = 8

	harnessColumnMinWidth  = 7
	harnessColumnMaxWidth  = 14
	providerColumnMinWidth = 8
	providerColumnMaxWidth = 18
	modelColumnMinWidth    = 14
	modelColumnMaxWidth    = 32
	listColumnMinWidth     = 12
	listColumnMaxWidth     = 36
)

const (
	appBackgroundColor   = "#1b1b2a"
	panelBackgroundColor = appBackgroundColor
	tableSeparatorColor  = "#565766"
	outerBorderColor     = "#4a4b59"
	sectionBorderColor   = "#3d3e49"
)

func columnsForModeAndTab(g groupByMode, t tabMode) []column {
	switch t {
	case tabTokens:
		return []column{
			{name: "bucket", field: "bucket"},
			{name: "sessions", field: "sessions", numeric: true},
			{name: "input", field: "inputTokens", numeric: true},
			{name: "output", field: "outputTokens", numeric: true},
			{name: "reasoning", field: "reasoningTokens", numeric: true},
			{name: "cache R", field: "cacheReadTokens", numeric: true},
			{name: "cache W", field: "cacheWriteTokens", numeric: true},
			{name: "total", field: "totalTokens", numeric: true},
		}
	case tabModels:
		return append([]column{
			{name: "model", field: "model"},
			{name: "providers", field: "providers"},
			{name: "harnesses", field: "harnesses"},
			{name: "sessions", field: "sessions", numeric: true},
		}, tokenColumns()...)
	case tabProviders:
		return append([]column{
			{name: "provider", field: "provider"},
			{name: "models", field: "models"},
			{name: "harnesses", field: "harnesses"},
			{name: "sessions", field: "sessions", numeric: true},
		}, tokenColumns()...)
	case tabHarnesses:
		return append([]column{
			{name: "harness", field: "harness"},
			{name: "providers", field: "providers"},
			{name: "models", field: "models"},
			{name: "sessions", field: "sessions", numeric: true},
		}, tokenColumns()...)
	case tabSessions:
		return append([]column{
			{name: "latest", field: "latest"},
			{name: "session", field: "sessionID"},
			{name: "harness", field: "harness"},
			{name: "providers", field: "providers"},
			{name: "models", field: "models"},
			{name: "ctx used", field: "contextUsedTokens", numeric: true},
		}, tokenColumns()...)
	}

	grouping := []column{{name: "day", field: "day"}}
	switch g {
	case groupByHour:
		grouping = append(grouping, column{name: "hour", field: "hour"})
	case groupBySession:
		grouping = append(grouping,
			column{name: "session id", field: "sessionID"},
			column{name: "thinking", field: "thinkingLevels"},
		)
	}
	grouping = append(grouping,
		column{name: "harness", field: "harness"},
		column{name: "provider", field: "provider"},
		column{name: "model", field: "model"},
	)

	switch t {
	default:
		return grouping
	}
}

func tokenColumns() []column {
	return []column{
		{name: "input", field: "inputTokens", numeric: true},
		{name: "output", field: "outputTokens", numeric: true},
		{name: "reasoning", field: "reasoningTokens", numeric: true},
		{name: "cache R", field: "cacheReadTokens", numeric: true},
		{name: "cache W", field: "cacheWriteTokens", numeric: true},
		{name: "total", field: "totalTokens", numeric: true},
	}
}

type renderRow struct {
	bucket            string
	sessions          string
	latest            string
	latestValue       int64
	harness           string
	harnesses         string
	day               string
	hour              string
	sessionID         string
	provider          string
	providers         string
	model             string
	models            string
	thinkingLevels    string
	tpsAvg            string
	tpsMean           string
	tpsMedian         string
	inputTokens       string
	inputValue        int64
	outputTokens      string
	outputValue       int64
	reasoningTokens   string
	cacheReadTokens   string
	cacheReadValue    int64
	cacheWriteTokens  string
	contextUsedTokens string
	contextUsedValue  int64
	totalTokens       string
	totalValue        int64
	requests          string
	retries           string
	toolName          string
	toolCalls         string
	toolErrors        string
}

func displaySessionID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[len(value)-8:]
}

func displayModel(value string) string {
	if index := strings.LastIndex(value, "/"); index >= 0 && index < len(value)-1 {
		return value[index+1:]
	}
	return value
}

func formatWeightedTPS(throughputTokens int64, durationMs int64) string {
	if durationMs <= 0 || throughputTokens <= 0 {
		return ""
	}
	return formatTPS(float64(throughputTokens) / (float64(durationMs) / 1000))
}

func formatMeanTPS(tpsMean float64) string {
	if tpsMean <= 0 {
		return ""
	}
	return formatTPS(tpsMean)
}

func formatMedianTPS(tpsMedian float64) string {
	if tpsMedian <= 0 {
		return ""
	}
	return formatTPS(tpsMedian)
}

func formatTPS(value float64) string {
	return fmt.Sprintf("%.2f", value)
}

func formatTokens(value int64) string {
	if value == 0 {
		return ""
	}
	abs := value
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs < 1000:
		return strconv.FormatInt(value, 10)
	case abs < 1_000_000:
		return fmt.Sprintf("%dK", value/1000)
	default:
		return fmt.Sprintf("%dM", value/1_000_000)
	}
}

func formatContextTokens(value int64) string {
	if value == 0 {
		return ""
	}
	abs := value
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs < 1000:
		return strconv.FormatInt(value, 10)
	case abs < 1_000_000:
		return fmt.Sprintf("%dk", (abs+999)/1000)
	default:
		return fmt.Sprintf("%dM", value/1_000_000)
	}
}

func formatLatest(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.UnixMilli(value).Local().Format("2006-01-02 15:04")
}

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			Background(lipgloss.Color(appBackgroundColor))
	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Background(lipgloss.Color(panelBackgroundColor))
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			Background(lipgloss.Color(appBackgroundColor))

	dimensionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("87"))
	textCellStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	mutedCellStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	inputStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("113"))
	outputStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	reasoningStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("176"))
	cacheReadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	cacheWriteStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("179"))
	contextUsedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	totalStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("157"))
	appSurfaceStyle  = lipgloss.NewStyle().Background(lipgloss.Color(appBackgroundColor))
	rowOddStyle      = appSurfaceStyle
	rowEvenStyle     = appSurfaceStyle

	borderStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color(tableSeparatorColor)).Background(lipgloss.Color(appBackgroundColor))
	outerBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(outerBorderColor)).
				BorderBackground(lipgloss.Color(appBackgroundColor)).
				Background(lipgloss.Color(appBackgroundColor))
	sectionBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(sectionBorderColor)).
				BorderBackground(lipgloss.Color(panelBackgroundColor)).
				Background(lipgloss.Color(panelBackgroundColor))
)

func renderTable(rows []renderRow, g groupByMode, tab tabMode) string {
	return renderTableWithWidth(rows, g, tab, 0)
}

func renderTableViewport(rows []renderRow, g groupByMode, tab tabMode, width int, horizontalOffset int) string {
	return renderTableViewportWithRows(rows, g, tab, width, horizontalOffset, 0)
}

func renderTableViewportWithRows(rows []renderRow, g groupByMode, tab tabMode, width int, horizontalOffset int, minDataRows int) string {
	return renderTableViewportWithReferenceRows(rows, rows, g, tab, width, horizontalOffset, minDataRows)
}

func renderTableViewportWithReferenceRows(rows []renderRow, referenceRows []renderRow, g groupByMode, tab tabMode, width int, horizontalOffset int, minDataRows int) string {
	if width <= 0 {
		return ""
	}
	return horizontalViewport(renderTableWithReferenceRowsWidth(rows, referenceRows, g, tab, width, minDataRows, "No rows match the current scope."), horizontalOffset, width)
}

func renderTableViewportWithSort(rows []renderRow, referenceRows []renderRow, g groupByMode, tab tabMode, sort sortMode, width int, horizontalOffset int, minDataRows int) string {
	if width <= 0 {
		return ""
	}
	return horizontalViewport(renderTableWithReferenceRowsAndSortWidth(rows, referenceRows, g, tab, sort, width, minDataRows, "No rows match the current scope."), horizontalOffset, width)
}

func renderLoadingTableViewport(g groupByMode, tab tabMode, width int, minDataRows int) string {
	if width <= 0 {
		return ""
	}
	return horizontalViewport(renderTableWithReferenceRowsWidth(nil, loadingReferenceRows(tab), g, tab, width, minDataRows, "Loading data..."), 0, width)
}

func renderLoadingTableViewportWithSort(g groupByMode, tab tabMode, sort sortMode, width int, minDataRows int) string {
	if width <= 0 {
		return ""
	}
	return horizontalViewport(renderTableWithReferenceRowsAndSortWidth(nil, loadingReferenceRows(tab), g, tab, sort, width, minDataRows, "Loading data..."), 0, width)
}

func renderTableWidth(rows []renderRow, g groupByMode, tab tabMode) int {
	return lipgloss.Width(renderTable(rows, g, tab))
}

func renderTableWidthWithSort(rows []renderRow, g groupByMode, tab tabMode, sort sortMode) int {
	return lipgloss.Width(renderTableWithReferenceRowsAndSort(rows, rows, g, tab, sort, 0, "No rows match the current scope."))
}

func renderTableWidthWithSortAndViewport(rows []renderRow, g groupByMode, tab tabMode, sort sortMode, width int) int {
	return lipgloss.Width(renderTableWithReferenceRowsAndSortWidth(rows, rows, g, tab, sort, width, 0, "No rows match the current scope."))
}

func horizontalViewport(value string, horizontalOffset int, width int) string {
	if width <= 0 {
		return ""
	}
	if horizontalOffset < 0 {
		horizontalOffset = 0
	}

	lines := strings.Split(value, "\n")
	hasTrailingNewline := len(lines) > 0 && lines[len(lines)-1] == ""
	if hasTrailingNewline {
		lines = lines[:len(lines)-1]
	}

	for i, line := range lines {
		lines[i] = padANSI(ansi.Cut(line, horizontalOffset, horizontalOffset+width), width)
	}

	result := strings.Join(lines, "\n")
	if hasTrailingNewline {
		return result + "\n"
	}
	return result
}

func padANSI(value string, width int) string {
	padding := width - ansi.StringWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func renderTableWithWidth(rows []renderRow, g groupByMode, tab tabMode, width int) string {
	return renderTableWithMinRows(rows, g, tab, 0, "No rows match the current scope.")
}

func renderTableWithMinRows(rows []renderRow, g groupByMode, tab tabMode, minDataRows int, emptyMessage string) string {
	return renderTableWithReferenceRows(rows, rows, g, tab, minDataRows, emptyMessage)
}

func renderTableWithReferenceRows(rows []renderRow, referenceRows []renderRow, g groupByMode, tab tabMode, minDataRows int, emptyMessage string) string {
	return renderTableWithReferenceRowsAndSort(rows, referenceRows, g, tab, "", minDataRows, emptyMessage)
}

func renderTableWithReferenceRowsAndSort(rows []renderRow, referenceRows []renderRow, g groupByMode, tab tabMode, sort sortMode, minDataRows int, emptyMessage string) string {
	return renderTableWithReferenceRowsAndSortWidth(rows, referenceRows, g, tab, sort, 0, minDataRows, emptyMessage)
}

func renderTableWithReferenceRowsWidth(rows []renderRow, referenceRows []renderRow, g groupByMode, tab tabMode, minLineWidth int, minDataRows int, emptyMessage string) string {
	return renderTableWithReferenceRowsAndSortWidth(rows, referenceRows, g, tab, "", minLineWidth, minDataRows, emptyMessage)
}

func renderTableWithReferenceRowsAndSortWidth(rows []renderRow, referenceRows []renderRow, g groupByMode, tab tabMode, sort sortMode, minLineWidth int, minDataRows int, emptyMessage string) string {
	cols := columnsForModeAndTab(g, tab)

	formatted := formatRenderRows(rows, cols)
	widthFormatted := formatted
	if len(referenceRows) > 0 {
		widthFormatted = formatRenderRows(referenceRows, cols)
	}

	widths := make([]int, len(cols))
	minWidths := make([]int, len(cols))
	maxWidths := make([]int, len(cols))
	headerLabels := make([]string, len(cols))
	for i, col := range cols {
		headerLabels[i] = headerLabel(col, tab, sort)
		headerWidth := ansi.StringWidth(headerLabels[i])
		minWidth, maxWidth := columnWidthBounds(col, headerWidth)
		minWidths[i] = minWidth
		maxWidths[i] = maxWidth
		widths[i] = minWidth
		if maxWidth > 0 && widths[i] > maxWidth {
			widths[i] = maxWidth
		}
	}
	for _, values := range widthFormatted {
		for i, value := range values {
			if valueWidth := ansi.StringWidth(value); valueWidth > widths[i] {
				widths[i] = valueWidth
			}
			if maxWidths[i] > 0 && widths[i] > maxWidths[i] {
				widths[i] = maxWidths[i]
			}
		}
	}
	widths = fitColumnWidths(widths, minWidths, cols, minLineWidth)

	var lines []string
	header := make([]string, len(cols))
	separator := make([]string, len(cols))
	for i := range cols {
		header[i] = padCell(truncateCell(headerLabels[i], widths[i]), widths[i], false)
		separator[i] = strings.Repeat("─", widths[i])
	}
	lines = append(lines, padStyledLine(headerStyle.Render(strings.Join(header, "  ")), minLineWidth, headerStyle))
	lines = append(lines, padStyledLine(borderStyle.Render(strings.Join(separator, "  ")), minLineWidth, appSurfaceStyle))

	for rowIndex, values := range formatted {
		cells := make([]string, len(cols))
		for i, value := range values {
			cells[i] = renderCell(padCell(truncateCell(value, widths[i]), widths[i], cols[i].numeric), cols[i], rowIndex)
		}
		lines = append(lines, padStyledLine(joinStyledCells(cells, rowIndex), minLineWidth, rowStyle(rowIndex)))
	}
	if len(formatted) == 0 {
		lines = append(lines, padStyledLine(hintStyle.Render(emptyMessage), minLineWidth, hintStyle))
	}
	for dataLines := max(len(formatted), 1); dataLines < minDataRows; dataLines++ {
		lines = append(lines, padStyledLine("", minLineWidth, appSurfaceStyle))
	}
	return strings.Join(lines, "\n") + "\n"
}

func padStyledLine(value string, width int, style lipgloss.Style) string {
	padding := width - ansi.StringWidth(value)
	if padding <= 0 {
		return value
	}
	return value + style.Render(strings.Repeat(" ", padding))
}

func renderOnAppSurface(value string, width int, height int) string {
	if width <= 0 || height <= 0 {
		return appSurfaceStyle.Render(value)
	}
	return lipgloss.Place(
		width,
		height,
		lipgloss.Left,
		lipgloss.Top,
		value,
		lipgloss.WithWhitespaceBackground(lipgloss.Color(appBackgroundColor)),
	)
}

func headerLabel(col column, tab tabMode, sort sortMode) string {
	if sort == "" || col.field != sortField(tab, sort) {
		return col.name
	}
	return col.name + sortIndicator
}

func sortField(tab tabMode, sort sortMode) string {
	switch sort {
	case sortInput:
		return "inputTokens"
	case sortOutput:
		return "outputTokens"
	case sortCacheRead:
		return "cacheReadTokens"
	case sortTokens:
		return "totalTokens"
	case sortName:
		return rowNameField(tab)
	case sortDate:
		if tab == tabSessions {
			return "latest"
		}
		return rowNameField(tab)
	default:
		return ""
	}
}

func rowNameField(tab tabMode) string {
	switch tab {
	case tabModels:
		return "model"
	case tabProviders:
		return "provider"
	case tabHarnesses:
		return "harness"
	case tabSessions:
		return "sessionID"
	default:
		return "bucket"
	}
}

func renderCell(value string, col column, rowIndex int) string {
	style := cellStyleForColumn(col).Inherit(rowStyle(rowIndex))
	return style.Render(value)
}

func joinStyledCells(cells []string, rowIndex int) string {
	gap := rowStyle(rowIndex).Render("  ")
	return strings.Join(cells, gap)
}

func rowStyle(rowIndex int) lipgloss.Style {
	if rowIndex%2 == 1 {
		return rowEvenStyle
	}
	return rowOddStyle
}

func cellStyleForColumn(col column) lipgloss.Style {
	switch col.field {
	case "model", "provider", "harness", "bucket", "latest", "sessionID":
		return dimensionStyle
	case "models", "providers", "harnesses", "sessions":
		return mutedCellStyle
	case "inputTokens":
		return inputStyle
	case "outputTokens":
		return outputStyle
	case "reasoningTokens":
		return reasoningStyle
	case "cacheReadTokens":
		return cacheReadStyle
	case "cacheWriteTokens":
		return cacheWriteStyle
	case "contextUsedTokens":
		return contextUsedStyle
	case "totalTokens":
		return totalStyle
	default:
		return textCellStyle
	}
}

func formatRenderRows(rows []renderRow, cols []column) [][]string {
	formatted := make([][]string, 0, len(rows))
	for _, row := range rows {
		values := make([]string, len(cols))
		for i, c := range cols {
			switch c.field {
			case "bucket":
				values[i] = row.bucket
			case "sessions":
				values[i] = row.sessions
			case "latest":
				values[i] = row.latest
			case "harness":
				values[i] = row.harness
			case "harnesses":
				values[i] = row.harnesses
			case "day":
				values[i] = row.day
			case "hour":
				values[i] = row.hour
			case "sessionID":
				values[i] = displaySessionID(row.sessionID)
			case "thinkingLevels":
				values[i] = row.thinkingLevels
			case "provider":
				values[i] = row.provider
			case "providers":
				values[i] = row.providers
			case "model":
				values[i] = displayModel(row.model)
			case "models":
				values[i] = row.models
			case "tpsAvg":
				values[i] = row.tpsAvg
			case "tpsMean":
				values[i] = row.tpsMean
			case "tpsMedian":
				values[i] = row.tpsMedian
			case "inputTokens":
				values[i] = row.inputTokens
			case "outputTokens":
				values[i] = row.outputTokens
			case "reasoningTokens":
				values[i] = row.reasoningTokens
			case "cacheReadTokens":
				values[i] = row.cacheReadTokens
			case "cacheWriteTokens":
				values[i] = row.cacheWriteTokens
			case "contextUsedTokens":
				values[i] = row.contextUsedTokens
			case "totalTokens":
				values[i] = row.totalTokens
			case "requests":
				values[i] = row.requests
			case "retries":
				values[i] = row.retries
			case "toolName":
				values[i] = row.toolName
			case "toolCalls":
				values[i] = row.toolCalls
			case "toolErrors":
				values[i] = row.toolErrors
			default:
				values[i] = ""
			}
		}
		formatted = append(formatted, values)
	}
	return formatted
}

func loadingReferenceRows(tab tabMode) []renderRow {
	row := renderRow{
		bucket:            "9999-99-99",
		sessions:          "99999",
		latest:            "9999-99-99 99:99",
		sessionID:         "session-99999999",
		harness:           "opencode",
		harnesses:         "opencode, codex",
		provider:          "openai-codex",
		providers:         "openai, anthropic",
		model:             "gpt-5-codex-preview",
		models:            "gpt-5-codex, claude",
		inputTokens:       "999M",
		outputTokens:      "999M",
		reasoningTokens:   "999M",
		cacheReadTokens:   "999M",
		cacheWriteTokens:  "999M",
		contextUsedTokens: "999M",
		totalTokens:       "999M",
	}
	return []renderRow{row}
}

func columnWidthBounds(col column, headerWidth int) (int, int) {
	switch col.field {
	case "bucket":
		return bucketColumnWidth, bucketColumnWidth
	case "latest":
		return latestColumnWidth, latestColumnWidth
	case "model":
		return max(headerWidth, modelColumnMinWidth), modelColumnMaxWidth
	case "models":
		return max(headerWidth, listColumnMinWidth), listColumnMaxWidth
	case "provider":
		return max(headerWidth, providerColumnMinWidth), providerColumnMaxWidth
	case "providers":
		return max(headerWidth, listColumnMinWidth), listColumnMaxWidth
	case "harness":
		return max(headerWidth, harnessColumnMinWidth), harnessColumnMaxWidth
	case "harnesses":
		return max(headerWidth, listColumnMinWidth), listColumnMaxWidth
	case "sessionID":
		return sessionColumnWidth, sessionColumnWidth
	default:
		return headerWidth, 0
	}
}

func fitColumnWidths(widths []int, minWidths []int, cols []column, lineWidth int) []int {
	if lineWidth <= 0 || len(widths) == 0 {
		return widths
	}
	available := lineWidth - ((len(widths) - 1) * tableColumnGapWidth)
	if available <= 0 || sumInts(widths) <= available {
		return widths
	}

	for _, priority := range []int{1, 2} {
		for i, col := range cols {
			if shrinkPriority(col) != priority || widths[i] <= minWidths[i] {
				continue
			}
			overflow := sumInts(widths) - available
			if overflow <= 0 {
				return widths
			}
			shrinkBy := min(widths[i]-minWidths[i], overflow)
			widths[i] -= shrinkBy
		}
	}

	return widths
}

func shrinkPriority(col column) int {
	switch col.field {
	case "models", "providers", "harnesses":
		return 1
	case "model", "provider", "harness":
		return 2
	default:
		return 0
	}
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func truncateCell(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	markerWidth := ansi.StringWidth(truncationMarker)
	if width <= markerWidth {
		return ansi.Cut(value, 0, width)
	}

	limit := width - markerWidth
	var builder strings.Builder
	used := 0
	for _, r := range value {
		runeWidth := ansi.StringWidth(string(r))
		if used+runeWidth > limit {
			break
		}
		builder.WriteRune(r)
		used += runeWidth
	}
	return builder.String() + truncationMarker
}

func padCell(value string, width int, numeric bool) string {
	padding := width - ansi.StringWidth(value)
	if padding <= 0 {
		return value
	}
	if numeric {
		return strings.Repeat(" ", padding) + value
	}
	return value + strings.Repeat(" ", padding)
}
