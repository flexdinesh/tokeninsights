package views

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Filter toolbar texts, as the web's FilterToolbar.
const (
	filtersLabel   = "Filters"
	dashboardLabel = "Dashboard filters"
	filtersNote    = "Date and usage filters apply to every view. Location filters apply only to Repo. Sync always refreshes all supported harnesses."
)

// filterRow is the row under the header, as the web's: the filter
// toolbar, which the instance offers with its filter values, beside the
// period navigation, across the page gutter above a fine rule. It shows
// once the instance gave the query.
func filterRow(c *ui.Context, w *model.Window) {
	instance, ok := w.Instance()
	if !ok {
		return
	}
	t := theme.Current(c)
	row := ui.Row(c).Wrap().AlignItems(ui.Start).GapX(theme.Unit*3).GapY(theme.Unit*2).
		MarginX(-theme.Gutter).Padding(theme.Unit*2, theme.Gutter).BorderWidth(0, 0, theme.Rule, 0).BorderColor(t.Line)
	row.Children(func() {
		if instance.Capabilities.Has(serverfeatures.Facets) {
			FilterToolbar(c, w)
		}
		DateNavigation(c.Key("period"), w)
	})
}

// FilterToolbar is the web's FilterToolbar: the Filters caption, the date
// filter and the usage filters in a wrapping row, then the active filters.
func FilterToolbar(c *ui.Context, w *model.Window) ui.Element {
	section := ui.Column(c).MinWidth(0).Grow(1).Role(ui.RoleRegion).Label(dashboardLabel).Description(filtersNote)
	return section.Children(func() {
		ui.Row(c).Wrap().AlignItems(ui.Center).Gap(theme.Unit * 2).Children(func() {
			filtersCaption(c)
			DateFilter(c.Key("date"), w)
			dimensionFilters(c, w)
		})
		activeFilters(c, w)
	})
}

// filtersCaption is the muted Filters caption with its icon.
func filtersCaption(c *ui.Context) {
	t := theme.Current(c)
	caption := ui.Row(c).AlignItems(ui.Center).Gap(theme.Unit*2).Margin(0, theme.Unit*2, 0, 0).TextColor(t.InkSubtle)
	theme.Label(caption).Children(func() {
		kit.Icon(c, icons.Filter())
		ui.Text(c, filtersLabel).SingleLine()
	})
}

// dimensionFilters is where the harness, provider, model and session
// filters go, after the date filter; they arrive with their facets.
func dimensionFilters(*ui.Context, *model.Window) {}

// activeFilters is where the chips of the active filters and Clear All
// go, under the toolbar row; they arrive with the dimension filters.
func activeFilters(*ui.Context, *model.Window) {}
