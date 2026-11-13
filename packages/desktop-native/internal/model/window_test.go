package model

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

// fakeBackend answers each call with the next value its test sends.
// Excluded usage reads go to excluded instead of calls, each with its own
// answer, and wait for it even when cancelled: tests of other reads leave
// them unanswered, and those reads publish nothing while the test runs.
type fakeBackend struct {
	instances  chan instanceResult
	captures   chan error
	statuses   chan statusResult
	progresses chan progressResult
	usages     chan usageResult
	requests   chan analytics.Query
	facets     chan facetsResult
	facetReqs  chan application.FacetRequest
	calls      chan string
	excluded   chan excludedCall
	// done ends the test: unanswered reads return.
	done chan struct{}
}

// excludedCall is one excluded usage read: its request, and where the
// test answers it.
type excludedCall struct {
	request analytics.Query
	answer  chan usageResult
}

// excludedCallLimit is how many excluded reads a test may leave
// unanswered.
const excludedCallLimit = 64

// isExcludedRead reports whether query is the excluded usage read beside
// confirmed usage: one estimated row.
func isExcludedRead(query analytics.Query) bool {
	return query.Quality == string(dashboardview.QualityEstimated) && query.PageSize == 1
}

type instanceResult struct {
	instance application.Instance
	err      error
}

type statusResult struct {
	status application.Status
	err    error
}

type progressResult struct {
	progress collectorprogress.Snapshot
	err      error
}

type usageResult struct {
	usage application.Usage
	err   error
}

type facetsResult struct {
	facets application.Facets
	err    error
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{instances: make(chan instanceResult), captures: make(chan error), statuses: make(chan statusResult),
		progresses: make(chan progressResult), usages: make(chan usageResult), requests: make(chan analytics.Query, 8),
		facets: make(chan facetsResult), facetReqs: make(chan application.FacetRequest, 8), calls: make(chan string, 8),
		excluded: make(chan excludedCall, excludedCallLimit), done: make(chan struct{})}
}

// Usage sends its request to requests before it waits for its answer;
// an excluded read sends itself to excluded.
func (b *fakeBackend) Usage(ctx context.Context, query analytics.Query) (application.Usage, error) {
	if isExcludedRead(query) {
		call := excludedCall{request: query, answer: make(chan usageResult, 1)}
		b.excluded <- call
		select {
		case r := <-call.answer:
			return r.usage, r.err
		case <-b.done:
			return application.Usage{}, context.Canceled
		}
	}
	b.calls <- "usage"
	b.requests <- query
	select {
	case r := <-b.usages:
		return r.usage, r.err
	case <-ctx.Done():
		return application.Usage{}, ctx.Err()
	}
}

// Facets sends its request to facetReqs before it waits for its answer.
func (b *fakeBackend) Facets(ctx context.Context, request application.FacetRequest) (application.Facets, error) {
	b.calls <- "facets"
	b.facetReqs <- request
	select {
	case r := <-b.facets:
		return r.facets, r.err
	case <-ctx.Done():
		return application.Facets{}, ctx.Err()
	}
}

func (b *fakeBackend) Status(ctx context.Context) (application.Status, error) {
	b.calls <- "status"
	select {
	case r := <-b.statuses:
		return r.status, r.err
	case <-ctx.Done():
		return application.Status{}, ctx.Err()
	}
}

func (b *fakeBackend) Progress(ctx context.Context) (collectorprogress.Snapshot, error) {
	b.calls <- "progress"
	select {
	case r := <-b.progresses:
		return r.progress, r.err
	case <-ctx.Done():
		return collectorprogress.Snapshot{}, ctx.Err()
	}
}

func (b *fakeBackend) Instance(ctx context.Context) (application.Instance, error) {
	b.calls <- "instance"
	select {
	case r := <-b.instances:
		return r.instance, r.err
	case <-ctx.Done():
		return application.Instance{}, ctx.Err()
	}
}

func (b *fakeBackend) Capture(ctx context.Context) error {
	b.calls <- "capture"
	select {
	case err := <-b.captures:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// harness runs a window's workers as goroutines and its results when the
// test, standing in for the main thread, applies them, counting those
// that build the view (Update) and the redraws the window asks for
// (Invalidate). Its clock starts at clock and its timers fire only as the
// test advances it; cancel ends the window's work as closing it does.
type harness struct {
	t       *testing.T
	backend *fakeBackend
	updates chan func()
	window  *Window
	cancel  context.CancelFunc

	now    time.Time
	timers []*fakeTimer
	// updated counts results applied with Update; invalidated the
	// window's Invalidate calls.
	updated, invalidated int
}

// fakeTimer is one After of the harness: due at at, unless stopped.
type fakeTimer struct {
	at      time.Time
	fn      func()
	stopped bool
}

// clock is the wall time a harness starts at.
var clock = time.Date(2026, time.November, 13, 9, 0, 0, 0, time.UTC)

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, backend: newFakeBackend(), updates: make(chan func(), 8), now: clock}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	t.Cleanup(cancel)
	t.Cleanup(func() { close(h.backend.done) })
	apply := func(fn func()) {
		select {
		case h.updates <- fn:
		case <-h.backend.done:
		}
	}
	rt := Runtime{
		Context: ctx, Go: func(fn func()) { go fn() }, Apply: apply,
		Update:     func(fn func()) { apply(func() { h.updated++; fn() }) },
		Invalidate: func() { h.invalidated++ },
		After:      h.after,
		Now:        func() time.Time { return h.now },
	}
	connection := endpoint.Info{Transport: "ssh", Target: "ssh://alice@studio:2222", State: "connected", Hostname: "studio.local"}
	h.window = NewWindow(connection, h.backend, rt)
	return h
}

// after arms a timer the test fires by advancing the clock.
func (h *harness) after(d time.Duration, fn func()) (stop func()) {
	timer := &fakeTimer{at: h.now.Add(d), fn: fn}
	h.timers = append(h.timers, timer)
	return func() { timer.stopped = true }
}

// armed is the timer that will fire next, or nil.
func (h *harness) armed() *fakeTimer {
	var next *fakeTimer
	for _, timer := range h.timers {
		if !timer.stopped && (next == nil || timer.at.Before(next.at)) {
			next = timer
		}
	}
	return next
}

// armedCount counts the timers that will fire.
func (h *harness) armedCount() int {
	n := 0
	for _, timer := range h.timers {
		if !timer.stopped {
			n++
		}
	}
	return n
}

// nextWake is how long until the next timer fires; ok is false when none
// will.
func (h *harness) nextWake() (time.Duration, bool) {
	if next := h.armed(); next != nil {
		return next.at.Sub(h.now), true
	}
	return 0, false
}

// advance moves the clock on by d, firing the timers due meanwhile at
// their times, on the test's main thread.
func (h *harness) advance(d time.Duration) {
	end := h.now.Add(d)
	for next := h.armed(); next != nil && !next.at.After(end); next = h.armed() {
		h.now, next.stopped = next.at, true
		next.fn()
	}
	h.now = end
}

// resetCounts forgets the results and redraws counted so far.
func (h *harness) resetCounts() { h.updated, h.invalidated = 0, 0 }

// expectCall waits for the backend to receive name.
func (h *harness) expectCall(name string) {
	h.t.Helper()
	if got := <-h.backend.calls; got != name {
		h.t.Fatalf("backend call %q, want %q", got, name)
	}
}

// expectCalls waits for the backend to receive names, in any order, as
// reads started together race.
func (h *harness) expectCalls(names ...string) {
	h.t.Helper()
	got := make([]string, len(names))
	for i := range names {
		got[i] = <-h.backend.calls
	}
	want := slices.Clone(names)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		h.t.Fatalf("backend calls %q, want %q", got, want)
	}
}

// apply runs the next published update on the test's main thread.
func (h *harness) apply() { (<-h.updates)() }

// noCall fails when the backend received a call nobody expected.
func (h *harness) noCall() {
	h.t.Helper()
	select {
	case got := <-h.backend.calls:
		h.t.Fatalf("unexpected backend call %q", got)
	default:
	}
}

// testInstance offers usage and collector progress; facetsInstance adds
// filter values.
var testInstance = application.Instance{
	InstanceID: "inst-4f1c", DatasetID: "default", Hostname: "studio", Timezone: "Europe/London",
	Capabilities: serverfeatures.Capabilities{serverfeatures.Usage, serverfeatures.CollectorProgress},
	Permissions:  serverfeatures.Permissions{serverfeatures.Read},
	Defaults:     analytics.Selection{Period: "month", From: "2026-11-01", To: "2026-11-30", Bucket: "day"},
}

// syncInstance is testInstance without usage, for tests of the status
// and progress reads alone.
var syncInstance = func() application.Instance {
	instance := testInstance
	instance.Capabilities = serverfeatures.Capabilities{serverfeatures.CollectorProgress}
	return instance
}()

var facetsInstance = func() application.Instance {
	instance := testInstance
	instance.Capabilities = serverfeatures.Capabilities{serverfeatures.Usage, serverfeatures.Facets, serverfeatures.CollectorProgress}
	return instance
}()

func TestLoadStartsQueryFromInstanceDefaults(t *testing.T) {
	h := newHarness(t)
	w := h.window
	if _, ok := w.Query(); ok || w.Hostname() != "studio.local" || w.Tab() != DefaultTab {
		t.Fatal("query before the instance loaded")
	}
	w.Load()
	w.Load() // coalesces while loading
	h.expectCall("instance")
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	h.expectCalls("status", "progress") // polling starts once the instance loaded
	h.noCall()
	instance, ok := w.Instance()
	if !ok || instance.InstanceID != "inst-4f1c" || w.Hostname() != "studio" {
		t.Fatalf("instance %+v, %v", instance, ok)
	}
	query, ok := w.Query()
	want := dashboardview.InitialQuery(testInstance.Defaults, DefaultTab).WithPageSize(TablePageSize)
	if !ok || query.Tab != want.Tab || query.From != want.From || query.Sort != want.Sort || query.PageSize != want.PageSize {
		t.Fatalf("query %+v, want %+v", query, want)
	}
	w.Load() // loaded instances do not reload
	h.noCall()
}

func TestInstanceFailureReloadsAfterRefresh(t *testing.T) {
	h := newHarness(t)
	w := h.window
	w.Load()
	h.expectCall("instance")
	h.backend.instances <- instanceResult{err: errors.New("backend busy")}
	h.apply()
	if _, ok := w.Instance(); ok || w.InstanceError() == nil {
		t.Fatal("failed load kept no error")
	}
	if _, ok := w.Query(); ok {
		t.Fatal("query without an instance")
	}
	w.Refresh()
	h.expectCall("capture")
	h.backend.captures <- nil
	h.apply()
	h.expectCall("instance")
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	if _, ok := w.Instance(); !ok || w.InstanceError() != nil {
		t.Fatal("refresh did not reload the instance")
	}
}

func TestRefreshCoalescesAndReportsFailure(t *testing.T) {
	h := newHarness(t)
	w := h.window
	h.loaded(syncInstance)
	h.answerReads(statusAt(1, 1), progressOf())

	if !w.Refresh() || w.Refresh() || !w.Refreshing() {
		t.Fatal("overlapping refresh was not coalesced")
	}
	h.expectCall("capture")
	h.noCall()
	h.backend.captures <- errors.New("capture failed")
	h.apply()
	if w.Refreshing() || w.RefreshError() == nil {
		t.Fatal("failed refresh kept running or lost its error")
	}
	h.answerReads(statusAt(1, 1), progressOf())
	if !w.Refresh() || w.RefreshError() != nil {
		t.Fatal("a new refresh kept the old failure")
	}
	h.expectCall("capture")
	h.backend.captures <- nil
	h.apply()
	if w.Refreshing() || w.RefreshError() != nil {
		t.Fatal("successful refresh")
	}
	h.answerReads(statusAt(1, 1), progressOf())
	h.noCall()
}

func TestSelectTabKeepsFiltersAndResetsSort(t *testing.T) {
	h := newHarness(t)
	w := h.window
	w.SelectTab(dashboardview.TabModels)
	if w.Tab() != dashboardview.TabModels {
		t.Fatal("tab before load")
	}
	w.Load()
	h.expectCall("instance")
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	query, _ := w.Query()
	if query.Tab != dashboardview.TabModels || query.Sort != dashboardview.DefaultSort(dashboardview.TabModels) {
		t.Fatalf("query on the tab chosen before load: %+v", query)
	}
	w.SelectTab(dashboardview.TabContext)
	query, _ = w.Query()
	if w.Tab() != dashboardview.TabContext || query.Tab != dashboardview.TabContext || query.Sort != dashboardview.SortAverageContext || query.From != testInstance.Defaults.From {
		t.Fatalf("query after selecting context: %+v", query)
	}
}
