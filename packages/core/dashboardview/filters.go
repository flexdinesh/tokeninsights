package dashboardview

import (
	"slices"
	"strings"

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
