package views

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
)

// Collector attempts as the backend reports them during and after a run.
var (
	runStarted  = readsAt.Add(-2 * time.Minute).UnixMilli()
	runFinished = readsAt.Add(-time.Minute).UnixMilli()

	collecting = collectorprogress.Attempt{
		AttemptID: "attempt-2", Stage: dashboardview.StageCapturing, AcknowledgedEntries: 1204, PendingKnown: true, Pending: 36,
		Harnesses:   map[string]string{"claude-code": "complete", "codex": "running", "opencode": "waiting", "pi": "skipped"},
		StartedAtMS: runFinished, UpdatedAtMS: readsAt.UnixMilli(),
	}
	accepted = collectorprogress.Attempt{
		AttemptID: "attempt-1", Stage: dashboardview.StageAccepted, AcknowledgedEntries: 18342,
		Harnesses:   map[string]string{"claude-code": "complete", "codex": "complete", "opencode": "skipped", "pi": "complete"},
		StartedAtMS: runStarted, UpdatedAtMS: runFinished, FinishedAtMS: runFinished,
	}
	failed = collectorprogress.Attempt{
		AttemptID: "attempt-3", Stage: dashboardview.StageFailed, AcknowledgedEntries: 312, ErrorCode: "submission_failed",
		Harnesses:   map[string]string{"claude-code": "complete", "codex": "failed", "opencode": "skipped", "pi": "complete"},
		StartedAtMS: runStarted, UpdatedAtMS: runFinished, FinishedAtMS: runFinished,
	}
)

// withAttempts is a loaded fixture whose progress reports attempts.
func withAttempts(attempts ...collectorprogress.Attempt) *fixture {
	f := loaded()
	f.backend.progress.Attempts = attempts
	return f
}

const readyLabel = statusLabelPrefix + "Ready"

// hover rests the pointer on the middle of the element showing name.
func hover(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	r, ok := tt.Find(name)
	if !ok {
		t.Fatalf("no %q in %q", name, tt.Texts())
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
}

// popoverOpen reports whether the status popover shows.
func popoverOpen(tt *ui.Tester) bool {
	return tt.HasText(identityPrefix + studioInstance.InstanceID)
}

// away is a point of the page far from the header.
func away(tt *ui.Tester) { tt.Move(windowWidth/2, windowHeight-1) }

func TestStatusPopoverOpensOnHoverAndClosesAfterDelay(t *testing.T) {
	f := withAttempts(accepted)
	tt := f.synced(windowWidth, windowHeight, false)
	if popoverOpen(tt) {
		t.Fatal("popover open before hover")
	}
	hover(t, tt, readyLabel)
	tt.Advance(settle)
	if !popoverOpen(tt) {
		t.Fatal("hover did not open the popover")
	}
	away(tt)
	tt.Advance(hoverCloseDelay - 10*time.Millisecond)
	if !popoverOpen(tt) {
		t.Fatal("popover closed before the hover delay")
	}
	tt.Advance(settle)
	if popoverOpen(tt) {
		t.Fatal("popover stayed after the pointer left")
	}
}

func TestStatusPopoverStaysWhileThePointerCrossesToIt(t *testing.T) {
	f := withAttempts(accepted)
	tt := f.synced(windowWidth, windowHeight, false)
	hover(t, tt, readyLabel)
	tt.Advance(settle)
	hover(t, tt, identityPrefix+studioInstance.InstanceID)
	tt.Advance(settle)
	if !popoverOpen(tt) {
		t.Fatal("moving onto the popover closed it")
	}
	away(tt)
	tt.Advance(settle)
	if popoverOpen(tt) {
		t.Fatal("popover stayed after the pointer left it")
	}
}

func TestStatusPopoverPinsOnClickUntilDismissed(t *testing.T) {
	f := withAttempts(accepted)
	tt := f.synced(windowWidth, windowHeight, false)
	if err := tt.Click(readyLabel); err != nil {
		t.Fatal(err)
	}
	away(tt)
	tt.Advance(settle)
	if !popoverOpen(tt) {
		t.Fatal("a click did not pin the popover")
	}
	if err := tt.Click(readyLabel); err != nil {
		t.Fatal(err)
	}
	away(tt)
	tt.Advance(settle)
	if !popoverOpen(tt) {
		t.Fatal("a second click closed the popover")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(settle)
	if popoverOpen(tt) {
		t.Fatal("Escape left the popover open")
	}
	// Dismissed, it is no longer pinned: hovering opens it until the
	// pointer leaves.
	hover(t, tt, readyLabel)
	tt.Advance(settle)
	away(tt)
	tt.Advance(settle)
	if popoverOpen(tt) {
		t.Fatal("a dismissed popover stayed pinned")
	}
}

func TestStatusPopoverPinsOnKeyboardFocus(t *testing.T) {
	f := withAttempts(accepted)
	tt := f.synced(windowWidth, windowHeight, false)
	for range len(tt.AccessAll()) {
		if tt.Focused(readyLabel) {
			break
		}
		tt.Key(0, ui.KeyTab)
	}
	if !tt.Focused(readyLabel) {
		t.Fatal("Tab never reached the status")
	}
	tt.Advance(settle)
	if !popoverOpen(tt) {
		t.Fatal("focus did not open the popover")
	}
	tt.Key(0, ui.KeyTab)
	tt.Advance(settle)
	if !popoverOpen(tt) {
		t.Fatal("moving the focus on closed a pinned popover")
	}
}

func TestStatusPopoverShowsDetailsAndProgress(t *testing.T) {
	f := withAttempts(accepted, collecting)
	tt := f.synced(windowWidth, windowHeight, false)
	if err := tt.Click(statusLabelPrefix + "Collecting usage"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(settle)
	assertTexts(t, tt, "Collecting usage", identityPrefix+studioInstance.InstanceID, latestCollection,
		"Collecting usage · 1,204 accepted · 36 remaining", "Usage accepted · 18,342 accepted",
		"claude-code: Collected", "codex: Collecting", "opencode: Waiting", "pi: No new usage", "opencode: No new usage")
	if tt.HasText("CPU") || tt.HasText("RAM") {
		t.Fatal("native shows backend CPU or RAM")
	}
	if _, ok := tt.AccessByRole(ui.RoleHeading, latestCollection); !ok {
		t.Fatal("no Latest collection heading")
	}
	if _, ok := tt.AccessByRole(ui.RoleGroup, progressLabel); !ok {
		t.Fatal("no collector progress group")
	}
	if _, ok := tt.AccessByRole(ui.RoleList, harnessListLabel); !ok {
		t.Fatal("no harness list")
	}
	if a, ok := tt.AccessByRole(ui.RoleButton, "Refreshing…"); !ok || !a.Disabled {
		t.Fatalf("Refresh while collecting: %+v", a)
	}
	// The active attempt comes first; only the finished one has a heading.
	active, _ := tt.Find("Collecting usage · 1,204 accepted · 36 remaining")
	heading, _ := tt.Find(latestCollection)
	if active.Y >= heading.Y {
		t.Fatal("the latest collection precedes the active attempt")
	}
}

func TestStatusShowsLastIngestionInServerTime(t *testing.T) {
	d := StatusDetails{
		Value:      dashboardview.StatusValue{Label: "Ready", State: dashboardview.StatusReady},
		InstanceID: studioInstance.InstanceID, LastSynced: runFinished, Timezone: "Asia/Tokyo",
	}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Justify(ui.End).Padding(32).Children(func() { DashboardStatus(c.Key("status"), d) })
	}, windowWidth, windowHeight)
	if err := tt.Click(readyLabel); err != nil {
		t.Fatal(err)
	}
	tt.Advance(settle)
	want := ingestionPrefix + dashboardview.FormatServerDateTime(runFinished, "Asia/Tokyo")
	assertTexts(t, tt, want, "Ready")
	if a, ok := tt.Access(want); !ok || a.Description != dashboardview.ServerZoneLabel("Asia/Tokyo") {
		t.Fatalf("last ingestion: %+v", a)
	}
	if tt.HasText(latestCollection) {
		t.Fatal("progress without progress")
	}
}

func TestStatusFollowsTheLadder(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *fixture)
		label string
		tone  kit.DotTone
		line  string
	}{
		{"ready", func(*fixture) {}, "Ready", kit.DotSuccess, "connected"},
		{"collecting", func(f *fixture) { f.backend.progress.Attempts = []collectorprogress.Attempt{collecting} }, "Collecting usage", kit.DotPending, "connected"},
		{"failed collection", func(f *fixture) { f.backend.progress.Attempts = []collectorprogress.Attempt{failed} }, "Collection needs attention", kit.DotDanger, "connected"},
		{"status failure", func(f *fixture) { f.backend.statusErr = errors.New("backend busy") }, "Connection unavailable", kit.DotDanger, connectionUnavailable},
		{"progress failure", func(f *fixture) { f.backend.progressErr = errors.New("progress unavailable") }, "Collection status unavailable", kit.DotDanger, "connected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := loaded()
			tc.setup(f)
			tt := f.synced(windowWidth, windowHeight, false)
			if _, ok := tt.AccessByRole(ui.RoleButton, statusLabelPrefix+tc.label); !ok {
				t.Fatalf("no status %q", tc.label)
			}
			if got := footerTone(f.window); got != tc.tone {
				t.Fatalf("footer tone %v, want %v", got, tc.tone)
			}
			assertTexts(t, tt, "SSH · ssh://alice@studio.lan:2222 · "+tc.line)
		})
	}
}

func TestStatusPollsWhileShownAndPausesWhileHidden(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	if f.backend.statuses != 1 {
		t.Fatalf("status reads %d after the first frame", f.backend.statuses)
	}
	tt.Advance(model.PollInterval)
	f.sync(tt)
	if f.backend.statuses != 2 {
		t.Fatalf("status reads %d after one interval", f.backend.statuses)
	}
	tt.SetWindowHidden(true)
	tt.Advance(3 * model.PollInterval)
	f.sync(tt)
	if f.backend.statuses != 2 {
		t.Fatalf("a hidden window read status: %d", f.backend.statuses)
	}
	tt.SetWindowHidden(false)
	f.sync(tt)
	if f.backend.statuses != 3 {
		t.Fatalf("showing did not read status at once: %d", f.backend.statuses)
	}
}

// TestStatusImages renders the shell with the status popover open while
// ready, loading and needing attention, in both appearances, and writes
// PNGs when TOKENINSIGHTS_NATIVE_GALLERY_DIR is set.
func TestStatusImages(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, shot := range []struct {
		name     string
		attempts []collectorprogress.Attempt
		label    string
	}{
		{"ready", []collectorprogress.Attempt{accepted}, "Ready"},
		{"loading", []collectorprogress.Attempt{accepted, collecting}, "Collecting usage"},
		{"attention", []collectorprogress.Attempt{failed}, "Collection needs attention"},
	} {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(shot.name+"/"+appearance, func(t *testing.T) {
				f := withAttempts(shot.attempts...)
				tt := f.synced(windowWidth, windowHeight, dark)
				if err := tt.Click(statusLabelPrefix + shot.label); err != nil {
					t.Fatal(err)
				}
				tt.Advance(settle)
				assertTexts(t, tt, identityPrefix+studioInstance.InstanceID, latestCollection)
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, "status-"+shot.name+"-"+appearance+".png"), tt)
			})
		}
	}
}
