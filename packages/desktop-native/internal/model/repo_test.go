package model

import (
	"slices"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// onRepo is a harness whose window offers facets and shows Repo: its
// usage and facets answered, and its excluded usage read drained.
func onRepo(t *testing.T) *harness {
	h := facetsSynced(t)
	h.expectExcluded()
	h.window.SelectTab(dashboardview.TabRepo)
	usage, facets := h.expectUsageAndFacets()
	if usage.Tab != string(dashboardview.TabRepo) || usage.LocationGroup != analytics.RepoGroupRepository ||
		facets.Query.Tab != string(dashboardview.TabRepo) || facets.Query.LocationGroup != "" {
		t.Fatalf("repo requests %+v %+v", usage, facets)
	}
	h.answerUsage(usageAt(1, 1), nil)
	h.answerFacets(facetsAt(1, 1), nil)
	h.expectExcluded()
	return h
}

func TestLocationGroupRereadsUsageAndExcludedButNotFacets(t *testing.T) {
	h := onRepo(t)
	w := h.window
	w.SetLocationGroup(analytics.RepoGroupRepository) // the grouping shown
	h.noCall()

	w.SetLocationGroup(analytics.RepoGroupDirectory)
	request := h.expectUsage()
	h.noCall() // facets are not grouped
	if request.LocationGroup != analytics.RepoGroupDirectory || request.Page != 1 || request.Tab != string(dashboardview.TabRepo) {
		t.Fatalf("grouped request %+v", request)
	}
	if _, ok := w.Usage(); ok || w.UsageMatchesView() {
		t.Fatal("usage of another grouping stayed, as the web keeps none of another summary scope")
	}
	h.answerUsage(usageAt(1, 1), nil)
	if usage, _ := w.Usage(); usage.LocationGroup != analytics.RepoGroupDirectory || !w.UsageMatchesView() {
		t.Fatalf("directory usage %+v", usage)
	}
	if call := h.expectExcluded(); call.request.LocationGroup != analytics.RepoGroupDirectory || call.request.Tab != string(dashboardview.TabRepo) {
		t.Fatalf("excluded request %+v", call.request)
	}
}

func TestLocationFiltersReadUsageFacetsAndExcluded(t *testing.T) {
	h := onRepo(t)
	w := h.window
	w.UpdateLocations(dashboardview.LocationsChange(dashboardview.LocationRepositories, []string{"repo-1"}))
	usage, facets := h.expectUsageAndFacets()
	if !slices.Equal(usage.RepositoryKeys, []string{"repo-1"}) || !slices.Equal(facets.Query.RepositoryKeys, []string{"repo-1"}) {
		t.Fatalf("repository requests %+v %+v", usage, facets)
	}
	h.answerUsage(usageAt(1, 1), nil)
	h.answerFacets(facetsAt(1, 1), nil)
	if call := h.expectExcluded(); !slices.Equal(call.request.RepositoryKeys, []string{"repo-1"}) {
		t.Fatalf("excluded request %+v", call.request)
	}

	w.UpdateLocations(dashboardview.LocationsChange(dashboardview.LocationDirectories, []string{"directory-2", "unknown"}))
	usage, facets = h.expectUsageAndFacets()
	if !slices.Equal(usage.DirectoryKeys, []string{"directory-2", "unknown"}) || !slices.Equal(usage.RepositoryKeys, []string{"repo-1"}) ||
		!slices.Equal(facets.Query.DirectoryKeys, []string{"directory-2", "unknown"}) {
		t.Fatalf("directory requests %+v %+v", usage, facets)
	}
	h.answerUsage(usageAt(1, 1), nil)
	h.answerFacets(facetsAt(1, 1), nil)
	h.expectExcluded()

	w.ClearFilters()
	usage, _ = h.expectUsageAndFacets()
	if len(usage.RepositoryKeys) != 0 || len(usage.DirectoryKeys) != 0 {
		t.Fatalf("Clear All kept locations %+v", usage)
	}
}

func TestLeavingRepoDropsLocationFilters(t *testing.T) {
	h := onRepo(t)
	w := h.window
	w.SetLocationGroup(analytics.RepoGroupDirectory)
	h.expectUsage()
	h.answerUsage(usageAt(1, 1), nil)
	h.expectExcluded()
	w.UpdateLocations(dashboardview.LocationsChange(dashboardview.LocationDirectories, []string{"directory-2"}))
	h.expectUsageAndFacets()
	h.answerUsage(usageAt(1, 1), nil)
	h.answerFacets(facetsAt(1, 1), nil)
	h.expectExcluded()

	w.SelectTab(dashboardview.TabModels)
	usage, facets := h.expectUsageAndFacets()
	if usage.LocationGroup != "" || len(usage.DirectoryKeys) != 0 || facets.Query.Tab != "" || len(facets.Query.DirectoryKeys) != 0 {
		t.Fatalf("models requests %+v %+v", usage, facets)
	}
	h.answerUsage(usageAt(1, 1), nil)
	h.answerFacets(facetsAt(1, 1), nil)
	h.expectExcluded()

	w.SelectTab(dashboardview.TabRepo)
	usage, _ = h.expectUsageAndFacets()
	if len(usage.DirectoryKeys) != 0 || usage.LocationGroup != analytics.RepoGroupDirectory {
		t.Fatalf("returning to Repo: %+v; want no locations and the grouping kept, as dashboardview.Query.WithTab", usage)
	}
}

func TestLocationChangesBeforeTheInstanceDoNothing(t *testing.T) {
	h := newHarness(t)
	w := h.window
	w.SetLocationGroup(analytics.RepoGroupDirectory)
	w.UpdateLocations(dashboardview.LocationsChange(dashboardview.LocationRepositories, []string{"repo-1"}))
	h.noCall()
	if _, ok := w.Query(); ok {
		t.Fatal("query before the instance")
	}
}
