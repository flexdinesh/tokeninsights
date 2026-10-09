package cli

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/muesli/termenv"
)

func detailedRefreshModel(t *testing.T) interactiveModel {
	t.Helper()
	m := savedRefreshModel(t)
	m.options.local = &localruntime.Runtime{}
	m.options.syncOnStart = true
	m.refresh.attempt = &collectorprogress.Attempt{AttemptID: "current", Stage: "capturing", Harnesses: map[string]string{
		"opencode": "complete", "pi": "complete", "codex": "running", "claude-code": "waiting",
	}}
	m.refresh.capture = map[string]collectorprogress.Capture{
		"opencode":    {Phase: collectorprogress.CaptureComplete, TotalKnown: true, Total: 1, Checked: 1, Unchanged: 1},
		"pi":          {Phase: collectorprogress.CaptureComplete, TotalKnown: true, Total: 18, Checked: 18, Captured: 18},
		"codex":       {Phase: collectorprogress.CaptureReading, TotalKnown: true, Total: 240, Checked: 84, Captured: 80, Unchanged: 4, Active: 2},
		"claude-code": {Phase: collectorprogress.CaptureWaiting, TotalKnown: true, Total: 30},
	}
	return m
}

func TestCaptureShowsFinalizedSourcesAndUnknownDiscovery(t *testing.T) {
	m := detailedRefreshModel(t)
	plain := strings.Join(strings.Fields(ansi.Strip(strings.Join(m.captureRows(), "\n"))), " ")
	for _, want := range []string{"OpenCode", "Pi", "Codex", "Claude Code", "84/240 sources · 156 left", "0/30 sources · 30 left"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing %q: %s", want, plain)
		}
	}
	m.refresh.capture["codex"] = collectorprogress.Capture{Phase: collectorprogress.CaptureDiscovering}
	line := ansi.Strip(m.captureRows()[2])
	if !strings.Contains(line, "Discovering") || !strings.Contains(line, "sources unknown") || strings.Contains(line, "0/0") || strings.Contains(line, "left") {
		t.Fatal("discovery claimed a known denominator", line)
	}
	// A coarse complete status lacks source measurements; do not invent them.
	m.refresh.capture = nil
	line = ansi.Strip(m.captureRows()[0])
	if !strings.Contains(line, "Complete") || !strings.Contains(line, "sources unknown") || strings.Contains(line, "0/0") {
		t.Fatal("legacy progress fabricated source counts", line)
	}
}

func TestCaptureLabelsActivityEmptyAndUnchangedPrecisely(t *testing.T) {
	m := detailedRefreshModel(t)
	m.refresh.capture = map[string]collectorprogress.Capture{"codex": {Phase: collectorprogress.CaptureDiscovering}}
	if line := ansi.Strip(m.refreshLine()); !strings.Contains(line, "discovering local sources") || strings.Contains(line, "reading") {
		t.Fatal("discovery claimed admitted reading work", line)
	}
	m.refresh.capture["codex"] = collectorprogress.Capture{Phase: collectorprogress.CaptureReading, TotalKnown: true, Total: 5, Checked: 2, Failed: 1, Quarantined: 1, Active: 1}
	line := ansi.Strip(m.captureRows()[2])
	if !strings.Contains(line, "Reading") || !strings.Contains(line, "1 failed") || !strings.Contains(line, "1 quarantined") || strings.Contains(line, "Incomplete") {
		t.Fatal("prior failure hid continuing source work", line)
	}
	m.refresh.capture["codex"] = collectorprogress.Capture{Phase: collectorprogress.CaptureComplete, TotalKnown: true}
	if line := ansi.Strip(m.captureRows()[2]); !strings.Contains(line, "No sources") {
		t.Fatal("empty harness claimed captured evidence", line)
	}
	m.refresh.capture["codex"] = collectorprogress.Capture{Phase: collectorprogress.CaptureComplete, TotalKnown: true, Total: 5, Checked: 5, Unchanged: 5}
	if line := ansi.Strip(m.captureRows()[2]); !strings.Contains(line, "Unchanged") {
		t.Fatal("unchanged sources claimed newly captured evidence", line)
	}
	m.refresh.capture = nil
	m.refresh.startupAttemptID = m.refresh.attempt.AttemptID
	m.refresh.attempt.Harnesses = nil
	if plain := ansi.Strip(strings.Join(m.captureRows(), "\n")); strings.Count(plain, "Waiting") != 4 || strings.Contains(plain, "Not requested") {
		t.Fatal("startup discovery omitted requested harnesses", plain)
	}
}

func TestCaptureSelectionDoesNotMixAttemptsAndRetainsExpiredFailure(t *testing.T) {
	m := detailedRefreshModel(t)
	failed := collectorprogress.Capture{Phase: collectorprogress.CaptureFailed, TotalKnown: true, Total: 5, Checked: 4, Captured: 1, Unchanged: 1, Failed: 1, Quarantined: 1}
	snapshot := collectorprogress.Snapshot{Attempts: []collectorprogress.Attempt{
		{AttemptID: "old", Stage: "accepted", StartedAtMS: 1},
		{AttemptID: "new", Stage: "failed", StartedAtMS: 2},
	}}
	m = m.observeRefresh(sharedSyncMsg{collection: &snapshot, capture: map[string]map[string]collectorprogress.Capture{
		"old": {"codex": {Phase: collectorprogress.CaptureComplete, TotalKnown: true, Total: 999, Checked: 999, Captured: 999}},
		"new": {"codex": failed},
	}})
	plain := ansi.Strip(strings.Join(m.captureRows(), "\n"))
	for _, want := range []string{"4/5", "1 failed", "1 quarantined", "Incomplete"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("missing failed outcome %q: %s", want, plain)
		}
	}
	if strings.Contains(plain, "999") {
		t.Fatal("selected attempt inherited another attempt's counts", plain)
	}
	empty := collectorprogress.Snapshot{}
	m = m.observeRefresh(sharedSyncMsg{collection: &empty})
	if got := ansi.Strip(strings.Join(m.captureRows(), "\n")); got != plain {
		t.Fatal("retention erased unresolved capture failure", got)
	}
}

func TestDisabledStartupStillDisplaysQueuedCapture(t *testing.T) {
	m := detailedRefreshModel(t)
	m.options.syncOnStart = false
	m.refresh.attempt, m.refresh.capture = nil, nil
	plain := ansi.Strip(strings.Join(m.captureRows(), "\n"))
	if strings.Count(plain, "Disabled") != 4 || strings.Contains(plain, "sources unknown") {
		t.Fatal("saved-only viewer claimed active discovery", plain)
	}
	m.refresh.attempt = &collectorprogress.Attempt{AttemptID: "queued", Stage: "capturing", Harnesses: map[string]string{"codex": "running"}}
	m.refresh.capture = map[string]collectorprogress.Capture{"codex": {Phase: collectorprogress.CaptureReading, TotalKnown: true, Total: 10, Checked: 3, Active: 1}}
	plain = ansi.Strip(strings.Join(m.captureRows(), "\n"))
	if !strings.Contains(plain, "3/10 sources · 7 left") || strings.Contains(plain, "Disabled") {
		t.Fatal("startup policy hid an independently queued local attempt", plain)
	}
	m.options.local = nil
	if len(m.captureRows()) != 0 {
		t.Fatal("detailed capture leaked into distributed viewer")
	}
}

func TestLocalDeliveryAndProcessingExposeDifferentUnits(t *testing.T) {
	m := detailedRefreshModel(t)
	m.refresh.attempt.Stage = "submitting"
	m.refresh.attempt.PendingKnown = true
	m.refresh.attempt.AcknowledgedEntries, m.refresh.attempt.Pending = 8000, 2000
	line := ansi.Strip(m.refreshLine())
	if !strings.Contains(line, "8000 entries accepted") || !strings.Contains(line, "2000 pending") || strings.Contains(line, "Usage refreshed") {
		t.Fatal("submission did not preserve acceptance semantics", line)
	}
	m.refresh.attempt.Stage = "accepted"
	m.sharedSync.Running = true
	m.refresh.statusKnown, m.refresh.status.Pending = true, 12
	line = ansi.Strip(m.refreshLine())
	if !strings.Contains(line, "12 groups pending") || strings.Contains(line, "8000") || strings.Contains(line, "Usage refreshed") {
		t.Fatal("processing backlog attributed to submitted entries", line)
	}
}

func TestCaptureRemainsVisibleWithStableViewportAndDrawers(t *testing.T) {
	oldProfile, oldDark := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	t.Cleanup(func() { lipgloss.SetColorProfile(oldProfile); lipgloss.SetHasDarkBackground(oldDark) })
	for _, dark := range []bool{false, true} {
		lipgloss.SetHasDarkBackground(dark)
		for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.Ascii} {
			lipgloss.SetColorProfile(profile)
			for _, size := range [][2]int{{120, 35}, {80, 24}, {60, 18}} {
				m := detailedRefreshModel(t)
				m.width, m.height = size[0], size[1]
				baselineHeader, baselineRows := len(m.deskHeader()), m.maxVisibleRows()
				for _, stage := range []string{"capturing", "accepted", "failed", "interrupted"} {
					m.refresh.attempt.Stage = stage
					if stage == "failed" {
						m.refresh.capture["codex"] = collectorprogress.Capture{Phase: collectorprogress.CaptureFailed, TotalKnown: true, Total: 240, Checked: 84, Captured: 81, Failed: 2, Quarantined: 1}
					}
					view := m.View()
					assertDeskBounds(t, view, size[0], size[1])
					plain := ansi.Strip(view)
					if stage == "capturing" && !strings.Contains(plain, "sources") {
						t.Fatal("compact progress omitted the measured unit", plain)
					}
					for _, want := range []string{"OpenCode", "Pi", "Codex", "Claude", "100", "q Quit"} {
						if !strings.Contains(plain, want) {
							t.Fatalf("%dx%d %s hid %q: %s", size[0], size[1], stage, want, plain)
						}
					}
					if stage == "failed" && (!strings.Contains(plain, "2 failed") || !strings.Contains(plain, "1 quarantined")) {
						t.Fatal("compact row hid failure outcomes", plain)
					}
					if len(m.deskHeader()) != baselineHeader || m.maxVisibleRows() != baselineRows {
						t.Fatal("attempt transition changed header or viewport geometry")
					}
					progress := strings.Split(view, "\n")[:m.drawerTop()]
					m.popup = popupFilters
					drawer := m.View()
					assertDeskBounds(t, drawer, size[0], size[1])
					if got := strings.Join(strings.Split(drawer, "\n")[:m.drawerTop()], "\n"); got != strings.Join(progress, "\n") {
						t.Fatal("drawer obscured local progress")
					}
					m.popup = popupNone
				}
			}
		}
	}
}
