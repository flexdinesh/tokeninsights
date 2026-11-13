package views

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
)

func readoutOf(label, value, valueLabel, detail string) kit.Readout {
	return kit.Readout{Label: label, Value: value, ValueLabel: valueLabel, Detail: detail}
}

func TestSummaryReadoutsMatchTheWeb(t *testing.T) {
	want := []kit.Readout{
		readoutOf("Total tokens", "647K", "Total tokens: 647,000", "Included in your usage total"),
		readoutOf("Input", "369K", "Input: 369,000", "Uncached prompt tokens"),
		readoutOf("Output", "78K", "Output: 78,000", "13K reasoning tokens reported"),
		readoutOf("Cache read", "178K", "Cache read: 178,000", "9K cache write tokens"),
		readoutOf("Sessions shown", "4", "Sessions shown: 4", "5 synced across all dates & harnesses"),
	}
	got := SummaryReadouts(studioUsage.Summary, false)
	if len(got) != len(want) {
		t.Fatalf("readouts %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("readout %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	summary := studioUsage.Summary
	summary.TotalTokens, summary.SyncedSessions = 1_234_567, 1_204
	excluded := SummaryReadouts(summary, true)
	if excluded[0] != readoutOf("Excluded tokens", "1.2M", "Excluded tokens: 1,234,567", "Excluded from your usage total") ||
		excluded[4] != readoutOf("Excluded sessions shown", "4", "Excluded sessions shown: 4", "1,204 with excluded usage across all dates & harnesses") {
		t.Fatalf("excluded readouts %+v", excluded)
	}
}

func TestSummaryShowsUsageAndNarrowsWithTheWindow(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	if _, ok := tt.AccessByRole(ui.RoleRegion, summaryLabel); !ok {
		t.Fatal("no summary strip")
	}
	assertTexts(t, tt, totalReadout, "Input: 369,000", "13K reasoning tokens reported", "5 synced across all dates & harnesses")
	if tt.HasText(loadingDashboard) || tt.HasText(noUsage) {
		t.Fatal("loaded usage kept the skeleton or said none")
	}
	total, _ := tt.Find("Total tokens")
	input, _ := tt.Find("Input")
	if input.Y != total.Y || input.X <= total.X {
		t.Fatalf("wide readouts not in one row: %v %v", total, input)
	}
	tt.SetSize(minWidth, minHeight)
	tt.Frame()
	total, _ = tt.Find("Total tokens")
	input, _ = tt.Find("Input")
	if input.Y <= total.Y {
		t.Fatalf("narrow readouts kept one row: %v %v", total, input)
	}
}

func TestSkeletonWhileTheFirstUsageLoads(t *testing.T) {
	f := loaded()
	tt := f.tester(windowWidth, windowHeight, false)
	f.workers.step() // status and progress answer; usage reads
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleStatus, loadingDashboard); !ok {
		t.Fatalf("no skeleton in %q", tt.Texts())
	}
	if tt.HasText(totalReadout) {
		t.Fatal("readouts before usage")
	}
	f.sync(tt)
	if tt.HasText(loadingDashboard) {
		t.Fatal("the skeleton stayed")
	}
	assertTexts(t, tt, totalReadout)
}

func TestTabSwitchHoldsTheViewWhileItLoads(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	if err := tt.Click("Harnesses"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.AccessByRole(ui.RoleStatus, updatingResults); !ok {
		t.Fatalf("no view skeleton in %q", tt.Texts())
	}
	assertTexts(t, tt, "/ Harnesses", totalReadout)
	f.sync(tt)
	if tt.HasText(updatingResults) {
		t.Fatal("the view skeleton stayed")
	}
	if got := f.backend.requests[len(f.backend.requests)-1]; got.Tab != string(dashboardview.TabHarnesses) {
		t.Fatalf("request %+v", got)
	}
}

func TestUsageFailureAlertRetries(t *testing.T) {
	f := loaded()
	f.backend.usageErr = errors.New("usage unavailable")
	tt := f.synced(windowWidth, windowHeight, false)
	if a, ok := tt.AccessByRole(ui.RoleAlert, ""); !ok || !tt.HasText("usage unavailable") {
		t.Fatalf("no usage alert: %+v in %q", a, tt.Texts())
	}
	if tt.HasText(loadingDashboard) {
		t.Fatal("a failed read kept the skeleton")
	}
	f.backend.usageErr = nil
	if err := tt.Click(retryRequest); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if tt.HasText("usage unavailable") || len(f.backend.requests) != 2 {
		t.Fatalf("retry: %d requests", len(f.backend.requests))
	}
	assertTexts(t, tt, totalReadout)
}

func TestCollectionAlertsRetryProgress(t *testing.T) {
	f := loaded()
	f.backend.progressErr = errors.New("progress unavailable")
	tt := f.synced(windowWidth, windowHeight, false)
	assertTexts(t, tt, collectionUnknown, totalReadout)
	f.backend.progressErr = nil
	f.backend.progress.Attempts = []collectorprogress.Attempt{failed}
	if err := tt.Click(retryRequest); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if tt.HasText(collectionUnknown) {
		t.Fatal("Retry Request did not read progress")
	}
	if _, ok := tt.AccessByRole(ui.RoleAlert, collectionFailed); !ok {
		t.Fatalf("no failed collection line in %q", tt.Texts())
	}
}

func TestEmptyUsageSaysNoneSaved(t *testing.T) {
	f := loaded()
	f.backend.usage.Summary = analytics.ViewerSummaryRow{}
	tt := f.synced(windowWidth, windowHeight, false)
	if _, ok := tt.AccessByRole(ui.RoleStatus, noUsage); !ok {
		t.Fatalf("no empty message in %q", tt.Texts())
	}
	// As on the web, the zero readouts stay under the message.
	if tt.HasText(loadingDashboard) || !tt.HasText("Total tokens: 0") {
		t.Fatalf("empty usage: %q", tt.Texts())
	}

	// While collection runs, no usage yet means the first usage is still
	// on its way: the skeleton holds its place.
	f2 := loaded()
	f2.backend.usage.Summary = analytics.ViewerSummaryRow{}
	f2.backend.progress.Attempts = []collectorprogress.Attempt{collecting}
	tt = f2.synced(windowWidth, windowHeight, false)
	if _, ok := tt.AccessByRole(ui.RoleStatus, loadingDashboard); !ok || tt.HasText(noUsage) {
		t.Fatalf("waiting for the first usage: %q", tt.Texts())
	}
	if emptyMessage(true, false) != noExcludedUsage || emptyMessage(false, true) != noIncludedUsage {
		t.Fatal("empty messages")
	}
}

func TestProcessingNotes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(f *fixture)
		texts []string
	}{
		{"pending", func(f *fixture) { f.backend.status.Pending = 1_204 }, []string{"Processing usage · 1,204 pending."}},
		{"rebuilding", func(f *fixture) { f.backend.status.Metadata.TargetGeneration = 4 }, []string{rebuildingUsage}},
		{"retrying", func(f *fixture) { f.backend.status.Failed = 2 }, []string{retryingUsage + savedTotals}},
		{"retry scheduled", func(f *fixture) {
			f.backend.status.Failed, f.backend.status.Pending = 2, 2
			f.backend.status.FailedRetryAtMs = readsAt.Add(time.Hour).UnixMilli()
		}, []string{"Some usage needs a processing retry. Retry scheduled at Nov 13, 2026, 10:30." + savedTotals}},
		{"unresolved", func(f *fixture) { f.backend.usage.Unresolved = 3 }, []string{"3" + unresolvedSuffix}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := loaded()
			tc.setup(f)
			tt := f.synced(windowWidth, windowHeight, false)
			assertTexts(t, tt, tc.texts...)
		})
	}
}

// TestResultsImages renders the results loaded on the Tokens tab, loading,
// failed and empty, in both appearances, and writes PNGs when
// TOKENINSIGHTS_NATIVE_GALLERY_DIR is set.
func TestResultsImages(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, shot := range []struct {
		name  string
		setup func(f *fixture)
		text  string
	}{
		{"loaded", func(*fixture) {}, totalReadout},
		{"skeleton", nil, ""},
		{"error", func(f *fixture) {
			f.backend.usageErr = errors.New("usage read failed: backend closed the connection")
		}, retryRequest},
		{"empty", func(f *fixture) { f.backend.usage.Summary = analytics.ViewerSummaryRow{} }, noUsage},
	} {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(shot.name+"/"+appearance, func(t *testing.T) {
				f := loaded()
				var tt *ui.Tester
				if shot.setup == nil {
					tt = f.tester(windowWidth, windowHeight, dark)
					f.workers.step()
					tt.Frame()
					if _, ok := tt.AccessByRole(ui.RoleStatus, loadingDashboard); !ok {
						t.Fatal("no skeleton")
					}
				} else {
					shot.setup(f)
					tt = f.synced(windowWidth, windowHeight, dark)
					assertTexts(t, tt, shot.text)
				}
				tt.Advance(settle)
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, "results-"+shot.name+"-"+appearance+".png"), tt)
			})
		}
	}
}
