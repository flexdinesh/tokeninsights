// Package views builds the native dashboard from the kit, one element per
// file, as immediate-mode functions over a window's model. Views read the
// model and call its actions on the main thread; they own no I/O.
package views

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Shell is what one window's app shell shows and acts on: the window's
// model and the app-wide appearance, which SetAppearance changes.
type Shell struct {
	Window        *model.Window
	Appearance    theme.Appearance
	SetAppearance func(theme.Appearance)
}

// portedTabs are the tabs native views show. Others stay disabled in the
// sidebar until their views arrive.
var portedTabs = map[dashboardview.Tab]bool{}

// ported reports whether native views show tab.
func ported(tab dashboardview.Tab) bool { return portedTabs[tab] }

// Messages the content region shows.
const (
	placeholderText = "Native views arrive in upcoming builds."
	refreshFailed   = "Refresh failed. Saved usage remains available. Try Refresh again."
	instanceFailed  = "Backend details couldn’t load. Use Refresh to retry."
)

// AppShell builds a window: the sidebar beside a column of the header and
// the scrolling page, which holds the selected tab's content above the
// footer. It themes the window from s.Appearance and keeps the window's
// status current while it shows.
func AppShell(c *ui.Context, s Shell) ui.Element {
	t := s.Appearance.Tokens(c.Theme().Dark)
	c.SetTheme(t.Widgets())
	poll(c, s.Window)
	shell := ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Surface).TextColor(t.Ink)
	return shell.Children(func() {
		Sidebar(c, s.Window)
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
			Header(c, s)
			ui.Scroll(c).Grow(1).Children(func() {
				page(c, s.Window)
			})
		})
	})
}

// page is the centered content column, the web's PageContent: messages,
// the tab's content, then the footer, which follows the content as on the
// web.
func page(c *ui.Context, w *model.Window) {
	ui.Column(c).FillWidth().MaxWidth(theme.ContentMax).Margin(0, ui.Auto).
		PaddingX(theme.Gutter).Role(ui.RoleMain).Children(func() {
		messages(c, w)
		placeholder(c)
		Footer(c, w)
	})
}

// messages are the quiet danger lines for failed backend reads under the
// header.
func messages(c *ui.Context, w *model.Window) {
	t := theme.Current(c)
	for _, failed := range []struct {
		show bool
		text string
	}{
		{w.InstanceError() != nil, instanceFailed},
		{w.RefreshError() != nil, refreshFailed},
	} {
		if failed.show {
			theme.Label(ui.Text(c, failed.text)).TextColor(t.StatusDanger).Role(ui.RoleAlert).PaddingY(theme.Unit * 2)
		}
	}
}

// placeholder stands in for a tab native views do not show yet.
func placeholder(c *ui.Context) ui.Element {
	t := theme.Current(c)
	return ui.Column(c).Center().Padding(theme.Unit*12, 0).Children(func() {
		theme.Body(ui.Text(c, placeholderText)).TextColor(t.InkMuted).TextAlign(ui.Center).MaxWidth(theme.ProseWidth)
	})
}

// poll starts the window's due status reads and wakes the view when the
// next is due. A hidden window builds as it hides and as it shows, so
// polling pauses and resumes with it; a closed window builds no more.
func poll(c *ui.Context, w *model.Window) {
	if wait, ok := w.Poll(c.Now(), c.WindowHidden()); ok {
		c.After(wait)
	}
}
