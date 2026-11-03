package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-mygo/internal/link"
)

const (
	appName      = "TokenInsights MyGo"
	windowWidth  = 1180
	windowHeight = 780
	minWidth     = 720
	minHeight    = 520
)

// viewer owns native windows on the GUI thread. Each window's worker exclusively
// owns a backend session. Closing cancels immediately; joining stays off the GUI.
type viewer struct {
	connect connector
	windows map[string]*nativeWindow
	workers []*loader
}

type nativeWindow struct {
	window *mygo.Window
	model  *viewModel
}

func newViewer(connect connector) *viewer {
	return &viewer{connect: connect, windows: make(map[string]*nativeWindow)}
}

func (v *viewer) open(raw string) {
	target, err := link.Parse(raw)
	if err != nil {
		v.fail("Cannot open this link.", err.Error())
		return
	}
	if !target.Loopback {
		if err := mygo.Shell.OpenExternal(target.URL); err != nil {
			v.fail("Cannot open the browser.", err.Error())
		}
		return
	}
	v.show(target.Origin, target.URL)
}

func (v *viewer) show(key, target string) {
	if existing, ok := v.windows[key]; ok {
		existing.window.Show()
		existing.window.Focus()
		return
	}
	model := newViewModel(v.connect, target)
	// Keep unfinished closed-window workers until their session cleanup joins.
	active := v.workers[:0]
	for _, worker := range v.workers {
		select {
		case <-worker.done:
		default:
			active = append(active, worker)
		}
	}
	v.workers = append(active, model.worker)
	window := mygo.NewWindow(mygo.WindowOptions{
		Title: appName, Content: ui.View(model.view),
		Width: windowWidth, Height: windowHeight, MinWidth: minWidth, MinHeight: minHeight,
	})
	v.windows[key] = &nativeWindow{window: window, model: model}
	window.OnClosed(func() { delete(v.windows, key); model.close() })
}

func (v *viewer) focus() {
	for _, existing := range v.windows {
		existing.window.Show()
		existing.window.Focus()
		return
	}
	v.show("local", "")
}

func (v *viewer) fail(message, detail string) {
	_, _ = mygo.Dialog.Message(mygo.MessageOptions{Type: mygo.MessageError, Title: appName, Message: message, Detail: detail})
}

func (v *viewer) close() {
	for _, worker := range v.workers {
		worker.cancel()
	}
	for _, worker := range v.workers {
		<-worker.done
	}
}
