// Package model owns what one native window shows and does: its connection
// identity, the backend instance, the dashboard query and its usage,
// refresh state and its synchronization with backend status and collector
// progress.
// Views read it and call its actions on the main thread; its I/O runs on
// worker goroutines that publish results back on the main thread.
package model

import (
	"context"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
)

// Backend is the session a window reads and refreshes through: its own
// backend lease. Calls run on worker goroutines and honor ctx.
type Backend interface {
	// Instance describes the backend: identity, hostname, timezone and
	// query defaults.
	Instance(ctx context.Context) (application.Instance, error)
	// Capture runs default capture and returns once it finished.
	Capture(ctx context.Context) error
	// Status observes the published snapshot and processing of the
	// session's dataset.
	Status(ctx context.Context) (application.Status, error)
	// Progress observes the backend's collection attempts.
	Progress(ctx context.Context) (collectorprogress.Snapshot, error)
	// Usage reads one page of usage for a query, with the summary and
	// chart of the whole query.
	Usage(ctx context.Context, query analytics.Query) (application.Usage, error)
	// Facets reads the filter values of a query, sessions matching its
	// search.
	Facets(ctx context.Context, request application.FacetRequest) (application.Facets, error)
}

// Runtime is how a window runs I/O off the main thread and publishes its
// results on it. Context ends the window's work; Go starts a worker the
// app joins on quit; Update runs a function on the main thread and builds
// the window's view again; Now is the wall clock reads are stamped with.
type Runtime struct {
	Context context.Context
	Go      func(func())
	Update  func(func())
	Now     func() time.Time
}

// DefaultTab is the tab a new window opens on, as the web's index route.
const DefaultTab = dashboardview.TabTokens

// Window is one window's dashboard state. Its methods run on the main
// thread only.
type Window struct {
	backend    Backend
	rt         Runtime
	connection endpoint.Info

	tab         dashboardview.Tab
	instance    *application.Instance
	instanceErr error
	loading     bool
	query       dashboardview.Query

	refreshing bool
	refreshErr error

	sync     syncState
	usage    usageState
	facets   facetsState
	sessions sessionSearch
}

// NewWindow returns the state of a window connected as connection. Call
// Load to read the backend instance.
func NewWindow(connection endpoint.Info, backend Backend, rt Runtime) *Window {
	w := &Window{backend: backend, rt: rt, connection: connection, tab: DefaultTab, facets: facetsState{anyRequest: true}}
	w.OnSnapshotChange(w.usageSnapshotChanged)
	w.OnSnapshotChange(w.facetsSnapshotChanged)
	return w
}

// Connection is the window's connection: transport, target and state.
func (w *Window) Connection() endpoint.Info { return w.connection }

// Tab is the selected tab.
func (w *Window) Tab() dashboardview.Tab { return w.tab }

// Instance is the backend instance once loaded.
func (w *Window) Instance() (application.Instance, bool) {
	if w.instance == nil {
		return application.Instance{}, false
	}
	return *w.instance, true
}

// InstanceError is why the instance last failed to load, or nil.
func (w *Window) InstanceError() error { return w.instanceErr }

// Hostname names the backend host: the instance's once loaded, else the
// connection's.
func (w *Window) Hostname() string {
	if w.instance != nil && w.instance.Hostname != "" {
		return w.instance.Hostname
	}
	return w.connection.Hostname
}

// Query is the dashboard query, available once the instance gave its
// defaults.
func (w *Window) Query() (dashboardview.Query, bool) { return w.query, w.instance != nil }

// Refreshing reports whether a refresh is running.
func (w *Window) Refreshing() bool { return w.refreshing }

// RefreshError is why the last refresh failed, or nil.
func (w *Window) RefreshError() error { return w.refreshErr }

// Load reads the backend instance unless it is loaded or loading; a
// failed read loads again. The first load starts the query from the
// instance's defaults on the selected tab, in pages of TablePageSize.
func (w *Window) Load() {
	if w.instance != nil && w.instanceErr == nil {
		return
	}
	w.loadInstance()
}

// loadInstance reads the backend instance unless a read runs. A loaded
// instance stays until another replaces it.
func (w *Window) loadInstance() {
	if w.loading {
		return
	}
	w.loading = true
	ctx := w.rt.Context
	w.rt.Go(func() {
		instance, err := w.backend.Instance(ctx)
		w.rt.Update(func() { w.loaded(instance, err) })
	})
}

func (w *Window) loaded(instance application.Instance, err error) {
	w.loading = false
	if err != nil {
		w.instanceErr = err
		return
	}
	first := w.instance == nil
	w.instance, w.instanceErr = &instance, nil
	if first {
		w.query = dashboardview.InitialQuery(instance.Defaults, w.tab).WithPageSize(TablePageSize)
	}
	w.syncReads()
}

// SelectTab shows tab, keeping the query's filters, and reads its usage
// and facets.
func (w *Window) SelectTab(tab dashboardview.Tab) {
	if tab == w.tab {
		return
	}
	w.tab = tab
	if w.instance != nil {
		w.query = w.query.WithTab(tab)
		w.syncReads()
	}
}

// SortBy sorts the rows by sort, as the web's sortBy: the current sort
// flips its direction. The rows are read again from the first page, as
// the backend sorts them.
func (w *Window) SortBy(sort dashboardview.Sort) {
	if w.instance == nil {
		return
	}
	w.query = w.query.SortBy(sort)
	w.syncReads()
}

// ClearFilters removes every dimension, location and date filter and
// reads the usage of what remains.
func (w *Window) ClearFilters() {
	if w.instance == nil {
		return
	}
	w.query = w.query.ClearFilters()
	w.syncReads()
}

// Refresh starts capture and reports whether it started: a refresh while
// one runs coalesces into it. A finished refresh reads status and progress
// again, as the web's, and loads an instance that failed to load before.
func (w *Window) Refresh() bool {
	if w.refreshing {
		return false
	}
	w.refreshing, w.refreshErr = true, nil
	ctx := w.rt.Context
	w.rt.Go(func() {
		err := w.backend.Capture(ctx)
		w.rt.Update(func() { w.refreshed(err) })
	})
	return true
}

func (w *Window) refreshed(err error) {
	w.refreshing, w.refreshErr = false, err
	w.RefetchStatus()
	w.readProgress()
	w.Load()
}
