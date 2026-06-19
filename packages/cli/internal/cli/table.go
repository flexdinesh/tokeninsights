package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

type reloadMsg struct {
	rows       []renderRow
	lastSyncMs int64
	err        error
}

type filterValuesMsg struct {
	dimension filterDimension
	values    []string
	err       error
}

type popupMode int

const (
	popupNone popupMode = iota
	popupDateRange
	popupBucket
	popupSort
	popupFilterValues
)

type groupByMode string

const (
	groupByNone    groupByMode = ""
	groupByHour    groupByMode = "hour"
	groupBySession groupByMode = "session"
)

type filterDimension int

const (
	filterProvider filterDimension = iota
	filterModel
	filterHarness
)

type interactiveModel struct {
	rows             []renderRow
	groupBy          groupByMode
	activeTab        tabMode
	period           period
	width            int
	height           int
	scrollOffset     int
	horizontalOffset int
	popup            popupMode
	popupCursor      int
	filterDimension  filterDimension
	filterValues     []string
	filterSelections map[string]bool
	filterLoading    bool
	filterErr        error
	ctx              context.Context
	options          tableOptions
	now              time.Time
	err              error
	loading          bool
	reloadInFlight   bool
	lastSyncMs       int64
	cachedWidth      int
	baseHeight       int
	perRowHeight     int
}

var aggregationTabs = []tabMode{tabTokens, tabModels, tabProviders, tabHarnesses, tabSessions, tabContext}
var dateRangeOptions = []period{periodToday, periodYesterday, periodWeek, periodMonth, periodYear, periodAllTime}
var bucketOptions = []timeBucket{bucketDay, bucketWeek, bucketMonth, bucketYear}
var defaultSortOptions = []sortMode{sortDate, sortTokens, sortInput, sortOutput, sortCacheRead, sortName}
var contextSortOptions = []sortMode{sortAverageContext, sortMedianContext, sortMaxContext, sortSessions, sortHarness, sortProvider, sortModel}

const initialLoadingPaintDelay = 75 * time.Millisecond

func (m interactiveModel) Init() tea.Cmd {
	return nil
}

func (m interactiveModel) reloadCmd() tea.Cmd {
	return func() tea.Msg {
		rows, err := loadRows(m.ctx, m.options, m.now, m.groupBy, m.activeTab)
		if err != nil {
			return reloadMsg{err: err}
		}
		lastSyncMs, err := loadLastCompletedSync(m.ctx, m.options)
		if err != nil {
			return reloadMsg{err: err}
		}
		return reloadMsg{rows: rows, lastSyncMs: lastSyncMs}
	}
}

func (m interactiveModel) deferredReloadCmd(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		rows, err := loadRows(m.ctx, m.options, m.now, m.groupBy, m.activeTab)
		if err != nil {
			return reloadMsg{err: err}
		}
		lastSyncMs, err := loadLastCompletedSync(m.ctx, m.options)
		if err != nil {
			return reloadMsg{err: err}
		}
		return reloadMsg{rows: rows, lastSyncMs: lastSyncMs}
	})
}

func (m interactiveModel) filterValuesCmd(dimension filterDimension) tea.Cmd {
	return func() tea.Msg {
		values, err := loadFilterValues(m.ctx, m.options, m.now, dimension)
		return filterValuesMsg{dimension: dimension, values: values, err: err}
	}
}

const minVisibleRows = 5
const sectionHorizontalInset = 1

func (m interactiveModel) maxVisibleRows() int {
	if m.height <= 0 {
		return 0
	}
	if m.width == m.cachedWidth && m.perRowHeight > 0 {
		available := m.height - m.baseHeight
		if available <= 0 {
			return minVisibleRows
		}
		// Leave a 3-row safety margin so dynamic content doesn't push the table
		// over the terminal edge and cause the view to jump during scroll.
		maxRows := max(0, available-3) / m.perRowHeight
		return max(minVisibleRows, maxRows)
	}
	if m.groupBy == groupByHour {
		return max(minVisibleRows, (m.height-14)/3)
	}
	return max(minVisibleRows, (m.height-14)/2)
}

func (m interactiveModel) measureHeights() interactiveModel {
	if m.width <= 0 {
		return m
	}

	title := renderTitle(m.width, m.period)

	var tabs []string
	for _, tab := range aggregationTabs {
		label := tab.String()
		if tab == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	tabBox := sectionBorderStyle.Width(m.width - 4).Render(tabBar)

	hintText := "tab/shift+tab switch · ↑/↓ j/k scroll · ←/→ scroll · home/end horizontal · d date · g bucket · s sort · p/m/h filters · q quit  ·  99999-99999 of 99999  ·  x 99999/99999"
	if filters := activeFiltersLabel(m.options.filters); filters != "" {
		hintText += "  ·  " + filters
	}
	hint := hintStyle.Render(hintText)
	hintBox := sectionBorderStyle.Width(m.width - 4).Render(hint)

	emptyTable := tableSectionStyle.Render(renderTableViewport([]renderRow{}, m.groupBy, m.activeTab, m.tableViewportWidth(), 0))
	contentBase := lipgloss.JoinVertical(lipgloss.Left, title, tabBox, emptyTable, hintBox)
	baseFull := outerBorderStyle.Width(m.width - 2).Render(contentBase)
	m.baseHeight = lipgloss.Height(baseFull)

	sampleRow := renderRow{
		day: "2006-01-01", harness: "oc", provider: "openai", model: "gpt-4o",
		inputTokens: "1000", outputTokens: "100", reasoningTokens: "10",
		cacheReadTokens: "5", cacheWriteTokens: "1", contextUsedTokens: "1006", totalTokens: "1116",
		averageContextUsedTokens: "1006", medianContextUsedTokens: "1006", maxContextUsedTokens: "1006",
		tpsAvg: "12.34", tpsMean: "56.78", tpsMedian: "45.67",
		requests: "3", retries: "1", toolName: "bash", toolCalls: "5", toolErrors: "1",
	}
	if m.groupBy == groupByHour {
		sampleRow.hour = "12:00"
	}
	if m.groupBy == groupBySession {
		sampleRow.sessionID = "sess_12345678"
		sampleRow.thinkingLevels = "low"
	}

	// Measure cost of a single data row (no separators).
	oneRowTable := tableSectionStyle.Render(renderTableViewport([]renderRow{sampleRow}, m.groupBy, m.activeTab, m.tableViewportWidth(), 0))
	contentOneRow := lipgloss.JoinVertical(lipgloss.Left, title, tabBox, oneRowTable, hintBox)
	oneRowFull := outerBorderStyle.Width(m.width - 2).Render(contentOneRow)
	perDataRow := lipgloss.Height(oneRowFull) - m.baseHeight
	if perDataRow <= 0 {
		perDataRow = 1
	}

	m.perRowHeight = perDataRow
	m.cachedWidth = m.width

	return m
}

func clampScroll(offset int, totalRows int, visible int) int {
	if totalRows <= 0 || visible <= 0 {
		return 0
	}
	maxOffset := totalRows - visible
	if maxOffset < 0 {
		return 0
	}
	if offset < 0 {
		return 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func clampHorizontalScroll(offset int, contentWidth int, viewportWidth int) int {
	if contentWidth <= 0 || viewportWidth <= 0 || contentWidth <= viewportWidth {
		return 0
	}
	maxOffset := contentWidth - viewportWidth
	if offset < 0 {
		return 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func (m interactiveModel) tableViewportWidth() int {
	return max(0, m.width-2-(sectionHorizontalInset*2))
}

func (m interactiveModel) visibleRows() []renderRow {
	return m.visibleRowsFrom(m.scrollOffset)
}

func (m interactiveModel) visibleRowsFrom(offset int) []renderRow {
	budget := m.maxVisibleRows()
	if budget <= 0 || len(m.rows) == 0 {
		return nil
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(m.rows) {
		offset = len(m.rows) - 1
	}

	cols := columnsForModeAndTab(m.groupBy, m.activeTab)
	used := 0
	end := offset
	for end < len(m.rows) {
		rowHeight := renderRowLineCount(m.rows[end], cols)
		if end > offset && used+rowHeight > budget {
			break
		}
		used += rowHeight
		end++
	}
	return m.rows[offset:end]
}

func (m interactiveModel) maxScrollOffset() int {
	if len(m.rows) == 0 {
		return 0
	}
	budget := m.maxVisibleRows()
	if budget <= 0 {
		return 0
	}

	cols := columnsForModeAndTab(m.groupBy, m.activeTab)
	used := 0
	offset := len(m.rows)
	for offset > 0 {
		rowHeight := renderRowLineCount(m.rows[offset-1], cols)
		if used+rowHeight > budget && offset < len(m.rows) {
			break
		}
		used += rowHeight
		offset--
	}
	return offset
}

func (m interactiveModel) clampScrollOffset() interactiveModel {
	maxOffset := m.maxScrollOffset()
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
	return m
}

func renderRowLineCount(row renderRow, cols []column) int {
	formatted := formatRenderRows([]renderRow{row}, cols)
	if len(formatted) == 0 {
		return 1
	}
	return formattedRowHeight(formatted[0])
}

func (m interactiveModel) maxHorizontalOffset(rows []renderRow) int {
	contentWidth := m.tableContentWidth(rows)
	viewportWidth := m.tableViewportWidth()
	if contentWidth <= 0 || viewportWidth <= 0 || contentWidth <= viewportWidth {
		return 0
	}
	return contentWidth - viewportWidth
}

func (m interactiveModel) clampHorizontalOffset() interactiveModel {
	m.horizontalOffset = clampHorizontalScroll(m.horizontalOffset, m.tableContentWidth(m.rows), m.tableViewportWidth())
	return m
}

func (m interactiveModel) tableContentWidth(rows []renderRow) int {
	return renderTableWidthWithSortAndViewport(rows, m.groupBy, m.activeTab, activeSort(m.activeTab, m.options.sort), m.tableViewportWidth())
}

func (m interactiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case reloadMsg:
		m.loading = false
		m.reloadInFlight = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.rows = msg.rows
		m.lastSyncMs = msg.lastSyncMs
		m = m.clampScrollOffset()
		m = m.clampHorizontalOffset()
		return m, nil
	case filterValuesMsg:
		if m.popup != popupFilterValues || msg.dimension != m.filterDimension {
			return m, nil
		}
		m.filterLoading = false
		m.filterErr = msg.err
		if msg.err != nil {
			return m, nil
		}
		current := m.currentFilterValues(m.filterDimension)
		m.filterValues = mergeSortedValues(msg.values, current)
		m.filterSelections = selectedValuesMap(current)
		m.popupCursor = clampPopupCursor(m.popupCursor, len(m.filterValues))
		return m, nil
	case tea.KeyMsg:
		if m.popup != popupNone {
			return m.handlePopupKey(msg)
		}
		switch msg.Type {
		case tea.KeyTab:
			m.activeTab = nextAggregationTab(m.activeTab, 1)
			m.scrollOffset = 0
			m.horizontalOffset = 0
			m.loading = true
			m.reloadInFlight = true
			m = m.measureHeights()
			return m, m.reloadCmd()
		case tea.KeyShiftTab:
			m.activeTab = nextAggregationTab(m.activeTab, -1)
			m.scrollOffset = 0
			m.horizontalOffset = 0
			m.loading = true
			m.reloadInFlight = true
			m = m.measureHeights()
			return m, m.reloadCmd()
		case tea.KeyUp:
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
			m = m.clampHorizontalOffset()
			return m, nil
		case tea.KeyDown:
			m.scrollOffset++
			m = m.clampScrollOffset()
			m = m.clampHorizontalOffset()
			return m, nil
		case tea.KeyRight:
			m.horizontalOffset = clampHorizontalScroll(m.horizontalOffset+1, m.tableContentWidth(m.rows), m.tableViewportWidth())
			return m, nil
		case tea.KeyLeft:
			m.horizontalOffset = clampHorizontalScroll(m.horizontalOffset-1, m.tableContentWidth(m.rows), m.tableViewportWidth())
			return m, nil
		case tea.KeyHome:
			m.horizontalOffset = 0
			return m, nil
		case tea.KeyEnd:
			m.horizontalOffset = m.maxHorizontalOffset(m.rows)
			return m, nil
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyRunes:
			switch string(msg.Runes) {
			case "q":
				return m, tea.Quit
			case "1", "2", "3", "4", "5", "6":
				index := int(msg.Runes[0] - '1')
				m.activeTab = aggregationTabs[index]
				m.scrollOffset = 0
				m.horizontalOffset = 0
				m.loading = true
				m.reloadInFlight = true
				m = m.measureHeights()
				return m, m.reloadCmd()
			case "d":
				m.popup = popupDateRange
				m.popupCursor = indexOfPeriod(m.options.period)
				return m, nil
			case "g":
				m.popup = popupBucket
				m.popupCursor = indexOfBucket(m.options.bucket)
				return m, nil
			case "s":
				m.popup = popupSort
				m.popupCursor = indexOfSort(activeSort(m.activeTab, m.options.sort), m.activeTab)
				m.filterErr = nil
				return m, nil
			case "p":
				return m.openFilterValues(filterProvider)
			case "m":
				return m.openFilterValues(filterModel)
			case "h":
				return m.openFilterValues(filterHarness)
			case "j":
				m.scrollOffset++
				m = m.clampScrollOffset()
				m = m.clampHorizontalOffset()
				return m, nil
			case "k":
				if m.scrollOffset > 0 {
					m.scrollOffset--
				}
				m = m.clampHorizontalOffset()
				return m, nil
			case "l":
				m.horizontalOffset = clampHorizontalScroll(m.horizontalOffset+1, m.tableContentWidth(m.rows), m.tableViewportWidth())
				return m, nil
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.measureHeights()
		m = m.clampScrollOffset()
		m = m.clampHorizontalOffset()
		if m.loading && !m.reloadInFlight {
			m.reloadInFlight = true
			return m, m.deferredReloadCmd(initialLoadingPaintDelay)
		}
	}
	return m, nil
}

func indexOfPeriod(value period) int {
	for i, option := range dateRangeOptions {
		if option == value {
			return i
		}
	}
	return 0
}

func indexOfBucket(value timeBucket) int {
	for i, option := range bucketOptions {
		if option == value {
			return i
		}
	}
	return 0
}

func indexOfSort(value sortMode, activeTab tabMode) int {
	for i, option := range sortOptionsForTab(activeTab) {
		if option == value {
			return i
		}
	}
	return 0
}

func nextAggregationTab(active tabMode, delta int) tabMode {
	activeIndex := 0
	for i, tab := range aggregationTabs {
		if tab == active {
			activeIndex = i
			break
		}
	}
	next := activeIndex + delta
	for next < 0 {
		next += len(aggregationTabs)
	}
	for next >= len(aggregationTabs) {
		next -= len(aggregationTabs)
	}
	return aggregationTabs[next]
}

func (m interactiveModel) handlePopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.popup {
	case popupDateRange:
		return m.handleDateRangePopupKey(msg)
	case popupBucket:
		return m.handleBucketPopupKey(msg)
	case popupSort:
		return m.handleSortPopupKey(msg)
	case popupFilterValues:
		return m.handleFilterValuesKey(msg)
	default:
		return m, nil
	}
}

func (m interactiveModel) handleDateRangePopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.popupCursor = movePopupCursor(m.popupCursor, len(dateRangeOptions), -1)
		return m, nil
	case tea.KeyDown:
		m.popupCursor = movePopupCursor(m.popupCursor, len(dateRangeOptions), 1)
		return m, nil
	case tea.KeyEnter:
		return m.applyDateRangePopup()
	case tea.KeySpace:
		return m.applyDateRangePopup()
	case tea.KeyEsc:
		m.popup = popupNone
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			m.popup = popupNone
			return m, nil
		case "j":
			m.popupCursor = movePopupCursor(m.popupCursor, len(dateRangeOptions), 1)
			return m, nil
		case "k":
			m.popupCursor = movePopupCursor(m.popupCursor, len(dateRangeOptions), -1)
			return m, nil
		case " ":
			return m.applyDateRangePopup()
		}
	}
	return m, nil
}

func (m interactiveModel) applyDateRangePopup() (tea.Model, tea.Cmd) {
	newPeriod := dateRangeOptions[m.popupCursor]
	m.popup = popupNone
	if newPeriod != m.options.period {
		m.options.period = newPeriod
		m.period = newPeriod
		m.scrollOffset = 0
		m.horizontalOffset = 0
		m.loading = true
		m.reloadInFlight = true
		m = m.measureHeights()
		return m, m.reloadCmd()
	}
	return m, nil
}

func (m interactiveModel) handleBucketPopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.popupCursor = movePopupCursor(m.popupCursor, len(bucketOptions), -1)
		return m, nil
	case tea.KeyDown:
		m.popupCursor = movePopupCursor(m.popupCursor, len(bucketOptions), 1)
		return m, nil
	case tea.KeyEnter:
		return m.applyBucketPopup()
	case tea.KeySpace:
		return m.applyBucketPopup()
	case tea.KeyEsc:
		m.popup = popupNone
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			m.popup = popupNone
			return m, nil
		case "j":
			m.popupCursor = movePopupCursor(m.popupCursor, len(bucketOptions), 1)
			return m, nil
		case "k":
			m.popupCursor = movePopupCursor(m.popupCursor, len(bucketOptions), -1)
			return m, nil
		case " ":
			return m.applyBucketPopup()
		}
	}
	return m, nil
}

func (m interactiveModel) applyBucketPopup() (tea.Model, tea.Cmd) {
	newBucket := bucketOptions[m.popupCursor]
	m.popup = popupNone
	if newBucket != m.options.bucket {
		m.options.bucket = newBucket
		m.scrollOffset = 0
		m.horizontalOffset = 0
		m.loading = true
		m.reloadInFlight = true
		m = m.measureHeights()
		return m, m.reloadCmd()
	}
	return m, nil
}

func (m interactiveModel) handleSortPopupKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	options := sortOptionsForTab(m.activeTab)
	switch msg.Type {
	case tea.KeyUp:
		m.popupCursor = movePopupCursor(m.popupCursor, len(options), -1)
		return m, nil
	case tea.KeyDown:
		m.popupCursor = movePopupCursor(m.popupCursor, len(options), 1)
		return m, nil
	case tea.KeyEnter:
		return m.applySortPopup()
	case tea.KeySpace:
		return m.applySortPopup()
	case tea.KeyEsc:
		m.popup = popupNone
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			m.popup = popupNone
			return m, nil
		case "j":
			m.popupCursor = movePopupCursor(m.popupCursor, len(options), 1)
			return m, nil
		case "k":
			m.popupCursor = movePopupCursor(m.popupCursor, len(options), -1)
			return m, nil
		case " ":
			return m.applySortPopup()
		}
	}
	return m, nil
}

func (m interactiveModel) applySortPopup() (tea.Model, tea.Cmd) {
	options := sortOptionsForTab(m.activeTab)
	newSort := options[m.popupCursor]
	m.popup = popupNone
	if newSort != m.options.sort {
		m.options.sort = newSort
		m.scrollOffset = 0
		m.horizontalOffset = 0
		m.loading = true
		m.reloadInFlight = true
		m = m.measureHeights()
		return m, m.reloadCmd()
	}
	return m, nil
}

func (m interactiveModel) openFilterValues(dimension filterDimension) (tea.Model, tea.Cmd) {
	m.filterDimension = dimension
	m.popup = popupFilterValues
	m.popupCursor = 0
	m.filterValues = nil
	m.filterSelections = selectedValuesMap(m.currentFilterValues(m.filterDimension))
	m.filterLoading = true
	m.filterErr = nil
	return m, m.filterValuesCmd(m.filterDimension)
}

func (m interactiveModel) handleFilterValuesKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyUp:
		m.popupCursor = movePopupCursor(m.popupCursor, len(m.filterValues), -1)
		return m, nil
	case tea.KeyDown:
		m.popupCursor = movePopupCursor(m.popupCursor, len(m.filterValues), 1)
		return m, nil
	case tea.KeyEnter:
		return m.applyFilterValues()
	case tea.KeySpace:
		return m.toggleFilterValue(), nil
	case tea.KeyEsc:
		m.popup = popupNone
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "q":
			m.popup = popupNone
			return m, nil
		case "j":
			m.popupCursor = movePopupCursor(m.popupCursor, len(m.filterValues), 1)
			return m, nil
		case "k":
			m.popupCursor = movePopupCursor(m.popupCursor, len(m.filterValues), -1)
			return m, nil
		case "c":
			m.filterSelections = make(map[string]bool)
			return m, nil
		case " ":
			return m.toggleFilterValue(), nil
		}
	}
	return m, nil
}

func (m interactiveModel) toggleFilterValue() interactiveModel {
	if len(m.filterValues) == 0 {
		return m
	}
	if m.filterSelections == nil {
		m.filterSelections = make(map[string]bool)
	}
	value := m.filterValues[m.popupCursor]
	m.filterSelections[value] = !m.filterSelections[value]
	return m
}

func (m interactiveModel) applyFilterValues() (tea.Model, tea.Cmd) {
	if m.filterLoading || m.filterErr != nil {
		return m, nil
	}
	selected := selectedValues(m.filterValues, m.filterSelections)
	switch m.filterDimension {
	case filterProvider:
		m.options.filters.providers = stringList(selected)
	case filterModel:
		m.options.filters.models = stringList(selected)
	case filterHarness:
		m.options.filters.harnesses = stringList(selected)
	}
	m.popup = popupNone
	m.scrollOffset = 0
	m.horizontalOffset = 0
	m.loading = true
	m.reloadInFlight = true
	m = m.measureHeights()
	return m, m.reloadCmd()
}

func movePopupCursor(cursor int, length int, delta int) int {
	if length <= 0 {
		return 0
	}
	cursor += delta
	for cursor < 0 {
		cursor += length
	}
	for cursor >= length {
		cursor -= length
	}
	return cursor
}

func sortOptionsForTab(activeTab tabMode) []sortMode {
	if activeTab == tabContext {
		return contextSortOptions
	}
	return defaultSortOptions
}

func clampPopupCursor(cursor int, length int) int {
	if length <= 0 || cursor < 0 {
		return 0
	}
	if cursor >= length {
		return length - 1
	}
	return cursor
}

func (m interactiveModel) currentFilterValues(dimension filterDimension) []string {
	switch dimension {
	case filterProvider:
		return []string(m.options.filters.providers)
	case filterModel:
		return []string(m.options.filters.models)
	case filterHarness:
		return []string(m.options.filters.harnesses)
	default:
		return nil
	}
}

func selectedValuesMap(values []string) map[string]bool {
	result := make(map[string]bool)
	for _, value := range values {
		result[value] = true
	}
	return result
}

func selectedValues(values []string, selections map[string]bool) []string {
	var result []string
	for _, value := range values {
		if selections[value] {
			result = append(result, value)
		}
	}
	return result
}

func mergeSortedValues(a []string, b []string) []string {
	set := make(map[string]bool)
	for _, value := range a {
		if value != "" {
			set[value] = true
		}
	}
	for _, value := range b {
		if value != "" {
			set[value] = true
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("63")).
			Padding(0, 2)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Background(lipgloss.Color(panelBackgroundColor)).
				Padding(0, 2)

	tableSectionStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(appBackgroundColor)).
				Padding(0, sectionHorizontalInset)

	popupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("212")).
			BorderBackground(lipgloss.Color(panelBackgroundColor)).
			Background(lipgloss.Color(panelBackgroundColor)).
			Padding(1, 2)

	popupTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212")).
			Background(lipgloss.Color(panelBackgroundColor))
	popupCursorStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("63")).
				Background(lipgloss.Color(panelBackgroundColor))
	popupItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color(panelBackgroundColor))
)

func groupByLabel(g groupByMode) string {
	switch g {
	case groupBySession:
		return "session"
	case groupByHour:
		return "hour"
	default:
		return "day"
	}
}

func periodLabel(value period) string {
	switch value {
	case periodAllTime:
		return "all time"
	default:
		return strings.ReplaceAll(string(value), "_", " ")
	}
}

func filterDimensionLabel(dimension filterDimension) string {
	switch dimension {
	case filterProvider:
		return "provider"
	case filterModel:
		return "model"
	case filterHarness:
		return "harness"
	default:
		return ""
	}
}

func renderTitle(width int, value period) string {
	return titleStyle.Width(max(0, width-2)).Render(fmt.Sprintf("TokenInsights %s", value))
}

func (m interactiveModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	if m.popup != popupNone {
		return renderOnAppSurface(
			lipgloss.Place(
				m.width,
				m.height,
				lipgloss.Center,
				lipgloss.Center,
				m.renderPopup(),
				lipgloss.WithWhitespaceBackground(lipgloss.Color(appBackgroundColor)),
			),
			m.width,
			m.height,
		)
	}

	title := renderTitle(m.width, m.period)

	var tabs []string
	for _, tab := range aggregationTabs {
		label := tab.String()
		if tab == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	tabBox := sectionBorderStyle.Width(m.width - 4).Render(tabBar)

	visible := m.maxVisibleRows()
	visibleRows := m.visibleRows()
	viewportWidth := m.tableViewportWidth()
	maxHorizontal := 0
	horizontalOffset := 0
	selectedSort := activeSort(m.activeTab, m.options.sort)
	if !m.loading {
		contentWidth := m.tableContentWidth(m.rows)
		maxHorizontal = max(0, contentWidth-viewportWidth)
		horizontalOffset = clampHorizontalScroll(m.horizontalOffset, contentWidth, viewportWidth)
	}

	hintText := "tab/shift+tab switch · ↑/↓ j/k scroll · ←/→ scroll · home/end horizontal · d date · g bucket · s sort · p/m/h filters · q quit"
	visibleCount := len(visibleRows)
	if !m.loading && visibleCount > 0 && len(m.rows) > visibleCount {
		end := m.scrollOffset + visibleCount
		if end > len(m.rows) {
			end = len(m.rows)
		}
		hintText += fmt.Sprintf("  ·  %5d-%5d of %5d", m.scrollOffset+1, end, len(m.rows))
	}
	if maxHorizontal > 0 {
		hintText += fmt.Sprintf("  ·  x %5d/%5d", horizontalOffset+1, maxHorizontal+1)
	}
	if filters := activeFiltersLabel(m.options.filters); filters != "" {
		hintText += "  ·  " + filters
	}
	if m.loading {
		hintText += fmt.Sprintf("  ·  loading  ·  sync %s", formatLastSync(m.lastSyncMs))
	} else {
		hintText += fmt.Sprintf("  ·  total %s  ·  rows %d  ·  sync %s", formatTokens(totalTokens(m.rows)), len(m.rows), formatLastSync(m.lastSyncMs))
	}
	hint := hintStyle.Render(hintText)
	hintBox := sectionBorderStyle.Width(m.width - 4).Render(hint)

	var body string
	if m.loading {
		body = renderLoadingTableViewportWithSort(m.groupBy, m.activeTab, selectedSort, viewportWidth, visible)
	} else {
		body = renderTableViewportWithSort(visibleRows, m.rows, m.groupBy, m.activeTab, selectedSort, viewportWidth, horizontalOffset, visible)
	}
	body = tableSectionStyle.Render(body)

	content := lipgloss.JoinVertical(lipgloss.Left, title, tabBox, body, hintBox)
	rendered := outerBorderStyle.Width(m.width - 2).Render(content)
	return renderOnAppSurface(rendered, m.width, m.height)
}

func activeFiltersLabel(f filters) string {
	var parts []string
	if len(f.providers) > 0 {
		parts = append(parts, "provider="+strings.Join([]string(f.providers), ","))
	}
	if len(f.models) > 0 {
		parts = append(parts, "model="+strings.Join([]string(f.models), ","))
	}
	if len(f.harnesses) > 0 {
		parts = append(parts, "harness="+strings.Join([]string(f.harnesses), ","))
	}
	if len(parts) == 0 {
		return ""
	}
	return "filters: " + strings.Join(parts, " ")
}

func totalTokens(rows []renderRow) int64 {
	var total int64
	for _, row := range rows {
		total += row.totalValue
	}
	return total
}

func formatLastSync(value int64) string {
	if value <= 0 {
		return "never"
	}
	return formatLatest(value)
}

func (m interactiveModel) renderPopup() string {
	switch m.popup {
	case popupDateRange:
		return m.renderDateRangePopup()
	case popupBucket:
		return m.renderBucketPopup()
	case popupSort:
		return m.renderSortPopup()
	case popupFilterValues:
		return m.renderFilterValuesPopup()
	default:
		return ""
	}
}

func (m interactiveModel) renderDateRangePopup() string {
	title := popupTitleStyle.Render("Date range")
	var options []string
	for i, opt := range dateRangeOptions {
		cursor := "  "
		style := popupItemStyle
		if i == m.popupCursor {
			cursor = "> "
			style = popupCursorStyle
		}
		options = append(options, style.Render(cursor+periodLabel(opt)))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, options...)
	help := hintStyle.Render("space/enter = select · esc = close")
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
}

func (m interactiveModel) renderBucketPopup() string {
	title := popupTitleStyle.Render("Time bucket")
	var options []string
	for i, opt := range bucketOptions {
		cursor := "  "
		style := popupItemStyle
		if i == m.popupCursor {
			cursor = "> "
			style = popupCursorStyle
		}
		options = append(options, style.Render(cursor+string(opt)))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, options...)
	help := hintStyle.Render("space/enter = select · esc = close")
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
}

func (m interactiveModel) renderSortPopup() string {
	title := popupTitleStyle.Render("Sort")
	sortModes := sortOptionsForTab(m.activeTab)
	var options []string
	for i, opt := range sortModes {
		cursor := "  "
		style := popupItemStyle
		if i == m.popupCursor {
			cursor = "> "
			style = popupCursorStyle
		}
		options = append(options, style.Render(cursor+string(opt)))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, options...)
	help := hintStyle.Render("space/enter = select · esc = close")
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
}

func (m interactiveModel) renderFilterValuesPopup() string {
	title := popupTitleStyle.Render("Filter " + filterDimensionLabel(m.filterDimension))
	var body string
	if m.filterLoading {
		body = popupItemStyle.Render("Loading values...")
	} else if m.filterErr != nil {
		body = popupItemStyle.Render(fmt.Sprintf("Error: %v", m.filterErr))
	} else if len(m.filterValues) == 0 {
		body = popupItemStyle.Render("No values available")
	} else {
		var options []string
		for i, value := range m.filterValues {
			cursor := "  "
			style := popupItemStyle
			if i == m.popupCursor {
				cursor = "> "
				style = popupCursorStyle
			}
			checked := "[ ] "
			if m.filterSelections[value] {
				checked = "[x] "
			}
			options = append(options, style.Render(cursor+checked+value))
		}
		body = lipgloss.JoinVertical(lipgloss.Left, options...)
	}
	help := hintStyle.Render("space = select · enter = apply · esc = close without applying")
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
}

func RunInteractive(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	options, err := parseTableOptions(args, stderr, false, periodMonth)
	if err != nil {
		return err
	}

	_, err = tea.NewProgram(interactiveModel{
		ctx:              ctx,
		options:          options,
		now:              now,
		period:           options.period,
		groupBy:          groupByNone,
		activeTab:        tabTokens,
		popupCursor:      0,
		filterSelections: make(map[string]bool),
		loading:          true,
	}, tea.WithAltScreen(), tea.WithInput(os.Stdin), tea.WithOutput(stdout)).Run()
	return err
}

func filterFromOptions(options tableOptions, now time.Time) db.Filter {
	start := periodStart(now, options.period)
	end := periodEnd(now, options.period)
	if options.filters.dayFrom != "" || options.filters.dayTo != "" {
		start = time.Time{}
		end = time.Time{}
	}
	return db.Filter{
		Start:      start,
		End:        end,
		SessionIDs: []string(options.filters.sessionIDs),
		Providers:  []string(options.filters.providers),
		Models:     []string(options.filters.models),
		Harnesses:  []string(options.filters.harnesses),
		DayFrom:    options.filters.dayFrom,
		DayTo:      options.filters.dayTo,
	}
}

func loadFilterValues(ctx context.Context, options tableOptions, now time.Time, dimension filterDimension) ([]string, error) {
	database, err := db.Open(options.dbPath)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	filter := filterFromOptions(options, now)
	switch dimension {
	case filterProvider:
		return db.AvailableProviders(ctx, database, filter)
	case filterModel:
		return db.AvailableModels(ctx, database, filter)
	case filterHarness:
		return db.AvailableHarnesses(ctx, database, filter)
	default:
		return nil, nil
	}
}

func loadLastCompletedSync(ctx context.Context, options tableOptions) (int64, error) {
	database, err := db.Open(options.dbPath)
	if err != nil {
		return 0, err
	}
	defer database.Close()
	return db.LastCompletedSync(ctx, database)
}

func loadRows(ctx context.Context, options tableOptions, now time.Time, groupBy groupByMode, activeTab tabMode) ([]renderRow, error) {
	database, err := db.Open(options.dbPath)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	f := filterFromOptions(options, now)
	if activeTab == tabTokens {
		aggRows, err := db.ViewerTokenBuckets(ctx, database, f, db.TimeBucket(options.bucket))
		if err != nil {
			return nil, err
		}
		result := make([]renderRow, len(aggRows))
		for i, r := range aggRows {
			result[i] = renderRow{
				bucket:           r.Bucket,
				sessions:         formatTokens(r.SessionCount),
				inputTokens:      formatTokens(r.InputTokens),
				inputValue:       r.InputTokens,
				outputTokens:     formatTokens(r.OutputTokens),
				outputValue:      r.OutputTokens,
				reasoningTokens:  formatTokens(r.ReasoningTokens),
				cacheReadTokens:  formatTokens(r.CacheReadTokens),
				cacheReadValue:   r.CacheReadTokens,
				cacheWriteTokens: formatTokens(r.CacheWriteTokens),
				totalTokens:      formatTokens(r.TotalTokens),
				totalValue:       r.TotalTokens,
				latestValue:      r.LatestAtMs,
			}
		}
		sortRenderRows(result, activeTab, options.sort)
		return result, nil
	}
	if activeTab == tabModels || activeTab == tabProviders || activeTab == tabHarnesses {
		aggRows, err := loadDimensionRows(ctx, database, f, activeTab)
		if err != nil {
			return nil, err
		}
		result := make([]renderRow, len(aggRows))
		for i, r := range aggRows {
			result[i] = renderRow{
				model:            r.Model,
				provider:         r.Provider,
				harness:          r.Harness,
				models:           r.Models,
				providers:        r.Providers,
				harnesses:        r.Harnesses,
				sessions:         formatTokens(r.SessionCount),
				inputTokens:      formatTokens(r.InputTokens),
				inputValue:       r.InputTokens,
				outputTokens:     formatTokens(r.OutputTokens),
				outputValue:      r.OutputTokens,
				reasoningTokens:  formatTokens(r.ReasoningTokens),
				cacheReadTokens:  formatTokens(r.CacheReadTokens),
				cacheReadValue:   r.CacheReadTokens,
				cacheWriteTokens: formatTokens(r.CacheWriteTokens),
				totalTokens:      formatTokens(r.TotalTokens),
				totalValue:       r.TotalTokens,
				latestValue:      r.LatestAtMs,
			}
		}
		sortRenderRows(result, activeTab, options.sort)
		return result, nil
	}
	if activeTab == tabSessions {
		aggRows, err := db.ViewerSessions(ctx, database, f)
		if err != nil {
			return nil, err
		}
		result := make([]renderRow, len(aggRows))
		for i, r := range aggRows {
			result[i] = renderRow{
				latest:            formatLatest(r.LatestAtMs),
				sessionID:         r.SessionID,
				harness:           r.Harness,
				providers:         r.Providers,
				models:            r.Models,
				contextUsedTokens: formatContextTokens(r.ContextUsedTokens),
				contextUsedValue:  r.ContextUsedTokens,
				inputTokens:       formatTokens(r.InputTokens),
				inputValue:        r.InputTokens,
				outputTokens:      formatTokens(r.OutputTokens),
				outputValue:       r.OutputTokens,
				reasoningTokens:   formatTokens(r.ReasoningTokens),
				cacheReadTokens:   formatTokens(r.CacheReadTokens),
				cacheReadValue:    r.CacheReadTokens,
				cacheWriteTokens:  formatTokens(r.CacheWriteTokens),
				totalTokens:       formatTokens(r.TotalTokens),
				totalValue:        r.TotalTokens,
				latestValue:       r.LatestAtMs,
			}
		}
		sortRenderRows(result, activeTab, options.sort)
		return result, nil
	}
	if activeTab == tabContext {
		aggRows, err := db.ViewerContext(ctx, database, f)
		if err != nil {
			return nil, err
		}
		result := make([]renderRow, len(aggRows))
		for i, r := range aggRows {
			result[i] = renderRow{
				harness:                  r.Harness,
				provider:                 r.Provider,
				model:                    r.Model,
				sessions:                 formatTokens(r.SessionCount),
				sessionsValue:            r.SessionCount,
				averageContextUsedTokens: formatContextTokens(r.AverageContextUsedTokens),
				averageContextUsedValue:  r.AverageContextUsedTokens,
				medianContextUsedTokens:  formatContextTokens(r.MedianContextUsedTokens),
				medianContextUsedValue:   r.MedianContextUsedTokens,
				maxContextUsedTokens:     formatContextTokens(r.MaxContextUsedTokens),
				maxContextUsedValue:      r.MaxContextUsedTokens,
				latestValue:              r.LatestAtMs,
			}
		}
		sortRenderRows(result, activeTab, options.sort)
		return result, nil
	}

	var g db.GroupBy
	switch groupBy {
	case groupByHour:
		g = db.GroupByDayHour
	case groupBySession:
		g = db.GroupByDaySession
	default:
		g = db.GroupByDay
	}

	var aggRows []db.Row
	switch activeTab {
	default:
		aggRows, err = db.AggregateTokens(ctx, database, f, g)
	}
	if err != nil {
		return nil, err
	}

	result := make([]renderRow, len(aggRows))
	for i, r := range aggRows {
		result[i] = renderRow{
			harness:          r.Harness,
			day:              r.Day,
			hour:             r.Hour,
			sessionID:        r.SessionID,
			provider:         r.Provider,
			model:            r.Model,
			thinkingLevels:   r.ThinkingLevels,
			tpsAvg:           formatWeightedTPS(r.ThroughputTokens, r.DurationMs),
			tpsMean:          formatMeanTPS(r.TpsMean),
			tpsMedian:        formatMedianTPS(r.TpsMedian),
			inputTokens:      formatTokens(r.InputTokens),
			inputValue:       r.InputTokens,
			outputTokens:     formatTokens(r.OutputTokens),
			outputValue:      r.OutputTokens,
			reasoningTokens:  formatTokens(r.ReasoningTokens),
			cacheReadTokens:  formatTokens(r.CacheReadTokens),
			cacheReadValue:   r.CacheReadTokens,
			cacheWriteTokens: formatTokens(r.CacheWriteTokens),
			totalTokens:      formatTokens(r.TotalTokens),
			totalValue:       r.TotalTokens,
			latestValue:      r.LatestAtMs,
			requests:         formatTokens(r.Requests),
			retries:          formatTokens(r.Retries),
			toolName:         r.ToolName,
			toolCalls:        formatTokens(r.ToolCalls),
			toolErrors:       formatTokens(r.ToolErrors),
		}
	}
	return result, nil
}

func loadDimensionRows(ctx context.Context, database *sql.DB, f db.Filter, activeTab tabMode) ([]db.ViewerDimensionRow, error) {
	switch activeTab {
	case tabModels:
		return db.ViewerModels(ctx, database, f)
	case tabProviders:
		return db.ViewerProviders(ctx, database, f)
	case tabHarnesses:
		return db.ViewerHarnesses(ctx, database, f)
	default:
		return nil, nil
	}
}

func sortRenderRows(rows []renderRow, activeTab tabMode, selected sortMode) {
	selected = activeSort(activeTab, selected)
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]
		switch selected {
		case sortName:
			return rowName(left, activeTab) < rowName(right, activeTab)
		case sortInput:
			if left.inputValue != right.inputValue {
				return left.inputValue > right.inputValue
			}
		case sortOutput:
			if left.outputValue != right.outputValue {
				return left.outputValue > right.outputValue
			}
		case sortCacheRead:
			if left.cacheReadValue != right.cacheReadValue {
				return left.cacheReadValue > right.cacheReadValue
			}
		case sortAverageContext:
			if left.averageContextUsedValue != right.averageContextUsedValue {
				return left.averageContextUsedValue > right.averageContextUsedValue
			}
		case sortMedianContext:
			if left.medianContextUsedValue != right.medianContextUsedValue {
				return left.medianContextUsedValue > right.medianContextUsedValue
			}
		case sortMaxContext:
			if left.maxContextUsedValue != right.maxContextUsedValue {
				return left.maxContextUsedValue > right.maxContextUsedValue
			}
		case sortSessions:
			if left.sessionsValue != right.sessionsValue {
				return left.sessionsValue > right.sessionsValue
			}
		case sortHarness:
			if left.harness != right.harness {
				return left.harness < right.harness
			}
		case sortProvider:
			if left.provider != right.provider {
				return left.provider < right.provider
			}
		case sortModel:
			if left.model != right.model {
				return left.model < right.model
			}
		case sortDate:
			if activeTab == tabTokens && left.bucket != right.bucket {
				return left.bucket > right.bucket
			}
			if left.latestValue != right.latestValue {
				return left.latestValue > right.latestValue
			}
		default:
			if left.totalValue != right.totalValue {
				return left.totalValue > right.totalValue
			}
		}
		if left.totalValue != right.totalValue {
			return left.totalValue > right.totalValue
		}
		return rowName(left, activeTab) < rowName(right, activeTab)
	})
}

func activeSort(activeTab tabMode, selected sortMode) sortMode {
	if selected != "" {
		return selected
	}
	switch activeTab {
	case tabTokens, tabSessions:
		return sortDate
	case tabContext:
		return sortAverageContext
	default:
		return sortTokens
	}
}

func rowName(row renderRow, activeTab tabMode) string {
	switch activeTab {
	case tabModels:
		return row.model
	case tabProviders:
		return row.provider
	case tabHarnesses:
		return row.harness
	case tabSessions:
		return row.sessionID
	case tabContext:
		return row.harness + "\x00" + row.provider + "\x00" + row.model
	default:
		return row.bucket
	}
}
