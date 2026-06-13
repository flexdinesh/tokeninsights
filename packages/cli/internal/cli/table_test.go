package cli

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	sessionKey := harness + ":" + sessionID
	_, err := database.Exec(`
		INSERT INTO canonical_sessions (
			semantic_key, harness, session_id, first_seen_at_ms, last_seen_at_ms
		) VALUES (?, ?, ?, ?, ?)
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
		) VALUES (?, ?, ?, 'test', 'test', 'test', ?, ?, ?, ?, 'message', 'exact', 100, 10, 5, 20, 1, 136)
	`, rawKey, harness, sessionID, recordedAtMs, sessionID, provider, model)
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
		) VALUES (?, ?, ?, ?, ?, ?, 'message', 'exact', 1, 100, 10, 5, 20, 1, 136, ?)
	`, rawKey+":canonical", recordedAtMs, harness, canonicalSessionID, provider, model, rawID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadRowsTokenTabUsesCanonicalTokens(t *testing.T) {
	database, dbPath := newLoadRowsTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalToken(t, database, recordedAt.UnixMilli(), "opencode", "ses_1", "openai", "gpt-5")

	rows, err := loadRows(context.Background(), tableOptions{dbPath: dbPath, period: periodAllTime}, recordedAt, groupByNone, tabTokens)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].harness != "opencode" || rows[0].provider != "openai" || rows[0].model != "gpt-5" {
		t.Fatalf("unexpected token row: %+v", rows[0])
	}
	if rows[0].inputTokens != "100" || rows[0].totalTokens != "136" {
		t.Fatalf("unexpected token totals: %+v", rows[0])
	}
}

func TestLoadRowsNonTokenTabsEmptyWithCanonicalTokens(t *testing.T) {
	database, dbPath := newLoadRowsTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local)
	insertLoadRowsCanonicalToken(t, database, recordedAt.UnixMilli(), "opencode", "ses_1", "openai", "gpt-5")

	tabs := []struct {
		name string
		tab  tabMode
	}{
		{name: "tps", tab: tabTPS},
		{name: "requests", tab: tabRequests},
		{name: "tool calls", tab: tabToolCalls},
		{name: "tool breakdown", tab: tabToolBreakdown},
	}

	for _, tab := range tabs {
		t.Run(tab.name, func(t *testing.T) {
			rows, err := loadRows(context.Background(), tableOptions{dbPath: dbPath, period: periodAllTime}, recordedAt, groupByNone, tab.tab)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 0 {
				t.Fatalf("got %d rows, want 0: %+v", len(rows), rows)
			}
		})
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
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestGroupingPopupResetsHorizontalOffset(t *testing.T) {
	m := interactiveModel{
		popup:            popupGrouping,
		popupCursor:      2,
		groupBy:          groupBySession,
		horizontalOffset: 12,
	}

	model, cmd := m.handleGroupingPopupKey(tea.KeyMsg{Type: tea.KeySpace})
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

func TestGroupingPopupSpaceAppliesSelection(t *testing.T) {
	m := interactiveModel{
		popup:       popupGrouping,
		popupCursor: 2,
		groupBy:     groupBySession,
	}

	model, cmd := m.handleGroupingPopupKey(tea.KeyMsg{Type: tea.KeySpace})
	updated, ok := model.(interactiveModel)
	if !ok {
		t.Fatalf("got model %T, want interactiveModel", model)
	}
	if updated.popup != popupNone {
		t.Fatalf("got popup %d, want popupNone", updated.popup)
	}
	if updated.groupBy != groupByHour {
		t.Fatalf("got groupBy %q, want %q", updated.groupBy, groupByHour)
	}
	if cmd == nil {
		t.Fatal("expected reload command")
	}
}

func TestFilterDimensionSpaceOpensValueSelection(t *testing.T) {
	m := interactiveModel{
		popup:       popupFilterDimension,
		popupCursor: 1,
	}

	model, cmd := m.handleFilterDimensionKey(tea.KeyMsg{Type: tea.KeySpace})
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
