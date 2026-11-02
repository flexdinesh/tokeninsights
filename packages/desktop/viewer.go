package main

import (
	"github.com/egoist/mygo"
	"github.com/flexdinesh/tokeninsights/packages/desktop/internal/link"
	"github.com/flexdinesh/tokeninsights/packages/desktop/internal/trust"
)

const (
	appName      = "TokenInsights"
	windowWidth  = 1280
	windowHeight = 860
	minWidth     = 480
	minHeight    = 360
	helpWidth    = 560
	helpHeight   = 360
)

// viewer owns dashboard windows, one per origin. MyGo calls it on the main
// thread; dialogs may re-enter it, so state is re-read after each dialog.
type viewer struct {
	trust   *trust.Store
	windows map[string]*mygo.Window
	help    *mygo.Window
	started bool
}

func newViewer(store *trust.Store) *viewer {
	return &viewer{trust: store, windows: map[string]*mygo.Window{}}
}

// ready shows the help page unless a launch link already opened a dashboard.
func (v *viewer) ready() {
	v.started = true
	if len(v.windows) == 0 {
		v.showHelp()
	}
}

func (v *viewer) open(raw string) {
	target, err := link.Parse(raw)
	if err != nil {
		v.fail("Cannot open this link.", err.Error())
		return
	}
	if !target.Loopback && !v.approve(target.Origin) {
		v.ensureWindow()
		return
	}
	if window, ok := v.windows[target.Origin]; ok {
		if err := window.Page().LoadURL(target.URL); err != nil {
			v.fail("Cannot open dashboard.", err.Error())
			return
		}
		window.Show()
		window.Focus()
		return
	}
	v.windows[target.Origin] = v.dashboardWindow(target)
	if v.help != nil {
		v.help.Close()
	}
}

// approve asks once per non-loopback origin and remembers approval when possible.
func (v *viewer) approve(origin string) bool {
	if v.trust != nil {
		trusted, err := v.trust.Trusted(origin)
		if err != nil {
			v.fail("Cannot read trusted servers.", err.Error())
		} else if trusted {
			return true
		}
	}
	result, err := mygo.Dialog.Message(mygo.MessageOptions{
		Type:          mygo.MessageQuestion,
		Title:         appName,
		Message:       "Open " + origin + "?",
		Detail:        "This link opens a TokenInsights dashboard from a server that is not on this computer. Only continue if you started it or trust it.",
		Buttons:       []string{"Open", "Cancel"},
		DefaultButton: 1,
		CancelButton:  1,
	})
	if err != nil || result.Button != 0 {
		return false
	}
	if v.trust != nil {
		if err := v.trust.Trust(origin); err != nil {
			v.fail("Cannot remember this server.", err.Error())
		}
	}
	return true
}

func (v *viewer) dashboardWindow(target link.Target) *mygo.Window {
	origin := target.Origin
	window := mygo.NewWindow(mygo.WindowOptions{
		Title: appName + " · " + origin, URL: target.URL,
		Width: windowWidth, Height: windowHeight, MinWidth: minWidth, MinHeight: minHeight,
		Page: mygo.PageOptions{Untrusted: true},
	})
	page := window.Page()
	page.OnPageTitleUpdated(func(e *mygo.TitleEvent) {
		e.PreventDefault()
		title := appName
		if e.Title != "" {
			title = e.Title
		}
		window.SetTitle(title + " · " + origin)
	})
	// Keep each window on its origin; other http(s) pages open in the browser.
	page.OnWillNavigate(func(e *mygo.NavigateEvent) {
		if link.SameOrigin(e.URL, origin) {
			return
		}
		e.PreventDefault()
		if _, err := link.Dashboard(e.URL); err == nil {
			_ = mygo.Shell.OpenExternal(e.URL)
		}
	})
	window.OnClosed(func() {
		if v.windows[origin] == window {
			delete(v.windows, origin)
		}
	})
	return window
}

func (v *viewer) showHelp() {
	if v.help != nil {
		v.help.Focus()
		return
	}
	window := mygo.NewWindow(mygo.WindowOptions{
		Title: appName, Width: helpWidth, Height: helpHeight,
		Page: mygo.PageOptions{Untrusted: true},
	})
	window.Page().LoadHTML(helpPage, "")
	window.OnClosed(func() {
		if v.help == window {
			v.help = nil
		}
	})
	v.help = window
}

// ensureWindow keeps a launched app from running without any window.
func (v *viewer) ensureWindow() {
	if v.started && len(v.windows) == 0 && v.help == nil {
		v.showHelp()
	}
}

func (v *viewer) focus() {
	for _, window := range v.windows {
		window.Show()
		window.Focus()
		return
	}
	v.showHelp()
}

// fail reports an error; the dialog result carries no decision.
func (v *viewer) fail(message, detail string) {
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{Type: mygo.MessageError, Title: appName, Message: message, Detail: detail})
	v.ensureWindow()
}

const helpPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>TokenInsights</title>
<style>
:root { color-scheme: light dark; font-family: system-ui, sans-serif; }
body { margin: 0; min-height: 100vh; display: grid; place-content: center; padding: 2rem; box-sizing: border-box; }
h1 { font-size: 1.25rem; margin: 0 0 .75rem; }
p { margin: .5rem 0; line-height: 1.5; }
code { font-family: ui-monospace, monospace; padding: .1rem .35rem; border-radius: .25rem; background: color-mix(in srgb, currentColor 10%, transparent); }
</style>
</head>
<body>
<main>
<h1>TokenInsights</h1>
<p>Run <code>tokeninsights web</code> in a terminal. Your dashboard opens here.</p>
<p>Use <code>tokeninsights web --open=browser</code> to open the browser instead.</p>
</main>
</body>
</html>`
