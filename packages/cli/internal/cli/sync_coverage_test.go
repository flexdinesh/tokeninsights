package cli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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
	if !strings.Contains(ansi.Strip(view), "2026-09-26 …") {
		t.Fatalf("missing pending day status: %s", view)
	}
}

func TestDayCoverageStaysInlineAndPreservesDates(t *testing.T) {
	days := []db.DayCoverage{
		{Day: "2026-09-21", Status: "checked", HasUsage: true},
		{Day: "2026-09-22", Status: "empty"},
		{Day: "2026-09-23", Status: "pending"},
		{Day: "2026-09-24", Status: "partial"},
		{Day: "2026-09-25", Status: "partial", FailedSources: 1},
		{Day: "2026-09-26", Status: "unverified"},
	}
	rows := withDayCoverage([]renderRow{{bucket: "2026-09-21", totalTokens: "42", totalValue: 42}}, days, sortDate)
	view := ansi.Strip(renderTableViewportWithSortAndFocus(rows, rows, groupByNone, tabTokens, sortDate, 120, 0, len(rows), 0))
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	if len(lines) != len(days)+1 {
		t.Fatalf("day status added table lines: %q", view)
	}
	for _, want := range []string{"2026-09-21 ✓", "2026-09-22 ○", "2026-09-23 …", "2026-09-24 ↻", "2026-09-25 !", "2026-09-26 ?"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing inline status %q: %s", want, view)
		}
	}
	for _, row := range rows {
		if renderRowLineCount(row, columnsForModeAndTab(groupByNone, tabTokens)) != 1 {
			t.Fatalf("status makes date multiline: %+v", row)
		}
	}
}

func TestSourceCoverageSharesSyncWorkLine(t *testing.T) {
	m := interactiveModel{
		width: 120, height: 35,
		coverage:   []db.DayCoverage{{Status: "checked"}, {Status: "pending"}},
		sharedSync: db.SyncStatus{JobID: 1, Phase: "ready", DiscoveryComplete: true, TotalSources: 2, CheckedSources: 2, ReadySources: 2},
	}
	header := m.deskHeader()
	work := ansi.Strip(header[1])
	if !strings.Contains(work, "1/2 days checked · ready · 2/2 sources") || strings.Contains(strings.Join(header, "\n"), "Source coverage") {
		t.Fatalf("coverage not compact within sync work: %q", header)
	}
	if header[2] != "" {
		t.Fatalf("coverage added a separate header row: %q", header)
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

func TestStartupDayMarkersWaitForCurrentCheck(t *testing.T) {
	m := newInteractiveModel(context.Background(), tableOptions{bucket: bucketDay}, time.UnixMilli(2000), "local")
	defer m.cancelSync()
	m.width, m.height = 120, 35
	m.syncInFlight = true
	m.sharedSync = db.SyncStatus{JobID: 1, Phase: "ready", StartedAtMs: 1000}
	for _, step := range []struct {
		status      string
		checkedAtMs int64
		marker      string
		checkedDays string
	}{
		{"checked", 1000, "", "0/1 days checked"},
		{"partial", 0, "↻", "0/1 days checked"},
		{"checked", 3000, "✓", "1/1 days checked"},
	} {
		coverage := []db.DayCoverage{{Day: "2026-09-26", Status: step.status, CheckedAtMs: step.checkedAtMs, HasUsage: true}}
		rows := withDayCoverage([]renderRow{{bucket: "2026-09-26", totalTokens: "42", totalValue: 42}}, coverage, sortDate)
		model, _ := m.Update(snapshotMsg{reloadMsg{rows: rows, coverage: coverage}})
		m = model.(interactiveModel)
		view := ansi.Strip(m.View())
		if step.marker == "" && strings.Contains(view, "✓") {
			t.Fatalf("startup showed a previous check: %s", view)
		}
		if strings.Contains(view, "· ready") {
			t.Fatalf("startup showed the previous job as ready: %s", view)
		}
		if !strings.Contains(view, "2026-09-26 "+step.marker) || !strings.Contains(view, step.checkedDays) || !strings.Contains(view, "total 42") {
			t.Fatalf("wrong marker or saved usage for %s: %s", step.status, view)
		}
		if m.rows[0].coverageStatus != dayCoverageLabel(coverage[0]) {
			t.Fatal("presentation altered retained coverage")
		}
	}
}

func TestNoSyncRetainsSavedDayMarkersAndRetryClearsThem(t *testing.T) {
	coverage := []db.DayCoverage{{Day: "2026-09-26", Status: "checked", CheckedAtMs: 1000, HasUsage: true}, {Day: "2026-09-25", Status: "empty", CheckedAtMs: 1000}}
	rows := withDayCoverage([]renderRow{{bucket: "2026-09-26", totalTokens: "42", totalValue: 42}}, coverage, sortDate)
	m := newInteractiveModel(context.Background(), tableOptions{noSync: true, bucket: bucketDay}, time.UnixMilli(2000), "local")
	defer m.cancelSync()
	m.width, m.height = 120, 35
	model, _ := m.Update(reloadMsg{rows: rows, coverage: coverage})
	m = model.(interactiveModel)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "2026-09-26 ✓") || !strings.Contains(view, "2026-09-25 ○") {
		t.Fatalf("no-sync hid saved markers: %s", view)
	}
	m.options.noSync = false
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = model.(interactiveModel)
	view = ansi.Strip(m.View())
	if cmd == nil || strings.Contains(view, "✓") || strings.Contains(view, "○") || !strings.Contains(view, "total 42") {
		t.Fatalf("retry showed stale completion or hid usage: %s", view)
	}
}
