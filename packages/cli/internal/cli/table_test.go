package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
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
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
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

func writeTableTestPiAssistantSession(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join([]string{
		`{"type":"session","version":1,"id":"pi_s1","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/redacted/project"}`,
		`{"type":"message","id":"msg_a","parentId":null,"timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"assistant","content":[],"provider":"anthropic","model":"claude-sonnet-4","usage":{"input":100,"output":50,"cacheRead":0,"cacheWrite":0,"totalTokens":150},"timestamp":1770000001000}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setTableTestFileModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func assertTableTestCount(t *testing.T, database *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
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
	lastSync := time.Date(2026, 4, 24, 16, 30, 0, 0, time.Local).UnixMilli()
	model, _ = pending.Update(reloadMsg{rows: loadedRows, lastSyncMs: lastSync})
	loaded, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	loadedHeight := lipgloss.Height(loaded.View())
	loadedSummary, _ := viewSummaryLine(loaded.View())
	if !strings.Contains(loadedSummary, "rows 6") || !strings.Contains(loadedSummary, "total 816") {
		t.Fatalf("loaded view missing summary values: %q", loadedSummary)
	}

	if loadedHeight != pendingHeight {
		t.Fatalf("view height changed across tab reload: pending=%d loaded=%d", pendingHeight, loadedHeight)
	}
}

func TestReloadUpdatesStatuslineLastSync(t *testing.T) {
	m := interactiveModel{
		options:    tableOptions{period: periodMonth},
		statusline: newStatuslineModel("month", "workstation", 0),
		loading:    true,
	}
	lastSync := time.Date(2026, 7, 10, 17, 48, 0, 0, time.Local).UnixMilli()

	model, cmd := m.Update(reloadMsg{lastSyncMs: lastSync})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd != nil {
		t.Fatal("did not expect command")
	}
	if got := updated.statusline.value(statuslineLastSynced); got != "2026-07-10 17:48" {
		t.Fatalf("got last synced %q, want 2026-07-10 17:48", got)
	}
}

func TestInitialViewShowsLoadingState(t *testing.T) {
	m := interactiveModel{
		activeTab: tabTokens,
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

func TestFooterOmitsSummaryAndLastSyncInLoadingAndLoadedStates(t *testing.T) {
	lastSync := time.Date(2026, 7, 10, 17, 48, 0, 0, time.Local).UnixMilli()
	tests := []struct {
		name    string
		loading bool
	}{
		{name: "loading", loading: true},
		{name: "loaded", loading: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := interactiveModel{
				rows:       []renderRow{{totalTokens: "136", totalValue: 136}},
				activeTab:  tabTokens,
				statusline: newStatuslineModel("month", "workstation", lastSync),
				width:      120,
				height:     24,
				options:    tableOptions{period: periodMonth},
				loading:    test.loading,
				lastSyncMs: lastSync,
			}
			m = m.measureHeights()
			footer := viewHintLine(m.View())
			if test.loading && !strings.Contains(footer, "loading") {
				t.Fatalf("loading footer missing loading state: %q", footer)
			}
			if strings.Contains(footer, "sync") || strings.Contains(footer, "2026-07-10 17:48") || strings.Contains(footer, "total ") || strings.Contains(footer, "rows ") {
				t.Fatalf("footer repeated summary or sync metadata: %q", footer)
			}
		})
	}
}

func TestViewTableSummaryLoadedStates(t *testing.T) {
	tests := []struct {
		name        string
		activeTab   tabMode
		wantTotal   bool
		wantSummary string
	}{
		{name: "tokens", activeTab: tabTokens, wantTotal: true, wantSummary: "rows 0"},
		{name: "context", activeTab: tabContext, wantTotal: false, wantSummary: "rows 0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := interactiveModel{
				activeTab: test.activeTab,
				width:     100,
				height:    24,
				options:   tableOptions{period: periodMonth},
			}
			m = m.measureHeights()
			summary, _ := viewSummaryLine(m.View())
			if !strings.Contains(summary, test.wantSummary) {
				t.Fatalf("summary missing %q: %q", test.wantSummary, summary)
			}
			if got := strings.Contains(summary, "total 0"); got != test.wantTotal {
				t.Fatalf("summary total visibility = %v, want %v: %q", got, test.wantTotal, summary)
			}
		})
	}
}

func TestTableSummaryPinnedAcrossVerticalScroll(t *testing.T) {
	rows := make([]renderRow, 8)
	for i := range rows {
		rows[i] = renderRow{
			bucket:      fmt.Sprintf("2026-07-%02d", i+1),
			totalTokens: formatTokens(int64(i + 1)),
			totalValue:  int64(i + 1),
		}
	}
	m := interactiveModel{
		rows:      rows,
		activeTab: tabTokens,
		width:     72,
		height:    20,
		options:   tableOptions{period: periodMonth},
	}
	m = m.measureHeights()
	firstSummary, firstIndex := viewSummaryLine(m.View())

	m.scrollOffset = m.maxScrollOffset()
	lastSummary, lastIndex := viewSummaryLine(m.View())
	if firstSummary != lastSummary || firstIndex != lastIndex {
		t.Fatalf("summary moved while scrolling:\nfirst %d: %q\nlast  %d: %q", firstIndex, firstSummary, lastIndex, lastSummary)
	}
	if !strings.Contains(firstSummary, "rows 8") || !strings.Contains(firstSummary, "total 36") {
		t.Fatalf("summary does not use all rows: %q", firstSummary)
	}
}

func TestTableSummaryIgnoresHorizontalScroll(t *testing.T) {
	m := interactiveModel{
		rows: []renderRow{{
			provider:    "openai",
			models:      "gpt-5.5, gpt-5.4, gpt-5.3-codex-spark",
			harnesses:   "codex, opencode, pi",
			totalTokens: "136",
			totalValue:  136,
		}},
		activeTab: tabProviders,
		width:     74,
		height:    24,
		options:   tableOptions{period: periodMonth},
	}
	m = m.measureHeights()
	firstSummary, firstIndex := viewSummaryLine(m.View())
	if maxOffset := m.maxHorizontalOffset(m.rows); maxOffset <= 0 {
		t.Fatal("test fixture did not require horizontal scrolling")
	} else {
		m.horizontalOffset = maxOffset
	}
	lastSummary, lastIndex := viewSummaryLine(m.View())
	if firstSummary != lastSummary || firstIndex != lastIndex {
		t.Fatalf("summary changed during horizontal scroll:\nfirst %d: %q\nlast  %d: %q", firstIndex, firstSummary, lastIndex, lastSummary)
	}
}

func TestImplicitSyncProgressViewShowsHarnessStatusIcons(t *testing.T) {
	m := interactiveModel{
		activeTab:        tabTokens,
		width:            80,
		height:           24,
		options:          tableOptions{period: periodMonth},
		syncing:          true,
		syncProgressRows: initialSyncProgressRows(),
	}

	output := m.View()
	stripped := ansi.Strip(output)
	for _, want := range []string{"Syncing data", "OpenCode", "Pi", "Codex", "Claude Code", ".   OpenCode"} {
		if !strings.Contains(output, want) {
			t.Fatalf("sync progress view missing %q:\n%s", want, output)
		}
	}
	for _, status := range []string{"pending", "discovering", "syncing", "synced", "skipped", "failed"} {
		if strings.Contains(stripped, status) {
			t.Fatalf("sync progress row rendered status text %q:\n%s", status, stripped)
		}
	}
	if strings.Contains(output, "Loading data...") {
		t.Fatalf("sync progress view rendered table loading state:\n%s", output)
	}
}

func TestImplicitSyncProgressUsesAppBackground(t *testing.T) {
	previousProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
	})

	m := interactiveModel{
		activeTab:        tabTokens,
		width:            80,
		height:           24,
		options:          tableOptions{period: periodMonth},
		syncing:          true,
		syncProgressRows: initialSyncProgressRows(),
	}

	output := m.View()
	if strings.Contains(output, "\x1b[48;2;23;23;31m") {
		t.Fatalf("sync progress view contains darker panel background:\n%q", output)
	}
	if !strings.Contains(output, "\x1b[48;2;27;27;42m.   OpenCode") {
		t.Fatalf("sync progress row is not painted on app background:\n%q", output)
	}
	for _, line := range strings.Split(output, "\n") {
		if width := ansi.StringWidth(line); width != 80 {
			t.Fatalf("sync progress line width = %d, want 80 for %q", width, line)
		}
		assertLineCellsHaveBackground(t, line, "48;2;27;27;42")
	}
}

func TestImplicitSyncProgressUpdatesHarnessStatus(t *testing.T) {
	m := interactiveModel{
		activeTab:        tabTokens,
		width:            80,
		height:           24,
		options:          tableOptions{period: periodMonth},
		syncing:          true,
		syncProgressRows: initialSyncProgressRows(),
	}

	model, _ := m.Update(syncProgressMsg{event: pipeline.SyncProgressEvent{
		Harness: pipeline.HarnessOpenCode,
		Status:  pipeline.SyncProgressDiscovering,
	}})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}

	output := updated.View()
	if !strings.Contains(output, "|   OpenCode") {
		t.Fatalf("sync progress view missing updated status icon:\n%s", output)
	}
	if strings.Contains(ansi.Strip(output), "discovering") {
		t.Fatalf("sync progress view rendered status text:\n%s", output)
	}
}

func TestImplicitSyncProgressRendersTerminalStatusIcons(t *testing.T) {
	m := interactiveModel{
		activeTab: tabTokens,
		width:     80,
		height:    24,
		options:   tableOptions{period: periodMonth},
		syncing:   true,
		syncProgressRows: []syncProgressRow{
			{harness: pipeline.HarnessOpenCode, label: "OpenCode", status: pipeline.SyncProgressSynced},
			{harness: pipeline.HarnessPi, label: "Pi", status: pipeline.SyncProgressSkipped},
			{harness: pipeline.HarnessCodex, label: "Codex", status: pipeline.SyncProgressFailed},
		},
	}

	output := ansi.Strip(m.View())
	for _, want := range []string{"✓   OpenCode", "-   Pi", "✗   Codex"} {
		if !strings.Contains(output, want) {
			t.Fatalf("sync progress view missing %q:\n%s", want, output)
		}
	}
}

func TestImplicitSyncAnimationTickAdvancesWhileSyncing(t *testing.T) {
	m := interactiveModel{
		activeTab:        tabTokens,
		width:            80,
		height:           24,
		options:          tableOptions{period: periodMonth},
		syncing:          true,
		syncProgressRows: initialSyncProgressRows(),
	}

	model, cmd := m.Update(syncAnimationTickMsg{})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.syncFrame != 1 {
		t.Fatalf("syncFrame = %d, want 1", updated.syncFrame)
	}
	if cmd == nil {
		t.Fatal("expected another animation tick while syncing")
	}
}

func TestImplicitSyncSuccessTransitionsThroughLoadingDashboardToTableLoading(t *testing.T) {
	m := interactiveModel{
		activeTab:        tabTokens,
		width:            80,
		height:           24,
		options:          tableOptions{period: periodMonth},
		syncing:          true,
		syncProgressRows: initialSyncProgressRows(),
	}

	model, cmd := m.Update(syncDoneMsg{})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd == nil {
		t.Fatal("expected deferred dashboard loading command")
	}
	if !strings.Contains(updated.View(), "loading dashboard") {
		t.Fatalf("sync progress view missing loading dashboard status:\n%s", updated.View())
	}

	model, cmd = updated.Update(startDashboardLoadMsg{})
	loading, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if loading.syncing {
		t.Fatal("expected sync progress to be finished")
	}
	if !loading.loading || !loading.reloadInFlight {
		t.Fatalf("expected table loading state after sync: %+v", loading)
	}
	if cmd == nil {
		t.Fatal("expected dashboard reload command")
	}
	if !strings.Contains(loading.View(), "Loading data...") {
		t.Fatalf("expected existing table loading state:\n%s", loading.View())
	}

	model, cmd = loading.Update(syncAnimationTickMsg{})
	stopped, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if stopped.syncFrame != loading.syncFrame+1 {
		t.Fatalf("syncFrame = %d, want %d", stopped.syncFrame, loading.syncFrame+1)
	}
	if cmd != nil {
		t.Fatal("expected animation ticks to stop after sync progress exits")
	}
}

func TestImplicitSyncProcessesPendingNormalizationWork(t *testing.T) {
	ctx := context.Background()
	sourceRoot := t.TempDir()
	t.Setenv("HOME", filepath.Join(sourceRoot, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(sourceRoot, "xdg"))
	t.Setenv("CODEX_HOME", filepath.Join(sourceRoot, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(sourceRoot, "claude"))
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceRoot, "home", ".pi", "agent", "sessions", "project", "2026-01-01T00-00-00_pi_s1.jsonl")
	writeTableTestPiAssistantSession(t, sourcePath)
	setTableTestFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	summary, err := pipeline.Sync(ctx, pipeline.SyncOptions{
		DBPath:    dbPath,
		Harnesses: []pipeline.Harness{pipeline.HarnessPi},
		Normalize: false,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.RawFacts != 1 || summary.Canonical != 0 {
		t.Fatalf("unexpected setup summary: %+v", summary)
	}

	messages := make(chan tea.Msg, syncProgressBufferSize())
	cmd := interactiveModel{
		ctx: ctx,
		options: tableOptions{
			dbPath: dbPath,
			period: periodMonth,
		},
		now: now.Add(time.Hour),
	}.syncCmd(messages)
	msg := cmd()
	if msg != nil {
		t.Fatalf("sync command returned %T, want nil", msg)
	}
	var done syncDoneMsg
	for queued := range messages {
		if value, ok := queued.(syncDoneMsg); ok {
			done = value
		}
	}
	if done.err != nil {
		t.Fatal(done.err)
	}
	if done.summary.Canonical != 1 {
		t.Fatalf("implicit sync summary = %+v, want canonical work processed", done.summary)
	}

	database, err := db.OpenWritable(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	assertTableTestCount(t, database, "canonical_token_usage", 1)
	assertTableTestCount(t, database, "normalization_work_queue", 0)
}

func TestImplicitSyncStartsAfterWindowSize(t *testing.T) {
	m := interactiveModel{
		ctx:              context.Background(),
		activeTab:        tabTokens,
		options:          tableOptions{period: periodMonth, dbPath: filepath.Join(t.TempDir(), "tokeninsights.sqlite")},
		syncing:          true,
		syncProgressRows: initialSyncProgressRows(),
	}

	if cmd := m.Init(); cmd != nil {
		t.Fatal("implicit sync should wait until the terminal size is known")
	}

	model, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if cmd == nil {
		t.Fatal("expected implicit sync command after window size")
	}
	if !updated.syncing || !updated.syncInFlight {
		t.Fatalf("expected sync progress in flight: %+v", updated)
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
		statusline:  newStatuslineModel("month", "workstation", 0),
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
	if got := updated.statusline.value(statuslineDateRange); got != "yesterday" {
		t.Fatalf("got statusline daterange %q, want yesterday", got)
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func viewHintLine(output string) string {
	var lines []string
	found := false
	for _, line := range strings.Split(ansi.Strip(output), "\n") {
		if strings.Contains(line, "tab/shift+tab") {
			found = true
		}
		if found {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func viewSummaryLine(output string) (string, int) {
	for index, line := range strings.Split(ansi.Strip(output), "\n") {
		content := strings.TrimSpace(strings.Trim(line, "│"))
		if strings.HasPrefix(content, "rows ") {
			return line, index
		}
	}
	return "", -1
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
