package views

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Repo texts, as the web's App, FilterToolbar and UnknownLocation.
const (
	repoOptionsLabel     = "Repo view options"
	groupByLabel         = "Group by"
	groupByLocationLabel = "Group by location"

	unknownLocationName  = "unknown"
	showDirectoriesLabel = "Show directories"
	hideDirectoriesLabel = "Hide directories"
	recordedDirectories  = "Recorded directories"
	someWithoutDirectory = "Some usage has no recorded directory"
	noRecordedDirectory  = "No recorded directory"
)

// repoOptions is the row of the Repo view's options under the filter
// row, as the web's: the Group by select, which reads the usage of the
// grouping chosen. It shows on Repo once the instance gave the query.
func repoOptions(c *ui.Context, w *model.Window) {
	query, ok := w.Query()
	if !ok || query.Tab != dashboardview.TabRepo {
		return
	}
	t := theme.Current(c)
	row := ui.Row(c).Wrap().Gap(theme.Unit*3).MarginX(-theme.Gutter).Padding(theme.Unit*2, theme.Gutter).
		BorderWidth(0, 0, theme.Rule, 0).BorderColor(t.Line).Role(ui.RoleGroup).Label(repoOptionsLabel)
	row.Children(func() {
		options := []kit.Option[analytics.RepoGroup]{}
		for _, g := range dashboardview.LocationGroups() {
			options = append(options, kit.Option[analytics.RepoGroup]{Value: g.Group, Label: g.Label})
		}
		kit.ControlLabel(c, groupByLabel, func() {
			group := query.LocationGroup
			if kit.Select(c.Key("group"), &group, options, kit.SizeCompact).Label(groupByLocationLabel).Changed() {
				w.SetLocationGroup(group)
			}
		})
	})
}

// locationFilters are the Repo view's repository and directory filters
// after the usage filters, as the web's: the locations the query's facets
// offer, a selected location they no longer offer named "Repository
// (unavailable)". They show on Repo only.
func locationFilters(c *ui.Context, w *model.Window) {
	query, _ := w.Query()
	if query.Tab != dashboardview.TabRepo {
		return
	}
	facets, _ := w.Facets()
	for _, d := range dashboardview.LocationDimensions() {
		dimension := d.Key
		MultiSelect(c.Key(string(d.Key)), MultiSelectOptions{
			Label:       d.Label,
			Values:      dashboardview.LocationValues(facets.Facets, dimension),
			Selected:    query.Locations(dimension),
			MissingName: d.MissingName(),
			OnChange:    func(keys []string) { w.UpdateLocations(dashboardview.LocationsChange(dimension, keys)) },
		})
	}
}

// locationChips are the active location filters' chips on Repo, each
// named as its filter names the location and removing it, as the web's.
func locationChips(c *ui.Context, w *model.Window, query dashboardview.Query) {
	if query.Tab != dashboardview.TabRepo {
		return
	}
	facets, _ := w.Facets()
	for _, d := range dashboardview.LocationDimensions() {
		keys := query.Locations(d.Key)
		names := dashboardview.SelectedFilterNames(dashboardview.LocationValues(facets.Facets, d.Key), keys, d.MissingName())
		for i, key := range keys {
			remove := removePrefix + strings.ToLower(d.Label) + " " + names[i]
			if kit.Chip(c.Key(string(d.Key)+":"+key), d.Label, names[i], remove).Clicked() {
				rest := slices.DeleteFunc(slices.Clone(keys), func(k string) bool { return k == key })
				w.UpdateLocations(dashboardview.LocationsChange(d.Key, rest))
			}
		}
	}
}

// unknownLocation is the name cell of Repo usage without a known
// location, the web's UnknownLocation: "unknown" and, when directories
// were recorded for it, a disclosure of them, with a note that some usage
// has none while it shows, or the note alone when none were recorded.
func unknownLocation(c *ui.Context, row analytics.Row, open *bool) {
	t := theme.Current(c)
	directories := row.DirectoryNames
	ui.Column(c).MinWidth(0).Gap(theme.Unit).Children(func() {
		var disclosure ui.CollapsibleParts
		ui.Row(c).Wrap().AlignItems(ui.Center).GapX(theme.Unit * 2).MinWidth(0).Children(func() {
			theme.Medium(theme.Body(ui.Text(c, unknownLocationName))).SingleLine()
			if len(directories) > 0 {
				label := showDirectoriesLabel
				if *open {
					label = hideDirectoriesLabel
				}
				disclosure = kit.Disclosure(c.Key("directories"), open, label)
			}
		})
		if len(directories) > 0 {
			disclosure.Panel(func() {
				list := ui.Column(c).Gap(theme.Unit).PaddingStart(theme.Unit*2).BorderWidth(0, 0, 0, theme.Rule).BorderColor(t.Line).
					Role(ui.RoleList).Label(recordedDirectories)
				list.Children(func() {
					for _, directory := range directories {
						path := theme.Mono(theme.Label(ui.Text(c.Key(directory), directory))).TextColor(t.InkSubtle).Role(ui.RoleListItem)
						kit.Tooltip(c, path, directory)
					}
				})
			})
		}
		if row.HasUnknownDirectory && (*open || len(directories) == 0) {
			note := noRecordedDirectory
			if len(directories) > 0 {
				note = someWithoutDirectory
			}
			theme.Label(ui.Text(c, note)).TextColor(t.InkSubtle)
		}
	})
}
