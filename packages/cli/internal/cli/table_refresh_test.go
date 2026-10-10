package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
	"github.com/muesli/termenv"
)

func savedRefreshModel(t *testing.T) interactiveModel {
	t.Helper()
	m := newInteractiveModel(t.Context(), tableOptions{period: periodAllTime, bucket: bucketDay}, time.Now(), "fixture")
	t.Cleanup(m.cancelSync)
	m.width, m.height = 120, 35
	m.instanceID, m.dataEpoch, m.serviceReadiness, m.observedRevision = "instance", "database", "ready", 1
	m.loading, m.refresh.loaded = false, true
	m.rows = []renderRow{{bucket: "2026-10-09", totalValue: 100, totalTokens: "100"}}
	m.sessionCounts = querymodel.SessionCounts{Shown: 1, Synced: 1}
	m.refresh.displayedRevision, m.refresh.inputRevision, m.refresh.serverGeneration = 1, 1, 1
	return m
}

func refreshStatusMessage(pending int64, revision, inputRevision, generation, target int64) sharedSyncMsg {
	status := analytics.ProcessingStatus{Pending: pending, Metadata: dataengine.Metadata{
		Revision: revision, InputRevision: inputRevision, Generation: generation, TargetGeneration: target,
	}}
	snapshot := collectorprogress.Snapshot{Attempts: []collectorprogress.Attempt{{
		AttemptID: "attempt", Stage: "accepted", StartedAtMS: 1, UpdatedAtMS: 2, FinishedAtMS: 2,
	}}}
	busy := pending > 0 || generation != target
	return sharedSyncMsg{instanceID: "instance", dataEpoch: "database", readiness: "ready",
		status: querymodel.SyncStatus{Revision: revision, Running: busy}, pending: busy, refreshStatus: &status, collection: &snapshot}
}

func TestRefreshCompletionRequiresProcessingAndDisplayedPublication(t *testing.T) {
	m := savedRefreshModel(t)
	updated, _ := m.Update(refreshStatusMessage(1, 1, 2, 1, 1))
	m = updated.(interactiveModel)
	if !strings.Contains(m.refreshLine(), "processing") || !m.refreshBusy() || len(m.rows) != 1 {
		t.Fatal("accepted pending evidence appeared complete or hid saved usage", m.refreshLine())
	}
	updated, _ = m.Update(refreshStatusMessage(0, 2, 2, 1, 1))
	m = updated.(interactiveModel)
	if strings.Contains(m.refreshLine(), "Usage refreshed") {
		t.Fatal("queue drain appeared complete before the dashboard read")
	}
	updated, _ = m.Update(reloadMsg{preservePosition: true, instanceID: "instance", dataEpoch: "database", revision: 2,
		inputRevision: 2, serverGeneration: 1, refreshToken: m.refresh.token, rows: []renderRow{{bucket: "2026-10-09", totalValue: 200}}})
	m = updated.(interactiveModel)
	if m.refreshBusy() || !strings.Contains(m.refreshLine(), "Usage refreshed") || m.rows[0].totalValue != 200 {
		t.Fatal("successful displayed publication did not complete refresh", m.refreshLine())
	}
	updated, _ = m.Update(refreshStatusMessage(0, 2, 2, 1, 2))
	m = updated.(interactiveModel)
	if !m.refreshBusy() || strings.Contains(m.refreshLine(), "Usage refreshed") || m.rows[0].totalValue != 200 {
		t.Fatal("unfinished generation hid saved usage or appeared complete")
	}
}

func TestNoopAcceptanceRequiresANewDashboardRead(t *testing.T) {
	m := savedRefreshModel(t)
	updated, _ := m.Update(refreshStatusMessage(0, 1, 1, 1, 1))
	m = updated.(interactiveModel)
	if !m.refreshNeedsRead() || strings.Contains(m.refreshLine(), "Usage refreshed") {
		t.Fatal("unchanged acceptance reused a dashboard read preceding acceptance")
	}
}

func TestRefreshSummaryUsesSuccessOnlyAfterDisplayedPublication(t *testing.T) {
	oldProfile, oldDark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile); lipgloss.SetHasDarkBackground(oldDark) })
	for _, dark := range []bool{false, true} {
		lipgloss.SetHasDarkBackground(dark)
		for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.Ascii} {
			lipgloss.SetColorProfile(profile)
			m := savedRefreshModel(t)
			assertColor := func(label string, color lipgloss.AdaptiveColor) {
				t.Helper()
				line := m.refreshLine()
				plain := ansi.Strip(line)
				if !strings.Contains(plain, label) || line != lipgloss.NewStyle().Foreground(color).Render(plain) {
					t.Fatalf("%s used wrong label/color (dark=%v, profile=%v): %q", label, dark, profile, line)
				}
			}
			assertColor("Saved usage", themeMuted)
			updated, _ := m.Update(refreshStatusMessage(0, 2, 2, 1, 1))
			m = updated.(interactiveModel)
			assertColor("Updating displayed usage", themeAccent)
			updated, _ = m.Update(reloadMsg{preservePosition: true, instanceID: "instance", dataEpoch: "database", revision: 2,
				inputRevision: 2, serverGeneration: 1, refreshToken: m.refresh.token, rows: []renderRow{{totalValue: 200}}})
			m = updated.(interactiveModel)
			assertColor("Usage refreshed", themeSuccess)
			m.refresh.startupAttemptID = m.refresh.attempt.AttemptID
			m.refresh.result = &localruntime.CollectionResult{Result: collector.Result{}}
			assertColor("no new usage", themeMuted)
			m.refresh.queryErr = errors.New("query failure")
			assertColor("Display update failed", themeDanger)
		}
	}
}

func TestBackgroundReadFailureAndUnknownStatusPreserveVisibleUsage(t *testing.T) {
	m := savedRefreshModel(t)
	updated, _ := m.Update(refreshStatusMessage(0, 2, 2, 1, 1))
	m = updated.(interactiveModel)
	updated, _ = m.Update(reloadMsg{preservePosition: true, err: errors.New("private query failure")})
	m = updated.(interactiveModel)
	view := ansi.Strip(m.View())
	if m.err != nil || m.refresh.queryErr == nil || m.refreshBusy() || !strings.Contains(view, "100") || !strings.Contains(view, "Display update failed") {
		t.Fatal("failed background read erased data or kept spinning", view)
	}
	if strings.Contains(view, "private query failure") {
		t.Fatal("raw query error exposed in refresh strip")
	}
	m.refresh.queryErr = nil
	updated, _ = m.Update(sharedSyncMsg{err: errors.New("private status failure")})
	m = updated.(interactiveModel)
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "Refresh status unavailable") || !strings.Contains(view, "100") || m.refreshBusy() {
		t.Fatal("unknown status claimed completion or erased saved usage", view)
	}
}

func TestBackgroundPublicationPreservesFocusedIdentityAndDrawerDraft(t *testing.T) {
	m := savedRefreshModel(t)
	m.activeTab, m.cursor, m.scrollOffset, m.horizontalOffset = tabModels, 1, 1, 2
	m.rows = []renderRow{{model: "a", totalValue: 300}, {model: "b", totalValue: 200}, {model: "c", totalValue: 100}}
	m.popup, m.filterDimension, m.popupCursor = popupFilterValues, filterModel, 1
	m.filterValues = []string{"a", "b", "c"}
	m.options.filters.models = stringList{"a"}
	m.filterSelections = map[string]bool{"a": false, "b": true}
	updated, _ := m.Update(reloadMsg{preservePosition: true, instanceID: "instance", dataEpoch: "database", revision: 2,
		rows: []renderRow{{model: "new", totalValue: 400}, {model: "a", totalValue: 300}, {model: "c", totalValue: 250}, {model: "b", totalValue: 200}}})
	m = updated.(interactiveModel)
	if m.cursor != 3 || m.rows[m.cursor].model != "b" || m.popup != popupFilterValues || !reflect.DeepEqual(m.options.filters.models, stringList{"a"}) {
		t.Fatal("publication moved focused identity or changed applied scope")
	}
	updated, _ = m.Update(filterValuesMsg{instanceID: "instance", dataEpoch: "database", revision: 2, dimension: filterModel, values: []string{"a", "b", "new"}})
	m = updated.(interactiveModel)
	if m.filterSelections["a"] || !m.filterSelections["b"] || m.filterValues[m.popupCursor] != "b" || !reflect.DeepEqual(m.options.filters.models, stringList{"a"}) {
		t.Fatal("facet refresh overwrote unapplied selections or focused value", m.filterSelections)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(interactiveModel)
	if m.popup != popupNone || !reflect.DeepEqual(m.options.filters.models, stringList{"a"}) {
		t.Fatal("Cancel committed background-refreshed draft")
	}
}

func TestRefreshStripFitsThemesAndCompactTerminalsWithoutLayoutJumps(t *testing.T) {
	oldProfile, oldDark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile); lipgloss.SetHasDarkBackground(oldDark) })
	for _, dark := range []bool{false, true} {
		lipgloss.SetHasDarkBackground(dark)
		for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.Ascii} {
			lipgloss.SetColorProfile(profile)
			for _, size := range [][2]int{{120, 35}, {80, 24}, {60, 18}} {
				m := savedRefreshModel(t)
				m.width, m.height = size[0], size[1]
				m.refresh.collectionPending = true
				busyView := m.View()
				assertDeskBounds(t, busyView, size[0], size[1])
				if !strings.Contains(ansi.Strip(strings.Split(busyView, "\n")[1]), "Refreshing") || !strings.Contains(ansi.Strip(busyView), "100") || !strings.Contains(ansi.Strip(busyView), "q Quit") {
					t.Fatal("refresh status, data, or navigation hidden", busyView)
				}
				_, busyCoverage := viewSummaryLine(busyView)
				m.refresh.collectionPending = false
				idleView := m.View()
				_, idleCoverage := viewSummaryLine(idleView)
				if busyCoverage != idleCoverage || len(m.deskHeader()) == 0 {
					t.Fatal("completion moved the table or coverage")
				}
				m.popup = popupFilters
				if !strings.Contains(ansi.Strip(strings.Split(m.View(), "\n")[1]), "Saved usage") {
					t.Fatal("drawer hid the persistent refresh strip")
				}
			}
		}
	}
}

func TestColdStartEmptyStateWaitsForRefresh(t *testing.T) {
	m := savedRefreshModel(t)
	m.rows, m.sessionCounts = nil, querymodel.SessionCounts{}
	m.refresh.collectionPending = true
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Usage appears automatically") || strings.Contains(view, "No ingested usage yet") {
		t.Fatal("active first refresh presented a definitive empty state", view)
	}
}

func TestBackgroundFacetsPreserveClearAllDraft(t *testing.T) {
	m := savedRefreshModel(t)
	m.popup, m.filterDimension = popupFilterValues, filterModel
	m.options.filters.models = stringList{"a"}
	m.filterValues = []string{"a", "b"}
	m.filterSelections = map[string]bool{"a": true}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	m = updated.(interactiveModel)
	updated, _ = m.Update(filterValuesMsg{instanceID: "instance", dataEpoch: "database", revision: 1, dimension: filterModel, values: []string{"a", "b", "new"}})
	m = updated.(interactiveModel)
	if len(selectedValues(m.filterValues, m.filterSelections)) != 0 || !reflect.DeepEqual(m.options.filters.models, stringList{"a"}) {
		t.Fatal("background facets resurrected cleared draft or committed it", m.filterSelections)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := updated.(interactiveModel); len(got.options.filters.models) != 0 {
		t.Fatal("Apply did not commit clear-all draft", got.options.filters.models)
	}
}

func TestStaleBackgroundDashboardDoesNotHideSavedUsage(t *testing.T) {
	m := savedRefreshModel(t)
	updated, _ := m.Update(refreshStatusMessage(0, 3, 3, 1, 1))
	m = updated.(interactiveModel)
	updated, _ = m.Update(reloadMsg{preservePosition: true, instanceID: "instance", dataEpoch: "database", revision: 2,
		rows: []renderRow{{totalValue: 200}}})
	m = updated.(interactiveModel)
	if m.err != nil || m.rows[0].totalValue != 100 || !m.refreshNeedsRead() || !strings.Contains(ansi.Strip(m.View()), "100") {
		t.Fatal("stale background read hid or replaced the saved snapshot", m.err)
	}
}

func TestQueuedQuarantineAndExpiredFailureRemainVisible(t *testing.T) {
	m := savedRefreshModel(t)
	message := refreshStatusMessage(0, 1, 1, 1, 1)
	message.collection.Attempts[0].Stage = "failed"
	message.collection.Attempts[0].ErrorCode = "collection_failed"
	updated, _ := m.Update(message)
	m = updated.(interactiveModel)
	if !m.collectionFailed() || m.refreshBusy() || !strings.Contains(m.refreshLine(), "Refresh incomplete") {
		t.Fatal("queued incomplete collection claimed completion", m.refreshLine())
	}
	message.collection.Attempts = nil // Registry retention must not clear an unresolved UI failure.
	updated, _ = m.Update(message)
	m = updated.(interactiveModel)
	if !m.collectionFailed() || !strings.Contains(m.refreshLine(), "Refresh incomplete") {
		t.Fatal("expired progress erased unresolved failure", m.refreshLine())
	}
}
