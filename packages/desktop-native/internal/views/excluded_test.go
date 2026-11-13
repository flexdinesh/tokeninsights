package views

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// excludedSummary is usage excluded from the totals: two sessions whose
// tokens could not be confidently counted.
var excludedSummary = analytics.ViewerSummaryRow{
	TotalTokens: 18_600, InputTokens: 12_100, OutputTokens: 4_200, CacheReadTokens: 2_300,
	SessionCount: 2, SyncedSessions: 3,
}

// excludedRows are excludedSummary's rows of each tab the tests review.
func excludedRows() map[dashboardview.Tab][]analytics.Row {
	day := func(key string, date, total int64) analytics.Row {
		row := usageRow(key, key, 1, total)
		row.Date = date
		return row
	}
	return map[dashboardview.Tab][]analytics.Row{
		dashboardview.TabTokens: {day("2026-09-13", dayTwo, 7_400), day("2026-09-14", dayThree, 11_200)},
	}
}

// withExcluded is a loaded fixture whose backend has excluded usage.
func withExcluded() *fixture {
	f := loaded()
	f.backend.excluded, f.backend.excludedRows = excludedSummary, excludedRows()
	return f
}

const (
	// excludedTotalText is the notice's compact excluded total.
	excludedTotalText = excludedSeparator + "18.6K" + excludedTokensSuffix
	// excludedReadout names the excluded total readout.
	excludedReadout = "Excluded tokens: 18,600"
)

// click clicks label, failing the test when it does not show.
func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("%v in %q", err, tt.Texts())
	}
}

func TestExcludedNoticeTellsOfExcludedUsage(t *testing.T) {
	f := withExcluded()
	tt := f.synced(windowWidth, windowHeight, false)
	assertTexts(t, tt, totalReadout, excludedFromTotals, excludedTotalText, reviewExcluded)
	if total, ok := tt.Access(excludedTotalText); !ok || total.Description != "18,600 tokens" {
		t.Fatalf("excluded total: %+v", total)
	}
	if got := f.backend.excludedRequests; len(got) != 1 || got[0].Tab != string(dashboardview.TabTokens) || got[0].PageSize != 1 {
		t.Fatalf("excluded requests %+v", got)
	}
	notice, _ := tt.Find(excludedFromTotals)
	readouts, _ := tt.Find("Total tokens")
	if notice.Y <= readouts.Y {
		t.Fatalf("notice %v above the readouts %v", notice, readouts)
	}

	// Without excluded usage, no notice.
	tt = loaded().synced(windowWidth, windowHeight, false)
	if tt.HasText(excludedFromTotals) || tt.HasText(reviewExcluded) {
		t.Fatalf("notice without excluded usage: %q", tt.Texts())
	}
}

func TestReviewShowsExcludedUsageAndReturns(t *testing.T) {
	f := withExcluded()
	tt := f.synced(windowWidth, windowHeight, false)
	tt.UseScreenReader()
	click(t, tt, reviewExcluded)
	f.sync(tt)
	if got := f.backend.requests[len(f.backend.requests)-1]; got.Quality != string(dashboardview.QualityEstimated) || got.PageSize != 200 {
		t.Fatalf("review request %+v", got)
	}
	assertTexts(t, tt, excludedUsageHeading, backToUsage, excludedReviewScope, excludedReadout,
		dashboardview.TableTitle(dashboardview.TabTokens, dashboardview.QualityEstimated))
	if tt.HasText(excludedFromTotals) || tt.HasText(totalReadout) {
		t.Fatalf("review kept usage totals or the notice: %q", tt.Texts())
	}
	if h, ok := tt.AccessByRole(ui.RoleHeading, excludedUsageHeading); !ok || h.Level != 2 {
		t.Fatalf("review heading %+v", h)
	}
	if region, ok := tt.AccessByRole(ui.RoleRegion, excludedResultsLabel); !ok || !region.Focused {
		t.Fatalf("review results not focused: %+v", region)
	}
	if !slices.Contains(tt.Announcements(), showingExcluded) {
		t.Fatalf("announcements %q", tt.Announcements())
	}

	click(t, tt, backToUsage)
	f.sync(tt)
	if got := f.backend.requests[len(f.backend.requests)-1]; got.Quality != "" {
		t.Fatalf("back to usage request %+v", got)
	}
	assertTexts(t, tt, totalReadout, excludedFromTotals, excludedTotalText)
	if tt.HasText(excludedUsageHeading) || tt.HasText(backToUsage) {
		t.Fatalf("usage kept the review: %q", tt.Texts())
	}
	if region, ok := tt.AccessByRole(ui.RoleRegion, resultsLabel); !ok || !region.Focused {
		t.Fatalf("usage results not focused: %+v", region)
	}
	if !slices.Contains(tt.Announcements(), showingUsage) {
		t.Fatalf("announcements %q", tt.Announcements())
	}
	if len(f.backend.excludedRequests) != 1 {
		t.Fatalf("the same excluded read again: %+v", f.backend.excludedRequests)
	}
}

func TestExcludedFailureRetries(t *testing.T) {
	f := withExcluded()
	f.backend.excludedErr = errors.New("excluded unavailable")
	tt := f.synced(windowWidth, windowHeight, false)
	if _, ok := tt.AccessByRole(ui.RoleStatus, ""); !ok || !tt.HasText(excludedFailed) || !tt.HasText(retryExcluded) {
		t.Fatalf("no excluded failure in %q", tt.Texts())
	}
	if tt.HasText(excludedFromTotals) || tt.HasText("excluded unavailable") {
		t.Fatalf("failed excluded read: %q", tt.Texts())
	}
	assertTexts(t, tt, totalReadout, "Ready")
	f.backend.excludedErr = nil
	click(t, tt, retryExcluded)
	f.sync(tt)
	if tt.HasText(excludedFailed) {
		t.Fatal("Retry excluded usage kept the failure")
	}
	assertTexts(t, tt, excludedTotalText)
}

func TestEmptyUsageNamesExcludedUsage(t *testing.T) {
	f := withExcluded()
	f.backend.usage.Summary = analytics.ViewerSummaryRow{}
	tt := f.tester(windowWidth, windowHeight, false)
	f.workers.step() // status and progress answer; usage reads
	f.workers.step() // usage answers; excluded usage reads
	tt.Frame()
	if tt.HasText(noUsage) || tt.HasText(noIncludedUsage) {
		t.Fatalf("the empty message did not wait for excluded usage: %q", tt.Texts())
	}
	f.sync(tt)
	if _, ok := tt.AccessByRole(ui.RoleStatus, noIncludedUsage); !ok {
		t.Fatalf("no empty message in %q", tt.Texts())
	}

	// A failed excluded read leaves it unknown whether usage was saved.
	f = loaded()
	f.backend.usage.Summary = analytics.ViewerSummaryRow{}
	f.backend.excludedErr = errors.New("excluded unavailable")
	tt = f.synced(windowWidth, windowHeight, false)
	assertTexts(t, tt, noIncludedUsage)

	// Reviewing none says none was excluded.
	f = loaded()
	tt = f.synced(windowWidth, windowHeight, false)
	f.window.SetQuality(dashboardview.QualityEstimated)
	f.sync(tt)
	assertTexts(t, tt, noExcludedUsage)
}

func TestEmptyTableOffersTheReviewAndTheWayBack(t *testing.T) {
	f := withExcluded()
	f.backend.rows = nil
	tt := f.synced(windowWidth, windowHeight, false)
	assertTexts(t, tt, noMatches)
	if tt.HasText(clearFiltersLabel) {
		t.Fatal("Clear filters without filters")
	}
	// The notice and the empty state each offer the review.
	reviews := 0
	for _, a := range tt.AccessAll() {
		if a.Role == ui.RoleButton && a.Name == reviewExcluded {
			reviews++
		}
	}
	if reviews != 2 {
		t.Fatalf("%d review buttons", reviews)
	}

	f.window.SetQuality(dashboardview.QualityEstimated)
	f.backend.excludedRows = nil
	f.sync(tt)
	assertTexts(t, tt, noExcludedMatches, excludedScope+widerRange+".")
	backs := 0
	for _, a := range tt.AccessAll() {
		if a.Role == ui.RoleButton && a.Name == backToUsage {
			backs++
		}
	}
	if backs != 2 {
		t.Fatalf("%d back buttons", backs)
	}

	// Without excluded usage the empty state offers no review.
	f = loaded()
	f.backend.rows = nil
	tt = f.synced(windowWidth, windowHeight, false)
	assertTexts(t, tt, noMatches)
	if tt.HasText(reviewExcluded) {
		t.Fatal("review offered without excluded usage")
	}
}

// TestExcludedImages renders the excluded usage notice, its review and an
// empty table offering the review, in both appearances, and writes PNGs
// when TOKENINSIGHTS_NATIVE_GALLERY_DIR is set.
func TestExcludedImages(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, shot := range []struct {
		name    string
		prepare func(t *testing.T, f *fixture, tt *ui.Tester)
		text    string
	}{
		{"notice", nil, excludedTotalText},
		{"review", func(t *testing.T, f *fixture, tt *ui.Tester) {
			click(t, tt, reviewExcluded)
			f.sync(tt)
		}, excludedReviewScope},
		{"empty", func(t *testing.T, f *fixture, tt *ui.Tester) {
			f.backend.rows = nil
			f.backend.usage.Summary = analytics.ViewerSummaryRow{SyncedSessions: 5}
			f.window.UpdateSelection(dashboardview.CustomRangeChange("2026-11-01", "2026-11-30"))
			f.sync(tt)
			r, _ := tt.Find("Total tokens")
			tt.Scroll(r.X, r.Y, 0, pageEnd)
			tt.Frame()
		}, noMatches},
		{"error", func(t *testing.T, f *fixture, tt *ui.Tester) {
			f.backend.excludedErr = errors.New("excluded unavailable")
			f.window.UpdateSelection(dashboardview.CustomRangeChange("2026-09-01", "2026-09-30"))
			f.sync(tt)
		}, excludedFailed},
	} {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(shot.name+"/"+appearance, func(t *testing.T) {
				f := withExcluded()
				tt := f.synced(windowWidth, windowHeight, dark)
				if shot.prepare != nil {
					shot.prepare(t, f, tt)
				}
				assertTexts(t, tt, shot.text)
				tt.Advance(settle)
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, "excluded-"+shot.name+"-"+appearance+".png"), tt)
			})
		}
	}
}
