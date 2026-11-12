package model

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// facetsAt are facets of testInstance's dataset answered at revision and
// generation, as statusAt describes, offering sessions.
func facetsAt(revision, generation int64, sessions ...string) application.Facets {
	return application.Facets{InstanceID: testInstance.InstanceID, Facets: analytics.Facets{
		DatabaseID: "db-1", DatasetID: testInstance.DatasetID, Revision: revision, Generation: generation,
		Harnesses: []string{"codex", "pi"}, Providers: []string{"openai"}, Models: []string{"gpt-5"}, Sessions: sessions,
	}}
}

// expectFacets waits for a facets read and returns its request.
func (h *harness) expectFacets() application.FacetRequest {
	h.t.Helper()
	h.expectCall("facets")
	return <-h.backend.facetReqs
}

// answerFacets answers a running facets read and applies its result.
func (h *harness) answerFacets(facets application.Facets, err error) {
	h.backend.facets <- facetsResult{facets: facets, err: err}
	h.apply()
}

// expectUsageAndFacets waits for the usage and facets reads a change
// starts together and returns their requests.
func (h *harness) expectUsageAndFacets() (analytics.Query, application.FacetRequest) {
	h.t.Helper()
	h.expectCalls("usage", "facets")
	return <-h.backend.requests, <-h.backend.facetReqs
}

// facetsSynced is a harness whose window loaded facetsInstance, read
// status at revision 1, and the usage and facets it then read.
func facetsSynced(t *testing.T) *harness {
	h := newHarness(t)
	h.loaded(facetsInstance)
	h.window.Poll(at(0), false)
	h.answerReads(statusAt(1, 1), progressOf())
	h.expectUsageAndFacets()
	h.answerUsage(usageAt(1, 1), nil)
	h.answerFacets(facetsAt(1, 1, "ses-1", "ses-2"), nil)
	return h
}

func TestFacetsReadTheQueryWithoutSearchOnceStatusIsReady(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(facetsInstance)
	w.Poll(at(0), false)
	h.answerReads(statusAt(1, 1), progressOf())
	_, request := h.expectUsageAndFacets()
	query, _ := w.Query()
	if want := query.FacetRequest(""); request.Search != "" || !sameRequest(request.Query, want.Query) {
		t.Fatalf("facets request %+v, want %+v", request, want)
	}
	if _, ok := w.Facets(); ok || !w.FacetsPending() {
		t.Fatal("facets before any answer")
	}
	h.answerUsage(usageAt(1, 1), nil)
	assertStatus(t, w, "Loading filters…", dashboardview.StatusLoading)
	h.answerFacets(facetsAt(1, 1, "ses-1"), nil)
	facets, ok := w.Facets()
	if !ok || !slices.Equal(facets.Harnesses, []string{"codex", "pi"}) || w.FacetsPending() {
		t.Fatalf("facets %+v, %v", facets, ok)
	}
	assertStatus(t, w, "Ready", dashboardview.StatusReady)

	w.Poll(at(PollInterval), false)
	h.answerReads(statusAt(1, 1), progressOf())
	h.noCall() // an unchanged snapshot keeps its facets
}

func TestFacetsWaitForTheirCapability(t *testing.T) {
	h := synced(t) // testInstance offers no facets
	h.window.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionModels, []string{"gpt-5"}))
	h.expectUsage()
	h.noCall()
	h.answerUsage(usageAt(1, 1), nil)
	assertStatus(t, h.window, "Ready", dashboardview.StatusReady)
}

func TestFilterChangeRereadsFacetsKeepingTheValuesShown(t *testing.T) {
	h := facetsSynced(t)
	w := h.window
	w.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionModels, []string{"gpt-5"}))
	usage, request := h.expectUsageAndFacets()
	if !slices.Equal(request.Query.Selection.Models, []string{"gpt-5"}) || !slices.Equal(usage.Selection.Models, []string{"gpt-5"}) {
		t.Fatalf("filtered requests %+v %+v", usage, request)
	}
	if _, ok := w.Facets(); !ok || !w.FacetsPending() {
		t.Fatal("a filter change dropped the values shown")
	}
	h.answerUsage(usageAt(1, 1), nil)
	h.answerFacets(facetsAt(1, 1, "ses-2"), nil)
	if facets, _ := w.Facets(); !slices.Equal(facets.Sessions, []string{"ses-2"}) || w.FacetsPending() {
		t.Fatalf("facets %+v", facets)
	}

	// Sorting changes no facets request.
	w.SortBy(dashboardview.SortTotal)
	h.expectUsage()
	h.noCall()
}

func TestRevisionChangeRereadsFacets(t *testing.T) {
	h := facetsSynced(t)
	w := h.window
	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(2, 1), nil)
	h.expectCalls("instance", "usage", "facets")
	<-h.backend.requests
	<-h.backend.facetReqs
	if facets, ok := w.Facets(); !ok || facets.Revision != 1 || !w.FacetsPending() {
		t.Fatal("a revision change dropped the facets shown")
	}
	h.backend.instances <- instanceResult{instance: facetsInstance}
	h.apply()
	h.answerUsage(usageAt(2, 1), nil)
	h.answerFacets(facetsAt(2, 1), nil)
	if facets, _ := w.Facets(); facets.Revision != 2 || w.FacetsPending() {
		t.Fatalf("facets revision %d", facets.Revision)
	}
}

func TestIdentityChangeDropsFacets(t *testing.T) {
	h := facetsSynced(t)
	w := h.window
	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(1, 2), nil)
	if _, ok := w.Facets(); ok {
		t.Fatal("facets of another snapshot identity stayed")
	}
	h.expectCalls("instance", "usage", "facets")
	<-h.backend.requests
	<-h.backend.facetReqs
	h.backend.instances <- instanceResult{instance: facetsInstance}
	h.apply()
	h.answerUsage(usageAt(1, 2), nil)
	h.answerFacets(facetsAt(1, 2), nil)
	if facets, ok := w.Facets(); !ok || facets.Generation != 2 {
		t.Fatal("facets of the new identity")
	}
}

func TestSupersededFacetsAreDropped(t *testing.T) {
	h := facetsSynced(t)
	w := h.window
	w.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionModels, []string{"gpt-5"}))
	h.expectUsageAndFacets()
	w.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionModels, []string{"o3"}))
	h.apply() // the cancelled usage read answers late
	h.apply() // the cancelled facets read answers late
	if w.FacetsError() != nil || !w.FacetsPending() {
		t.Fatalf("a superseded facets read applied: %v", w.FacetsError())
	}
	_, request := h.expectUsageAndFacets()
	if !slices.Equal(request.Query.Selection.Models, []string{"o3"}) {
		t.Fatalf("latest facets request %+v", request)
	}
}

func TestFacetsFromAnotherSnapshotRereadStatusThenFacetsOnce(t *testing.T) {
	h := facetsSynced(t)
	w := h.window
	w.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionHarnesses, []string{"pi"}))
	h.expectUsageAndFacets()
	h.answerUsage(usageAt(1, 1), nil)
	h.answerFacets(facetsAt(1, 2), nil) // another generation
	h.expectCall("status")
	if facets, _ := w.Facets(); facets.Generation != 1 || w.FacetsError() != nil || !w.FacetsPending() {
		t.Fatal("facets of another snapshot were shown")
	}
	h.answerStatus(statusAt(1, 1), nil)
	h.expectFacets()                    // the status kept its snapshot: read once more
	h.answerFacets(facetsAt(0, 1), nil) // an older revision
	h.expectCall("status")
	h.answerStatus(statusAt(1, 1), nil)
	h.noCall()
	if !errors.Is(w.FacetsError(), ErrDataChanged) {
		t.Fatalf("facets error %v", w.FacetsError())
	}
	assertStatus(t, w, "Filters couldn’t load", dashboardview.StatusAttention)
}

func TestFacetsFailureWaitsForRetry(t *testing.T) {
	h := facetsSynced(t)
	w := h.window
	w.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionProviders, []string{"openai"}))
	h.expectUsageAndFacets()
	h.answerUsage(usageAt(1, 1), nil)
	h.answerFacets(application.Facets{}, errors.New("facets unavailable"))
	if _, ok := w.Facets(); !ok || w.FacetsError() == nil || w.FacetsPending() {
		t.Fatal("a failed read dropped the facets or lost its error")
	}
	assertStatus(t, w, "Filters couldn’t load", dashboardview.StatusAttention)

	w.Poll(at(PollInterval), false)
	h.answerReads(statusAt(1, 1), progressOf())
	h.noCall() // a failed read waits for Retry

	w.RetryFacets()
	if request := h.expectFacets(); !slices.Equal(request.Query.Selection.Providers, []string{"openai"}) {
		t.Fatalf("retry request %+v", request)
	}
	if w.FacetsError() != nil {
		t.Fatal("retry kept the error")
	}
	h.answerFacets(facetsAt(1, 1), nil)
	assertStatus(t, w, "Ready", dashboardview.StatusReady)
}

func TestSessionSearchWaitsForTypingToRest(t *testing.T) {
	h := facetsSynced(t)
	w := h.window
	if choices := w.SessionChoices(); choices.Searching || !slices.Equal(choices.Values, []string{"ses-1", "ses-2"}) {
		t.Fatalf("choices without search %+v", choices)
	}
	w.SearchSessions("ab", at(0))
	if wait, ok := w.PollSessionSearch(at(SessionSearchDelay / 2)); !ok || wait != SessionSearchDelay/2 {
		t.Fatalf("wait %v, %v", wait, ok)
	}
	if choices := w.SessionChoices(); !choices.Searching || !choices.Loading || len(choices.Values) != 0 {
		t.Fatalf("choices while typing %+v", choices)
	}
	typed := at(SessionSearchDelay * 3 / 4)
	w.SearchSessions("abc", typed) // typing again restarts the wait
	if wait, ok := w.PollSessionSearch(at(SessionSearchDelay)); !ok || wait != typed.Add(SessionSearchDelay).Sub(at(SessionSearchDelay)) {
		t.Fatalf("wait after typing again %v, %v", wait, ok)
	}
	h.noCall()
	if _, ok := w.PollSessionSearch(typed.Add(SessionSearchDelay)); ok {
		t.Fatal("a searched text still waits")
	}
	request := h.expectFacets()
	query, _ := w.Query()
	if want := query.FacetRequest("abc"); request.Search != "abc" || !sameRequest(request.Query, want.Query) {
		t.Fatalf("search request %+v", request)
	}
	if choices := w.SessionChoices(); !choices.Loading {
		t.Fatalf("choices while searching %+v", choices)
	}
	h.answerFacets(facetsAt(1, 1, "abc-1"), nil)
	if choices := w.SessionChoices(); choices.Loading || choices.Err != nil || !slices.Equal(choices.Values, []string{"abc-1"}) {
		t.Fatalf("matches %+v", choices)
	}
	if facets, _ := w.Facets(); !slices.Equal(facets.Sessions, []string{"ses-1", "ses-2"}) {
		t.Fatal("a session search replaced the query's facets")
	}

	// Typing more hides the matches until the new text is searched.
	w.SearchSessions("abcd", at(time.Second))
	if choices := w.SessionChoices(); !choices.Loading || len(choices.Values) != 0 {
		t.Fatalf("stale matches %+v", choices)
	}
	// Clearing the text offers the query's sessions at once and searches
	// nothing.
	w.SearchSessions("", at(time.Second))
	w.PollSessionSearch(at(time.Second + SessionSearchDelay))
	h.noCall()
	if choices := w.SessionChoices(); choices.Searching || !slices.Equal(choices.Values, []string{"ses-1", "ses-2"}) {
		t.Fatalf("choices after clearing %+v", choices)
	}
}

func TestSupersededSessionSearchIsDropped(t *testing.T) {
	h := facetsSynced(t)
	w := h.window
	w.SearchSessions("ab", at(0))
	w.PollSessionSearch(at(SessionSearchDelay))
	h.expectFacets()
	w.SearchSessions("ac", at(time.Second))
	w.PollSessionSearch(at(time.Second + SessionSearchDelay))
	h.apply() // the cancelled "ab" read answers late
	if choices := w.SessionChoices(); len(choices.Values) != 0 || !choices.Loading {
		t.Fatalf("a superseded search applied: %+v", choices)
	}
	if request := h.expectFacets(); request.Search != "ac" {
		t.Fatalf("latest search %+v", request)
	}
	h.answerFacets(facetsAt(1, 1, "ac-1"), nil)
	if choices := w.SessionChoices(); !slices.Equal(choices.Values, []string{"ac-1"}) {
		t.Fatalf("matches %+v", choices)
	}
}

func TestSessionSearchFailureRetries(t *testing.T) {
	h := facetsSynced(t)
	w := h.window
	w.SearchSessions("ab", at(0))
	w.PollSessionSearch(at(SessionSearchDelay))
	h.expectFacets()
	h.answerFacets(application.Facets{}, errors.New("search unavailable"))
	if choices := w.SessionChoices(); choices.Err == nil || choices.Loading {
		t.Fatalf("failed search %+v", choices)
	}
	if w.FacetsError() != nil {
		t.Fatal("a failed session search failed the filter values")
	}
	w.RetrySessionSearch()
	if request := h.expectFacets(); request.Search != "ab" {
		t.Fatalf("retry %+v", request)
	}
	if choices := w.SessionChoices(); choices.Err != nil || !choices.Loading {
		t.Fatalf("retrying search %+v", choices)
	}
	h.answerFacets(facetsAt(1, 1, "ab-1"), nil)
	if choices := w.SessionChoices(); !slices.Equal(choices.Values, []string{"ab-1"}) {
		t.Fatalf("matches %+v", choices)
	}

	// A filter change searches the same text in the new query.
	w.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionHarnesses, []string{"pi"}))
	h.expectCalls("usage", "facets", "facets")
	<-h.backend.requests
	searches := []string{(<-h.backend.facetReqs).Search, (<-h.backend.facetReqs).Search}
	slices.Sort(searches)
	if !slices.Equal(searches, []string{"", "ab"}) {
		t.Fatalf("facets searches %q", searches)
	}
}
