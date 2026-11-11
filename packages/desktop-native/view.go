package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// appearance is the app-wide theme choice; system follows the OS.
type appearance int

const (
	appearanceSystem appearance = iota
	appearanceLight
	appearanceDark
)

// dashboard is the state one window shows. It reads the shared appearance and
// requests capture through its own session.
type dashboard struct {
	info       endpoint.Info
	appearance *appearance
	capture    func(context.Context) error
	refreshing bool
	refreshErr string
}

func newDashboard(info endpoint.Info, choice *appearance, capture func(context.Context) error) *dashboard {
	return &dashboard{info: info, appearance: choice, capture: capture}
}

// beginRefresh reports whether a new refresh started; overlapping requests coalesce.
func (d *dashboard) beginRefresh() bool {
	if d.refreshing {
		return false
	}
	d.refreshing, d.refreshErr = true, ""
	return true
}

func (d *dashboard) endRefresh(err error) {
	d.refreshing = false
	if err != nil {
		d.refreshErr = err.Error()
	}
}

// tokens resolves the window's appearance against the system's.
func (d *dashboard) tokens(systemDark bool) *theme.Tokens {
	switch *d.appearance {
	case appearanceLight:
		return theme.For(false)
	case appearanceDark:
		return theme.For(true)
	}
	return theme.For(systemDark)
}

func (d *dashboard) view(c *ui.Context) {
	t := d.tokens(c.Theme().Dark)
	c.SetTheme(t.Widgets())
	ui.Column(c).Fill().Center().Gap(theme.SectionGap).Padding(theme.Gutter).Children(func() {
		theme.Brand(ui.Text(c, "TokenInsights")).TextColor(t.Ink)
		theme.Title(ui.Text(c, d.info.Hostname)).TextColor(t.Ink)
		theme.Label(ui.Text(c, connectionLine(d.info))).TextColor(t.InkMuted)
		status := "Native views arrive in upcoming builds. TokenInsights Desktop has the full dashboard."
		color := t.InkMuted
		if d.refreshing {
			status = "Refreshing…"
		}
		if d.refreshErr != "" {
			status, color = "Refresh failed: "+d.refreshErr, t.StatusDanger
		}
		theme.Body(ui.Text(c, status)).TextColor(color).TextAlign(ui.Center).MaxWidth(theme.SidebarWidth * 3)
	})
}

// connectionLine matches the web header: TRANSPORT · target · state.
func connectionLine(info endpoint.Info) string {
	return strings.ToUpper(info.Transport) + " · " + info.Target + " · " + info.State
}
