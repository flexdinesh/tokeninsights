package views

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// hoverChart rests the pointer on the plot named title at fraction x of
// its width, half way up.
func hoverChart(t *testing.T, tt *ui.Tester, title string, x float32) {
	t.Helper()
	plot := chartPlot(t, tt, title)
	tt.Move(plot.Bounds.X+x*plot.Bounds.W, plot.Bounds.Y+plot.Bounds.H/2)
}

// chartPlot is the plot area named title.
func chartPlot(t *testing.T, tt *ui.Tester, title string) ui.AccessInfo {
	t.Helper()
	for _, a := range tt.AccessAll() {
		if a.Name == title && a.Role != ui.RoleRegion && a.Role != ui.RoleHeading {
			return a
		}
	}
	t.Fatalf("no %q plot", title)
	return ui.AccessInfo{}
}

// clickAccess clicks the middle of the element of role named name.
func clickAccess(t *testing.T, tt *ui.Tester, role ui.Role, name string) {
	t.Helper()
	a, ok := tt.AccessByRole(role, name)
	if !ok {
		t.Fatalf("no %v %q", role, name)
	}
	tt.ClickAt(a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2)
}

func TestTimelineHoverShowsTheBucketsExactValue(t *testing.T) {
	tt := loaded().onTab(t, dashboardview.TabTokens, false)
	if tt.HasText("Total: 257,000") {
		t.Fatal("a tooltip shows before hover")
	}
	hoverChart(t, tt, "Usage over time", 0.5)
	assertTexts(t, tt, "2026-09-13 · Sep 13, 2026, 11:00 · Europe/London", "Total: 257,000")
	if _, ok := tt.AccessByRole(ui.RoleTooltip, ""); !ok {
		t.Fatal("no tooltip role")
	}
	hoverChart(t, tt, "Usage over time", 0.05)
	assertTexts(t, tt, "Total: 220,000")
	tt.Move(2, 2)
	if tt.HasText("Total: 220,000") {
		t.Fatal("the tooltip stays after the pointer leaves")
	}
}

func TestTimelineSelectsFromTheKeyboard(t *testing.T) {
	tt := loaded().onTab(t, dashboardview.TabTokens, false)
	plot := chartPlot(t, tt, "Usage over time")
	tt.ClickAt(plot.Bounds.X+2, plot.Bounds.Y+plot.Bounds.H/2)
	tt.Move(2, 2)
	tt.Key(0, ui.KeyEnd)
	assertTexts(t, tt, "Total: 170,000")
	tt.Key(0, ui.KeyLeft)
	assertTexts(t, tt, "Total: 257,000")
	if a := chartPlot(t, tt, "Usage over time"); a.Value != "2026-09-13 · Sep 13, 2026, 11:00 · Europe/London: Total: 257,000" {
		t.Fatalf("plot value %q", a.Value)
	}
	tt.Key(0, ui.KeyEscape)
	if tt.HasText("Total: 257,000") {
		t.Fatal("Escape kept the selection")
	}
}

func TestMetricChoiceRedrawsWithoutReading(t *testing.T) {
	f := loaded()
	tt := f.onTab(t, dashboardview.TabTokens, false)
	reads := len(f.backend.requests)
	clickAccess(t, tt, ui.RoleRadio, "Cache read")
	f.sync(tt)
	if len(f.backend.requests) != reads {
		t.Fatalf("a metric read usage: %+v", f.backend.requests[reads:])
	}
	if a, ok := tt.AccessByRole(ui.RoleRadio, "Cache read"); !ok || !a.Checked {
		t.Fatalf("cache read not chosen: %+v", a)
	}
	hoverChart(t, tt, "Usage over time", 0.5)
	assertTexts(t, tt, "Cache read: 71,000")
	// The choice stays with the window across tabs.
	if err := tt.Click("Models"); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if err := tt.Click("Tokens"); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if a, ok := tt.AccessByRole(ui.RoleRadio, "Cache read"); !ok || !a.Checked {
		t.Fatalf("metric not kept: %+v", a)
	}
}

func TestBucketSelectReadsTheBucket(t *testing.T) {
	f := loaded()
	tt := f.onTab(t, dashboardview.TabTokens, false)
	assertTexts(t, tt, bucketLabel, "Day")
	if err := tt.Click(timeBucketLabel); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Week"); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if got := f.lastRequest(); got.Selection.Bucket != "week" || got.Page != 1 {
		t.Fatalf("bucket request %+v", got)
	}
	if q, _ := f.window.Query(); q.Bucket != "week" {
		t.Fatalf("query bucket %q", q.Bucket)
	}
	assertTexts(t, tt, "Week")
}

func TestShareBarsLabelSharesAndLegendFilters(t *testing.T) {
	f := loaded()
	tt := f.onTab(t, dashboardview.TabModels, false)
	assertTexts(t, tt, "Usage by model", dashboardview.ChartDescription(dashboardview.TabModels), "Countable tokens", "48.1%", "31.7%", "20.2%")
	if tt.HasText(chartMetricLabel) || tt.HasText(bucketLabel) {
		t.Fatal("category chart shows timeline controls")
	}
	legend, ok := tt.AccessByRole(ui.RoleGroup, chartLegendLabel)
	if !ok {
		t.Fatalf("no legend: %+v", legend)
	}
	item, ok := tt.AccessByRole(ui.RoleButton, "claude-sonnet-4-5 31.7%")
	if !ok || item.Description != filterPrefix+"claude-sonnet-4-5" {
		t.Fatalf("legend item %+v", item)
	}
	clickAccess(t, tt, ui.RoleButton, "claude-sonnet-4-5 31.7%")
	f.sync(tt)
	if got := f.lastRequest(); !slices.Equal(got.Selection.Models, []string{"claude-sonnet-4-5"}) || got.Tab != string(dashboardview.TabModels) {
		t.Fatalf("legend filter %+v", got)
	}
	assertTexts(t, tt, "Remove model claude-sonnet-4-5")
}

func TestLongCategoryLabelsShortenWithTheWholeOnHover(t *testing.T) {
	f := loaded()
	f.backend.rows[dashboardview.TabProviders] = []analytics.Row{usageRow("amazon-bedrock-anthropic", "amazon-bedrock-anthropic", 1, 647_000)}
	tt := f.onTab(t, dashboardview.TabProviders, false)
	assertTexts(t, tt, "amazon-bedrock-…", "100%")
	if a, ok := tt.Access("amazon-bedrock-…"); !ok || a.Description != "amazon-bedrock-anthropic" {
		t.Fatalf("shortened label %+v", a)
	}
}

func TestContextChartShowsItsLegendAndFootnote(t *testing.T) {
	tt := loaded().onTab(t, dashboardview.TabContext, false)
	assertTexts(t, tt, "Session peak context", "Prompt-side tokens", dashboardview.ContextFootnote)
	if _, ok := tt.AccessByRole(ui.RoleList, contextLegendLabel); !ok {
		t.Fatal("no context legend")
	}
	for _, series := range dashboardview.ContextSeries() {
		if _, ok := tt.AccessByRole(ui.RoleListItem, series.Label); !ok {
			t.Errorf("no %s legend item", series.Label)
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleGroup, chartLegendLabel); ok {
		t.Fatal("context offers filter links")
	}
	hoverChart(t, tt, "Session peak context", 0.1)
	assertTexts(t, tt, "gpt-5 · codex · openai", "Average: 118,000", "Median: 121,000", "Maximum: 184,000")
}

func TestEmptyChartSaysNoUsageInRange(t *testing.T) {
	f := loaded()
	f.backend.charts = map[dashboardview.Tab][]analytics.Row{dashboardview.TabModels: {}}
	tt := f.onTab(t, dashboardview.TabModels, false)
	assertTexts(t, tt, "Usage by model", dashboardview.ChartEmptyLabel(dashboardview.QualityConfirmed))
	if _, ok := tt.AccessByRole(ui.RoleGroup, chartLegendLabel); ok {
		t.Fatal("an empty chart shows a legend")
	}
}

// TestChartImages renders the Tokens timeline at rest and hovered, the
// Models share bars, the Context chart and an empty chart in both
// appearances, and writes PNGs when TOKENINSIGHTS_NATIVE_GALLERY_DIR is
// set.
func TestChartImages(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, shot := range []struct {
		name    string
		tab     dashboardview.Tab
		prepare func(t *testing.T, f *fixture, tt *ui.Tester)
		texts   []string
	}{
		{"tokens", dashboardview.TabTokens, nil, []string{"Usage over time", "2026-09-12", "2026-09-14", "Bucket"}},
		{"tokens-hover", dashboardview.TabTokens, func(t *testing.T, _ *fixture, tt *ui.Tester) {
			hoverChart(t, tt, "Usage over time", 0.5)
		}, []string{"2026-09-13 · Sep 13, 2026, 11:00 · Europe/London", "Total: 257,000"}},
		{"models", dashboardview.TabModels, nil, []string{"Usage by model", "48.1%", "gpt-5", "Countable tokens"}},
		{"models-hover", dashboardview.TabModels, func(t *testing.T, _ *fixture, tt *ui.Tester) {
			hoverChart(t, tt, "Usage by model", 0.5)
		}, []string{"Total tokens: 205,000 (31.7%)"}},
		{"context", dashboardview.TabContext, nil, []string{"Session peak context", "Average", "Median", "Maximum", dashboardview.ContextFootnote}},
		{"empty", dashboardview.TabTokens, func(t *testing.T, f *fixture, tt *ui.Tester) {
			f.backend.charts = map[dashboardview.Tab][]analytics.Row{dashboardview.TabTokens: {}}
			f.window.RetryUsage()
			f.sync(tt)
		}, []string{"No usage in this range"}},
	} {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(shot.name+"/"+appearance, func(t *testing.T) {
				f := loaded()
				tt := f.onTab(t, shot.tab, dark)
				if shot.prepare != nil {
					shot.prepare(t, f, tt)
				}
				tt.Advance(settle)
				assertTexts(t, tt, shot.texts...)
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, "chart-"+shot.name+"-"+appearance+".png"), tt)
			})
		}
	}
}
