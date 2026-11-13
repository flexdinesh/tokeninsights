package views

import (
	"strings"

	"github.com/egoist/mygo/ui/chart"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// Chart scale and text rules, as the web's UsageChart.
const (
	// chartYTicks is about how many round values the Y axis marks.
	chartYTicks = 4
	// chartBarInner and chartBarOuter pad category bands as the web's
	// 10% bar category gap on each side of a bar: a fifth of a band's
	// step between bars and a tenth before the first and after the last.
	chartBarInner float32 = 0.2
	chartBarOuter float32 = 0.1
	// chartLonePoint is where a timeline of one bucket puts it: the
	// middle of the plot.
	chartLonePoint float32 = 0.5
	// chartTotalSeries names the bars of a category chart.
	chartTotalSeries = "Total tokens"
	tipSeparator     = " · "
	valueSeparator   = ": "
)

// chartInput is what a usage chart shows: the chart rows of a usage
// answer for tab, the timeline metric, the filtered total the shares
// divide and the server timezone dates show in.
type chartInput struct {
	rows        []analytics.Row
	tab         dashboardview.Tab
	metric      dashboardview.Sort
	totalTokens int64
	timezone    string
}

// chartSeries is a usage chart's geometry and texts, prepared once per
// usage answer and metric rather than as the chart builds or draws: the
// rows' axis labels and their widths, the selection points, each series'
// heights, the Y scale and its ticks, and the tooltip of each row.
type chartSeries struct {
	timeline, shares, context bool
	// names are the rows' names, the values legend items filter by.
	names []string
	// ticks are the X axis labels, category labels truncated as the
	// web's, and tickWidths their measured widths.
	ticks      []string
	tickWidths []float32
	// xs are the rows' points along the plot: the timeline's points or
	// the middles of the category bands.
	xs []float32
	// ys are each series' heights of the rows: the timeline metric, the
	// category totals, or Context's average, median and maximum.
	ys [][]float32
	// band places category bars; cursor spans each row's whole step.
	band, cursor chart.Band
	y            chart.Linear
	yTicks       []chart.Tick
	// yAxisWidth is the widest Y axis label.
	yAxisWidth float32
	// shareTexts are the rows' shares of the filtered total.
	shareTexts []string
	// tipTitles and tipLines are the rows' tooltips; descriptions say
	// the same to assistive technology in one line.
	tipTitles    []string
	tipLines     [][]string
	descriptions []string
}

// newChartSeries prepares the chart of in, measuring axis labels with
// measure.
func newChartSeries(in chartInput, measure func(string) float32) chartSeries {
	n := len(in.rows)
	s := chartSeries{
		timeline: dashboardview.ChartIsTimeline(in.tab), shares: dashboardview.ChartShowsTokenShares(in.tab),
		context: in.tab == dashboardview.TabContext,
		names:   make([]string, n), ticks: make([]string, n), tickWidths: make([]float32, n), xs: make([]float32, n),
		shareTexts: make([]string, n), tipTitles: make([]string, n), tipLines: make([][]string, n), descriptions: make([]string, n),
	}
	series := s.seriesOf(in.metric)
	values := make([][]int64, len(series))
	high := int64(0)
	for k, metric := range series {
		values[k] = make([]int64, n)
		for i, row := range in.rows {
			value, _ := dashboardview.RowValue(row, metric.Key)
			values[k][i] = value
			high = max(high, value)
		}
	}
	labels := make([]string, n)
	for i, row := range in.rows {
		labels[i] = dashboardview.ChartRowLabel(row, in.tab)
		s.names[i] = row.Name
		s.ticks[i] = labels[i]
		if !s.timeline {
			s.ticks[i] = dashboardview.TruncateChartLabel(labels[i])
		}
		s.tickWidths[i] = measure(s.ticks[i])
		s.shareTexts[i] = dashboardview.TokenShare(row.Total, in.totalTokens)
	}
	s.band = chart.NewBand(labels...).Padding(chartBarInner, chartBarOuter)
	s.cursor = chart.NewBand(labels...)
	for i := range n {
		switch {
		case !s.timeline:
			s.xs[i] = s.band.At(i)
		case n == 1:
			s.xs[i] = chartLonePoint
		default:
			s.xs[i] = float32(i) / float32(n-1)
		}
	}
	// An empty or all-zero chart still spans one token, as a scale needs
	// a domain.
	s.y = chart.NewLinear(0, float64(max(high, 1))).Nice(chartYTicks)
	s.y.Format = func(v float64) string { return dashboardview.CompactCount(int64(v)) }
	s.yTicks = s.y.Ticks(chartYTicks)
	for _, tick := range s.yTicks {
		s.yAxisWidth = max(s.yAxisWidth, measure(tick.Label))
	}
	s.ys = make([][]float32, len(series))
	for k := range series {
		s.ys[k] = make([]float32, n)
		for i, value := range values[k] {
			s.ys[k][i] = s.y.At(float64(value))
		}
	}
	for i, row := range in.rows {
		s.tipTitles[i] = labels[i]
		if s.timeline && row.Date > 0 {
			s.tipTitles[i] += tipSeparator + dashboardview.FormatServerDateTime(row.Date, in.timezone) +
				tipSeparator + dashboardview.ServerZoneLabel(in.timezone)
		}
		lines := make([]string, len(series))
		for k, metric := range series {
			lines[k] = metric.Label + valueSeparator + dashboardview.ExactCount(values[k][i])
			if s.shares {
				lines[k] += " (" + s.shareTexts[i] + ")"
			}
		}
		s.tipLines[i] = lines
		s.descriptions[i] = s.tipTitles[i] + valueSeparator + strings.Join(lines, ", ")
	}
	return s
}

// seriesOf is what the chart plots: the timeline metric, Context's
// average, median and maximum, or the category totals.
func (s chartSeries) seriesOf(metric dashboardview.Sort) []dashboardview.ChartMetric {
	switch {
	case s.timeline:
		for _, m := range dashboardview.ChartMetrics() {
			if m.Key == metric {
				return []dashboardview.ChartMetric{m}
			}
		}
		return dashboardview.ChartMetrics()[:1]
	case s.context:
		return dashboardview.ContextSeries()
	}
	return []dashboardview.ChartMetric{{Key: dashboardview.SortTotal, Label: chartTotalSeries}}
}

// visibleTicks are the X axis labels that fit a plot width wide, keeping
// gap between neighbors, as the web's preserveEnd interval: the last
// label always shows, and each earlier one shows when it ends gap before
// the last shown starts. A timeline's first label starts at its point and
// its last ends at its point; other labels center on theirs. Each tick's
// Value is its row. It appends to dst, which it reuses.
func (s chartSeries) visibleTicks(width, gap float32, dst []chart.Tick) []chart.Tick {
	dst = dst[:0]
	n := len(s.xs)
	start := float32(0)
	for i := n - 1; i >= 0; i-- {
		from, to := s.tickSpan(i, width)
		if i < n-1 && to+gap > start {
			continue
		}
		dst = append(dst, chart.Tick{At: s.xs[i], Value: float64(i), Label: s.ticks[i]})
		start = from
	}
	// Ticks were collected from the end; the axis wants them in order.
	for i, j := 0, len(dst)-1; i < j; i, j = i+1, j-1 {
		dst[i], dst[j] = dst[j], dst[i]
	}
	return dst
}

// tickSpan is where row i's X axis label goes along a plot width wide.
func (s chartSeries) tickSpan(i int, width float32) (from, to float32) {
	at, w := s.xs[i]*width, s.tickWidths[i]
	switch s.tickAnchor(i) {
	case anchorStart:
		return at, at + w
	case anchorEnd:
		return at - w, at
	}
	return at - w/2, at + w/2
}

// tickAnchor is how row i's X axis label sits on its point, as the web's
// TimelineTick: a timeline's first label starts there and its last ends
// there, when it has more than one.
type tickAnchor int

const (
	anchorMiddle tickAnchor = iota
	anchorStart
	anchorEnd
)

func (s chartSeries) tickAnchor(i int) tickAnchor {
	n := len(s.xs)
	switch {
	case !s.timeline || n <= 1:
		return anchorMiddle
	case i == 0:
		return anchorStart
	case i == n-1:
		return anchorEnd
	}
	return anchorMiddle
}
