package dashboardview

import (
	"slices"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
)

// Tab is a dashboard view (an aggregation mode, ADR 0018).
type Tab string

const (
	TabTokens    Tab = "tokens"
	TabModels    Tab = "models"
	TabProviders Tab = "providers"
	TabHarnesses Tab = "harnesses"
	TabSessions  Tab = "sessions"
	TabContext   Tab = "context"
	TabRepo      Tab = "repo"
)

// Tabs returns every tab in navigation order.
func Tabs() []Tab {
	return []Tab{TabTokens, TabModels, TabProviders, TabHarnesses, TabSessions, TabContext, TabRepo}
}

// Sort is a server-owned sort field.
type Sort string

const (
	SortName           Sort = "name"
	SortDate           Sort = "date"
	SortTotal          Sort = "total"
	SortInput          Sort = "input"
	SortOutput         Sort = "output"
	SortReasoning      Sort = "reasoning"
	SortCacheRead      Sort = "cacheRead"
	SortCacheWrite     Sort = "cacheWrite"
	SortSessions       Sort = "sessions"
	SortContext        Sort = "context"
	SortAverageContext Sort = "averageContext"
	SortMedianContext  Sort = "medianContext"
	SortMaxContext     Sort = "maxContext"
	SortHarness        Sort = "harness"
	SortProvider       Sort = "provider"
	SortModel          Sort = "model"
)

// Direction orders rows.
type Direction string

const (
	Ascending  Direction = "asc"
	Descending Direction = "desc"
)

// Quality selects confirmed usage or the excluded (estimated) review mode.
type Quality string

const (
	QualityConfirmed Quality = "confirmed"
	QualityEstimated Quality = "estimated"
)

// DateMode marks explicit bounds as a navigated calendar period rather than a
// custom range. The empty value means custom (or no bounds).
type DateMode string

// DateModeCalendar marks bounds produced by calendar navigation.
const DateModeCalendar DateMode = "calendar"

// DefaultPageSize is the page size of a new query.
const DefaultPageSize = 50

// Query is one window's dashboard query state, the twin of the TypeScript
// DashboardQuery. Values are immutable by convention: every transition returns
// a new Query and never mutates shared slices. JSON names match DashboardQuery
// so shared fixtures decode directly.
type Query struct {
	analytics.Selection
	DateMode      DateMode            `json:"dateMode,omitempty"`
	Quality       Quality             `json:"quality"`
	Tab           Tab                 `json:"tab"`
	LocationGroup analytics.RepoGroup `json:"locationGroup"`
	Repositories  []string            `json:"repositories"`
	Directories   []string            `json:"directories"`
	Sort          Sort                `json:"sort"`
	Direction     Direction           `json:"direction"`
	Page          int                 `json:"page"`
	PageSize      int                 `json:"pageSize"`
}

// DefaultSort is the sort a tab starts with.
func DefaultSort(tab Tab) Sort {
	switch tab {
	case TabContext:
		return SortAverageContext
	case TabTokens, TabSessions:
		return SortDate
	default:
		return SortTotal
	}
}

// contextSorts are the only sorts the Context tab offers.
var contextSorts = []Sort{SortAverageContext, SortMedianContext, SortMaxContext, SortSessions, SortHarness, SortProvider, SortModel}

// CompatibleSort returns sort when a restored query may use it on tab, else
// the tab's default, like the web's queryFromSearch: Context accepts only its
// own sorts; other tabs reject Context-only and dimension sorts.
func CompatibleSort(tab Tab, sort Sort) Sort {
	allowed := slices.Contains(contextSorts, sort)
	if tab != TabContext {
		allowed = sort == SortSessions || !slices.Contains(contextSorts, sort)
	}
	if !allowed {
		return DefaultSort(tab)
	}
	return sort
}

// InitialQuery is the query for a new window on tab, starting from the backend defaults.
func InitialQuery(defaults analytics.Selection, tab Tab) Query {
	return Query{
		Selection: cloneSelection(defaults), Quality: QualityConfirmed, Tab: tab,
		LocationGroup: analytics.RepoGroupRepository, Repositories: []string{}, Directories: []string{},
		Sort: DefaultSort(tab), Direction: Descending, Page: 1, PageSize: DefaultPageSize,
	}
}

// SelectionChange sets the non-nil fields of the selection. A non-nil DateMode
// replaces the date mode (pointing at "" clears it); nil keeps it, so filter
// changes preserve calendar navigation.
type SelectionChange struct {
	Period, Bucket, From, To               *string
	Providers, Models, Harnesses, Sessions *[]string
	DateMode                               *DateMode
}

// LocationChange sets the non-nil Repo location filters.
type LocationChange struct {
	Repositories, Directories *[]string
}

// WithQuality switches between confirmed and excluded usage.
func (q Query) WithQuality(quality Quality) Query {
	q.Quality, q.Page = quality, 1
	return q
}

// WithSelection applies a selection change and returns to the first page.
func (q Query) WithSelection(change SelectionChange) Query {
	assign(&q.Period, change.Period)
	assign(&q.Bucket, change.Bucket)
	assign(&q.From, change.From)
	assign(&q.To, change.To)
	assignClone(&q.Providers, change.Providers)
	assignClone(&q.Models, change.Models)
	assignClone(&q.Harnesses, change.Harnesses)
	assignClone(&q.Sessions, change.Sessions)
	assign(&q.DateMode, change.DateMode)
	q.Page = 1
	return q
}

func assign[T any](target, value *T) {
	if value != nil {
		*target = *value
	}
}

func assignClone(target, value *[]string) {
	if value != nil {
		*target = clone(*value)
	}
}

// Reset restores the backend default selection and clears date mode and locations.
func (q Query) Reset(defaults analytics.Selection) Query {
	q.Selection, q.DateMode = cloneSelection(defaults), ""
	q.Repositories, q.Directories, q.Page = []string{}, []string{}, 1
	return q
}

// ClearFilters removes every dimension, location and date bound while keeping
// the period, bucket and view.
func (q Query) ClearFilters() Query {
	q.Providers, q.Models, q.Harnesses, q.Sessions = []string{}, []string{}, []string{}, []string{}
	q.Repositories, q.Directories = []string{}, []string{}
	q.From, q.To, q.DateMode, q.Page = "", "", "", 1
	return q
}

// HasFilters reports whether ClearFilters would change the rows: a
// dimension or location filter or a date bound is set, as the web's
// results table decides whether to offer Clear filters.
func (q Query) HasFilters() bool {
	for _, values := range [][]string{q.Providers, q.Models, q.Harnesses, q.Sessions, q.Repositories, q.Directories} {
		if len(values) > 0 {
			return true
		}
	}
	return q.From != "" || q.To != ""
}

// WithTab switches views. Filters persist, location filters only on Repo, and
// sorting resets to the tab default.
func (q Query) WithTab(tab Tab) Query {
	if tab != TabRepo {
		q.Repositories, q.Directories = []string{}, []string{}
	}
	q.Tab, q.Sort, q.Direction, q.Page = tab, DefaultSort(tab), Descending, 1
	return q
}

// WithLocationGroup groups Repo rows by repository or directory.
func (q Query) WithLocationGroup(group analytics.RepoGroup) Query {
	q.LocationGroup, q.Page = group, 1
	return q
}

// WithLocations applies a Repo location filter change.
func (q Query) WithLocations(change LocationChange) Query {
	assignClone(&q.Repositories, change.Repositories)
	assignClone(&q.Directories, change.Directories)
	q.Page = 1
	return q
}

// SortBy selects a sort. The current field flips direction; a new name or
// dimension field starts ascending, any other field descending.
func (q Query) SortBy(sort Sort) Query {
	switch {
	case q.Sort == sort && q.Direction == Ascending:
		q.Direction = Descending
	case q.Sort == sort:
		q.Direction = Ascending
	case sort == SortName || sort == SortHarness || sort == SortProvider || sort == SortModel:
		q.Direction = Ascending
	default:
		q.Direction = Descending
	}
	q.Sort, q.Page = sort, 1
	return q
}

// WithPage moves to a page.
func (q Query) WithPage(page int) Query {
	q.Page = page
	return q
}

// WithPageSize changes the page size and returns to the first page.
func (q Query) WithPageSize(size int) Query {
	q.PageSize, q.Page = size, 1
	return q
}

// ToAnalytics is the read request the web client sends for this query:
// confirmed quality and empty bounds are left to backend defaults, and
// location options are sent only for Repo.
func (q Query) ToAnalytics() analytics.Query {
	request := analytics.Query{
		Selection: cloneSelection(q.Selection), Tab: string(q.Tab), Sort: string(q.Sort),
		Direction: string(q.Direction), Page: q.Page, PageSize: q.PageSize,
	}
	if q.Quality == QualityEstimated {
		request.Quality = string(QualityEstimated)
	}
	if q.Tab == TabRepo {
		request.LocationGroup = q.LocationGroup
		request.RepositoryKeys, request.DirectoryKeys = clone(q.Repositories), clone(q.Directories)
	}
	return request
}

// FacetRequest is the filter-choice request the web client sends: the usage
// request without sorting, paging or location grouping, scoped to Repo only
// on that tab.
func (q Query) FacetRequest(search string) application.FacetRequest {
	request := q.ToAnalytics()
	if q.Tab != TabRepo {
		request.Tab = ""
	}
	request.LocationGroup, request.Sort, request.Direction, request.Page, request.PageSize = "", "", "", 0, 0
	return application.FacetRequest{Query: request, Search: search}
}

func cloneSelection(selection analytics.Selection) analytics.Selection {
	selection.Providers, selection.Models = clone(selection.Providers), clone(selection.Models)
	selection.Harnesses, selection.Sessions = clone(selection.Harnesses), clone(selection.Sessions)
	return selection
}

// clone copies values so queries never share backing arrays; nil stays empty.
func clone(values []string) []string { return append([]string{}, values...) }
