package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

type reloadMsg struct {
	rows          []renderRow
	lastSyncMs    int64
	sessionCounts db.SessionCounts
	err           error
}

type filterValuesMsg struct {
	dimension filterDimension
	values    []string
	err       error
}

type syncProgressMsg struct {
	event pipeline.SyncProgressEvent
}

type syncDoneMsg struct {
	summary pipeline.Summary
	err     error
}

type startDashboardLoadMsg struct{}

type syncAnimationTickMsg struct{}

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
	statusline       statuslineModel
	tableSummary     tableSummaryModel
	sessionCounts    db.SessionCounts
	width            int
	height           int
	scrollOffset     int
	cursor           int
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
	syncing          bool
	syncInFlight     bool
	syncMessages     <-chan tea.Msg
	syncProgressRows []syncProgressRow
	syncStatus       pipeline.SyncProgressStatus
	syncFrame        int
	syncSummary      pipeline.Summary
	syncErr          error
	lastSyncMs       int64
	cachedWidth      int
	baseHeight       int
	perRowHeight     int
}

type syncProgressRow struct {
	harness pipeline.Harness
	label   string
	status  pipeline.SyncProgressStatus
}

var aggregationTabs = []tabMode{tabTokens, tabModels, tabProviders, tabHarnesses, tabSessions, tabContext}
var dateRangeOptions = []period{periodToday, periodYesterday, periodWeek, periodMonth, periodYear, periodAllTime}
var bucketOptions = []timeBucket{bucketDay, bucketWeek, bucketMonth, bucketYear}
var defaultSortOptions = []sortMode{sortDate, sortTokens, sortInput, sortOutput, sortCacheRead, sortName}
var contextSortOptions = []sortMode{sortAverageContext, sortMedianContext, sortMaxContext, sortSessions, sortHarness, sortProvider, sortModel}

const initialLoadingPaintDelay = 75 * time.Millisecond
const syncAnimationInterval = 120 * time.Millisecond

var syncSpinnerFrames = []string{"|", "/", "-", "\\"}

func initialSyncProgressRows() []syncProgressRow {
	rows := make([]syncProgressRow, 0, len(pipeline.SupportedHarnesses))
	for _, harness := range pipeline.SupportedHarnesses {
		rows = append(rows, syncProgressRow{
			harness: harness,
			label:   syncHarnessDisplayName(harness),
			status:  "pending",
		})
	}
	return rows
}

func syncHarnessDisplayName(harness pipeline.Harness) string {
	switch harness {
	case pipeline.HarnessOpenCode:
		return "OpenCode"
	case pipeline.HarnessPi:
		return "Pi"
	case pipeline.HarnessCodex:
		return "Codex"
	case pipeline.HarnessClaudeCode:
		return "Claude Code"
	default:
		return string(harness)
	}
}

func (m interactiveModel) Init() tea.Cmd {
	return nil
}

func newInteractiveModel(ctx context.Context, options tableOptions, now time.Time, hostname string) interactiveModel {
	m := interactiveModel{
		ctx:              ctx,
		options:          options,
		now:              now,
		groupBy:          groupByNone,
		activeTab:        tabTokens,
		popupCursor:      0,
		filterSelections: make(map[string]bool),
		loading:          options.noSync,
		syncing:          !options.noSync,
		syncProgressRows: initialSyncProgressRows(),
	}
	m.statusline = newStatuslineModel(statuslineDateRangeLabel(options), string(options.bucket), string(activeSort(tabTokens, options.sort)), hostname, 0)
	m.tableSummary = newTableSummaryModel(nil, m.activeTab, m.loading)
	return m
}

func (m interactiveModel) reconcileStatusline() interactiveModel {
	if len(m.statusline.items) == 0 {
		m.statusline = newStatuslineModel(statuslineDateRangeLabel(m.options), string(m.options.bucket), string(activeSort(m.activeTab, m.options.sort)), "unknown", m.lastSyncMs)
		return m
	}
	m.statusline = m.statusline.
		withValue(statuslineDateRange, statuslineDateRangeLabel(m.options)).
		withValue(statuslineBucket, string(m.options.bucket)).
		withValue(statuslineSort, string(activeSort(m.activeTab, m.options.sort))).
		withValue(statuslineLastSynced, formatLastSync(m.lastSyncMs))
	return m
}

func (m interactiveModel) renderStatusline() string {
	m = m.reconcileStatusline()
	return m.statusline.View(max(0, m.width-2))
}

func (m interactiveModel) reconcileTableSummary() interactiveModel {
	m.tableSummary = newTableSummaryModel(m.rows, m.activeTab, m.loading)
	m.tableSummary.sessionCounts = m.sessionCounts
	return m
}

func (m interactiveModel) renderTableSummary() string {
	m = m.reconcileTableSummary()
	return m.tableSummary.View(m.tableViewportWidth())
}

func renderTableSection(body string, summary string) string {
	body = strings.TrimSuffix(body, "\n")
	return tableSectionStyle.Render(lipgloss.JoinVertical(lipgloss.Left, body, "", summary))
}

func (m interactiveModel) reloadCmd() tea.Cmd {
	return func() tea.Msg {
		return m.loadDashboard()
	}
}

func (m interactiveModel) deferredReloadCmd(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return m.loadDashboard()
	})
}

func (m interactiveModel) loadDashboard() reloadMsg {
	database, err := db.Open(m.options.dbPath)
	if err != nil {
		return reloadMsg{err: err}
	}
	defer func() { _ = database.Close() }()
	tx, err := db.BeginAnalyticsRead(m.ctx, database)
	if err != nil {
		return reloadMsg{err: err}
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := loadRowsFromReader(m.ctx, tx, m.options, m.now, m.groupBy, m.activeTab)
	if err != nil {
		return reloadMsg{err: err}
	}
	counts, err := db.ViewerSessionCounts(m.ctx, tx, filterFromOptions(m.options, m.now))
	if err != nil {
		return reloadMsg{err: err}
	}
	lastSyncMs, err := db.LastCompletedSync(m.ctx, tx)
	return reloadMsg{rows: rows, lastSyncMs: lastSyncMs, sessionCounts: counts, err: err}
}

func (m interactiveModel) filterValuesCmd(dimension filterDimension) tea.Cmd {
	return func() tea.Msg {
		values, err := loadFilterValues(m.ctx, m.options, m.now, dimension)
		return filterValuesMsg{dimension: dimension, values: values, err: err}
	}
}

func (m interactiveModel) maxVisibleRows() int {
	if m.height <= 0 {
		return 0
	}
	available := m.height - tuiChromeHeight
	if available <= 0 {
		return tuiMinVisibleLines
	}
	return max(tuiMinVisibleLines, available)
}

func (m interactiveModel) measureHeights() interactiveModel {
	m.baseHeight = tuiChromeHeight
	m.perRowHeight = 1
	m.cachedWidth = m.width
	return m
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
	return max(0, m.width-2)
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

func (m interactiveModel) resetRowPosition() interactiveModel {
	m.scrollOffset = 0
	m.cursor = 0
	return m
}

func (m interactiveModel) clampCursor() interactiveModel {
	if len(m.rows) == 0 {
		m.cursor = 0
		return m
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	return m
}

func (m interactiveModel) ensureCursorVisible() interactiveModel {
	m = m.clampCursor()
	if m.cursor < m.scrollOffset {
		m.scrollOffset = m.cursor
	}
	for i := 0; i < len(m.rows); i++ {
		if m.cursor < m.scrollOffset+len(m.visibleRowsFrom(m.scrollOffset)) {
			break
		}
		m.scrollOffset++
	}
	return m.clampScrollOffset()
}

func (m interactiveModel) moveCursor(delta int) interactiveModel {
	if len(m.rows) == 0 {
		return m
	}
	m = m.clampCursor()
	m.cursor += delta
	m = m.clampCursor()
	m = m.ensureCursorVisible()
	return m.clampHorizontalOffset()
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
	case syncProgressMsg:
		m = m.withSyncProgress(msg.event)
		if m.syncing && m.syncInFlight && m.syncMessages != nil {
			return m, readSyncProgressCmd(m.syncMessages)
		}
		return m, nil
	case syncAnimationTickMsg:
		m.syncFrame++
		if m.syncing {
			return m, syncAnimationCmd()
		}
		return m, nil
	case syncDoneMsg:
		if msg.err != nil {
			m.syncSummary = msg.summary
			m.syncErr = msg.err
			return m, tea.Quit
		}
		m.syncSummary = msg.summary
		m.syncInFlight = false
		m.syncStatus = pipeline.SyncProgressLoading
		return m, m.deferredDashboardLoadCmd(initialLoadingPaintDelay)
	case startDashboardLoadMsg:
		m.syncing = false
		m.loading = true
		m = m.reconcileTableSummary()
		m.reloadInFlight = true
		m = m.measureHeights()
		return m, m.reloadCmd()
	case reloadMsg:
		m.loading = false
		m.reloadInFlight = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.rows = msg.rows
		m.sessionCounts = msg.sessionCounts
		m.lastSyncMs = msg.lastSyncMs
		m = m.reconcileStatusline()
		m = m.reconcileTableSummary()
		m = m.resetRowPosition()
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
			m = m.resetRowPosition()
			m.horizontalOffset = 0
			m.loading = true
			m = m.reconcileTableSummary()
			m.reloadInFlight = true
			m = m.measureHeights()
			return m, m.reloadCmd()
		case tea.KeyShiftTab:
			m.activeTab = nextAggregationTab(m.activeTab, -1)
			m = m.resetRowPosition()
			m.horizontalOffset = 0
			m.loading = true
			m = m.reconcileTableSummary()
			m.reloadInFlight = true
			m = m.measureHeights()
			return m, m.reloadCmd()
		case tea.KeyUp:
			m = m.moveCursor(-1)
			return m, nil
		case tea.KeyDown:
			m = m.moveCursor(1)
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
				m = m.resetRowPosition()
				m.horizontalOffset = 0
				m.loading = true
				m = m.reconcileTableSummary()
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
				m = m.moveCursor(1)
				return m, nil
			case "k":
				m = m.moveCursor(-1)
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
		m = m.ensureCursorVisible()
		m = m.clampHorizontalOffset()
		if m.syncing && !m.syncInFlight {
			syncMessages := make(chan tea.Msg, syncProgressBufferSize())
			m.syncMessages = syncMessages
			m.syncInFlight = true
			return m, tea.Batch(m.syncCmd(syncMessages), readSyncProgressCmd(syncMessages), syncAnimationCmd())
		}
		if m.loading && !m.reloadInFlight {
			m.reloadInFlight = true
			return m, m.deferredReloadCmd(initialLoadingPaintDelay)
		}
	}
	return m, nil
}

func (m interactiveModel) withSyncProgress(event pipeline.SyncProgressEvent) interactiveModel {
	if event.Harness == "" {
		m.syncStatus = event.Status
		return m
	}
	for i, row := range m.syncProgressRows {
		if row.harness == event.Harness {
			m.syncProgressRows[i].status = event.Status
			return m
		}
	}
	return m
}

func (m interactiveModel) deferredDashboardLoadCmd(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return startDashboardLoadMsg{}
	})
}

func syncAnimationCmd() tea.Cmd {
	return tea.Tick(syncAnimationInterval, func(time.Time) tea.Msg {
		return syncAnimationTickMsg{}
	})
}

func syncProgressBufferSize() int {
	return len(pipeline.SupportedHarnesses)*3 + 4
}

func (m interactiveModel) syncCmd(messages chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		summary, err := pipeline.Sync(m.ctx, pipeline.SyncOptions{
			DBPath:    m.options.dbPath,
			Harnesses: pipeline.SupportedHarnesses,
			Normalize: true,
			Now:       m.now,
			Progress: func(event pipeline.SyncProgressEvent) {
				messages <- syncProgressMsg{event: event}
			},
		})
		messages <- syncDoneMsg{summary: summary, err: err}
		close(messages)
		return nil
	}
}

func readSyncProgressCmd(messages <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-messages
		if !ok {
			return nil
		}
		return msg
	}
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
		m = m.reconcileStatusline()
		m = m.resetRowPosition()
		m.horizontalOffset = 0
		m.loading = true
		m = m.reconcileTableSummary()
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
		m = m.resetRowPosition()
		m.horizontalOffset = 0
		m.loading = true
		m = m.reconcileTableSummary()
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
		m = m.resetRowPosition()
		m.horizontalOffset = 0
		m.loading = true
		m = m.reconcileTableSummary()
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
	m = m.resetRowPosition()
	m.horizontalOffset = 0
	m.loading = true
	m = m.reconcileTableSummary()
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

// Tab, section, and popup styles live in theme.go.

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

func (m interactiveModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	if m.syncing {
		return m.renderSyncProgress()
	}

	if m.popup != popupNone {
		return renderOnAppSurface(
			lipgloss.Place(
				m.width,
				m.height,
				lipgloss.Center,
				lipgloss.Center,
				m.renderPopup(),
			),
			m.width,
			m.height,
		)
	}

	statusline := m.renderStatusline()

	var tabs []string
	for i, tab := range aggregationTabs {
		label := fmt.Sprintf("%d %s", i+1, tab.String())
		if tab == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	tabBar := strings.Join(tabs, tabGapStyle.Render(" "))
	tabMeta := renderTabMeta(m.options.bucket, activeSort(m.activeTab, m.options.sort))
	tabStrip := tabBarStyle.Render(renderTabsWithMeta(tabBar, tabMeta, max(0, m.width-2)))
	footerDivider := dividerStyle.Render(strings.Repeat("─", max(0, m.width)))

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

	keysNav := "tab switch view · ↑/↓ j/k scroll · ←/→ scroll · 1-6 tabs · q quit"
	keysFilter := "d date · g bucket · s sort · p provider · m model · h harness"
	positionText := ""
	visibleCount := len(visibleRows)
	if !m.loading && visibleCount > 0 && len(m.rows) > visibleCount {
		end := m.scrollOffset + visibleCount
		if end > len(m.rows) {
			end = len(m.rows)
		}
		positionText = fmt.Sprintf("%d-%d of %d", m.scrollOffset+1, end, len(m.rows))
	}
	if maxHorizontal > 0 {
		if positionText != "" {
			positionText += " · "
		}
		positionText += fmt.Sprintf("x %d/%d", horizontalOffset+1, maxHorizontal+1)
	}
	if m.loading {
		if positionText != "" {
			positionText += " · "
		}
		positionText += "loading"
	}
	footer := lipgloss.JoinVertical(lipgloss.Left,
		renderFooterLine(keysNav, positionText, m.width),
		renderFooterLine(keysFilter, activeFiltersLabel(m.options.filters), m.width),
	)

	var body string
	focusRow := -1
	if m.loading {
		body = renderLoadingTableViewportWithSort(m.groupBy, m.activeTab, selectedSort, viewportWidth, visible)
	} else {
		if focus := m.cursor - m.scrollOffset; focus >= 0 && focus < len(visibleRows) {
			focusRow = focus
		}
		body = renderTableViewportWithSortAndFocus(visibleRows, m.rows, m.groupBy, m.activeTab, selectedSort, viewportWidth, horizontalOffset, visible, focusRow)
	}
	body = renderTableSection(body, m.renderTableSummary())

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		statusline,
		"",
		tabStrip,
		"",
		body,
		"",
		footerDivider,
		footer,
	)
	return renderOnAppSurface(content, m.width, m.height)
}

func renderFooterLine(left string, right string, width int) string {
	if width <= 0 {
		return ""
	}
	if right == "" {
		return footerStyle.Render(truncateCell(left, width))
	}
	gap := " · "
	available := width - len(gap) - len(right)
	if available < 0 {
		return footerDimStyle.Render(truncateCell(right, width))
	}
	left = truncateCell(left, available)
	line := left + gap + right
	if len(line) < width {
		line += strings.Repeat(" ", width-len(line))
	}
	return footerStyle.Render(line)
}

func renderTabMeta(bucket timeBucket, sort sortMode) string {
	parts := []string{}
	if value := strings.TrimSpace(string(bucket)); value != "" {
		parts = append(parts, "bucket: "+value)
	}
	if value := strings.TrimSpace(string(sort)); value != "" {
		parts = append(parts, "sort: "+value)
	}
	if len(parts) == 0 {
		return ""
	}
	return statuslineItemStyle.Render(strings.Join(parts, " · "))
}

func renderTabsWithMeta(tabs string, meta string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(meta) == 0 {
		return tabs
	}
	tabsWidth := ansi.StringWidth(tabs)
	metaWidth := ansi.StringWidth(meta)
	if tabsWidth+1+metaWidth > width {
		return tabs
	}
	return tabs + strings.Repeat(" ", width-tabsWidth-metaWidth) + meta
}

func (m interactiveModel) renderSyncProgress() string {
	width := max(1, m.width)
	height := max(1, m.height)
	title := titleStyle.Render("Syncing data")
	subtitle := hintStyle.Render("Refreshing all supported harnesses")
	switch m.syncStatus {
	case pipeline.SyncProgressResetting:
		subtitle = hintStyle.Render("Resetting local usage data for compatibility")
	case pipeline.SyncProgressRebuilding:
		subtitle = hintStyle.Render("Rebuilding usage from all configured local harnesses")
	}
	lines := []string{title, subtitle}
	rawRows := make([]string, 0, len(m.syncProgressRows)+2)
	for _, row := range m.syncProgressRows {
		rawRows = append(rawRows, fmt.Sprintf("%s %-12s", m.syncProgressStatusIcon(row.status), row.label))
	}
	if m.syncStatus != "" {
		rawRows = append(rawRows, "", fmt.Sprintf("%s %s", m.syncProgressStatusIcon(m.syncStatus), m.syncStatus))
	}
	lines = append(lines, "")
	lines = append(lines, rawRows...)
	return renderSyncProgressOnAppSurface(lines, width, height)
}

func (m interactiveModel) syncProgressStatusIcon(status pipeline.SyncProgressStatus) string {
	switch status {
	case "", "pending":
		return syncSkipStyle.Render(padSyncProgressIcon(syncPendingDots(m.syncFrame)))
	case pipeline.SyncProgressDiscovering, pipeline.SyncProgressSyncing, pipeline.SyncProgressNormalizing, pipeline.SyncProgressLoading, pipeline.SyncProgressResetting, pipeline.SyncProgressRebuilding:
		return syncBusyStyle.Render(padSyncProgressIcon(syncSpinnerFrame(m.syncFrame)))
	case pipeline.SyncProgressSynced:
		return syncOKStyle.Render(padSyncProgressIcon("✓"))
	case pipeline.SyncProgressSkipped:
		return syncSkipStyle.Render(padSyncProgressIcon("-"))
	case pipeline.SyncProgressFailed:
		return syncFailStyle.Render(padSyncProgressIcon("✗"))
	default:
		return syncSkipStyle.Render(padSyncProgressIcon("?"))
	}
}

func syncPendingDots(frame int) string {
	count := frame%3 + 1
	return strings.Repeat(".", count) + strings.Repeat(" ", 3-count)
}

func syncSpinnerFrame(frame int) string {
	if len(syncSpinnerFrames) == 0 {
		return ""
	}
	return syncSpinnerFrames[frame%len(syncSpinnerFrames)]
}

func padSyncProgressIcon(value string) string {
	width := 3
	padding := width - lipgloss.Width(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

func renderSyncProgressOnAppSurface(lines []string, width int, height int) string {
	contentWidth := 0
	for _, line := range lines {
		contentWidth = max(contentWidth, lipgloss.Width(line))
	}
	leftPadding := max(0, (width-contentWidth)/2)
	topPadding := max(0, (height-len(lines))/2)
	rendered := make([]string, 0, height)
	for row := 0; row < height; row++ {
		contentIndex := row - topPadding
		if contentIndex < 0 || contentIndex >= len(lines) {
			rendered = append(rendered, strings.Repeat(" ", width))
			continue
		}
		line := lines[contentIndex]
		rightPadding := max(0, width-leftPadding-lipgloss.Width(line))
		rendered = append(rendered, strings.Repeat(" ", leftPadding)+line+strings.Repeat(" ", rightPadding))
	}
	return strings.Join(rendered, "\n")
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
			cursor = "› "
			style = popupCursorStyle
		}
		options = append(options, style.Render(cursor+periodLabel(opt)))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, options...)
	help := hintStyle.Render("space/enter select · esc close")
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
}

func (m interactiveModel) renderBucketPopup() string {
	title := popupTitleStyle.Render("Time bucket")
	var options []string
	for i, opt := range bucketOptions {
		cursor := "  "
		style := popupItemStyle
		if i == m.popupCursor {
			cursor = "› "
			style = popupCursorStyle
		}
		options = append(options, style.Render(cursor+string(opt)))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, options...)
	help := hintStyle.Render("space/enter select · esc close")
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
			cursor = "› "
			style = popupCursorStyle
		}
		options = append(options, style.Render(cursor+string(opt)))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, options...)
	help := hintStyle.Render("space/enter select · esc close")
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
				cursor = "› "
				style = popupCursorStyle
			}
			checked := "○ "
			if m.filterSelections[value] {
				checked = "● "
			}
			options = append(options, style.Render(cursor+checked+value))
		}
		body = lipgloss.JoinVertical(lipgloss.Left, options...)
	}
	help := hintStyle.Render("space select · enter apply · esc close")
	return popupStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", help))
}

func filterFromOptions(options tableOptions, now time.Time) db.Filter {
	return selectionFromOptions(options).Filter(now)
}

func loadFilterValues(ctx context.Context, options tableOptions, now time.Time, dimension filterDimension) ([]string, error) {
	database, err := db.Open(options.dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = database.Close() }()
	tx, err := db.BeginAnalyticsRead(ctx, database)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	filter := filterFromOptions(options, now)
	switch dimension {
	case filterProvider:
		return db.AvailableProviders(ctx, tx, filter)
	case filterModel:
		return db.AvailableModels(ctx, tx, filter)
	case filterHarness:
		return db.AvailableHarnesses(ctx, tx, filter)
	default:
		return nil, nil
	}
}

func loadLastCompletedSync(ctx context.Context, options tableOptions) (int64, error) {
	database, err := db.Open(options.dbPath)
	if err != nil {
		return 0, err
	}
	defer func() { _ = database.Close() }()
	tx, err := db.BeginAnalyticsRead(ctx, database)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	return db.LastCompletedSync(ctx, tx)
}

func loadRows(ctx context.Context, options tableOptions, now time.Time, groupBy groupByMode, activeTab tabMode) ([]renderRow, error) {
	database, err := db.Open(options.dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = database.Close() }()
	tx, err := db.BeginAnalyticsRead(ctx, database)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	return loadRowsFromReader(ctx, tx, options, now, groupBy, activeTab)
}

func loadRowsFromReader(ctx context.Context, database db.Reader, options tableOptions, now time.Time, groupBy groupByMode, activeTab tabMode) ([]renderRow, error) {
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
	var err error
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

func loadDimensionRows(ctx context.Context, database db.Reader, f db.Filter, activeTab tabMode) ([]db.ViewerDimensionRow, error) {
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
