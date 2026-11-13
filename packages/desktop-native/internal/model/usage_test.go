package model

import (
	"errors"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// usageAt is usage of testInstance's dataset answered at revision and
// generation, as statusAt describes.
func usageAt(revision, generation int64) application.Usage {
	return application.Usage{InstanceID: testInstance.InstanceID, Dashboard: analytics.Dashboard{
		DatabaseID: "db-1", DatasetID: testInstance.DatasetID, Revision: revision, Generation: generation,
		Summary:    analytics.ViewerSummaryRow{TotalTokens: 647_000, SessionCount: 4, SyncedSessions: 5},
		LastSynced: clock.UnixMilli(), Page: 1, PageSize: TablePageSize,
	}}
}

// expectUsage waits for a usage read and returns its request.
func (h *harness) expectUsage() analytics.Query {
	h.t.Helper()
	h.expectCall("usage")
	return <-h.backend.requests
}

// answerUsage answers a running usage read and applies its result.
func (h *harness) answerUsage(usage application.Usage, err error) {
	h.backend.usages <- usageResult{usage: usage, err: err}
	h.apply()
}

// synced is a harness whose window loaded testInstance and read status at
// revision 1 and the usage it then read.
func synced(t *testing.T) *harness {
	h := newHarness(t)
	h.loaded(testInstance)
	h.window.Poll(at(0), false)
	h.answerReads(statusAt(1, 1), progressOf())
	if got := h.expectUsage(); got.Tab != string(DefaultTab) {
		t.Fatalf("first usage read %+v", got)
	}
	h.answerUsage(usageAt(1, 1), nil)
	return h
}

func TestUsageReadsTheQueryOnceStatusIsReady(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(testInstance)
	w.Poll(at(0), false)
	h.expectCalls("status", "progress")
	waiting := statusAt(1, 1)
	waiting.Readiness = "starting"
	h.answerStatus(waiting, nil)
	h.answerProgress(progressOf(), nil)
	h.noCall() // analytics wait for readiness
	if _, ok := w.Usage(); ok || !w.UsageFetching() {
		t.Fatal("usage before any read")
	}

	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(1, 1), nil)
	query, _ := w.Query()
	request := h.expectUsage()
	if want := query.ToAnalytics(); !sameRequest(request, want) || request.Page != 1 || request.PageSize != TablePageSize {
		t.Fatalf("usage request %+v, want %+v", request, want)
	}
	assertStatus(t, w, "Loading…", dashboardview.StatusLoading)
	h.answerUsage(usageAt(1, 1), nil)
	usage, ok := w.Usage()
	if !ok || usage.Summary.TotalTokens != 647_000 || usage.Tab != DefaultTab || w.UsageFetching() || w.UsagePlaceholder() || !w.UsageMatchesView() {
		t.Fatalf("usage %+v, %v", usage, ok)
	}
	assertStatus(t, w, "Ready", dashboardview.StatusReady)

	w.Poll(at(PollInterval), false)
	h.answerReads(statusAt(1, 1), progressOf())
	h.noCall() // an unchanged snapshot keeps its usage
}

func TestTabSwitchReadsItsViewKeepingTheSummary(t *testing.T) {
	h := synced(t)
	w := h.window
	w.SelectTab(dashboardview.TabModels)
	request := h.expectUsage()
	if request.Tab != string(dashboardview.TabModels) || request.Sort != string(dashboardview.SortTotal) {
		t.Fatalf("tab request %+v", request)
	}
	usage, ok := w.Usage()
	if !ok || usage.Tab != DefaultTab || !w.UsagePlaceholder() || w.UsageMatchesView() {
		t.Fatal("the tab switch dropped the summary or showed the old view")
	}
	assertStatus(t, w, "Refreshing usage…", dashboardview.StatusLoading)
	h.answerUsage(usageAt(1, 1), nil)
	if usage, _ := w.Usage(); usage.Tab != dashboardview.TabModels || w.UsagePlaceholder() || !w.UsageMatchesView() {
		t.Fatalf("models usage %+v", usage)
	}
}

func TestSupersededReadsAreDropped(t *testing.T) {
	h := synced(t)
	w := h.window
	w.SelectTab(dashboardview.TabModels)
	h.expectUsage()
	w.SelectTab(dashboardview.TabProviders)
	h.apply() // the models read, cancelled, answers late
	if w.UsageError() != nil || !w.UsageFetching() {
		t.Fatalf("a superseded read applied: %v", w.UsageError())
	}
	if request := h.expectUsage(); request.Tab != string(dashboardview.TabProviders) {
		t.Fatalf("latest request %+v", request)
	}
	h.answerUsage(usageAt(1, 1), nil)
	if usage, _ := w.Usage(); usage.Tab != dashboardview.TabProviders {
		t.Fatalf("usage of %s", usage.Tab)
	}
}

func TestRevisionChangeRereadsKeepingUsage(t *testing.T) {
	h := synced(t)
	w := h.window
	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(2, 1), nil)
	h.expectCalls("instance", "usage")
	<-h.backend.requests
	if usage, ok := w.Usage(); !ok || usage.Revision != 1 || !w.UsageMatchesView() {
		t.Fatal("a revision change dropped the usage shown")
	}
	assertStatus(t, w, "Refreshing usage…", dashboardview.StatusLoading)
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	h.answerUsage(usageAt(2, 1), nil)
	if usage, _ := w.Usage(); usage.Revision != 2 || w.UsagePlaceholder() {
		t.Fatalf("usage revision %d", usage.Revision)
	}
	assertStatus(t, w, "Ready", dashboardview.StatusReady)
}

func TestIdentityChangeDropsUsage(t *testing.T) {
	h := synced(t)
	w := h.window
	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(1, 2), nil)
	if _, ok := w.Usage(); ok {
		t.Fatal("usage of another snapshot identity stayed")
	}
	h.expectCalls("instance", "usage")
	<-h.backend.requests
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	h.answerUsage(usageAt(1, 2), nil)
	if usage, ok := w.Usage(); !ok || usage.Generation != 2 {
		t.Fatal("usage of the new identity")
	}
}

func TestNewerUsageRereadsStatus(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(testInstance)
	w.Poll(at(0), false)
	h.answerReads(statusAt(1, 1), progressOf())
	h.expectUsage()
	h.answerUsage(usageAt(2, 1), nil)
	if usage, ok := w.Usage(); !ok || usage.Revision != 2 {
		t.Fatal("newer usage was not shown")
	}
	h.expectCall("status")
	h.answerStatus(statusAt(2, 1), nil)
	h.expectCalls("instance", "usage")
	<-h.backend.requests
}

func TestUsageFromAnotherSnapshotRereadsStatusThenUsageOnce(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(testInstance)
	w.Poll(at(0), false)
	h.answerReads(statusAt(1, 1), progressOf())
	h.expectUsage()
	h.answerUsage(usageAt(1, 2), nil) // another generation
	h.expectCall("status")
	if _, ok := w.Usage(); ok || w.UsageError() != nil || !w.UsageFetching() {
		t.Fatal("usage of another snapshot was shown")
	}
	h.answerStatus(statusAt(1, 1), nil)
	h.expectUsage()                   // the status kept its snapshot: read once more
	h.answerUsage(usageAt(0, 1), nil) // an older revision
	h.expectCall("status")
	h.answerStatus(statusAt(1, 1), nil)
	h.noCall()
	if !errors.Is(w.UsageError(), ErrDataChanged) || w.UsageFetching() {
		t.Fatalf("usage error %v", w.UsageError())
	}
	assertStatus(t, w, "Usage couldn’t load", dashboardview.StatusAttention)
}

func TestUsageFailureKeepsUsageUntilRetry(t *testing.T) {
	h := synced(t)
	w := h.window
	w.SelectTab(dashboardview.TabSessions)
	h.expectUsage()
	h.answerUsage(application.Usage{}, errors.New("usage unavailable"))
	if _, ok := w.Usage(); !ok || w.UsageError() == nil || w.UsageFetching() {
		t.Fatal("a failed read dropped the usage or lost its error")
	}
	assertStatus(t, w, "Usage couldn’t load", dashboardview.StatusAttention)

	w.Poll(at(PollInterval), false)
	h.answerReads(statusAt(1, 1), progressOf())
	h.noCall() // a failed read waits for Retry

	w.RetryUsage()
	if request := h.expectUsage(); request.Tab != string(dashboardview.TabSessions) {
		t.Fatalf("retry request %+v", request)
	}
	if w.UsageError() != nil {
		t.Fatal("retry kept the error")
	}
	h.answerUsage(usageAt(1, 1), nil)
	assertStatus(t, w, "Ready", dashboardview.StatusReady)
}

func TestRetriesReadWhatFailed(t *testing.T) {
	h := newHarness(t)
	w := h.window
	w.Load()
	h.expectCall("instance")
	h.backend.instances <- instanceResult{err: errors.New("backend busy")}
	h.apply()
	if w.ConnectionError() == nil {
		t.Fatal("no connection error")
	}
	w.RetryConnection()
	h.expectCall("instance")
	h.backend.instances <- instanceResult{instance: syncInstance}
	h.apply()

	w.Poll(at(0), false)
	h.expectCalls("status", "progress")
	h.answerStatus(application.Status{}, errors.New("status unavailable"))
	h.answerProgress(progressOf(), errors.New("progress unavailable"))
	if w.ConnectionError() == nil || !w.SyncState().CollectionUnknown {
		t.Fatal("read failures")
	}
	w.RetryConnection()
	h.expectCall("status")
	h.answerStatus(statusAt(1, 1), nil)
	w.RetryProgress()
	h.expectCall("progress")
	h.answerProgress(progressOf(), nil)
	if w.ConnectionError() != nil || w.SyncState().CollectionUnknown {
		t.Fatal("retries did not recover")
	}
	if status, ok := w.Status(); !ok || status.Metadata.Revision != 1 {
		t.Fatal("status after retry")
	}
}
