package views

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
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

	activeFiltersLabel  = "Active filters"
	removePrefix        = "Remove "
	customDatesLabel    = "Custom dates"
	clearAllLabel       = "Clear All"
	sessionValuesFailed = "Session values couldn’t load."
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

// dimensionFilters are the harness, provider and model filters, then the
// session filter, after the date filter, and on Repo its location
// filters, as the web's.
func dimensionFilters(c *ui.Context, w *model.Window) {
	query, _ := w.Query()
	facets, _ := w.Facets()
	for _, d := range dashboardview.FilterDimensions() {
		if d.Key == dashboardview.DimensionSessions {
			sessionFilter(c.Key(string(d.Key)), w, d.Label, query.Sessions)
			continue
		}
		dimension := d.Key
		MultiSelect(c.Key(string(d.Key)), MultiSelectOptions{
			Label:    d.Label,
			Values:   dashboardview.StringOptions(dashboardview.FacetValues(facets.Facets, dimension)),
			Selected: query.Values(dimension),
			OnChange: func(values []string) { w.UpdateSelection(dashboardview.DimensionChange(dimension, values)) },
		})
	}
	locationFilters(c, w)
}

// sessionFilter is the web's SessionFilter: the session multi-select over
// the query's sessions, or the backend's matches of the search typed,
// which the window searches once typing rests.
func sessionFilter(c *ui.Context, w *model.Window, label string, selected []string) {
	choices := w.SessionChoices()
	search := w.SessionSearch()
	o := MultiSelectOptions{
		Label:    label,
		Values:   dashboardview.StringOptions(choices.Values),
		Selected: selected,
		Search:   &search,
		Loading:  choices.Loading,
		OnChange: func(values []string) {
			w.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionSessions, values))
		},
	}
	if choices.Err != nil {
		o.Error, o.Retry = sessionValuesFailed, w.RetrySessionSearch
	}
	MultiSelect(c, o)
	if search != w.SessionSearch() {
		w.SearchSessions(search, c.Now())
	}
	if wait, ok := w.PollSessionSearch(c.Now()); ok {
		c.After(wait)
	}
}

// activeFilters are the chips of the active filters, each removing its
// filter, and Clear All, under the toolbar row, as the web's. They show
// while a dimension filter, a location filter on Repo or custom dates are
// set.
func activeFilters(c *ui.Context, w *model.Window) {
	query, _ := w.Query()
	if !query.HasActiveFilters() {
		return
	}
	row := ui.Row(c).Wrap().Gap(theme.Unit*2).Margin(theme.Unit*2, 0, 0, 0).Role(ui.RoleGroup).Label(activeFiltersLabel)
	row.Children(func() {
		for _, d := range dashboardview.FilterDimensions() {
			values := query.Values(d.Key)
			for _, value := range values {
				remove := removePrefix + strings.ToLower(d.Label) + " " + value
				if kit.Chip(c.Key(string(d.Key)+":"+value), d.Label, value, remove).Clicked() {
					rest := slices.DeleteFunc(slices.Clone(values), func(v string) bool { return v == value })
					w.UpdateSelection(dashboardview.DimensionChange(d.Key, rest))
				}
			}
		}
		locationChips(c, w, query)
		if query.CustomDates() && kit.Chip(c.Key("dates"), "", customDatesLabel, customDatesLabel).Clicked() {
			w.UpdateSelection(dashboardview.CustomRangeChange("", ""))
		}
		if kit.Button(c.Key("clear"), clearAllLabel, kit.ButtonStyle{Variant: kit.Link, Size: kit.SizeCompact}).Clicked() {
			w.ClearFilters()
		}
	})
}
