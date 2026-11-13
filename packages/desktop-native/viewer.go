package main

import (
	"context"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/preferences"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/session"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/views"
)

const (
	appName      = "TokenInsights Native"
	windowWidth  = 1280
	windowHeight = 850
	minWidth     = 720
	minHeight    = 520
	localKey     = "local"
)

// viewer owns native windows on the GUI thread. Each window exclusively owns
// one backend lease. Attaching and releasing leases stay off the GUI thread;
// close joins them.
type viewer struct {
	connect    session.Connector
	windows    map[*mygo.Window]*model.Window
	connecting map[string]bool
	launches   map[string]bool
	appearance *appearance
	ctx        context.Context
	cancel     context.CancelFunc
	workers    sync.WaitGroup
}

// newViewer starts with saved preferences; a nil store keeps choices for
// this run only.
func newViewer(connect session.Connector, store *preferences.Store, saved preferences.Preferences) *viewer {
	ctx, cancel := context.WithCancel(context.Background())
	v := &viewer{connect: connect, windows: make(map[*mygo.Window]*model.Window), connecting: make(map[string]bool),
		launches: make(map[string]bool), ctx: ctx, cancel: cancel}
	v.appearance = &appearance{choice: saved.Appearance, store: store, applied: v.applyAppearance, spawn: v.workers.Go}
	return v
}

// openArguments opens each native launch link, or focuses when there are none.
func (v *viewer) openArguments(args []string) {
	opened := false
	for _, arg := range args {
		if _, err := desktoplaunch.ID(browser.NativeApp, arg); err == nil {
			v.open(arg)
			opened = true
		}
	}
	if !opened {
		v.focus()
	}
}

func (v *viewer) open(raw string) {
	if _, err := desktoplaunch.ID(browser.NativeApp, raw); err != nil {
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
	for window := range v.windows {
		raise(window)
		return
	}
	if len(v.connecting) == 0 {
		v.start(localKey, "")
	}
}

// start attaches a backend off the GUI thread, then opens or reports on it.
func (v *viewer) start(key, link string) {
	if v.connecting[key] {
		return
	}
	v.connecting[key] = true
	v.workers.Go(func() {
		connected, err := v.connect(v.ctx, link)
		if err == nil && v.ctx.Err() != nil {
			_ = connected.Close()
			return
		}
		mygo.RunOnMain(func() { v.finish(key, link, connected, err) })
	})
}

func (v *viewer) finish(key, link string, connected *session.Session, err error) {
	delete(v.connecting, key)
	switch {
	case err != nil:
		if v.ctx.Err() == nil && link == "" && v.retry(err) {
			v.start(key, link)
			return
		}
		if link != "" && v.ctx.Err() == nil {
			v.fail("Cannot open requested connection.", err.Error())
		}
		v.quitIfIdle()
	default:
		v.show(connected)
	}
}

func (v *viewer) retry(err error) bool {
	result, dialogErr := mygo.Dialog.Message(mygo.MessageOptions{
		Type: mygo.MessageError, Title: appName, Message: "Couldn't start TokenInsights Native", Detail: err.Error(),
		Buttons: []string{"Retry", "Quit"}, DefaultButton: 0, CancelButton: 1,
	})
	return dialogErr == nil && result.Button == 0
}

// show opens a window that owns connected until it closes. The window's
// reads end as it closes, before its lease is released.
func (v *viewer) show(connected *session.Session) {
	ctx, cancel := context.WithCancel(v.ctx)
	var window *mygo.Window
	state := model.NewWindow(connected.Info, sessionBackend{connected}, model.Runtime{
		Context: ctx, Go: v.workers.Go, Update: func(fn func()) { window.Update(fn) }, Now: time.Now,
	})
	window = mygo.NewWindow(mygo.WindowOptions{
		Title: appName, Content: ui.View(func(c *ui.Context) {
			views.AppShell(c, views.Shell{Window: state, Appearance: v.appearance.choice, SetAppearance: v.appearance.set})
		}),
		Width: windowWidth, Height: windowHeight, MinWidth: minWidth, MinHeight: minHeight,
	})
	v.windows[window] = state
	closed := false
	window.OnClosed(func() {
		closed = true
		cancel()
		delete(v.windows, window)
		v.release(connected)
	})
	if err := connected.Acknowledge(); err != nil {
		window.Close()
		v.fail("Cannot acknowledge requested connection.", "Run tokeninsights again.")
		return
	}
	state.Load()
	// A lost backend leaves nothing to show; reopening attaches a new lease.
	v.workers.Go(func() {
		select {
		case <-v.ctx.Done():
		case <-connected.Done():
			mygo.RunOnMain(func() {
				if closed || v.ctx.Err() != nil {
					return
				}
				v.fail("Backend disconnected", "Reopen TokenInsights Native to reconnect. Saved usage remains available.")
				window.Close()
			})
		}
	})
}

// refresh requests capture for window's session; overlapping requests
// coalesce.
func (v *viewer) refresh(window *mygo.Window) {
	if state, ok := v.windows[window]; ok {
		state.Refresh()
	}
}

// applyAppearance shows a new appearance choice in native chrome and every
// window.
func (v *viewer) applyAppearance(choice theme.Appearance) {
	mygo.Theme.SetSource(themeSource(choice))
	for window := range v.windows {
		window.Invalidate()
	}
}

// release returns a lease off the GUI thread, since closing joins its session.
func (v *viewer) release(connected *session.Session) {
	v.workers.Go(func() { _ = connected.Close() })
}

// quitIfIdle ends a launch that never produced a window.
func (v *viewer) quitIfIdle() {
	if len(v.windows) == 0 && len(v.connecting) == 0 {
		mygo.App.Quit()
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

// menu is the native menu bar. Items act on the window whose menu was chosen,
// or the focused window where the platform gives none.
func (v *viewer) menu() *mygo.Menu {
	target := func(window *mygo.Window) *mygo.Window {
		if window != nil {
			return window
		}
		return mygo.FocusedWindow()
	}
	view := []*mygo.MenuItem{
		{Label: "Refresh", Accelerator: "CmdOrCtrl+R", Click: func(_ *mygo.MenuItem, window *mygo.Window) { v.refresh(target(window)) }},
		mygo.Separator(),
	}
	view = append(view, v.appearance.menuItems()...)
	view = append(view, mygo.Separator(), &mygo.MenuItem{Role: mygo.RoleToggleFullScreen})
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{{Role: mygo.RoleClose}, {Role: mygo.RoleQuit}}},
		{Role: mygo.RoleEditMenu},
		{Label: "View", Submenu: view},
		{Role: mygo.RoleWindowMenu},
	})
	v.appearance.menu = menu
	return menu
}
