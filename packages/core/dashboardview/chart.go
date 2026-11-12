package dashboardview

import (
	"strings"
	"unicode/utf16"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
)

// ChartMetric is a selectable chart series.
type ChartMetric struct {
	Key   Sort
	Label string
}

// ChartMetrics lists the timeline metric choices; Total is the default.
func ChartMetrics() []ChartMetric {
	return []ChartMetric{
		{SortTotal, "Total"}, {SortInput, "Input"}, {SortOutput, "Output"},
		{SortCacheRead, "Cache read"}, {SortCacheWrite, "Cache write"}, {SortReasoning, "Reasoning"},
	}
}

// ContextSeries lists the Context chart's bars and legend in order.
func ContextSeries() []ChartMetric {
	return []ChartMetric{{SortAverageContext, "Average"}, {SortMedianContext, "Median"}, {SortMaxContext, "Maximum"}}
}

// ContextFootnote explains Context chart values.
const ContextFootnote = "Context = input + cache read + cache write. Each session contributes its peak within the selected range."

// ChartPaletteSize is the number of categorical chart colors (chart-1..chart-12).
const ChartPaletteSize = 12

// ChartColorIndex is the zero-based categorical color of the row at index.
func ChartColorIndex(index int) int { return index % ChartPaletteSize }

// ChartIsTimeline reports whether tab charts usage over time instead of categories.
func ChartIsTimeline(tab Tab) bool { return tab == TabTokens || tab == TabSessions }

// ChartTitle titles the chart for the query's tab, grouping and quality.
func ChartTitle(q Query) string {
	var title string
	switch {
	case ChartIsTimeline(q.Tab):
		title = "Usage over time"
	case q.Tab == TabContext:
		title = "Session peak context"
	case q.Tab == TabRepo:
		title = "Usage by " + string(q.LocationGroup)
	case q.Tab == TabHarnesses:
		title = "Usage by harness"
	default:
		title = "Usage by " + strings.TrimSuffix(string(q.Tab), "s")
	}
	if q.Quality != QualityEstimated {
		return title
	}
	if q.Tab == TabContext {
		return "Excluded session peak context"
	}
	return strings.Replace(title, "Usage", "Excluded usage", 1)
}

// ChartDescription describes what the chart shows.
func ChartDescription(tab Tab) string {
	switch {
	case ChartIsTimeline(tab):
		return "Token usage across the selected range"
	case tab == TabContext:
		return "Top 12 groups by average in-range session peak"
	case tab == TabRepo:
		return "Top 12 location rows by total tokens · % of filtered total"
	}
	return "Top 12 by total tokens · % of filtered total · select a label to filter"
}

// ChartUnitLabel names the unit of a category chart.
func ChartUnitLabel(q Query) string {
	switch {
	case q.Tab == TabContext:
		return "Prompt-side tokens"
	case q.Quality == QualityEstimated:
		return "Excluded tokens"
	}
	return "Countable tokens"
}

// ChartEmptyLabel replaces a chart with no rows.
func ChartEmptyLabel(quality Quality) string {
	if quality == QualityEstimated {
		return "No excluded usage in this range"
	}
	return "No usage in this range"
}

// Dimension is a usage filter dimension.
type Dimension string

const (
	DimensionHarnesses Dimension = "harnesses"
	DimensionProviders Dimension = "providers"
	DimensionModels    Dimension = "models"
	DimensionSessions  Dimension = "sessions"
)

// ChartFilterDimension is the filter a chart legend item selects on tab.
func ChartFilterDimension(tab Tab) (Dimension, bool) {
	switch tab {
	case TabModels:
		return DimensionModels, true
	case TabProviders:
		return DimensionProviders, true
	case TabHarnesses:
		return DimensionHarnesses, true
	}
	return "", false
}

// ChartShowsTokenShares reports whether bars carry a share of the filtered total.
func ChartShowsTokenShares(tab Tab) bool {
	_, filters := ChartFilterDimension(tab)
	return filters || tab == TabRepo
}

// ChartRowLabel labels a chart row: "model · harness · provider" on Context, else its name.
func ChartRowLabel(row analytics.Row, tab Tab) string {
	if tab == TabContext {
		return row.Model + " · " + row.Harness + " · " + row.Provider
	}
	return row.Name
}

// Category axis labels longer than chartLabelMax UTF-16 units keep chartLabelKeep and an ellipsis.
const (
	chartLabelMax  = 17
	chartLabelKeep = 15
)

// TruncateChartLabel shortens a category axis label like the web (lengths in
// UTF-16 code units). A surrogate pair crossing the cut is dropped whole where
// the web would keep a lone surrogate.
func TruncateChartLabel(label string) string {
	if len(utf16.Encode([]rune(label))) <= chartLabelMax {
		return label
	}
	var kept strings.Builder
	units := 0
	for _, r := range label {
		units += utf16.RuneLen(r)
		if units > chartLabelKeep {
			break
		}
		kept.WriteRune(r)
	}
	return kept.String() + "…"
}
