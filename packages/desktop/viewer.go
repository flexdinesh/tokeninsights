package main

import (
	"context"
	"sync"

	"github.com/egoist/mygo"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/desktop/internal/link"
)

const (
	appName      = "TokenInsights Desktop"
	windowWidth  = 1280
	windowHeight = 850
	minWidth     = 360
	minHeight    = 480
	localKey     = "local"
)

type connector func(ctx context.Context, target string) (*dashboard, error)

// viewer owns dashboard windows on the GUI thread, one per dashboard origin.
// Each window exclusively owns a backend lease. Attaching and releasing leases
// stay off the GUI thread; close joins them.
type viewer struct {
	connect    connector
	windows    map[string]*mygo.Window
	connecting map[string]bool
	launches   map[string]bool
	ctx        context.Context
	cancel     context.CancelFunc
	workers    sync.WaitGroup
}

func newViewer(connect connector) *viewer {
	ctx, cancel := context.WithCancel(context.Background())
	return &viewer{connect: connect, windows: make(map[string]*mygo.Window), connecting: make(map[string]bool), launches: make(map[string]bool), ctx: ctx, cancel: cancel}
}

func (v *viewer) open(raw string) {
	if _, err := desktoplaunch.ID(raw); err != nil {
		v.fail("Cannot open this link.", err.Error())
		return
	}
	if v.launches[raw] {
		return
	}
	v.launches[raw] = true
	v.start(raw, raw)
}

func (v *viewer) focus() {
	for _, window := range v.windows {
		raise(window)
		return
	}
	if len(v.connecting) == 0 {
		v.start(localKey, "")
	}
}

// start attaches a backend off the GUI thread, then opens or reports on it.
func (v *viewer) start(key, target string) {
	if v.connecting[key] {
		return
	}
	v.connecting[key] = true
	v.workers.Go(func() {
		connected, err := v.connect(v.ctx, target)
		if err == nil && v.ctx.Err() != nil {
			_ = connected.Close()
			return
		}
		mygo.RunOnMain(func() { v.finish(key, target, connected, err) })
	})
}

func (v *viewer) finish(key, target string, connected *dashboard, err error) {
	delete(v.connecting, key)
	switch {
	case err != nil:
		if v.ctx.Err() == nil && target == "" && v.retry(err) {
			v.start(key, target)
			return
		}
		if target != "" && v.ctx.Err() == nil {
			v.fail("Cannot open requested connection.", err.Error())
		}
		v.quitIfIdle()
	default:
		v.show(connected)
	}
}

func (v *viewer) retry(err error) bool {
	result, dialogErr := mygo.Dialog.Message(mygo.MessageOptions{
		Type: mygo.MessageError, Title: appName, Message: "Couldn't start TokenInsights Desktop", Detail: err.Error(),
		Buttons: []string{"Retry", "Quit"}, DefaultButton: 0, CancelButton: 1,
	})
	return dialogErr == nil && result.Button == 0
}

// show opens connected, or releases it when its origin already has a window.
func (v *viewer) show(connected *dashboard) {
	target, err := link.Dashboard(connected.URL)
	if err != nil {
		v.release(connected)
		v.fail("Cannot open this dashboard.", err.Error())
		return
	}
	origin := target.Origin
	if window, ok := v.windows[origin]; ok {
		v.release(connected)
		v.load(window, target.URL)
		return
	}
	window := mygo.NewWindow(mygo.WindowOptions{
		Title: appName, URL: target.URL,
		Width: windowWidth, Height: windowHeight, MinWidth: minWidth, MinHeight: minHeight,
		// The renderer has browser capabilities only: no bound Go methods.
		Page: mygo.PageOptions{Untrusted: true},
	})
	page := window.Page()
	page.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	page.OnWillDownload(func(e *mygo.DownloadEvent) { e.PreventDefault() })
	page.OnWillNavigate(func(e *mygo.NavigateEvent) {
		switch navigationFor(e.URL, origin) {
		case navigateStay:
		case navigateBrowser:
			e.PreventDefault()
			v.openExternal(e.URL)
		case navigateDeny:
			e.PreventDefault()
		}
	})
	v.windows[origin] = window
	closed := false
	window.OnClosed(func() {
		closed = true
		delete(v.windows, origin)
		v.release(connected)
	})
	if err := connected.acknowledge(); err != nil {
		window.Close()
		v.fail("Cannot acknowledge requested connection.", "Run tokeninsights again.")
		return
	}

	// A lost backend leaves nothing to show; reopening attaches a new lease.
	v.workers.Go(func() {
		select {
		case <-v.ctx.Done():
		case <-connected.Done():
			mygo.RunOnMain(func() {
				if closed || v.ctx.Err() != nil {
					return
				}
				v.fail("Backend disconnected", "Reopen TokenInsights Desktop to reconnect. Saved usage remains available.")
				window.Close()
			})
		}
	})
}

func (v *viewer) load(window *mygo.Window, target string) {
	if err := window.Page().LoadURL(target); err != nil {
		v.fail("Cannot open this dashboard.", err.Error())
	}
	raise(window)
}

// release returns a lease off the GUI thread, since closing joins its session.
func (v *viewer) release(connected *dashboard) {
	v.workers.Go(func() { _ = connected.Close() })
}

// quitIfIdle ends a launch that never produced a window.
func (v *viewer) quitIfIdle() {
	if len(v.windows) == 0 && len(v.connecting) == 0 {
		mygo.App.Quit()
	}
}

func (v *viewer) openExternal(target string) {
	if err := mygo.Shell.OpenExternal(target); err != nil {
		v.fail("Cannot open the browser.", err.Error())
	}
}

func (v *viewer) fail(message, detail string) {
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{Type: mygo.MessageError, Title: appName, Message: message, Detail: detail})
}

// close cancels attachment and joins lease cleanup after the event loop ends.
func (v *viewer) close() {
	v.cancel()
	v.workers.Wait()
}

func raise(window *mygo.Window) {
	window.Show()
	window.Focus()
}

type navigation int

const (
	navigateStay navigation = iota
	navigateBrowser
	navigateDeny
)

// navigationFor keeps a window on its dashboard origin. Other http(s) pages
// open in the browser, and every other scheme goes nowhere.
func navigationFor(raw, origin string) navigation {
	if link.SameOrigin(raw, origin) {
		return navigateStay
	}
	if _, err := link.Dashboard(raw); err == nil {
		return navigateBrowser
	}
	return navigateDeny
}
