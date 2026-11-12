package dashboardview_test

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

type fixtureAction struct {
	Type  string
	Value json.RawMessage
}

func TestQueryFixtures(t *testing.T) {
	var fixture struct {
		Defaults       analytics.Selection
		DefaultSort    map[dashboardview.Tab]dashboardview.Sort
		CompatibleSort []struct {
			Tab            dashboardview.Tab
			Sort, Expected dashboardview.Sort
		}
		Reducer []struct {
			Name      string
			Tab       dashboardview.Tab
			Actions   []fixtureAction
			Expected  dashboardview.Query
			APIParams [][2]string `json:"apiParams"`
		}
	}
	readFixture(t, "query.json", &fixture)
	if len(fixture.DefaultSort) != len(dashboardview.Tabs()) {
		t.Fatalf("defaultSort covers %d tabs", len(fixture.DefaultSort))
	}
	for tab, want := range fixture.DefaultSort {
		if got := dashboardview.DefaultSort(tab); got != want {
			t.Errorf("DefaultSort(%s) = %s, want %s", tab, got, want)
		}
	}
	for _, c := range fixture.CompatibleSort {
		if got := dashboardview.CompatibleSort(c.Tab, c.Sort); got != c.Expected {
			t.Errorf("CompatibleSort(%s, %s) = %s, want %s", c.Tab, c.Sort, got, c.Expected)
		}
	}
	for _, c := range fixture.Reducer {
		t.Run(c.Name, func(t *testing.T) {
			query := dashboardview.InitialQuery(fixture.Defaults, c.Tab)
			for _, action := range c.Actions {
				query = apply(t, query, action)
			}
			if !reflect.DeepEqual(query, c.Expected) {
				t.Errorf("query = %+v\nwant    %+v", query, c.Expected)
			}
			if c.APIParams != nil {
				if got := apiParams(query.ToAnalytics()); !reflect.DeepEqual(got, c.APIParams) {
					t.Errorf("api params = %q\nwant         %q", got, c.APIParams)
				}
			}
		})
	}
}

func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func apply(t *testing.T, query dashboardview.Query, action fixtureAction) dashboardview.Query {
	t.Helper()
	switch action.Type {
	case "quality":
		return query.WithQuality(decode[dashboardview.Quality](t, action.Value))
	case "selection":
		return query.WithSelection(selectionChange(t, decode[map[string]json.RawMessage](t, action.Value)))
	case "reset":
		return query.Reset(decode[analytics.Selection](t, action.Value))
	case "clearFilters":
		return query.ClearFilters()
	case "tab":
		return query.WithTab(decode[dashboardview.Tab](t, action.Value))
	case "locationGroup":
		return query.WithLocationGroup(decode[analytics.RepoGroup](t, action.Value))
	case "locations":
		var change dashboardview.LocationChange
		for key, raw := range decode[map[string]json.RawMessage](t, action.Value) {
			values := decode[[]string](t, raw)
			switch key {
			case "repositories":
				change.Repositories = &values
			case "directories":
				change.Directories = &values
			default:
				t.Fatalf("unknown location field %q", key)
			}
		}
		return query.WithLocations(change)
	case "sort":
		return query.SortBy(decode[dashboardview.Sort](t, action.Value))
	case "page":
		return query.WithPage(decode[int](t, action.Value))
	case "pageSize":
		return query.WithPageSize(decode[int](t, action.Value))
	}
	t.Fatalf("unknown action %q", action.Type)
	return query
}

// selectionChange maps a partial TypeScript DateSelection; null dateMode is an explicit clear.
func selectionChange(t *testing.T, fields map[string]json.RawMessage) dashboardview.SelectionChange {
	t.Helper()
	var change dashboardview.SelectionChange
	for key, raw := range fields {
		switch key {
		case "period":
			change.Period = new(decode[string](t, raw))
		case "bucket":
			change.Bucket = new(decode[string](t, raw))
		case "from":
			change.From = new(decode[string](t, raw))
		case "to":
			change.To = new(decode[string](t, raw))
		case "providers":
			change.Providers = new(decode[[]string](t, raw))
		case "models":
			change.Models = new(decode[[]string](t, raw))
		case "harnesses":
			change.Harnesses = new(decode[[]string](t, raw))
		case "sessions":
			change.Sessions = new(decode[[]string](t, raw))
		case "dateMode":
			mode := decode[*dashboardview.DateMode](t, raw)
			if mode == nil {
				mode = new(dashboardview.DateMode)
			}
			change.DateMode = mode
		default:
			t.Fatalf("unknown selection field %q", key)
		}
	}
	return change
}

// apiParams lists a request in the web client's URLSearchParams order.
func apiParams(q analytics.Query) [][2]string {
	params := [][2]string{
		{"period", q.Selection.Period}, {"bucket", q.Selection.Bucket}, {"tab", q.Tab}, {"sort", q.Sort},
		{"direction", q.Direction}, {"page", strconv.Itoa(q.Page)}, {"pageSize", strconv.Itoa(q.PageSize)},
	}
	for _, scalar := range [][2]string{{"quality", q.Quality}, {"from", q.Selection.From}, {"to", q.Selection.To}} {
		if scalar[1] != "" {
			params = append(params, scalar)
		}
	}
	repeated := func(key string, values []string) {
		for _, value := range values {
			params = append(params, [2]string{key, value})
		}
	}
	repeated("provider", q.Selection.Providers)
	repeated("model", q.Selection.Models)
	repeated("harness", q.Selection.Harnesses)
	repeated("session", q.Selection.Sessions)
	if q.LocationGroup != "" {
		params = append(params, [2]string{"locationGroup", string(q.LocationGroup)})
	}
	repeated("repository", q.RepositoryKeys)
	repeated("directory", q.DirectoryKeys)
	return params
}

func TestFacetRequestDropsPagingAndNonRepoScope(t *testing.T) {
	defaults := analytics.Selection{Period: "week", Bucket: "day", Providers: []string{"openai"}}
	tokens := dashboardview.InitialQuery(defaults, dashboardview.TabTokens).WithQuality(dashboardview.QualityEstimated).WithPage(3)
	request := tokens.FacetRequest("abc")
	want := analytics.Query{Quality: "estimated", Selection: analytics.Selection{Period: "week", Bucket: "day", Providers: []string{"openai"}, Models: []string{}, Harnesses: []string{}, Sessions: []string{}}}
	if request.Search != "abc" || !reflect.DeepEqual(request.Query, want) {
		t.Errorf("tokens facets = %+v, want %+v", request, want)
	}
	repo := dashboardview.InitialQuery(defaults, dashboardview.TabRepo).
		WithLocationGroup(analytics.RepoGroupDirectory).
		WithLocations(dashboardview.LocationChange{Repositories: &[]string{"repo-key"}})
	got := repo.FacetRequest("").Query
	if got.Tab != "repo" || got.LocationGroup != "" || got.Sort != "" || got.Page != 0 || !reflect.DeepEqual(got.RepositoryKeys, []string{"repo-key"}) {
		t.Errorf("repo facets = %+v", got)
	}
}

func TestTransitionsNeverShareSlices(t *testing.T) {
	providers := []string{"openai"}
	query := dashboardview.InitialQuery(analytics.Selection{Period: "week", Bucket: "day", Providers: providers}, dashboardview.TabTokens)
	providers[0] = "changed"
	if query.Providers[0] != "openai" {
		t.Fatal("InitialQuery aliases defaults")
	}
	models := []string{"model-a"}
	next := query.WithSelection(dashboardview.SelectionChange{Models: &models})
	models[0] = "changed"
	if next.Models[0] != "model-a" || len(query.Models) != 0 {
		t.Fatalf("WithSelection aliases input or mutates its receiver: %v %v", next.Models, query.Models)
	}
	request := next.ToAnalytics()
	request.Selection.Models[0] = "changed"
	if next.Models[0] != "model-a" {
		t.Fatal("ToAnalytics aliases query state")
	}
}

func TestHasFiltersMatchesClearFilters(t *testing.T) {
	plain := dashboardview.InitialQuery(analytics.Selection{Period: "all", Bucket: "day"}, dashboardview.TabTokens)
	if plain.HasFilters() {
		t.Fatal("a query without filters has filters")
	}
	providers, repositories := []string{"openai"}, []string{"repo-1"}
	for name, query := range map[string]dashboardview.Query{
		"bounds":       dashboardview.InitialQuery(analytics.Selection{Period: "month", From: "2026-11-01", To: "2026-11-30"}, dashboardview.TabTokens),
		"from":         plain.WithSelection(dashboardview.SelectionChange{From: new("2026-11-01")}),
		"providers":    plain.WithSelection(dashboardview.SelectionChange{Providers: &providers}),
		"repositories": plain.WithTab(dashboardview.TabRepo).WithLocations(dashboardview.LocationChange{Repositories: &repositories}),
	} {
		if !query.HasFilters() || query.ClearFilters().HasFilters() {
			t.Errorf("%s: HasFilters %v, after ClearFilters %v", name, query.HasFilters(), query.ClearFilters().HasFilters())
		}
	}
}

func TestHasActiveFiltersAsTheWebToolbar(t *testing.T) {
	plain := dashboardview.InitialQuery(analytics.Selection{Period: "month", Bucket: "day"}, dashboardview.TabTokens)
	if plain.HasActiveFilters() {
		t.Fatal("a query without filters shows active filters")
	}
	calendar := dashboardview.DateModeCalendar
	navigated := plain.WithSelection(dashboardview.SelectionChange{From: new("2026-10-01"), To: new("2026-10-31"), DateMode: &calendar})
	if navigated.HasActiveFilters() {
		t.Fatal("a navigated calendar period is an active filter")
	}
	repositories := []string{"repo-1"}
	located := plain.WithTab(dashboardview.TabRepo).WithLocations(dashboardview.LocationChange{Repositories: &repositories})
	for name, query := range map[string]dashboardview.Query{
		"custom":   plain.WithSelection(dashboardview.CustomRangeChange("2026-11-01", "")),
		"sessions": plain.WithSelection(dashboardview.DimensionChange(dashboardview.DimensionSessions, []string{"ses-1"})),
		"repo":     located,
	} {
		if !query.HasActiveFilters() || query.ClearFilters().HasActiveFilters() {
			t.Errorf("%s: active %v, after ClearFilters %v", name, query.HasActiveFilters(), query.ClearFilters().HasActiveFilters())
		}
	}
	// Location filters show only on Repo, as the web's.
	located.Tab = dashboardview.TabTokens
	if located.HasActiveFilters() {
		t.Fatal("location filters off Repo are active")
	}
}

func TestDimensionValuesAndChanges(t *testing.T) {
	plain := dashboardview.InitialQuery(analytics.Selection{Period: "month", Bucket: "day"}, dashboardview.TabTokens)
	facets := analytics.Facets{Harnesses: []string{"pi"}, Providers: []string{"openai"}, Models: []string{"gpt-5"}, Sessions: []string{"ses-1"}}
	for _, d := range dashboardview.FilterDimensions() {
		values := dashboardview.FacetValues(facets, d.Key)
		if len(values) != 1 {
			t.Fatalf("%s facet values %v", d.Key, values)
		}
		query := plain.WithSelection(dashboardview.DimensionChange(d.Key, values))
		got := query.Values(d.Key)
		if !reflect.DeepEqual(got, values) {
			t.Fatalf("%s values %v, want %v", d.Key, got, values)
		}
		values[0] = "changed"
		if query.Values(d.Key)[0] == "changed" {
			t.Fatalf("%s change aliases its values", d.Key)
		}
	}
}
