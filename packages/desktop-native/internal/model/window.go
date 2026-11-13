// Package model owns what one native window shows and does: its connection
// identity, the backend instance, the dashboard query and refresh state.
// Views read it and call its actions on the main thread; its I/O runs on
// worker goroutines that publish results back on the main thread.
package model

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
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
}

// Runtime is how a window runs I/O off the main thread and publishes its
// results on it. Context ends the window's work; Go starts a worker the
// app joins on quit; Update runs a function on the main thread and builds
// the window's view again.
type Runtime struct {
	Context context.Context
	Go      func(func())
	Update  func(func())
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
}

// NewWindow returns the state of a window connected as connection. Call
// Load to read the backend instance.
func NewWindow(connection endpoint.Info, backend Backend, rt Runtime) *Window {
	return &Window{backend: backend, rt: rt, connection: connection, tab: DefaultTab}
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

// Load reads the backend instance unless it is loaded or loading. The
// first load starts the query from the instance's defaults on the
// selected tab.
func (w *Window) Load() {
	if w.instance != nil || w.loading {
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
	w.instance, w.instanceErr = &instance, nil
	w.query = dashboardview.InitialQuery(instance.Defaults, w.tab)
}

// SelectTab shows tab, keeping the query's filters.
func (w *Window) SelectTab(tab dashboardview.Tab) {
	if tab == w.tab {
		return
	}
	w.tab = tab
	if w.instance != nil {
		w.query = w.query.WithTab(tab)
	}
}

// Refresh starts capture and reports whether it started: a refresh while
// one runs coalesces into it. A finished refresh loads an instance that
// failed to load before.
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
	w.Load()
}
