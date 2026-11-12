package views

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/chart"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Usage chart texts, as the web's UsageChart.
const (
	chartMetricLabel   = "Chart metric"
	bucketLabel        = "Bucket"
	timeBucketLabel    = "Time bucket"
	chartLegendLabel   = "Filter by chart item"
	contextLegendLabel = "Context chart legend"
	filterPrefix       = "Filter "
)

// Usage chart geometry, as the web's UsageChart; the plot's height is
// theme.ChartHeight.
const (
	// chartPlotTop is the room above the plot; chartPlotTopShares leaves
	// shares room above the tallest bar.
	chartPlotTop       float32 = 12
	chartPlotTopShares float32 = 28
	// chartPlotEnd is the room after the plot, chartContainerEnd the
	// chart's own end padding.
	chartPlotEnd      float32 = 12
	chartContainerEnd         = theme.Unit
	// chartAxisGap separates axis labels from the plot.
	chartAxisGap float32 = 8
	// chartAxisHeight is the X axis's row under the plot, room for a line
	// of labels at enlarged text sizes.
	chartAxisHeight float32 = 48
	// chartTickGap is the least room between X axis labels.
	chartTickGap float32 = 40
	// chartLineWidth strokes the timeline; chartRuleWidth its selection's
	// rule; chartDotSize and chartDotRing its points.
	chartLineWidth float32 = 2
	chartRuleWidth float32 = 1
	chartDotSize   float32 = 10
	chartDotRing   float32 = 2
	// chartBarMax caps a bar's width; chartBarRadius rounds a share
	// bar's top corners; chartBarGap separates Context's bars of a row.
	chartBarMax    float32 = 64
	chartBarRadius float32 = 4
	chartBarGap    float32 = 4
	// chartShareGap lifts a share above its bar.
	chartShareGap float32 = 4
	// chartCategoryWidth is the least width of each category of a share
	// chart, and chartAxisAllowance the room of its Y axis: a chart
	// narrower than both scrolls sideways.
	chartCategoryWidth float32 = 56
	chartAxisAllowance float32 = 64
	// chartTipOffset keeps a tooltip clear of the point it describes.
	chartTipOffset float32 = 12
	// chartPanelPadding is the inline room of the legends and footnote.
	chartPanelPadding = theme.Unit * 4
)

// chartState is a window's usage chart state, kept by its results region:
// the timeline metric (per window, in memory, ADR 0032), the row the
// pointer or keyboard selects, and the chart prepared for the usage shown,
// with the X axis labels that fit its width and the timeline's path.
type chartState struct {
	metric   dashboardview.Sort
	selected int
	key      chartKey
	prepared bool
	series   chartSeries
	ticks    []chart.Tick
	tickFor  float32
	path     ui.Path
}

func newChartState() chartState {
	return chartState{metric: dashboardview.SortTotal, selected: -1}
}

// chartKey is what a prepared chart shows: a usage answer of a request
// and snapshot, the metric, and the timezone of its dates.
type chartKey struct {
	request              analytics.Query
	databaseID, timezone string
	revision, generation int64
	metric               dashboardview.Sort
}

func (k chartKey) equal(other chartKey) bool {
	return k.databaseID == other.databaseID && k.timezone == other.timezone && k.revision == other.revision &&
		k.generation == other.generation && k.metric == other.metric && model.SameRequest(k.request, other.request)
}

// prepare prepares the chart of usage unless it shows already, measuring
// axis labels in c; a new chart drops the selection.
func (s *chartState) prepare(c *ui.Context, usage model.Usage, timezone string) {
	key := chartKey{
		request: usage.Request, databaseID: usage.DatabaseID, timezone: timezone,
		revision: usage.Revision, generation: usage.Generation, metric: s.metric,
	}
	if s.prepared && s.key.equal(key) {
		return
	}
	measure := func(text string) float32 {
		w, _ := c.MeasureText(0, ui.Span{Text: text, Font: theme.Font, Size: theme.SizeLabel})
		return w
	}
	s.series = newChartSeries(chartInput{
		rows: usage.Chart, tab: usage.Tab, metric: s.metric, totalTokens: usage.Summary.TotalTokens, timezone: timezone,
	}, measure)
	s.key, s.prepared, s.selected, s.tickFor = key, true, -1, -1
}

// axisTicks are the X axis labels that fit the plot as wide as the window
// gives it, chosen again only as that width changes.
func (s *chartState) axisTicks(c *ui.Context) []chart.Tick {
	width := s.plotWidth(c)
	if width != s.tickFor {
		s.ticks, s.tickFor = s.series.visibleTicks(width, chartTickGap, s.ticks), width
	}
	return s.ticks
}

// plotWidth is the width of the plot: the page's content beside the
// sidebar, as wide as a share chart's categories need, less the Y axis
// and the room after the plot.
func (s *chartState) plotWidth(c *ui.Context) float32 {
	width, _ := c.Size()
	content := min(width-theme.SidebarWidth, theme.ContentMax) - 2*theme.Gutter
	return max(content, s.minWidth()) - chartContainerEnd - chartPlotEnd - s.series.yAxisWidth - chartAxisGap
}

// minWidth is the least width of a share chart, whose percentages need
// room; other charts take any width.
func (s *chartState) minWidth() float32 {
	if !s.series.shares {
		return 0
	}
	return float32(len(s.series.xs))*chartCategoryWidth + chartAxisAllowance
}

// usageChart is the web's UsageChart under the readouts: the section
// header with the timeline's metric and bucket controls or the category
// chart's unit, the chart of the usage's chart rows or the empty label,
// then the legend that filters by a category, or Context's legend and
// footnote.
func usageChart(c *ui.Context, w *model.Window, s *chartState) {
	usage, _ := w.Usage()
	query, _ := w.Query()
	instance, _ := w.Instance()
	t := theme.Current(c)
	title := dashboardview.ChartTitle(query)
	timeline := dashboardview.ChartIsTimeline(query.Tab)
	section := ui.Column(c).MarginX(-theme.Gutter).Padding(theme.Unit*2, theme.Gutter).
		BorderWidth(0, 0, theme.Rule, 0).BorderColor(t.Line).Role(ui.RoleRegion).Label(title)
	section.Children(func() {
		header := kit.Section{Title: title, Description: dashboardview.ChartDescription(query.Tab), HideDescription: timeline}
		kit.SectionHeader(c, header, func() {
			if timeline {
				timelineControls(c, w, s, query)
				return
			}
			theme.Label(ui.Text(c, dashboardview.ChartUnitLabel(query))).TextColor(t.InkSubtle)
		}).Padding(0, 0, theme.Unit*2, 0)
		if len(usage.Chart) == 0 {
			emptyChart(c, query.Quality)
			return
		}
		s.prepare(c, usage, instance.Timezone)
		plotChart(c, s, title)
		if dimension, ok := dashboardview.ChartFilterDimension(query.Tab); ok {
			chartLegend(c, w, s, dimension)
		}
		if s.series.context {
			contextLegend(c)
		}
	})
}

// timelineControls are the timeline's metric choice and its bucket
// select, as the web's: a metric shows at once, a bucket reads usage.
func timelineControls(c *ui.Context, w *model.Window, s *chartState, query dashboardview.Query) {
	metrics := []kit.Option[dashboardview.Sort]{}
	for _, m := range dashboardview.ChartMetrics() {
		metrics = append(metrics, kit.Option[dashboardview.Sort]{Value: m.Key, Label: m.Label})
	}
	kit.Segmented(c.Key("metric"), chartMetricLabel, &s.metric, metrics, kit.ButtonStyle{Variant: kit.Quiet, Size: kit.SizeCompact})
	buckets := []kit.Option[string]{}
	for _, b := range dashboardview.BucketOptions() {
		buckets = append(buckets, kit.Option[string]{Value: b.Key, Label: b.Label})
	}
	kit.ControlLabel(c, bucketLabel, func() {
		bucket := query.Bucket
		if kit.Select(c.Key("bucket"), &bucket, buckets, kit.SizeCompact).Label(timeBucketLabel).Changed() && bucket != query.Bucket {
			w.UpdateSelection(dashboardview.SelectionChange{Bucket: &bucket})
		}
	})
}

// emptyChart holds the chart's place with the web's empty label.
func emptyChart(c *ui.Context, quality dashboardview.Quality) {
	t := theme.Current(c)
	box := ui.Box(c).MinHeight(theme.ChartHeight).Padding(theme.Unit * 4).Center()
	box.Children(func() {
		theme.Body(ui.Text(c, dashboardview.ChartEmptyLabel(quality))).TextColor(t.InkSubtle)
	})
}

// plotChart builds the plot, theme.ChartHeight high with its axes, named
// title: the timeline or the bars, scrolling sideways when a share
// chart's categories need more room than the page gives.
func plotChart(c *ui.Context, s *chartState, title string) {
	top := chartPlotTop
	if s.series.shares {
		top = chartPlotTopShares
	}
	ticks := s.axisTicks(c)
	build := func() {
		box := ui.Column(c).Height(theme.ChartHeight).Padding(top, chartContainerEnd+chartPlotEnd, 0, 0).FontFeatures("tnum")
		box.Children(func() {
			axes := chart.Axes{Left: s.series.yTicks, Bottom: ticks, Label: func(side chart.Side, tick chart.Tick) { axisLabel(c, s, side, tick) }}
			chart.Plot(c, axes, func(area ui.Element) {
				area.Label(title)
				if s.series.timeline {
					timelinePlot(c, s, area)
					return
				}
				barsPlot(c, s, area)
			}).Grow(1).GapX(chartAxisGap)
		})
		if s.minWidth() > 0 {
			box.MinWidth(s.minWidth())
		}
	}
	width, _ := c.Size()
	if s.minWidth() > min(width-theme.SidebarWidth, theme.ContentMax)-2*theme.Gutter {
		ui.ScrollHorizontal(c).Children(build)
		return
	}
	build()
}

// axisLabel is one axis label in label type and subtle ink: the Y axis's
// compact counts, the X axis's dates or category names in a row as high
// as the web's axis. A timeline's first and last dates start and end at
// their points; a shortened category name shows the whole on hover.
func axisLabel(c *ui.Context, s *chartState, side chart.Side, tick chart.Tick) {
	t := theme.Current(c)
	if side != chart.Bottom {
		theme.Label(ui.Text(c, tick.Label).NoWrap()).TextColor(t.InkSubtle)
		return
	}
	row := int(tick.Value)
	// The slot centers the label on its point; a margin as wide as the
	// label moves it to start or end there.
	slot := ui.Column(c).Width(0).Height(chartAxisHeight - chartAxisGap).AlignItems(ui.Center)
	slot.Children(func() {
		label := theme.Label(ui.Text(c.Key(row), tick.Label).NoWrap()).TextColor(t.InkSubtle)
		switch s.series.tickAnchor(row) {
		case anchorStart:
			label.MarginStart(s.series.tickWidths[row])
		case anchorEnd:
			label.MarginEnd(s.series.tickWidths[row])
		}
		if full := s.series.tipTitles[row]; !s.series.timeline && full != tick.Label {
			kit.Tooltip(c, label, full)
		}
	})
}

// gridLines paints a fine rule across the plot at each Y tick.
func gridLines(p *ui.Painter, r ui.Rect, ticks []chart.Tick, color ui.Color) {
	for _, tick := range ticks {
		y := chart.Y(r, tick.At)
		p.Line(r.X, y, r.X+r.W, y, theme.Rule, color)
	}
}

// timelinePlot draws the metric's line in chart-1 without a fill over
// the grid, and the point the pointer or the arrows select: a rule, the
// point and its tooltip. A lone bucket shows its point.
func timelinePlot(c *ui.Context, s *chartState, area ui.Element) {
	t := theme.Current(c)
	xs, ys := s.series.xs, s.series.ys[0]
	area.Draw(func(p *ui.Painter, r ui.Rect) {
		gridLines(p, r, s.series.yTicks, t.Line)
		chart.LineInto(&s.path, r, xs, ys, chart.CurveLinear)
		p.StrokePath(&s.path, chartLineWidth, t.Chart[0])
	})
	chart.Select(c, area, &s.selected, xs, func(i int) string { return s.series.descriptions[i] })
	i := s.selected
	if i >= 0 {
		chart.RuleX(c, xs[i], chartRuleWidth).Background(t.LineStrong).IgnoreInput(true)
	}
	if i < 0 && len(xs) == 1 {
		i = 0
	}
	if i >= 0 {
		chart.Point(c, xs[i], ys[i], chartDotSize, chartDotSize).Radius(chartDotSize/2).
			Background(t.Chart[0]).Border(chartDotRing, t.Surface).IgnoreInput(true)
	}
	selectionTip(c, s)
}

// barsPlot draws a bar for each row over the grid, in its category
// color, or Context's three bars in chart-1 to chart-3, with shares above
// share bars; the row the pointer or the arrows select takes the hover
// surface behind its bars and shows its tooltip.
func barsPlot(c *ui.Context, s *chartState, area ui.Element) {
	t := theme.Current(c)
	area.Draw(func(p *ui.Painter, r ui.Rect) { gridLines(p, r, s.series.yTicks, t.Line) })
	chart.Select(c, area, &s.selected, s.series.xs, func(i int) string { return s.series.descriptions[i] })
	if i := s.selected; i >= 0 {
		chart.Rect(c, s.series.cursor.Band(i), chart.Span{Lo: 0, Hi: 1}).Background(t.SurfaceHover).IgnoreInput(true)
	}
	for i := range s.series.xs {
		group := chart.Rect(c.Key(i), s.series.band.Band(i), chart.Span{Lo: 0, Hi: 1}).IgnoreInput(true)
		group.Children(func() {
			bars := ui.Row(c).Fill().AlignItems(ui.End).Justify(ui.Center).Gap(chartBarGap)
			bars.Children(func() {
				for k, heights := range s.series.ys {
					color := t.Chart[dashboardview.ChartColorIndex(i)]
					if s.series.context {
						color = t.Chart[k]
					}
					bar := ui.Box(c).Grow(1).Basis(0).MaxWidth(chartBarMax).HeightPercent(100 * heights[i]).Background(color)
					if s.series.shares {
						bar.Radius(chartBarRadius, chartBarRadius, 0, 0)
					}
				}
			})
		})
		if s.series.shares {
			chart.Anchor(c, s.series.xs[i], s.series.ys[0][i], ui.Center, ui.End, func() {
				theme.Label(ui.Text(c, s.series.shareTexts[i]).NoWrap()).TextColor(t.Ink).Margin(0, 0, chartShareGap, 0)
			}).IgnoreInput(true)
		}
	}
	selectionTip(c, s)
}

// selectionTip shows the selected row's tooltip at the plot's top beside
// its point: after it in the plot's first half, before it in the second.
func selectionTip(c *ui.Context, s *chartState) {
	i := s.selected
	if i < 0 {
		return
	}
	x := s.series.xs[i]
	h := ui.Start
	if x > chartLonePoint {
		h = ui.End
	}
	chart.Anchor(c, x, 1, h, ui.Start, func() {
		tip := kit.ChartTip(c, s.series.tipTitles[i], s.series.tipLines[i])
		if h == ui.Start {
			tip.MarginStart(chartTipOffset)
		} else {
			tip.MarginEnd(chartTipOffset)
		}
	}).IgnoreInput(true)
}

// chartLegend is the web's legend under a category chart: a link for each
// row, in its bar's color with its share, that filters dimension by the
// row alone.
func chartLegend(c *ui.Context, w *model.Window, s *chartState, dimension dashboardview.Dimension) {
	t := theme.Current(c)
	row := ui.Row(c).Wrap().Justify(ui.Center).GapX(theme.Unit*3).GapY(theme.Unit*2).
		Padding(0, chartPanelPadding, theme.Unit*4, chartPanelPadding).Role(ui.RoleGroup).Label(chartLegendLabel)
	row.Children(func() {
		for i, name := range s.series.names {
			item := kit.LegendButton(c.Key(i), t.Chart[dashboardview.ChartColorIndex(i)], name, s.series.shareTexts[i])
			kit.Tooltip(c, item, filterPrefix+name)
			if item.Clicked() {
				w.UpdateSelection(dashboardview.DimensionChange(dimension, []string{name}))
			}
		}
	})
}

// contextLegend names Context's three bars by their colors and explains
// its values, as the web's.
func contextLegend(c *ui.Context) {
	t := theme.Current(c)
	list := ui.Row(c).Wrap().Justify(ui.Center).GapX(theme.Unit*4).GapY(theme.Unit*2).
		Padding(theme.Unit*2, chartPanelPadding, theme.Unit*4, chartPanelPadding).Role(ui.RoleList).Label(contextLegendLabel)
	list.Children(func() {
		for k, series := range dashboardview.ContextSeries() {
			item := ui.Row(c).Gap(theme.Unit * 2).Role(ui.RoleListItem).TextColor(t.InkMuted)
			item.Children(func() {
				kit.Swatch(c, t.Chart[k])
				theme.Label(ui.Text(c, series.Label))
			})
		}
	})
	theme.Label(ui.Text(c, dashboardview.ContextFootnote)).TextColor(t.InkSubtle).Padding(0, chartPanelPadding, theme.Unit*4, chartPanelPadding)
}
