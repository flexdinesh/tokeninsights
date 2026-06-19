package cli

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	_ "modernc.org/sqlite"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func newLoadRowsTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	if err := db.ResetAll(dbPath); err != nil {
		t.Fatal(err)
	}
	database, err := db.OpenWritable(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return database, dbPath
}

func insertLoadRowsCanonicalToken(t *testing.T, database *sql.DB, recordedAtMs int64, harness string, sessionID string, provider string, model string) {
	t.Helper()
	insertLoadRowsCanonicalTokenWithCounts(t, database, recordedAtMs, harness, sessionID, provider, model, 100, 10, 5, 20, 1, 136)
}

func insertLoadRowsCanonicalTokenWithCounts(t *testing.T, database *sql.DB, recordedAtMs int64, harness string, sessionID string, provider string, model string, input int64, output int64, reasoning int64, cacheRead int64, cacheWrite int64, total int64) {
	t.Helper()
	sessionKey := harness + ":" + sessionID
	_, err := database.Exec(`
		INSERT INTO canonical_sessions (
			semantic_key, harness, session_id, first_seen_at_ms, last_seen_at_ms
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(semantic_key) DO UPDATE SET last_seen_at_ms = excluded.last_seen_at_ms
	`, sessionKey, harness, sessionID, recordedAtMs, recordedAtMs)
	if err != nil {
		t.Fatal(err)
	}

	var canonicalSessionID int64
	if err := database.QueryRow("SELECT id FROM canonical_sessions WHERE semantic_key = ?", sessionKey).Scan(&canonicalSessionID); err != nil {
		t.Fatal(err)
	}

	rawKey := fmt.Sprintf("%s:%d:%s:%s", sessionKey, recordedAtMs, provider, model)
	_, err = database.Exec(`
		INSERT INTO raw_token_usage (
			raw_fact_key, harness, source_id, source_kind, collector, parser, observed_at_ms,
			session_id, provider, model, usage_scope, quality, input_tokens, output_tokens,
			reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens
		) VALUES (?, ?, ?, 'test', 'test', 'test', ?, ?, ?, ?, 'message', 'exact', ?, ?, ?, ?, ?, ?)
	`, rawKey, harness, sessionID, recordedAtMs, sessionID, provider, model, input, output, reasoning, cacheRead, cacheWrite, total)
	if err != nil {
		t.Fatal(err)
	}

	var rawID int64
	if err := database.QueryRow("SELECT id FROM raw_token_usage WHERE raw_fact_key = ?", rawKey).Scan(&rawID); err != nil {
		t.Fatal(err)
	}

	_, err = database.Exec(`
		INSERT INTO canonical_token_usage (
			semantic_key, recorded_at_ms, harness, session_id, provider, model, usage_scope, quality,
			is_countable, input_tokens, output_tokens, reasoning_tokens, cache_read_tokens,
			cache_write_tokens, total_tokens, primary_raw_fact_id
		) VALUES (?, ?, ?, ?, ?, ?, 'message', 'exact', 1, ?, ?, ?, ?, ?, ?, ?)
	`, rawKey+":canonical", recordedAtMs, harness, canonicalSessionID, provider, model, input, output, reasoning, cacheRead, cacheWrite, total, rawID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadRowsTokenTabUsesCanonicalTokens(t *testing.T) {
	database, dbPath := newLoadRowsTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalToken(t, database, recordedAt.UnixMilli(), "opencode", "ses_1", "openai", "gpt-5")

	rows, err := loadRows(context.Background(), tableOptions{dbPath: dbPath, period: periodAllTime, bucket: bucketDay}, recordedAt, groupByNone, tabTokens)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].bucket != "2026-04-24" || rows[0].sessions != "1" {
		t.Fatalf("unexpected token row: %+v", rows[0])
	}
	if rows[0].inputTokens != "100" || rows[0].totalTokens != "136" {
		t.Fatalf("unexpected token totals: %+v", rows[0])
	}
}

func TestLoadRowsSessionTabIncludesContextUsed(t *testing.T) {
	database, dbPath := newLoadRowsTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalToken(t, database, recordedAt.UnixMilli(), "opencode", "ses_1", "openai", "gpt-5")

	rows, err := loadRows(context.Background(), tableOptions{dbPath: dbPath, period: periodAllTime, bucket: bucketDay}, recordedAt, groupByNone, tabSessions)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].contextUsedTokens != "121" {
		t.Fatalf("unexpected session context used: %+v", rows[0])
	}
}

func TestLoadRowsTokenTabUsesTimeBucket(t *testing.T) {
	database, dbPath := newLoadRowsTestDB(t)
	defer database.Close()
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalToken(t, database, time.Date(2026, 4, 20, 9, 0, 0, 0, time.Local).UnixMilli(), "opencode", "ses_1", "openai", "gpt-5")
	insertLoadRowsCanonicalToken(t, database, time.Date(2026, 4, 24, 9, 0, 0, 0, time.Local).UnixMilli(), "pi", "ses_2", "anthropic", "claude")

	rows, err := loadRows(context.Background(), tableOptions{dbPath: dbPath, period: periodAllTime, bucket: bucketWeek}, now, groupByNone, tabTokens)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].bucket != "2026-04-20" || rows[0].sessions != "2" || rows[0].totalTokens != "272" {
		t.Fatalf("unexpected token row: %+v", rows[0])
	}
}

func TestLoadRowsYesterdayPeriodExcludesToday(t *testing.T) {
	database, dbPath := newLoadRowsTestDB(t)
	defer database.Close()
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalToken(t, database, now.AddDate(0, 0, -1).UnixMilli(), "opencode", "ses_1", "openai", "gpt-5")
	insertLoadRowsCanonicalToken(t, database, now.UnixMilli(), "pi", "ses_2", "anthropic", "claude")

	rows, err := loadRows(context.Background(), tableOptions{dbPath: dbPath, period: periodYesterday, bucket: bucketDay}, now, groupByNone, tabTokens)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].bucket != "2026-04-23" {
		t.Fatalf("got bucket %q, want 2026-04-23", rows[0].bucket)
	}
}

func TestLoadRowsCustomDayBoundsOverridePreset(t *testing.T) {
	database, dbPath := newLoadRowsTestDB(t)
	defer database.Close()
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalToken(t, database, time.Date(2026, 3, 15, 12, 0, 0, 0, time.Local).UnixMilli(), "opencode", "ses_1", "openai", "gpt-5")
	insertLoadRowsCanonicalToken(t, database, now.UnixMilli(), "pi", "ses_2", "anthropic", "claude")

	rows, err := loadRows(context.Background(), tableOptions{
		dbPath: dbPath,
		period: periodMonth,
		bucket: bucketDay,
		filters: filters{
			dayFrom: "2026-03-15",
			dayTo:   "2026-03-15",
		},
	}, now, groupByNone, tabTokens)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].bucket != "2026-03-15" {
		t.Fatalf("got bucket %q, want 2026-03-15", rows[0].bucket)
	}
}

func TestLoadRowsModelTabAggregatesByModel(t *testing.T) {
	database, dbPath := newLoadRowsTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalToken(t, database, recordedAt.UnixMilli(), "opencode", "ses_1", "openai", "gpt-5")
	insertLoadRowsCanonicalToken(t, database, recordedAt.Add(time.Second).UnixMilli(), "pi", "ses_2", "azure", "gpt-5")

	rows, err := loadRows(context.Background(), tableOptions{dbPath: dbPath, period: periodAllTime, bucket: bucketDay}, recordedAt, groupByNone, tabModels)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].model != "gpt-5" || rows[0].providers != "azure, openai" || rows[0].harnesses != "opencode, pi" || rows[0].sessions != "2" {
		t.Fatalf("unexpected model row: %+v", rows[0])
	}
}

func TestLoadRowsContextTabShowsSessionPeakContextLoadStats(t *testing.T) {
	database, dbPath := newLoadRowsTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalTokenWithCounts(t, database, recordedAt.UnixMilli(), "codex", "ses_1", "openai", "gpt-5", 100, 10, 5, 20, 1, 136)
	insertLoadRowsCanonicalTokenWithCounts(t, database, recordedAt.Add(time.Second).UnixMilli(), "codex", "ses_1", "openai", "gpt-5", 200, 20, 6, 30, 2, 258)
	insertLoadRowsCanonicalTokenWithCounts(t, database, recordedAt.Add(2*time.Second).UnixMilli(), "codex", "ses_2", "openai", "gpt-5", 50, 5, 1, 5, 0, 61)
	insertLoadRowsCanonicalTokenWithCounts(t, database, recordedAt.Add(3*time.Second).UnixMilli(), "opencode", "ses_3", "openai", "gpt-5", 500, 50, 10, 0, 0, 560)

	rows, err := loadRows(context.Background(), tableOptions{dbPath: dbPath, period: periodAllTime, bucket: bucketDay}, recordedAt, groupByNone, tabContext)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[0].harness != "opencode" || rows[0].provider != "openai" || rows[0].model != "gpt-5" || rows[0].sessions != "1" || rows[0].averageContextUsedTokens != "500" || rows[0].medianContextUsedTokens != "500" || rows[0].maxContextUsedTokens != "500" {
		t.Fatalf("unexpected first context row: %+v", rows[0])
	}
	if rows[1].harness != "codex" || rows[1].sessions != "2" || rows[1].averageContextUsedTokens != "143" || rows[1].medianContextUsedTokens != "143" || rows[1].maxContextUsedTokens != "232" {
		t.Fatalf("unexpected second context row: %+v", rows[1])
	}
}

func TestClampHorizontalScroll(t *testing.T) {
	if got := clampHorizontalScroll(-1, 100, 20); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
	if got := clampHorizontalScroll(90, 100, 20); got != 80 {
		t.Fatalf("got %d, want 80", got)
	}
	if got := clampHorizontalScroll(5, 20, 20); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}

func TestTableViewportWidthAlignsWithSectionContent(t *testing.T) {
	m := interactiveModel{width: 100}
	if got, want := m.tableViewportWidth(), 96; got != want {
		t.Fatalf("got viewport width %d, want %d", got, want)
	}
}

func TestHorizontalKeysScrollAndJump(t *testing.T) {
	m := interactiveModel{
		rows: []renderRow{{
			day:              "2026-04-24",
			harness:          "oc",
			provider:         "very-long-provider-name",
			model:            "very/long/model/name/that/needs/scrolling",
			inputTokens:      "1000",
			outputTokens:     "2000",
			reasoningTokens:  "3000",
			cacheReadTokens:  "4000",
			cacheWriteTokens: "5000",
			totalTokens:      "15000",
		}},
		groupBy:   groupByNone,
		activeTab: tabTokens,
		width:     30,
		height:    20,
	}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd != nil {
		t.Fatal("did not expect command")
	}
	if updated.horizontalOffset != 1 {
		t.Fatalf("got horizontal offset %d, want 1", updated.horizontalOffset)
	}

	model, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyEnd})
	updated, ok = model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd != nil {
		t.Fatal("did not expect command")
	}
	if updated.horizontalOffset <= 1 {
		t.Fatalf("got horizontal offset %d, want more than 1", updated.horizontalOffset)
	}

	model, cmd = updated.Update(tea.KeyMsg{Type: tea.KeyHome})
	updated, ok = model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd != nil {
		t.Fatal("did not expect command")
	}
	if updated.horizontalOffset != 0 {
		t.Fatalf("got horizontal offset %d, want 0", updated.horizontalOffset)
	}
}

func TestViewDoesNotShowHorizontalScrollWhenTableFitsAfterTruncation(t *testing.T) {
	m := interactiveModel{
		rows: []renderRow{{
			provider:         "openai",
			models:           "gpt-5.5, gpt-5.4, gpt-5.3-codex-spark, gpt-5.3-codex, gpt-5.2-codex",
			harnesses:        "codex, opencode, pi",
			sessions:         "260",
			inputTokens:      "31M",
			outputTokens:     "2M",
			reasoningTokens:  "919K",
			cacheReadTokens:  "330M",
			cacheWriteTokens: "1M",
			totalTokens:      "366M",
			totalValue:       366_000_000,
		}},
		activeTab: tabProviders,
		period:    periodAllTime,
		width:     124,
		height:    24,
		options:   tableOptions{period: periodAllTime, sort: sortTokens},
	}
	m = m.measureHeights()

	output := ansi.Strip(m.View())
	if strings.Contains(output, "x     1/") {
		t.Fatalf("view showed horizontal scroll even though table fits after truncation:\n%s", output)
	}
}

func TestEndKeyScrollsWhenMinimumTableWidthExceedsViewport(t *testing.T) {
	m := interactiveModel{
		rows: []renderRow{{
			provider:         "openai",
			models:           "gpt-5.5, gpt-5.4, gpt-5.3-codex-spark",
			harnesses:        "codex, opencode, pi",
			sessions:         "260",
			inputTokens:      "31M",
			outputTokens:     "2M",
			reasoningTokens:  "919K",
			cacheReadTokens:  "330M",
			cacheWriteTokens: "1M",
			totalTokens:      "366M",
			totalValue:       366_000_000,
		}},
		activeTab: tabProviders,
		period:    periodAllTime,
		width:     74,
		height:    24,
		options:   tableOptions{period: periodAllTime, sort: sortTokens},
	}
	m = m.measureHeights()

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd != nil {
		t.Fatal("did not expect command")
	}
	if updated.horizontalOffset <= 0 {
		t.Fatalf("got horizontal offset %d, want horizontal scroll to be available", updated.horizontalOffset)
	}
}

func TestTabSwitchResetsHorizontalOffset(t *testing.T) {
	m := interactiveModel{
		horizontalOffset: 12,
		activeTab:        tabTokens,
	}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.horizontalOffset != 0 {
		t.Fatalf("got horizontal offset %d, want 0", updated.horizontalOffset)
	}
	if updated.activeTab != tabModels {
		t.Fatalf("got active tab %q, want %q", updated.activeTab, tabModels)
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestViewHeightStableAcrossTabReloadRows(t *testing.T) {
	m := interactiveModel{
		rows: []renderRow{{
			bucket:      "2026-04-24",
			sessions:    "1",
			inputTokens: "100",
			totalTokens: "136",
			totalValue:  136,
		}},
		activeTab: tabTokens,
		period:    periodMonth,
		width:     72,
		height:    24,
		options:   tableOptions{period: periodMonth},
	}
	m = m.measureHeights()

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	pending, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	pendingHeight := lipgloss.Height(pending.View())
	if !strings.Contains(pending.View(), "Loading data...") {
		t.Fatalf("pending view missing loading state:\n%s", pending.View())
	}

	loadedRows := []renderRow{
		{model: "claude-3-5-sonnet-with-a-very-long-context-window", providers: "anthropic", harnesses: "pi", sessions: "1", inputTokens: "100", totalTokens: "136", totalValue: 136},
		{model: "gpt-5", providers: "openai", harnesses: "opencode", sessions: "1", inputTokens: "100", totalTokens: "136", totalValue: 136},
		{model: "o4-mini", providers: "openai", harnesses: "codex", sessions: "1", inputTokens: "100", totalTokens: "136", totalValue: 136},
		{model: "gemini-2.5-pro", providers: "google", harnesses: "opencode", sessions: "1", inputTokens: "100", totalTokens: "136", totalValue: 136},
		{model: "unknown", providers: "unknown", harnesses: "codex", sessions: "1", inputTokens: "100", totalTokens: "136", totalValue: 136},
		{model: "llama-4", providers: "meta", harnesses: "opencode", sessions: "1", inputTokens: "100", totalTokens: "136", totalValue: 136},
	}
	model, _ = pending.Update(reloadMsg{rows: loadedRows})
	loaded, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	loadedHeight := lipgloss.Height(loaded.View())

	if loadedHeight != pendingHeight {
		t.Fatalf("view height changed across tab reload: pending=%d loaded=%d", pendingHeight, loadedHeight)
	}
}

func TestInitialViewShowsLoadingState(t *testing.T) {
	m := interactiveModel{
		activeTab: tabTokens,
		period:    periodMonth,
		width:     72,
		height:    24,
		options:   tableOptions{period: periodMonth},
		loading:   true,
	}
	m = m.measureHeights()

	output := m.View()
	if !strings.Contains(output, "Loading data...") {
		t.Fatalf("initial view missing loading state:\n%s", output)
	}
	if !strings.Contains(output, "loading") {
		t.Fatalf("initial view missing loading hint:\n%s", output)
	}
}

func TestViewUsesConsistentPanelBackground(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	m := interactiveModel{
		rows: []renderRow{{
			bucket:      "2026-06-14",
			sessions:    "1",
			inputTokens: "35K",
			totalTokens: "114K",
			totalValue:  114000,
		}},
		activeTab: tabTokens,
		period:    periodMonth,
		width:     100,
		height:    24,
		options:   tableOptions{period: periodMonth},
	}
	m = m.measureHeights()

	output := m.View()
	if strings.Contains(output, "\x1b[48;2;23;23;31m") {
		t.Fatalf("view contains darker panel background:\n%q", output)
	}
	if !strings.Contains(output, "\x1b[48;2;27;27;42m") {
		t.Fatalf("view missing app background:\n%q", output)
	}
}

func TestInitialReloadStartsAfterWindowSize(t *testing.T) {
	m := interactiveModel{
		activeTab: tabTokens,
		period:    periodMonth,
		options:   tableOptions{period: periodMonth},
		loading:   true,
	}

	if cmd := m.Init(); cmd != nil {
		t.Fatal("initial reload should wait until the terminal size is known")
	}

	model, cmd := m.Update(tea.WindowSizeMsg{Width: 72, Height: 24})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd == nil {
		t.Fatal("expected initial reload command after window size")
	}
	if !updated.loading || !updated.reloadInFlight {
		t.Fatalf("expected loading reload in flight: %+v", updated)
	}
	if !strings.Contains(updated.View(), "Loading data...") {
		t.Fatalf("initial sized view missing loading state:\n%s", updated.View())
	}
}

func TestViewColumnWidthsStableAcrossVisibleRowWindows(t *testing.T) {
	rows := []renderRow{
		{model: "a", providers: "x", harnesses: "pi", sessions: "1", inputTokens: "1", totalTokens: "1", totalValue: 1},
		{model: "b", providers: "x", harnesses: "pi", sessions: "1", inputTokens: "1", totalTokens: "1", totalValue: 1},
		{model: "c", providers: "x", harnesses: "pi", sessions: "1", inputTokens: "1", totalTokens: "1", totalValue: 1},
		{model: "d", providers: "x", harnesses: "pi", sessions: "1", inputTokens: "1", totalTokens: "1", totalValue: 1},
		{model: "e", providers: "x", harnesses: "pi", sessions: "1", inputTokens: "1", totalTokens: "1", totalValue: 1},
		{model: "model-name-that-is-only-visible-after-scroll", providers: "provider-name-that-is-only-visible-after-scroll", harnesses: "opencode", sessions: "1", inputTokens: "1", totalTokens: "1", totalValue: 1},
	}
	m := interactiveModel{
		rows:      rows,
		activeTab: tabModels,
		period:    periodMonth,
		width:     120,
		height:    24,
		options:   tableOptions{period: periodMonth},
	}
	m = m.measureHeights()
	before := tableHeaderLine(m.View(), "model")

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	after := tableHeaderLine(updated.View(), "model")

	if after != before {
		t.Fatalf("table header changed after visible row window changed:\nbefore: %q\nafter:  %q", before, after)
	}
}

func TestVisibleRowsRespectMultilineRowBudget(t *testing.T) {
	m := interactiveModel{
		rows: []renderRow{
			{model: "a", providers: "anthropic, azure, openai", harnesses: "codex", sessions: "1", inputTokens: "1", totalTokens: "1", totalValue: 1},
			{model: "b", providers: "anthropic, azure, openai", harnesses: "pi", sessions: "1", inputTokens: "1", totalTokens: "1", totalValue: 1},
		},
		activeTab:    tabModels,
		width:        80,
		height:       8,
		cachedWidth:  80,
		perRowHeight: 1,
	}

	visibleRows := m.visibleRows()
	if len(visibleRows) != 1 || visibleRows[0].model != "a" {
		t.Fatalf("got visible rows %+v, want only first multiline row", visibleRows)
	}
	if maxOffset := m.maxScrollOffset(); maxOffset != 1 {
		t.Fatalf("got max scroll offset %d, want 1", maxOffset)
	}
}

func TestLoadingAndLoadedHeadersUseStableColumnWidths(t *testing.T) {
	base := interactiveModel{
		activeTab: tabModels,
		period:    periodMonth,
		width:     80,
		height:    24,
		options:   tableOptions{period: periodMonth},
		loading:   true,
	}
	base = base.measureHeights()
	loadingHeader := tableHeaderLine(base.View(), "model")

	base.loading = false
	base.rows = []renderRow{{
		model:       "gpt-5.5",
		providers:   "openai, openai-codex",
		harnesses:   "codex, opencode, pi",
		sessions:    "132",
		inputTokens: "19M",
		totalTokens: "227M",
		totalValue:  227_000_000,
	}}
	loadedHeader := tableHeaderLine(base.View(), "model")

	if loadedHeader != loadingHeader {
		t.Fatalf("table header changed from loading to loaded:\nloading: %q\nloaded:  %q", loadingHeader, loadedHeader)
	}
}

func tableHeaderLine(output string, marker string) string {
	for _, line := range strings.Split(ansi.Strip(output), "\n") {
		if strings.Contains(line, marker) {
			return line
		}
	}
	return ""
}

func TestNumberKeysJumpToAggregationTabs(t *testing.T) {
	m := interactiveModel{activeTab: tabTokens}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.activeTab != tabSessions {
		t.Fatalf("got active tab %q, want %q", updated.activeTab, tabSessions)
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestNumberKeySixJumpsToContextAggregationTab(t *testing.T) {
	m := interactiveModel{activeTab: tabTokens}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("6")})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.activeTab != tabContext {
		t.Fatalf("got active tab %q, want %q", updated.activeTab, tabContext)
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestSortRenderRowsContextTabUsesContextSortModes(t *testing.T) {
	rows := []renderRow{
		{harness: "codex", provider: "openai", model: "gpt-5", sessions: "2", sessionsValue: 2, averageContextUsedValue: 143, medianContextUsedValue: 143, maxContextUsedValue: 232},
		{harness: "opencode", provider: "openai", model: "gpt-5", sessions: "1", sessionsValue: 1, averageContextUsedValue: 500, medianContextUsedValue: 500, maxContextUsedValue: 500},
	}

	sortRenderRows(rows, tabContext, "")
	if rows[0].harness != "opencode" {
		t.Fatalf("default sort should use avg ctx descending: %+v", rows)
	}

	sortRenderRows(rows, tabContext, sortSessions)
	if rows[0].harness != "codex" {
		t.Fatalf("sessions sort should use session count descending: %+v", rows)
	}

	sortRenderRows(rows, tabContext, sortHarness)
	if rows[0].harness != "codex" {
		t.Fatalf("harness sort should be alphabetical: %+v", rows)
	}
}

func TestBucketPopupResetsHorizontalOffset(t *testing.T) {
	m := interactiveModel{
		popup:            popupBucket,
		popupCursor:      1,
		options:          tableOptions{bucket: bucketDay},
		horizontalOffset: 12,
	}

	model, cmd := m.handleBucketPopupKey(tea.KeyMsg{Type: tea.KeySpace})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.horizontalOffset != 0 {
		t.Fatalf("got horizontal offset %d, want 0", updated.horizontalOffset)
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestDateRangePopupSpaceAppliesSelection(t *testing.T) {
	m := interactiveModel{
		popup:       popupDateRange,
		popupCursor: 1,
		options:     tableOptions{period: periodMonth},
	}

	model, cmd := m.handleDateRangePopupKey(tea.KeyMsg{Type: tea.KeySpace})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.popup != popupNone {
		t.Fatalf("got popup %d, want popupNone", updated.popup)
	}
	if updated.options.period != periodYesterday {
		t.Fatalf("got period %q, want %q", updated.options.period, periodYesterday)
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestSortPopupSpaceAppliesSelection(t *testing.T) {
	m := interactiveModel{
		popup:       popupSort,
		popupCursor: 5,
		options:     tableOptions{sort: sortTokens},
	}

	model, cmd := m.handleSortPopupKey(tea.KeyMsg{Type: tea.KeySpace})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.popup != popupNone {
		t.Fatalf("got popup %d, want popupNone", updated.popup)
	}
	if updated.options.sort != sortName {
		t.Fatalf("got sort %q, want %q", updated.options.sort, sortName)
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestContextSortPopupShowsContextSortOptions(t *testing.T) {
	m := interactiveModel{
		activeTab: tabContext,
		popup:     popupSort,
	}

	output := ansi.Strip(m.renderSortPopup())
	for _, expected := range []string{"avg ctx", "median ctx", "max ctx", "sessions", "harness", "provider", "model"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("context sort popup missing %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "tokens") || strings.Contains(output, "cache read") {
		t.Fatalf("context sort popup should not include token-total sort options:\n%s", output)
	}
}

func TestHarnessShortcutOpensValueSelection(t *testing.T) {
	m := interactiveModel{}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.popup != popupFilterValues {
		t.Fatalf("got popup %d, want popupFilterValues", updated.popup)
	}
	if updated.filterDimension != filterHarness {
		t.Fatalf("got filter dimension %d, want filterHarness", updated.filterDimension)
	}
	if !updated.filterLoading {
		t.Fatal("expected filter values to be loading")
	}
	if cmd == nil {
		t.Fatal("expected filter values command")
	}
}

func TestNoReloadShortcut(t *testing.T) {
	m := interactiveModel{}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.popup != popupNone || updated.activeTab != tabTokens || updated.horizontalOffset != 0 || updated.scrollOffset != 0 {
		t.Fatalf("model changed on reload shortcut: %+v", updated)
	}
	if cmd != nil {
		t.Fatal("did not expect command")
	}
}

func TestFilterValuesSpaceTogglesSelection(t *testing.T) {
	m := interactiveModel{
		popup:        popupFilterValues,
		popupCursor:  1,
		filterValues: []string{"anthropic", "openai"},
		filterSelections: map[string]bool{
			"anthropic": true,
		},
	}

	model, cmd := m.handleFilterValuesKey(tea.KeyMsg{Type: tea.KeySpace})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd != nil {
		t.Fatal("did not expect command")
	}
	if !updated.filterSelections["anthropic"] {
		t.Fatal("expected existing selection to remain selected")
	}
	if !updated.filterSelections["openai"] {
		t.Fatal("expected highlighted value to be selected")
	}

	model, cmd = updated.handleFilterValuesKey(tea.KeyMsg{Type: tea.KeySpace})
	updated, ok = model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd != nil {
		t.Fatal("did not expect command")
	}
	if updated.filterSelections["openai"] {
		t.Fatal("expected highlighted value to be unselected")
	}
}

func TestFilterValuesClearIsStagedUntilEnter(t *testing.T) {
	m := interactiveModel{
		popup:        popupFilterValues,
		filterValues: []string{"anthropic", "openai"},
		filterSelections: map[string]bool{
			"anthropic": true,
			"openai":    true,
		},
		options: tableOptions{
			filters: filters{providers: stringList{"anthropic", "openai"}},
		},
	}

	model, cmd := m.handleFilterValuesKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd != nil {
		t.Fatal("did not expect command before applying")
	}
	if len(updated.options.filters.providers) != 2 {
		t.Fatalf("filter applied before enter: %+v", updated.options.filters.providers)
	}
	if updated.filterSelections["anthropic"] || updated.filterSelections["openai"] {
		t.Fatalf("expected staged selections cleared: %+v", updated.filterSelections)
	}
}

func TestFilterValuesEscapeCancelsStagedChanges(t *testing.T) {
	m := interactiveModel{
		popup:        popupFilterValues,
		filterValues: []string{"anthropic", "openai"},
		filterSelections: map[string]bool{
			"anthropic": true,
		},
		options: tableOptions{
			filters: filters{providers: stringList{"anthropic"}},
		},
	}

	model, cmd := m.handleFilterValuesKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd != nil {
		t.Fatal("did not expect command")
	}
	if updated.popup != popupNone {
		t.Fatalf("got popup %d, want popupNone", updated.popup)
	}
	if strings.Join([]string(updated.options.filters.providers), ",") != "anthropic" {
		t.Fatalf("filter changed after escape: %+v", updated.options.filters.providers)
	}
}
