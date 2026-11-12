package model

import (
	"context"
	"errors"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
)

// fakeBackend answers each call with the next value its test sends.
type fakeBackend struct {
	instances chan instanceResult
	captures  chan error
	calls     chan string
}

type instanceResult struct {
	instance application.Instance
	err      error
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{instances: make(chan instanceResult), captures: make(chan error), calls: make(chan string, 8)}
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

// harness runs a window's workers as goroutines and its updates when the
// test, standing in for the main thread, applies them.
type harness struct {
	t       *testing.T
	backend *fakeBackend
	updates chan func()
	window  *Window
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, backend: newFakeBackend(), updates: make(chan func(), 8)}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	rt := Runtime{Context: ctx, Go: func(fn func()) { go fn() }, Update: func(fn func()) { h.updates <- fn }}
	connection := endpoint.Info{Transport: "ssh", Target: "ssh://alice@studio:2222", State: "connected", Hostname: "studio.local"}
	h.window = NewWindow(connection, h.backend, rt)
	return h
}

// expectCall waits for the backend to receive name.
func (h *harness) expectCall(name string) {
	h.t.Helper()
	if got := <-h.backend.calls; got != name {
		h.t.Fatalf("backend call %q, want %q", got, name)
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

var testInstance = application.Instance{
	InstanceID: "inst-4f1c", Hostname: "studio", Timezone: "Europe/London",
	Defaults: analytics.Selection{Period: "month", From: "2026-11-01", To: "2026-11-30", Bucket: "day"},
}

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
	h.noCall()
	instance, ok := w.Instance()
	if !ok || instance.InstanceID != "inst-4f1c" || w.Hostname() != "studio" {
		t.Fatalf("instance %+v, %v", instance, ok)
	}
	query, ok := w.Query()
	want := dashboardview.InitialQuery(testInstance.Defaults, DefaultTab)
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
	w.Load()
	h.expectCall("instance")
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()

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
	if !w.Refresh() || w.RefreshError() != nil {
		t.Fatal("a new refresh kept the old failure")
	}
	h.expectCall("capture")
	h.backend.captures <- nil
	h.apply()
	if w.Refreshing() || w.RefreshError() != nil {
		t.Fatal("successful refresh")
	}
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
