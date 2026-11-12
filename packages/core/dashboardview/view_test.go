package dashboardview_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

func labels(columns []dashboardview.Column) []string {
	names := make([]string, len(columns))
	for index, column := range columns {
		names[index] = column.Label
	}
	return names
}

func TestColumnsPerTab(t *testing.T) {
	tokens := []string{"Total", "Input", "Output", "Cache R", "Cache W", "Reasoning"}
	want := map[dashboardview.Tab][]string{
		dashboardview.TabTokens:    append([]string{"Period", "Sessions"}, tokens...),
		dashboardview.TabModels:    append([]string{"Model", "Sessions"}, tokens...),
		dashboardview.TabProviders: append([]string{"Provider", "Sessions"}, tokens...),
		dashboardview.TabHarnesses: append([]string{"Harness", "Sessions"}, tokens...),
		dashboardview.TabSessions:  append(append([]string{"Session", "Last active"}, tokens...), "Ctx used"),
		dashboardview.TabContext:   {"Harness", "Provider", "Model", "Sessions", "Avg ctx", "Median ctx", "Max ctx"},
		dashboardview.TabRepo:      append([]string{"Location", "Providers", "Harnesses", "Models", "Sessions"}, tokens...),
	}
	for _, tab := range dashboardview.Tabs() {
		if got := labels(dashboardview.Columns(tab)); !slices.Equal(got, want[tab]) {
			t.Errorf("Columns(%s) = %q, want %q", tab, got, want[tab])
		}
	}
	sessions := dashboardview.Columns(dashboardview.TabSessions)
	if sessions[1].ID != dashboardview.SortDate || sessions[1].Numeric || !sessions[len(sessions)-1].Numeric {
		t.Errorf("sessions columns = %+v", sessions)
	}
}

func TestColumnWidths(t *testing.T) {
	name := dashboardview.Column{ID: dashboardview.SortName}
	cases := []struct {
		tab    dashboardview.Tab
		column dashboardview.Column
		want   dashboardview.ColumnWidth
	}{
		{dashboardview.TabModels, name, dashboardview.ColumnWidth{Default: 224, Min: 176}},
		{dashboardview.TabRepo, name, dashboardview.ColumnWidth{Default: 256, Min: 176}},
		{dashboardview.TabModels, dashboardview.Column{ID: dashboardview.SortTotal, Numeric: true}, dashboardview.ColumnWidth{Default: 128, Min: 112}},
		{dashboardview.TabContext, dashboardview.Column{ID: dashboardview.SortModel}, dashboardview.ColumnWidth{Default: 160, Min: 128}},
	}
	for _, c := range cases {
		if got := dashboardview.ColumnWidths(c.tab, c.column); got != c.want {
			t.Errorf("ColumnWidths(%s, %s) = %+v, want %+v", c.tab, c.column.ID, got, c.want)
		}
	}
	if !dashboardview.ColumnVisible(dashboardview.SortName, []dashboardview.Sort{dashboardview.SortName}) ||
		dashboardview.ColumnVisible(dashboardview.SortInput, []dashboardview.Sort{dashboardview.SortInput}) {
		t.Error("name must stay visible and hidden columns must hide")
	}
}

func TestSortOptionsAndHeaders(t *testing.T) {
	want := []string{"Date", "Name", "Total", "Input", "Output", "Cache R", "Cache W", "Reasoning", "Sessions", "Ctx used"}
	if got := labels(dashboardview.SortOptions(dashboardview.TabModels)); !slices.Equal(got, want) {
		t.Errorf("SortOptions(models) = %q", got)
	}
	if got := dashboardview.SortOptions(dashboardview.TabContext); !reflect.DeepEqual(got, dashboardview.Columns(dashboardview.TabContext)) {
		t.Errorf("SortOptions(context) = %+v", got)
	}
	if dashboardview.HeaderSort(dashboardview.TabTokens, dashboardview.SortName) != dashboardview.SortDate ||
		dashboardview.HeaderSort(dashboardview.TabModels, dashboardview.SortName) != dashboardview.SortName {
		t.Error("only Tokens period headers sort by date")
	}
}

func TestTableTitle(t *testing.T) {
	cases := map[string]string{
		dashboardview.TableTitle(dashboardview.TabModels, dashboardview.QualityConfirmed): "Models breakdown",
		dashboardview.TableTitle(dashboardview.TabModels, dashboardview.QualityEstimated): "Excluded models breakdown",
		dashboardview.TableTitle(dashboardview.TabRepo, dashboardview.QualityConfirmed):   "Repo totals",
		dashboardview.TableTitle(dashboardview.TabRepo, dashboardview.QualityEstimated):   "Excluded repo totals",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("title = %q, want %q", got, want)
		}
	}
}

func TestIdentityDetailsAndSummaryLines(t *testing.T) {
	row := analytics.Row{Harness: "codex", Provider: "openai, anthropic", Model: "codex", RepositoryName: "repo", LocationName: "dir"}
	cases := []struct {
		tab  dashboardview.Tab
		row  analytics.Row
		want []string
	}{
		{dashboardview.TabTokens, row, []string{}},
		{dashboardview.TabModels, row, []string{"codex", "openai", "anthropic"}},
		{dashboardview.TabHarnesses, row, []string{"openai", "anthropic", "codex"}},
		{dashboardview.TabSessions, row, []string{"codex", "openai", "anthropic"}},
		{dashboardview.TabRepo, row, []string{"repo"}},
		{dashboardview.TabRepo, analytics.Row{RepositoryName: "same", LocationName: "same"}, []string{}},
		{dashboardview.TabRepo, analytics.Row{RepositoryName: "unknown", LocationName: "dir"}, []string{}},
		{dashboardview.TabProviders, analytics.Row{Model: "m"}, []string{"m"}},
	}
	for _, c := range cases {
		if got := dashboardview.IdentityDetails(c.row, c.tab); !slices.Equal(got, c.want) {
			t.Errorf("IdentityDetails(%s) = %q, want %q", c.tab, got, c.want)
		}
	}
	if got := dashboardview.SummaryLines(" a ,\u00a0b\ufeff,, \u0085c"); !slices.Equal(got, []string{"a", "b", "\u0085c"}) {
		t.Errorf("SummaryLines = %q", got)
	}
}

func TestChartRules(t *testing.T) {
	query := func(tab dashboardview.Tab, quality dashboardview.Quality) dashboardview.Query {
		q := dashboardview.InitialQuery(analytics.Selection{Period: "week", Bucket: "day"}, tab).WithQuality(quality)
		return q.WithLocationGroup(analytics.RepoGroupDirectory)
	}
	titles := []struct {
		tab                 dashboardview.Tab
		confirmed, excluded string
	}{
		{dashboardview.TabTokens, "Usage over time", "Excluded usage over time"},
		{dashboardview.TabSessions, "Usage over time", "Excluded usage over time"},
		{dashboardview.TabModels, "Usage by model", "Excluded usage by model"},
		{dashboardview.TabProviders, "Usage by provider", "Excluded usage by provider"},
		{dashboardview.TabHarnesses, "Usage by harness", "Excluded usage by harness"},
		{dashboardview.TabContext, "Session peak context", "Excluded session peak context"},
		{dashboardview.TabRepo, "Usage by directory", "Excluded usage by directory"},
	}
	for _, c := range titles {
		if got := dashboardview.ChartTitle(query(c.tab, dashboardview.QualityConfirmed)); got != c.confirmed {
			t.Errorf("ChartTitle(%s) = %q, want %q", c.tab, got, c.confirmed)
		}
		if got := dashboardview.ChartTitle(query(c.tab, dashboardview.QualityEstimated)); got != c.excluded {
			t.Errorf("excluded ChartTitle(%s) = %q, want %q", c.tab, got, c.excluded)
		}
	}
	if dashboardview.ChartUnitLabel(query(dashboardview.TabContext, dashboardview.QualityEstimated)) != "Prompt-side tokens" ||
		dashboardview.ChartUnitLabel(query(dashboardview.TabModels, dashboardview.QualityEstimated)) != "Excluded tokens" ||
		dashboardview.ChartUnitLabel(query(dashboardview.TabRepo, dashboardview.QualityConfirmed)) != "Countable tokens" {
		t.Error("unit labels")
	}
	if dashboardview.ChartDescription(dashboardview.TabRepo) != "Top 12 location rows by total tokens · % of filtered total" ||
		dashboardview.ChartDescription(dashboardview.TabContext) != "Top 12 groups by average in-range session peak" {
		t.Error("descriptions")
	}
	for _, tab := range dashboardview.Tabs() {
		dimension, filters := dashboardview.ChartFilterDimension(tab)
		wantFilters := tab == dashboardview.TabModels || tab == dashboardview.TabProviders || tab == dashboardview.TabHarnesses
		if filters != wantFilters || filters && string(dimension) != string(tab) {
			t.Errorf("ChartFilterDimension(%s) = %q, %v", tab, dimension, filters)
		}
		if shares := dashboardview.ChartShowsTokenShares(tab); shares != (wantFilters || tab == dashboardview.TabRepo) {
			t.Errorf("ChartShowsTokenShares(%s) = %v", tab, shares)
		}
	}
	row := analytics.Row{Name: "n", Model: "m", Harness: "h", Provider: "p"}
	if dashboardview.ChartRowLabel(row, dashboardview.TabContext) != "m · h · p" || dashboardview.ChartRowLabel(row, dashboardview.TabModels) != "n" {
		t.Error("row labels")
	}
	if dashboardview.ChartColorIndex(12) != 0 || dashboardview.ChartColorIndex(13) != 1 {
		t.Error("palette cycles every 12 colors")
	}
}

// TestBucketOptionsFollowTheAPIEnum pins the bucket choices to the API's
// Bucket enum, the source of the web's bucketSchema options.
// openAPIEnum reads the values of the string enum schema name in
// docs/openapi.yaml.
func openAPIEnum(t *testing.T, name string) []string {
	t.Helper()
	spec, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, after, ok := strings.Cut(string(spec), "\n    "+name+":\n      type: string\n      enum:\n")
	if !ok {
		t.Fatalf("no %s enum in docs/openapi.yaml", name)
	}
	var enum []string
	for line := range strings.Lines(after) {
		value, ok := strings.CutPrefix(strings.TrimRight(line, "\n"), "        - ")
		if !ok {
			break
		}
		enum = append(enum, value)
	}
	return enum
}

func TestBucketOptionsFollowTheAPIEnum(t *testing.T) {
	enum := openAPIEnum(t, "Bucket")
	var keys []string
	for _, option := range dashboardview.BucketOptions() {
		keys = append(keys, option.Key)
		if want := strings.ToUpper(option.Key[:1]) + option.Key[1:]; option.Label != want {
			t.Errorf("bucket %q label %q, want %q", option.Key, option.Label, want)
		}
		if err := (analytics.Selection{Period: "week", Bucket: option.Key}).Validate(); err != nil {
			t.Errorf("bucket %q: %v", option.Key, err)
		}
	}
	if !slices.Equal(keys, enum) {
		t.Errorf("buckets %q, API enum %q", keys, enum)
	}
}

func TestTruncateChartLabel(t *testing.T) {
	cases := map[string]string{
		"exactly-17-chars!":    "exactly-17-chars!",
		"eighteen-characters":  "eighteen-charac…",
		"claude-sonnet-4-5-x":  "claude-sonnet-4…",
		"modèle-français-long": "modèle-français…",
		"abcdefghijklmn😀xyz":   "abcdefghijklmn…",
		"abcdefghijklm😀xyzabc": "abcdefghijklm😀…",
	}
	for label, want := range cases {
		if got := dashboardview.TruncateChartLabel(label); got != want {
			t.Errorf("TruncateChartLabel(%q) = %q, want %q", label, got, want)
		}
	}
}

func attempt(id, stage string, updated int64) collectorprogress.Attempt {
	return collectorprogress.Attempt{AttemptID: id, Stage: stage, UpdatedAtMS: updated}
}

func TestDeriveSync(t *testing.T) {
	status := &application.Status{InstanceID: "i", Readiness: "ready", Metadata: application.Publication{
		DatabaseID: "db", DatasetID: "d", Revision: 7, Generation: 2, TargetGeneration: 2,
	}}
	ready := dashboardview.DeriveSync(dashboardview.SyncInput{Status: status, DatasetID: "d"})
	if ready.Loading || ready.ProcessingPending || !ready.AnalyticsEnabled || ready.Revision != 7 || ready.Identity != "i/db/d/2" {
		t.Errorf("ready = %+v", ready)
	}
	if got := dashboardview.DeriveSync(dashboardview.SyncInput{DatasetID: "d"}); !got.Loading || got.AnalyticsEnabled {
		t.Errorf("no status = %+v", got)
	}
	if got := dashboardview.DeriveSync(dashboardview.SyncInput{Status: status, DatasetID: "other"}); got.AnalyticsEnabled {
		t.Error("another dataset must not enable analytics")
	}
	lagging := *status
	lagging.Metadata.TargetGeneration, lagging.Failed, lagging.FailedRetryAtMs = 3, 1, 2000
	backoff := dashboardview.DeriveSync(dashboardview.SyncInput{Status: &lagging, StatusReadAtMs: 1000, DatasetID: "d"})
	if !backoff.ProcessingPending || !backoff.ProcessingBackoff || !backoff.Loading {
		t.Errorf("backoff = %+v", backoff)
	}
	if retried := dashboardview.DeriveSync(dashboardview.SyncInput{Status: &lagging, StatusReadAtMs: 2000, DatasetID: "d"}); retried.ProcessingBackoff {
		t.Error("a due retry is not backoff")
	}

	attempts := []collectorprogress.Attempt{attempt("a", "accepted", 5), attempt("b", "capturing", 3), attempt("c", "waiting", 4)}
	active := dashboardview.DeriveSync(dashboardview.SyncInput{Status: status, DatasetID: "d", ProgressEnabled: true, Attempts: attempts})
	if active.CollectionStage != "capturing" || !active.CollectionPending || !active.CheckingAcceptance || active.CollectionFailed {
		t.Errorf("active = %+v", active)
	}
	if transition := dashboardview.AcceptedTransition(attempts); transition != "a/5" {
		t.Errorf("AcceptedTransition = %q", transition)
	}
	checked := dashboardview.DeriveSync(dashboardview.SyncInput{Status: status, DatasetID: "d", ProgressEnabled: true,
		Attempts: []collectorprogress.Attempt{attempt("a", "accepted", 5)}, CheckedAcceptance: "a/5"})
	if checked.CollectionPending || checked.CheckingAcceptance || checked.Loading {
		t.Errorf("checked = %+v", checked)
	}
	unknown := dashboardview.DeriveSync(dashboardview.SyncInput{Status: status, DatasetID: "d", ProgressEnabled: true, Attempts: attempts, ProgressFailed: true})
	if !unknown.CollectionUnknown || unknown.CollectionPending || unknown.CollectionStage != "" {
		t.Errorf("unknown = %+v", unknown)
	}
	failed := dashboardview.DeriveSync(dashboardview.SyncInput{Status: status, DatasetID: "d", ProgressEnabled: true,
		Attempts: []collectorprogress.Attempt{attempt("a", "accepted", 5), attempt("b", "interrupted", 5)}})
	if !failed.CollectionFailed {
		t.Error("ties pick the later attempt")
	}
	if disabled := dashboardview.DeriveSync(dashboardview.SyncInput{Status: status, DatasetID: "d", Attempts: attempts, ProgressPending: true}); disabled.CollectionPending || disabled.CollectionStage != "" {
		t.Errorf("disabled progress = %+v", disabled)
	}
}

func TestDashboardStatusPriority(t *testing.T) {
	ready := dashboardview.SyncState{AnalyticsEnabled: true}
	cases := []struct {
		name  string
		input dashboardview.StatusInput
		want  dashboardview.StatusValue
	}{
		{"connection first", dashboardview.StatusInput{ConnectionFailed: true, RefreshFailed: true}, dashboardview.StatusValue{Label: "Connection unavailable", State: dashboardview.StatusAttention}},
		{"refresh failure", dashboardview.StatusInput{RefreshFailed: true, RefreshPending: true}, dashboardview.StatusValue{Label: "Refresh failed", State: dashboardview.StatusAttention}},
		{"refreshing", dashboardview.StatusInput{RefreshPending: true, UsageFailed: true}, dashboardview.StatusValue{Label: "Refreshing usage…", State: dashboardview.StatusLoading}},
		{"usage failure", dashboardview.StatusInput{UsageFailed: true, FacetsFailed: true}, dashboardview.StatusValue{Label: "Usage couldn’t load", State: dashboardview.StatusAttention}},
		{"facets failure", dashboardview.StatusInput{FacetsFailed: true, Sync: dashboardview.SyncState{CollectionUnknown: true}}, dashboardview.StatusValue{Label: "Filters couldn’t load", State: dashboardview.StatusAttention}},
		{"progress unknown", dashboardview.StatusInput{Sync: dashboardview.SyncState{CollectionUnknown: true, CollectionFailed: true}}, dashboardview.StatusValue{Label: "Collection status unavailable", State: dashboardview.StatusAttention}},
		{"collection failed", dashboardview.StatusInput{Sync: dashboardview.SyncState{CollectionFailed: true, CollectionStage: "capturing"}}, dashboardview.StatusValue{Label: "Collection needs attention", State: dashboardview.StatusAttention}},
		{"collecting", dashboardview.StatusInput{Sync: dashboardview.SyncState{CollectionStage: "submitting", ProcessingPending: true}}, dashboardview.StatusValue{Label: "Submitting usage", State: dashboardview.StatusLoading}},
		{"retry pending", dashboardview.StatusInput{Sync: dashboardview.SyncState{ProcessingPending: true, ProcessingBackoff: true}}, dashboardview.StatusValue{Label: "Processing retry pending", State: dashboardview.StatusAttention}},
		{"processing", dashboardview.StatusInput{Sync: dashboardview.SyncState{ProcessingPending: true, Loading: true}}, dashboardview.StatusValue{Label: "Processing usage", State: dashboardview.StatusLoading}},
		{"sync loading", dashboardview.StatusInput{Sync: dashboardview.SyncState{Loading: true}}, dashboardview.StatusValue{Label: "Loading…", State: dashboardview.StatusLoading}},
		{"usage refetch", dashboardview.StatusInput{Sync: ready, UsageCapability: true, UsageFetching: true, UsageLoaded: true}, dashboardview.StatusValue{Label: "Refreshing usage…", State: dashboardview.StatusLoading}},
		{"usage first load", dashboardview.StatusInput{Sync: ready, UsageCapability: true, UsageFetching: true}, dashboardview.StatusValue{Label: "Loading…", State: dashboardview.StatusLoading}},
		{"usage without capability", dashboardview.StatusInput{Sync: ready, UsageFetching: true, FacetsCapability: true, FacetsPending: true}, dashboardview.StatusValue{Label: "Loading filters…", State: dashboardview.StatusLoading}},
		{"analytics disabled", dashboardview.StatusInput{UsageCapability: true, UsageFetching: true, FacetsCapability: true, FacetsPending: true}, dashboardview.StatusValue{Label: "Ready", State: dashboardview.StatusReady}},
		{"ready", dashboardview.StatusInput{Sync: ready, UsageCapability: true, FacetsCapability: true, UsageLoaded: true}, dashboardview.StatusValue{Label: "Ready", State: dashboardview.StatusReady}},
	}
	for _, c := range cases {
		if got := dashboardview.DashboardStatus(c.input); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestCollectorProgressLabels(t *testing.T) {
	attempts := []collectorprogress.Attempt{
		attempt("old", "accepted", 1), attempt("run", "capturing", 2), attempt("new", "failed", 3), attempt("wait", "waiting", 0),
	}
	var ids []string
	for _, visible := range dashboardview.VisibleAttempts(attempts) {
		ids = append(ids, visible.AttemptID)
	}
	if !slices.Equal(ids, []string{"run", "wait", "new"}) {
		t.Errorf("VisibleAttempts = %q", ids)
	}
	submission := attempt("s", "failed", 1)
	submission.ErrorCode = "submission_failed"
	if dashboardview.AttemptLabel(submission) != "Submission failed" || dashboardview.AttemptLabel(attempt("i", "interrupted", 1)) != "Collection interrupted" {
		t.Error("attempt labels")
	}
	if dashboardview.HarnessStateLabel("skipped") != "No new usage" || dashboardview.HarnessStateLabel("running") != "Collecting" {
		t.Error("harness labels")
	}
}

func TestFilterOptionsKeepEqualNamesInFacetOrder(t *testing.T) {
	known := []dashboardview.FilterOption{{Key: "b", Name: "same"}, {Key: "a", Name: "same"}}
	options := dashboardview.FilterOptions(known, []string{"a"}, "", "")
	if !reflect.DeepEqual(options, known) {
		t.Errorf("options = %+v", options)
	}
	names := dashboardview.SelectedFilterNames([]dashboardview.FilterOption{{Key: "k", Name: "name"}}, []string{"missing", "k", "unknown"}, "Repository (unavailable)")
	if !slices.Equal(names, []string{"Repository (unavailable)", "name", "unknown"}) {
		t.Errorf("SelectedFilterNames = %q", names)
	}
}

func TestRowValuesAndTexts(t *testing.T) {
	row := analytics.Row{
		Name: "gpt-5", Harness: "codex", Provider: "openai", Model: "gpt-5", Total: 1, Input: 2, Output: 3, Reasoning: 4,
		CacheRead: 5, CacheWrite: 6, Sessions: 7, Context: 8, AverageContext: 9, MedianContext: 10, MaxContext: 11,
	}
	for tab, want := range map[dashboardview.Tab][]int64{
		dashboardview.TabSessions: {0, 0, 1, 2, 3, 5, 6, 4, 8},
		dashboardview.TabContext:  {0, 0, 0, 7, 9, 10, 11},
	} {
		columns := dashboardview.Columns(tab)
		for i, column := range columns {
			value, ok := dashboardview.RowValue(row, column.ID)
			if ok != column.Numeric || value != want[i] {
				t.Errorf("%s %s = %d, %v", tab, column.ID, value, ok)
			}
		}
	}
	for column, want := range map[dashboardview.Sort]string{
		dashboardview.SortName: "gpt-5", dashboardview.SortHarness: "codex", dashboardview.SortProvider: "openai",
		dashboardview.SortModel: "gpt-5", dashboardview.SortDate: "",
	} {
		if got := dashboardview.RowText(row, column); got != want {
			t.Errorf("RowText(%s) = %q, want %q", column, got, want)
		}
	}
}

func TestLocationGroupsFollowTheAPIEnum(t *testing.T) {
	var groups []string
	for _, option := range dashboardview.LocationGroups() {
		key := string(option.Group)
		groups = append(groups, key)
		if want := strings.ToUpper(key[:1]) + key[1:]; option.Label != want {
			t.Errorf("group %q label %q, want %q", key, option.Label, want)
		}
	}
	if enum := openAPIEnum(t, "LocationGroup"); !slices.Equal(groups, enum) {
		t.Errorf("location groups %q, API enum %q", groups, enum)
	}
}

func TestLocationFiltersAsTheWebToolbar(t *testing.T) {
	facets := analytics.Facets{
		Repositories: []analytics.LocationOption{{Key: "repo-1", Name: "tokeninsights"}, {Key: "unknown", Name: ""}},
		Directories:  []analytics.LocationOption{{Key: "directory-2", Name: "~/workspace/client-a"}},
	}
	want := map[dashboardview.LocationDimension][]dashboardview.FilterOption{
		dashboardview.LocationRepositories: {{Key: "repo-1", Name: "tokeninsights"}, {Key: "unknown", Name: "unknown"}},
		dashboardview.LocationDirectories:  {{Key: "directory-2", Name: "~/workspace/client-a"}},
	}
	plain := dashboardview.InitialQuery(analytics.Selection{Period: "month", Bucket: "day"}, dashboardview.TabRepo)
	for _, d := range dashboardview.LocationDimensions() {
		values := dashboardview.LocationValues(facets, d.Key)
		if !reflect.DeepEqual(values, want[d.Key]) {
			t.Fatalf("%s values %+v, want %+v", d.Key, values, want[d.Key])
		}
		keys := []string{values[0].Key, "gone"}
		query := plain.WithLocations(dashboardview.LocationsChange(d.Key, keys))
		if got := query.Locations(d.Key); !slices.Equal(got, keys) {
			t.Fatalf("%s locations %v, want %v", d.Key, got, keys)
		}
		keys[0] = "changed"
		if query.Locations(d.Key)[0] == "changed" {
			t.Fatalf("%s change aliases its keys", d.Key)
		}
		names := dashboardview.SelectedFilterNames(values, query.Locations(d.Key), d.MissingName())
		if wantNames := []string{values[0].Name, d.Label + " (unavailable)"}; !slices.Equal(names, wantNames) {
			t.Fatalf("%s names %q, want %q", d.Key, names, wantNames)
		}
	}
	if dashboardview.LocationValues(facets, "sessions") != nil || dashboardview.LocationsChange("sessions", []string{"x"}) != (dashboardview.LocationChange{}) {
		t.Fatal("an unknown location dimension filters")
	}
}

func TestIsUnknownLocationOnlyOnRepo(t *testing.T) {
	unknown := analytics.Row{Key: "unknown\x00", Name: "unknown", LocationKey: "unknown", RepositoryName: "unknown"}
	known := analytics.Row{Key: "repo-1\x00", Name: "tokeninsights", LocationKey: "repo-1"}
	if !dashboardview.IsUnknownLocation(unknown, dashboardview.TabRepo) || dashboardview.IsUnknownLocation(known, dashboardview.TabRepo) ||
		dashboardview.IsUnknownLocation(unknown, dashboardview.TabModels) {
		t.Fatal("unknown location outside a Repo row without a location")
	}
}
