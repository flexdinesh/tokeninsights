package model

import (
	"errors"
	"reflect"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// excludedTotal is the excluded usage excludedAt answers.
const excludedTotal = 12_400

// excludedAt is estimated usage of testInstance's dataset at revision and
// generation: two sessions with excluded usage.
func excludedAt(revision, generation int64) application.Usage {
	usage := usageAt(revision, generation)
	usage.Quality = string(dashboardview.QualityEstimated)
	usage.Summary = analytics.ViewerSummaryRow{TotalTokens: excludedTotal, SessionCount: 2, SyncedSessions: 3}
	return usage
}

// expectExcluded waits for an excluded usage read.
func (h *harness) expectExcluded() excludedCall {
	h.t.Helper()
	return <-h.backend.excluded
}

// answerExcluded answers an excluded usage read and applies its result.
func (h *harness) answerExcluded(call excludedCall, usage application.Usage, err error) {
	call.answer <- usageResult{usage: usage, err: err}
	h.apply()
}

// noExcluded fails when an excluded usage read started.
func (h *harness) noExcluded() {
	h.t.Helper()
	select {
	case call := <-h.backend.excluded:
		h.t.Fatalf("unexpected excluded read %+v", call.request)
	default:
	}
}

// assertExcluded checks the excluded usage the window tells of.
func assertExcluded(t *testing.T, w *Window, want bool) {
	t.Helper()
	summary, ok := w.ExcludedUsage()
	if ok != want || (want && summary.TotalTokens != excludedTotal) {
		t.Fatalf("excluded usage %+v, %v; want %v", summary, ok, want)
	}
}

func TestExcludedUsageReadsOnceUsageAnswers(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(testInstance)
	h.answerReads(statusAt(1, 1), progressOf())
	h.expectUsage()
	h.noExcluded() // waits for usage, as the web's analytics.isSuccess
	if !w.ExcludedPending() {
		t.Fatal("excluded usage before any read")
	}
	h.answerUsage(usageAt(1, 1), nil)
	call := h.expectExcluded()
	query, _ := w.Query()
	if want := query.ExcludedUsageQuery().ToAnalytics(); !reflect.DeepEqual(call.request, want) {
		t.Fatalf("excluded request %+v, want %+v", call.request, want)
	}
	assertExcluded(t, w, false)
	assertStatus(t, w, "Ready", dashboardview.StatusReady) // the web's status ignores it
	h.answerExcluded(call, excludedAt(1, 1), nil)
	assertExcluded(t, w, true)
	if w.ExcludedPending() || w.ExcludedError() != nil {
		t.Fatal("answered excluded usage pending or failed")
	}

	// Every tab but Repo reads the Tokens row: the same read.
	w.SelectTab(dashboardview.TabModels)
	h.expectUsage()
	h.answerUsage(usageAt(1, 1), nil)
	h.noExcluded()
	assertExcluded(t, w, true)

	// No session with excluded usage, or another quality, tells of none.
	providers := []string{"openai"}
	w.query = w.query.WithSelection(dashboardview.SelectionChange{Providers: &providers})
	w.syncReads()
	h.expectUsage()
	h.answerUsage(usageAt(1, 1), nil)
	none := excludedAt(1, 1)
	none.Summary = analytics.ViewerSummaryRow{}
	h.answerExcluded(h.expectExcluded(), none, nil)
	assertExcluded(t, w, false)
	if w.ExcludedPending() {
		t.Fatal("excluded usage of no session pending")
	}
}

func TestExcludedUsageWaitsForSuccessfulUsage(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(testInstance)
	h.answerReads(statusAt(1, 1), progressOf())
	h.expectUsage()
	h.answerUsage(application.Usage{}, errors.New("usage unavailable"))
	h.noExcluded()
	w.RetryExcluded() // nothing to retry while usage fails
	h.noExcluded()
	w.RetryUsage()
	h.expectUsage()
	h.answerUsage(usageAt(1, 1), nil)
	h.answerExcluded(h.expectExcluded(), excludedAt(1, 1), nil)
	assertExcluded(t, w, true)
}

func TestReviewTogglesQualityAndRereadsUsage(t *testing.T) {
	h := synced(t)
	w := h.window
	h.answerExcluded(h.expectExcluded(), excludedAt(1, 1), nil)

	w.SetQuality(dashboardview.QualityEstimated)
	query, _ := w.Query()
	request := h.expectUsage()
	if query.Quality != dashboardview.QualityEstimated || request.Quality != "estimated" || request.Tab != string(DefaultTab) ||
		request.PageSize != TablePageSize {
		t.Fatalf("review query %+v, request %+v", query, request)
	}
	if _, ok := w.Usage(); ok {
		t.Fatal("confirmed totals stayed in review")
	}
	h.answerUsage(excludedAt(1, 1), nil)
	if usage, ok := w.Usage(); !ok || usage.Summary.TotalTokens != excludedTotal {
		t.Fatalf("reviewed usage %+v", usage)
	}
	h.noExcluded() // review reads no excluded count
	w.SetQuality(dashboardview.QualityEstimated)
	h.noCall()

	w.SetQuality(dashboardview.QualityConfirmed)
	if request := h.expectUsage(); request.Quality != "" {
		t.Fatalf("back to usage request %+v", request)
	}
	h.answerUsage(usageAt(1, 1), nil)
	h.noExcluded() // its excluded read is unchanged
	assertExcluded(t, w, true)
}

func TestExcludedFailureWaitsForRetry(t *testing.T) {
	h := synced(t)
	w := h.window
	h.answerExcluded(h.expectExcluded(), application.Usage{}, errors.New("excluded unavailable"))
	if w.ExcludedError() == nil || w.ExcludedPending() {
		t.Fatal("a failed excluded read lost its error")
	}
	assertExcluded(t, w, false)
	assertStatus(t, w, "Ready", dashboardview.StatusReady)

	h.advance(PollInterval)
	h.answerReads(statusAt(1, 1), progressOf())
	h.noExcluded() // a failed read waits for Retry

	w.RetryExcluded()
	call := h.expectExcluded()
	if w.ExcludedError() != nil {
		t.Fatal("retry kept the error")
	}
	h.answerExcluded(call, excludedAt(1, 1), nil)
	assertExcluded(t, w, true)
}

func TestRevisionChangeRereadsExcludedDroppingLateAnswers(t *testing.T) {
	h := synced(t)
	w := h.window
	first := h.expectExcluded()
	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(2, 1), nil)
	h.expectCalls("instance", "usage")
	<-h.backend.requests
	second := h.expectExcluded() // usage of revision 1 still shows: excluded reads beside it
	h.answerExcluded(first, excludedAt(1, 1), nil)
	if !w.ExcludedPending() {
		t.Fatal("a superseded excluded read applied")
	}
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	h.answerUsage(usageAt(2, 1), nil)
	h.answerExcluded(second, excludedAt(2, 1), nil)
	assertExcluded(t, w, true)

	// Excluded usage of an older revision than the usage shown tells of
	// none, as the web's.
	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(3, 1), nil)
	h.expectCalls("instance", "usage")
	<-h.backend.requests
	third := h.expectExcluded()
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	h.answerUsage(usageAt(3, 1), nil)
	assertExcluded(t, w, false)
	h.answerExcluded(third, excludedAt(3, 1), nil)
	assertExcluded(t, w, true)
}

func TestIdentityChangeDropsExcludedUsage(t *testing.T) {
	h := synced(t)
	w := h.window
	h.answerExcluded(h.expectExcluded(), excludedAt(1, 1), nil)
	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(1, 2), nil)
	if _, ok := w.ExcludedUsage(); ok || !w.ExcludedPending() {
		t.Fatal("excluded usage of another snapshot identity stayed")
	}
	h.expectCalls("instance", "usage")
	<-h.backend.requests
	h.noExcluded() // waits for the new identity's usage
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	h.answerUsage(usageAt(1, 2), nil)
	h.answerExcluded(h.expectExcluded(), excludedAt(1, 2), nil)
	assertExcluded(t, w, true)
}

func TestExcludedFromAnotherSnapshotRereadsOnceAfterStatus(t *testing.T) {
	h := synced(t)
	w := h.window
	h.answerExcluded(h.expectExcluded(), excludedAt(1, 2), nil) // another generation
	h.expectCall("status")
	if !w.ExcludedPending() || w.ExcludedError() != nil {
		t.Fatal("excluded usage of another snapshot was shown")
	}
	h.answerStatus(statusAt(1, 1), nil)
	h.answerExcluded(h.expectExcluded(), excludedAt(0, 1), nil) // an older revision
	h.expectCall("status")
	h.answerStatus(statusAt(1, 1), nil)
	h.noExcluded()
	if !errors.Is(w.ExcludedError(), ErrDataChanged) {
		t.Fatalf("excluded error %v", w.ExcludedError())
	}
}

func TestRepoReadsItsOwnExcludedUsage(t *testing.T) {
	h := synced(t)
	w := h.window
	h.expectExcluded()
	w.SelectTab(dashboardview.TabRepo)
	h.expectUsage()
	h.answerUsage(usageAt(1, 1), nil)
	call := h.expectExcluded()
	if call.request.Tab != string(dashboardview.TabRepo) || call.request.Sort != string(dashboardview.SortTotal) ||
		call.request.LocationGroup != analytics.RepoGroupRepository {
		t.Fatalf("repo excluded request %+v", call.request)
	}
}
