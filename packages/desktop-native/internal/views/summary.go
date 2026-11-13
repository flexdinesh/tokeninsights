package views

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Summary strip names, as the web's SummaryCards.
const (
	summaryLabel         = "Filtered usage summary"
	excludedSummaryLabel = "Filtered excluded usage summary"
)

// summaryReadoutCount is the number of usage readouts: total, input,
// output, cache read and sessions.
const summaryReadoutCount = 5

// SummaryReadouts are the five usage readouts of summary, as the web's
// SummaryCards: compact values named by their exact counts, the total
// first. Estimated readouts describe excluded usage.
func SummaryReadouts(summary analytics.ViewerSummaryRow, estimated bool) []kit.Readout {
	total, totalDetail := "Total tokens", "Included in your usage total"
	sessions, sessionsDetail := "Sessions shown", "synced"
	if estimated {
		total, totalDetail = "Excluded tokens", "Excluded from your usage total"
		sessions, sessionsDetail = "Excluded sessions shown", "with excluded usage"
	}
	readout := func(label string, value int64, detail string) kit.Readout {
		return kit.Readout{
			Label: label, Value: dashboardview.CompactCount(value),
			ValueLabel: label + ": " + dashboardview.ExactCount(value), Detail: detail,
		}
	}
	return []kit.Readout{
		readout(total, summary.TotalTokens, totalDetail),
		readout("Input", summary.InputTokens, "Uncached prompt tokens"),
		readout("Output", summary.OutputTokens, dashboardview.CompactCount(summary.ReasoningTokens)+" reasoning tokens reported"),
		readout("Cache read", summary.CacheReadTokens, dashboardview.CompactCount(summary.CacheWriteTokens)+" cache write tokens"),
		readout(sessions, summary.SessionCount,
			dashboardview.ExactCount(summary.SyncedSessions)+" "+sessionsDetail+" across all dates & harnesses"),
	}
}

// SummaryCards builds the readout strip of summary across the page,
// bleeding into the page gutter as the web's. It lays the readouts out in
// two columns when the page is narrower than theme.ReadoutNarrow.
func SummaryCards(c *ui.Context, summary analytics.ViewerSummaryRow, estimated bool) ui.Element {
	label := summaryLabel
	if estimated {
		label = excludedSummaryLabel
	}
	return kit.ReadoutStrip(c, label, SummaryReadouts(summary, estimated), readoutsNarrow(c)).MarginX(-theme.Gutter)
}

// readoutsNarrow reports whether the readout strip, as wide as the page
// beside the sidebar up to the content width, is narrower than
// theme.ReadoutNarrow, as the web's readout container query.
func readoutsNarrow(c *ui.Context) bool {
	width, _ := c.Size()
	return min(width-theme.SidebarWidth, theme.ContentMax) < theme.ReadoutNarrow
}
