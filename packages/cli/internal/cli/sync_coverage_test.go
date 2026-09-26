package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestDayPlaceholdersDoNotInventUsage(t *testing.T) {
	rows := withDayCoverage([]renderRow{{bucket: "2026-09-24", totalValue: 42, totalTokens: "42"}}, []db.DayCoverage{
		{Day: "2026-09-24", Status: "partial", HasUsage: true},
		{Day: "2026-09-25", Status: "empty"},
		{Day: "2026-09-26", Status: "pending"},
	}, sortDate)
	if len(rows) != 3 || rows[0].bucket != "2026-09-26" || rows[1].totalTokens != "0" || rows[0].totalTokens != "—" {
		t.Fatalf("unexpected calendar: %+v", rows)
	}
	summary := newTableSummaryModel(rows, tabTokens, false)
	if summary.rowCount != 1 || summary.totalValue != 42 {
		t.Fatalf("placeholders counted as facts: %+v", summary)
	}
	view := renderTable(rows, groupByNone, tabTokens)
	if !strings.Contains(view, "pending") {
		t.Fatalf("missing pending day status: %s", view)
	}
}

func TestFirstSyncPublishesSnapshotBeforeOverallSuccess(t *testing.T) {
	m := newInteractiveModel(context.Background(), tableOptions{}, time.Now(), "local")
	defer m.cancelSync()
	model, _ := m.Update(snapshotMsg{reloadMsg{rows: []renderRow{{totalValue: 42}}, revision: 1}})
	shown := model.(interactiveModel)
	if !shown.showingSnapshot || len(shown.rows) != 1 || shown.lastSyncMs != 0 {
		t.Fatal("first publication hidden until overall success")
	}
	model, cmd := shown.Update(syncDoneMsg{err: errors.New("one source failed")})
	failed := model.(interactiveModel)
	if failed.syncing || cmd == nil || len(failed.rows) != 1 {
		t.Fatal("ordinary failure discarded partial results")
	}
}

func TestQuitCancelsImplicitSync(t *testing.T) {
	m := newInteractiveModel(context.Background(), tableOptions{}, time.Now(), "local")
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !errors.Is(m.ctx.Err(), context.Canceled) {
		t.Fatal("quit left sync running")
	}
}
