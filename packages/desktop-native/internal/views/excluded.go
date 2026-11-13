package views

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Excluded usage texts, as the web's DashboardResults and ResultsTable.
const (
	excludedUsageHeading = "Excluded usage"
	backToUsage          = "Back to usage"
	excludedReviewScope  = "These saved token counts can’t be confidently included in your usage total. " +
		"They stay separate. Your date range and filters apply to this review."
	excludedFromTotals   = "Usage excluded from totals"
	excludedTokensSuffix = " tokens"
	excludedSeparator    = "· "
	reviewExcluded       = "Review excluded usage"
	excludedFailed       = "Excluded usage couldn’t load."
	retryExcluded        = "Retry excluded usage"

	showingExcluded = "Showing excluded usage. These counts stay outside your usage total."
	showingUsage    = "Showing usage totals."
)

// qualityChange follows the quality the results show, as the web's
// previousQuality: a switch between usage and excluded usage moves the
// focus to the results, without scrolling, and says what they show.
type qualityChange struct{ shown dashboardview.Quality }

// follow focuses region and announces what it shows when quality differs
// from the quality shown before, once a query is shown.
func (q *qualityChange) follow(c *ui.Context, region ui.Element, quality dashboardview.Quality) {
	if q.shown != "" && q.shown != quality {
		region.Focus(ui.FocusOptions{PreventScroll: true})
		if quality == dashboardview.QualityEstimated {
			c.Announce(showingExcluded)
		} else {
			c.Announce(showingUsage)
		}
	}
	q.shown = quality
}

// reviewHeader heads the review of excluded usage, as the web's: its
// title, the way back to usage and what the review shows.
func reviewHeader(c *ui.Context, w *model.Window) {
	t := theme.Current(c)
	box := ui.Column(c).PaddingY(theme.Unit*3).BorderWidth(0, 0, theme.Rule, 0).BorderColor(t.Line)
	box.Children(func() {
		ui.Row(c).Wrap().Justify(ui.SpaceBetween).GapX(theme.Unit * 4).GapY(theme.Unit * 2).Children(func() {
			theme.Title(ui.Text(c, excludedUsageHeading)).TextColor(t.Ink).Role(ui.RoleHeading).Level(2)
			back := kit.ButtonContent(c, kit.ButtonStyle{Variant: kit.Ghost}, func() {
				kit.Icon(c, icons.ArrowLeft())
				ui.Text(c, backToUsage).SingleLine()
			})
			if back.Clicked() {
				w.SetQuality(dashboardview.QualityConfirmed)
			}
		})
		theme.Body(ui.Text(c, excludedReviewScope)).TextColor(t.InkMuted).MaxWidth(theme.ProseWidth).Margin(theme.Unit, 0, 0, 0)
	})
}

// excludedNotice tells of the usage excluded from the totals shown, its
// compact total named by its exact count, with the way to review it, as
// the web's: a band across the page under the readouts.
func excludedNotice(c *ui.Context, w *model.Window, excluded analytics.ViewerSummaryRow) {
	t := theme.Current(c)
	excludedBand(c, ui.RoleNone, func() {
		ui.Row(c).Wrap().Gap(theme.Unit).MinWidth(0).TextColor(t.InkMuted).Children(func() {
			ui.Text(c, excludedFromTotals)
			total := ui.Text(c, excludedSeparator+dashboardview.CompactCount(excluded.TotalTokens)+excludedTokensSuffix)
			kit.Tooltip(c, total, dashboardview.ExactCount(excluded.TotalTokens)+excludedTokensSuffix)
		})
		review := kit.ButtonContent(c, kit.ButtonStyle{Variant: kit.Ghost, Size: kit.SizeCompact}, func() {
			ui.Text(c, reviewExcluded)
			kit.Icon(c, icons.ArrowRight())
		})
		if review.Shrink(1).MinWidth(0).Clicked() {
			w.SetQuality(dashboardview.QualityEstimated)
		}
	})
}

// excludedError says that excluded usage could not load, with its own
// retry, in the notice's place.
func excludedError(c *ui.Context, w *model.Window) {
	t := theme.Current(c)
	excludedBand(c, ui.RoleStatus, func() {
		ui.Text(c, excludedFailed).TextColor(t.InkMuted)
		if kit.Button(c, retryExcluded, kit.ButtonStyle{Variant: kit.Ghost, Size: kit.SizeCompact}).Clicked() {
			w.RetryExcluded()
		}
	})
}

// excludedBand is the row across the page, bleeding into its gutter
// above a rule, that the excluded notices share; role reads it out.
func excludedBand(c *ui.Context, role ui.Role, content func()) {
	t := theme.Current(c)
	band := ui.Row(c).Wrap().GapX(theme.Unit*4).GapY(theme.Unit*2).PaddingY(theme.Unit*2).PaddingX(theme.Gutter).
		MarginX(-theme.Gutter).BorderWidth(0, 0, theme.Rule, 0).BorderColor(t.Line)
	if role != ui.RoleNone {
		band.Role(role)
	}
	theme.Label(band).Children(content)
}

// qualityToggle is the empty table's way between usage and its excluded
// usage, as the web's: back to usage while reviewing, else to the review
// when excluded usage exists. ok is false when it offers neither.
func qualityToggle(w *model.Window, quality dashboardview.Quality) (label string, toggle func(), ok bool) {
	if quality == dashboardview.QualityEstimated {
		return backToUsage, func() { w.SetQuality(dashboardview.QualityConfirmed) }, true
	}
	if _, exists := w.ExcludedUsage(); exists {
		return reviewExcluded, func() { w.SetQuality(dashboardview.QualityEstimated) }, true
	}
	return "", nil, false
}
