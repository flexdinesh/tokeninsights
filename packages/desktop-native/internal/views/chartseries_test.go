package views

import (
	"math"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/chart"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// charWidth measures text as this many DIPs a character.
const charWidth = 8

func measureChars(text string) float32 { return float32(utf8.RuneCountInString(text)) * charWidth }

func tickLabels(ticks []chart.Tick) []string {
	labels := make([]string, len(ticks))
	for i, tick := range ticks {
		labels[i] = tick.Label
	}
	return labels
}

func TestTimelineSeriesPlotsTheMetricOverEvenPoints(t *testing.T) {
	rows := mockRows()[dashboardview.TabTokens]
	s := newChartSeries(chartInput{rows: rows, tab: dashboardview.TabSessions, metric: dashboardview.SortInput, timezone: "Europe/London"}, measureChars)
	if !s.timeline || s.shares || s.context || len(s.ys) != 1 {
		t.Fatalf("kind: timeline %v shares %v context %v series %d", s.timeline, s.shares, s.context, len(s.ys))
	}
	if !slices.Equal(s.xs, []float32{0, 0.5, 1}) {
		t.Errorf("xs %v", s.xs)
	}
	// Input peaks at 145,000: the axis rounds out to 150K in steps of 50K.
	if got := tickLabels(s.yTicks); !slices.Equal(got, []string{"0", "50K", "100K", "150K"}) {
		t.Errorf("y ticks %q", got)
	}
	if s.ys[0][1] != s.y.At(145_000) || s.ys[0][0] != s.y.At(126_000) {
		t.Errorf("heights %v", s.ys[0])
	}
	if s.yAxisWidth != measureChars("150K") {
		t.Errorf("y axis width %v", s.yAxisWidth)
	}
	if !slices.Equal(s.ticks, []string{"2026-09-12", "2026-09-13", "2026-09-14"}) {
		t.Errorf("ticks %q", s.ticks)
	}
	if s.tipTitles[1] != "2026-09-13 · Sep 13, 2026, 11:00 · Europe/London" || !slices.Equal(s.tipLines[1], []string{"Input: 145,000"}) {
		t.Errorf("tooltip %q %q", s.tipTitles[1], s.tipLines[1])
	}
	if s.descriptions[1] != "2026-09-13 · Sep 13, 2026, 11:00 · Europe/London: Input: 145,000" {
		t.Errorf("description %q", s.descriptions[1])
	}
}

func TestTimelineOfOneBucketCentersItAndUndatedBucketsHaveNoTime(t *testing.T) {
	row := analytics.Row{Name: "2026-W37", Total: 0}
	s := newChartSeries(chartInput{rows: []analytics.Row{row}, tab: dashboardview.TabTokens, metric: dashboardview.SortTotal}, measureChars)
	if !slices.Equal(s.xs, []float32{chartLonePoint}) || s.tipTitles[0] != "2026-W37" || s.tipLines[0][0] != "Total: 0" {
		t.Fatalf("lone bucket %v %q %q", s.xs, s.tipTitles[0], s.tipLines[0])
	}
	if s.tickAnchor(0) != anchorMiddle {
		t.Error("a lone label anchors at its start or end")
	}
	// All-zero usage keeps a domain to scale.
	if s.ys[0][0] != 0 || len(s.yTicks) == 0 {
		t.Errorf("zero chart: heights %v ticks %v", s.ys[0], s.yTicks)
	}
}

func TestShareSeriesBandsTruncatesAndShares(t *testing.T) {
	rows := []analytics.Row{
		usageRow("gpt-5", "gpt-5", 2, 311_000), usageRow("long", "claude-sonnet-4-5-20250929", 1, 205_000),
		usageRow("tiny", "tiny", 1, 100),
	}
	s := newChartSeries(chartInput{rows: rows, tab: dashboardview.TabModels, totalTokens: 647_000}, measureChars)
	if s.timeline || !s.shares || s.context {
		t.Fatal("models chart is not a share chart")
	}
	for i := range rows {
		if s.xs[i] != s.band.At(i) {
			t.Errorf("x %d = %v, want band middle %v", i, s.xs[i], s.band.At(i))
		}
	}
	// A fifth of a step between bars, a tenth outside the first and last.
	if step := s.band.Band(1).Lo - s.band.Band(0).Lo; !near(s.band.Band(0).Lo, step*chartBarOuter) || !near(s.band.Width(), step*(1-chartBarInner)) {
		t.Errorf("band %v step %v", s.band.Band(0), step)
	}
	if !slices.Equal(s.ticks, []string{"gpt-5", "claude-sonnet-4…", "tiny"}) || s.tipTitles[1] != "claude-sonnet-4-5-20250929" {
		t.Errorf("ticks %q title %q", s.ticks, s.tipTitles[1])
	}
	if !slices.Equal(s.shareTexts, []string{"48.1%", "31.7%", "<0.1%"}) {
		t.Errorf("shares %q", s.shareTexts)
	}
	if !slices.Equal(s.tipLines[0], []string{"Total tokens: 311,000 (48.1%)"}) || !slices.Equal(s.names, []string{"gpt-5", "claude-sonnet-4-5-20250929", "tiny"}) {
		t.Errorf("tooltip %q names %q", s.tipLines[0], s.names)
	}
}

// near reports whether two fractions agree to float32 rounding.
func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-6 }

func TestContextSeriesPlotsAverageMedianAndMaximum(t *testing.T) {
	rows := mockRows()[dashboardview.TabContext]
	s := newChartSeries(chartInput{rows: rows, tab: dashboardview.TabContext}, measureChars)
	if !s.context || s.shares || len(s.ys) != len(dashboardview.ContextSeries()) {
		t.Fatalf("context: shares %v series %d", s.shares, len(s.ys))
	}
	// The axis scales to the highest maximum, 184,000.
	if s.ys[2][0] != s.y.At(184_000) || s.y.Hi < 184_000 {
		t.Errorf("maximum %v of %v", s.ys[2][0], s.y)
	}
	if s.tipTitles[0] != "gpt-5 · codex · openai" ||
		!slices.Equal(s.tipLines[0], []string{"Average: 118,000", "Median: 121,000", "Maximum: 184,000"}) {
		t.Errorf("tooltip %q %q", s.tipTitles[0], s.tipLines[0])
	}
}

func TestVisibleTicksKeepTheLastAndDropOverlaps(t *testing.T) {
	rows := make([]analytics.Row, 5)
	for i := range rows {
		rows[i] = analytics.Row{Name: "2026-09-1" + string(rune('0'+i))}
	}
	s := newChartSeries(chartInput{rows: rows, tab: dashboardview.TabTokens, metric: dashboardview.SortTotal}, measureChars)
	label := measureChars(rows[0].Name)
	// Wide enough for every label and gap.
	if got := s.visibleTicks(10*label, chartTickGap, nil); len(got) != len(rows) || got[0].Value != 0 || got[4].Value != 4 {
		t.Errorf("wide ticks %v", got)
	}
	// Points 1.25 labels apart: every other label, ending with the last.
	got := s.visibleTicks(5*label, chartTickGap, nil)
	if labels := tickLabels(got); !slices.Equal(labels, []string{rows[0].Name, rows[2].Name, rows[4].Name}) {
		t.Errorf("narrow ticks %q", labels)
	}
	// Far too narrow: the last label alone.
	if got := s.visibleTicks(label, chartTickGap, got); len(got) != 1 || got[0].Value != 4 {
		t.Errorf("crowded ticks %v", got)
	}
	if s.tickAnchor(0) != anchorStart || s.tickAnchor(4) != anchorEnd || s.tickAnchor(2) != anchorMiddle {
		t.Error("timeline anchors")
	}
}
