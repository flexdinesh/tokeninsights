package dashboardview

import (
	"slices"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// DimensionOption labels a filter dimension.
type DimensionOption struct {
	Key   Dimension
	Label string
}

// FilterDimensions lists the usage filters in toolbar order.
func FilterDimensions() []DimensionOption {
	return []DimensionOption{
		{DimensionHarnesses, "Harness"}, {DimensionProviders, "Provider"},
		{DimensionModels, "Model"}, {DimensionSessions, "Session"},
	}
}

// Values are the values q filters dimension d by, in selection order.
func (q Query) Values(d Dimension) []string {
	switch d {
	case DimensionHarnesses:
		return q.Harnesses
	case DimensionProviders:
		return q.Providers
	case DimensionModels:
		return q.Models
	case DimensionSessions:
		return q.Sessions
	}
	return nil
}

// DimensionChange filters dimension d by values, as the web's
// updateSelection({ [d]: values }).
func DimensionChange(d Dimension, values []string) SelectionChange {
	values = clone(values)
	switch d {
	case DimensionHarnesses:
		return SelectionChange{Harnesses: &values}
	case DimensionProviders:
		return SelectionChange{Providers: &values}
	case DimensionModels:
		return SelectionChange{Models: &values}
	case DimensionSessions:
		return SelectionChange{Sessions: &values}
	}
	return SelectionChange{}
}

// FacetValues are the values facets offer for dimension d.
func FacetValues(facets analytics.Facets, d Dimension) []string {
	switch d {
	case DimensionHarnesses:
		return facets.Harnesses
	case DimensionProviders:
		return facets.Providers
	case DimensionModels:
		return facets.Models
	case DimensionSessions:
		return facets.Sessions
	}
	return nil
}

// HasActiveFilters reports whether the filter toolbar shows its active
// filters, as the web's FilterToolbar: a dimension filter, a location
// filter on Repo, or custom dates. Bounds of a navigated calendar period
// are not active filters.
func (q Query) HasActiveFilters() bool {
	for _, d := range FilterDimensions() {
		if len(q.Values(d.Key)) > 0 {
			return true
		}
	}
	if q.Tab == TabRepo && (len(q.Repositories) > 0 || len(q.Directories) > 0) {
		return true
	}
	return q.CustomDates()
}

// LocationDimension is a Repo-only location filter.
type LocationDimension string

const (
	LocationRepositories LocationDimension = "repositories"
	LocationDirectories  LocationDimension = "directories"
)

// LocationOption labels a Repo location filter.
type LocationOption struct {
	Key   LocationDimension
	Label string
}

// LocationDimensions lists the Repo location filters in toolbar order.
func LocationDimensions() []LocationOption {
	return []LocationOption{{LocationRepositories, "Repository"}, {LocationDirectories, "Directory"}}
}

// PeriodOption is a date preset.
type PeriodOption struct {
	Period string
	Label  string
}

// PeriodOptions lists the date presets; legacy "yesterday" displays as Daily.
func PeriodOptions() []PeriodOption {
	return []PeriodOption{
		{PeriodToday, "Daily"}, {PeriodWeek, "Weekly"}, {PeriodMonth, "Monthly"},
		{PeriodYear, "Yearly"}, {PeriodAll, "All time"},
	}
}

// FilterOption is a selectable filter value: a stable key and its display name.
type FilterOption struct {
	Key, Name string
}

// unknownKey is the location key whose name is always "unknown".
const unknownKey = "unknown"

// FilterOptions merges known values with selected keys the facets no longer
// return, orders them by name like String.prototype.localeCompare (CLDR root
// collation via x/text, stable for equal names) and keeps names containing
// search case-insensitively. Selected keys without a known name display
// missingName (or the key when missingName is empty); "unknown" stays "unknown".
// x/text may trail the browser's ICU/Unicode version for rare scripts, and
// lowercase matching uses per-rune Unicode folding, which differs from
// JavaScript only for context-sensitive letters such as final sigma.
func FilterOptions(known []FilterOption, selected []string, missingName, search string) []FilterOption {
	options := slices.Clone(known)
	for _, key := range selected {
		if !slices.ContainsFunc(known, func(option FilterOption) bool { return option.Key == key }) {
			options = append(options, FilterOption{Key: key, Name: fallbackName(key, missingName)})
		}
	}
	collator := collate.New(language.English)
	slices.SortStableFunc(options, func(a, b FilterOption) int { return collator.CompareString(a.Name, b.Name) })
	needle := strings.ToLower(search)
	return slices.DeleteFunc(options, func(option FilterOption) bool {
		return !strings.Contains(strings.ToLower(option.Name), needle)
	})
}

// SelectedFilterNames returns display names for selected keys in selection order.
func SelectedFilterNames(known []FilterOption, selected []string, missingName string) []string {
	names := make([]string, 0, len(selected))
	for _, key := range selected {
		index := slices.IndexFunc(known, func(option FilterOption) bool { return option.Key == key })
		if index >= 0 {
			names = append(names, known[index].Name)
		} else {
			names = append(names, fallbackName(key, missingName))
		}
	}
	return names
}

func fallbackName(key, missingName string) string {
	if key == unknownKey || missingName == "" {
		return key
	}
	return missingName
}

// StringOptions adapts plain facet values, whose key is their name.
func StringOptions(values []string) []FilterOption {
	options := make([]FilterOption, len(values))
	for index, value := range values {
		options[index] = FilterOption{Key: value, Name: value}
	}
	return options
}
