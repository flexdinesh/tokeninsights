package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/muesli/termenv"
)

func TestReadoutsUseExactFilteredComponentsAcrossViews(t *testing.T) {
	database, path := newLoadRowsTestDB(t)
	defer func() { _ = database.Close() }()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.Local)
	// Rounded table strings cannot reproduce these exact component sums.
	for i := range 3 {
		insertLoadRowsCanonicalTokenWithCounts(t, database, now.UnixMilli()+int64(i), "codex", fmt.Sprintf("s%d", i), "openai", "gpt-test", 1501, 1202, 203, 2304, 405, 5615)
	}
	insertLoadRowsCanonicalTokenWithCounts(t, database, now.UnixMilli(), "pi", "excluded", "other", "other-model", 99, 0, 0, 0, 0, 99)
	for _, tab := range []tabMode{tabTokens, tabModels, tabProviders, tabHarnesses, tabSessions} {
		t.Run(tab.String(), func(t *testing.T) {
			rows, err := loadRows(context.Background(), tableOptions{dbPath: path, period: periodAllTime, bucket: bucketDay, filters: filters{harnesses: stringList{"codex"}}}, now, groupByNone, tab)
			if err != nil {
				t.Fatal(err)
			}
			metrics := tokenReadouts(rows)
			for i, want := range []int64{16845, 4503, 3606, 609, 6912, 1215} {
				if metrics[i].value != want {
					t.Fatalf("%s = %d, want %d", metrics[i].label, metrics[i].value, want)
				}
			}
			m := interactiveModel{rows: rows, activeTab: tab, width: 120, height: 35}
			before := strings.Join(m.deskReadouts(118), "\n")
			m.scrollOffset, m.horizontalOffset = 1, 12
			if got := strings.Join(m.deskReadouts(118), "\n"); got != before {
				t.Fatal("scrolling changed readouts")
			}
		})
	}
}

func TestReadoutsNeverPresentStaleLoadingOrAdditiveContextValues(t *testing.T) {
	m := interactiveModel{rows: []renderRow{{totalValue: 999999, maxContextUsedValue: 123456}}, loading: true}
	loading := ansi.Strip(strings.Join(m.deskReadouts(118), "\n"))
	if strings.Contains(loading, "1M") || strings.Count(loading, "—") != 6 {
		t.Fatalf("stale loading values: %s", loading)
	}
	m.loading, m.activeTab = false, tabContext
	context := ansi.Strip(strings.Join(m.deskReadouts(118), "\n"))
	if strings.Contains(context, "Total tokens") || !strings.Contains(context, "not additive") {
		t.Fatalf("context must explain peaks: %s", context)
	}
}

func TestBillionScaleReadoutsFitCompactTerminal(t *testing.T) {
	m := interactiveModel{width: 80, height: 24, rows: []renderRow{{
		totalValue: 15_000_000_000, inputValue: 1_000_000_000, outputValue: 2_000_000_000,
		reasoningValue: 3_000_000_000, cacheReadValue: 4_000_000_000, cacheWriteValue: 5_000_000_000,
	}}}
	readouts := m.deskReadouts(m.tableViewportWidth())
	if strings.TrimSpace(ansi.Strip(readouts[0])) != "" || strings.TrimSpace(ansi.Strip(readouts[len(readouts)-1])) != "" {
		t.Fatal("readouts must have equal blank insets above and below the content")
	}
	for _, line := range readouts {
		if strings.Contains(line, "\n") || ansi.StringWidth(line) != 78 {
			t.Fatalf("readout wrapped: %q", line)
		}
	}
	values := ansi.Strip(strings.Join(readouts, "\n"))
	for _, want := range []string{"15000M", "1000M", "2000M", "3000M", "4000M", "5000M"} {
		if !strings.Contains(values, want) {
			t.Fatalf("missing %s: %s", want, values)
		}
	}
	assertDeskBounds(t, m.View(), 80, 24)
	if !strings.Contains(ansi.Strip(m.View()), "q Quit") {
		t.Fatal("large values displaced footer")
	}
}

func TestInstrumentDeskFitsTerminalAndKeepsNavigationAndCoverage(t *testing.T) {
	oldProfile, oldDark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile); lipgloss.SetHasDarkBackground(oldDark) })
	for _, dark := range []bool{false, true} {
		lipgloss.SetHasDarkBackground(dark)
		for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.Ascii} {
			lipgloss.SetColorProfile(profile)
			for _, size := range [][2]int{{160, 45}, {120, 35}, {80, 24}, {60, 18}} {
				for _, tab := range aggregationTabs {
					m := interactiveModel{width: size[0], height: size[1], activeTab: tab, options: tableOptions{period: periodAllTime, bucket: bucketDay}, sessionCounts: db.SessionCounts{Shown: 2, Synced: 8}, rows: []renderRow{{bucket: "2026-09-23", model: "model-a", totalTokens: "123", totalValue: 123}}}
					view := m.View()
					assertDeskBounds(t, view, size[0], size[1])
					for _, line := range append([]string{strings.Split(view, "\n")[0]}, m.deskReadouts(m.tableViewportWidth())...) {
						if strings.Contains(line, "48;2;") || strings.Contains(line, "48;5;") {
							t.Fatal("status and readouts must preserve the terminal background")
						}
					}
					plain := ansi.Strip(view)
					for _, want := range []string{"[", "6 ", "2 shown / 8 synced", "q Quit"} {
						if !strings.Contains(plain, want) {
							t.Fatalf("%dx%d missing %q: %s", size[0], size[1], want, plain)
						}
					}
					m.popup = popupFilters
					assertDeskBounds(t, m.View(), size[0], size[1])
				}
			}
		}
	}
}

func assertDeskBounds(t *testing.T, output string, width, height int) {
	t.Helper()
	lines := strings.Split(output, "\n")
	if len(lines) != height {
		t.Fatalf("height %d, want %d", len(lines), height)
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got != width {
			t.Fatalf("line width %d, want %d: %q", got, width, line)
		}
	}
}

func TestDrawerScrollsLongListsAndPreservesDashboard(t *testing.T) {
	m := interactiveModel{width: 120, height: 35, activeTab: tabTokens, popup: popupFilterValues, filterDimension: filterModel, filterSelections: map[string]bool{}, options: tableOptions{period: periodAllTime, bucket: bucketDay}, rows: []renderRow{{bucket: "2026-09-23", totalValue: 123}}}
	for i := range 100 {
		m.filterValues = append(m.filterValues, fmt.Sprintf("model-%03d", i))
	}
	m.popupCursor = 99
	plain := ansi.Strip(m.View())
	for _, want := range []string{"2026-09-23", "model-099", "Enter Apply", "Esc Cancel"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("drawer missing %q: %s", want, plain)
		}
	}
	if strings.Contains(plain, "model-000") {
		t.Fatal("list did not scroll to cursor")
	}
	assertDeskBounds(t, m.View(), 120, 35)
}

func TestDrawerCancelDiscardsDraftAndApplyCommits(t *testing.T) {
	base := interactiveModel{popup: popupFilterValues, filterDimension: filterModel, filterValues: []string{"model-a", "model-b"}, filterSelections: map[string]bool{"model-a": true}, options: tableOptions{filters: filters{models: stringList{"model-a"}}}, cursor: 3, scrollOffset: 2}
	base.popupCursor = 1
	draftModel, _ := base.Update(tea.KeyMsg{Type: tea.KeySpace})
	draft := draftModel.(interactiveModel)
	if len(draft.options.filters.models) != 1 {
		t.Fatal("draft altered applied filters")
	}
	cancelledModel, _ := draft.Update(tea.KeyMsg{Type: tea.KeyEsc})
	cancelled := cancelledModel.(interactiveModel)
	if cancelled.popup != popupNone || len(cancelled.options.filters.models) != 1 || cancelled.cursor != 3 || cancelled.scrollOffset != 2 {
		t.Fatal("cancel must preserve scope and table position")
	}
	appliedModel, cmd := draft.Update(tea.KeyMsg{Type: tea.KeyEnter})
	applied := appliedModel.(interactiveModel)
	if len(applied.options.filters.models) != 2 || !applied.loading || applied.popup != popupNone || cmd == nil {
		t.Fatal("apply must commit scope and reload")
	}
}

func TestDrawerCtrlCAlwaysQuits(t *testing.T) {
	for _, popup := range []popupMode{popupDateRange, popupFilterValues, popupHelp, popupFilters} {
		_, cmd := (interactiveModel{popup: popup}).Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Fatalf("popup %d swallowed Ctrl+C", popup)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatal("expected quit")
		}
	}
}

func TestCoverageStaysPinnedForEmptyAndTallRows(t *testing.T) {
	m := interactiveModel{width: 120, height: 35, activeTab: tabModels, sessionCounts: db.SessionCounts{Shown: 1, Synced: 8}, rows: []renderRow{{model: "short", totalValue: 1}}}
	_, baseline := viewSummaryLine(m.View())
	m.rows = nil
	_, empty := viewSummaryLine(m.View())
	if empty != baseline {
		t.Fatalf("empty coverage moved from %d to %d", baseline, empty)
	}
	values := make([]string, 50)
	for i := range values {
		values[i] = fmt.Sprintf("provider-%02d", i)
	}
	m.rows = []renderRow{{model: "tall", providers: strings.Join(values, ", "), totalValue: 1}}
	_, tall := viewSummaryLine(m.View())
	if tall != baseline {
		t.Fatalf("tall row moved coverage from %d to %d", baseline, tall)
	}
	if !strings.Contains(ansi.Strip(m.View()), "q Quit") {
		t.Fatal("tall row hid shortcuts")
	}
}

func TestDatePresetReplacesCustomBounds(t *testing.T) {
	m := interactiveModel{popup: popupDateRange, popupCursor: indexOfPeriod(periodMonth), options: tableOptions{period: periodMonth, filters: filters{dayFrom: "2026-01-01", dayTo: "2026-01-02"}}}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := updated.(interactiveModel)
	if got.options.filters.dayFrom != "" || got.options.filters.dayTo != "" || cmd == nil {
		t.Fatal("explicit preset must replace custom bounds, even for the same preset")
	}
}

func TestDeskRetryClearsFailureAfterSuccessfulReload(t *testing.T) {
	m := interactiveModel{err: errors.New("read failed"), width: 120, height: 35}
	if !strings.Contains(ansi.Strip(m.View()), "r Retry") {
		t.Fatal("error has no recovery action")
	}
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	retry := model.(interactiveModel)
	if cmd == nil || !retry.loading || retry.err != nil {
		t.Fatal("retry did not start")
	}
	model, _ = retry.Update(reloadMsg{rows: []renderRow{{totalValue: 42}}})
	loaded := model.(interactiveModel)
	if loaded.err != nil || loaded.loading || len(loaded.rows) != 1 {
		t.Fatal("reload did not recover")
	}
}
