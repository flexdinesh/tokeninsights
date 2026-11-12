package views

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Header builds the page header: the current view's heading, then the
// backend identity, Refresh, the backend status and the theme cycle. It
// wraps on narrow windows.
func Header(c *ui.Context, s Shell) ui.Element {
	t := theme.Current(c)
	w := s.Window
	bar := ui.Row(c).Wrap().Justify(ui.SpaceBetween).AlignItems(ui.Center).Gap(theme.Unit*4).
		MinHeight(theme.HeaderHeight).Padding(theme.Unit, theme.Gutter).
		Background(t.Surface).BorderWidth(0, 0, theme.Rule, 0).BorderColor(t.Line).Role(ui.RoleBanner)
	return bar.Children(func() {
		theme.View(ui.Text(c, "/ "+dashboardview.TabLabel(w.Tab()))).TextColor(t.InkSubtle).Role(ui.RoleHeading).Level(1)
		ui.Row(c).Wrap().Justify(ui.End).AlignItems(ui.Center).Gap(theme.Unit * 2).MinWidth(0).Shrink(1).Children(func() {
			identity(c, w)
			headerDivider(c)
			refreshButton(c.Key("refresh"), w)
			DashboardStatus(c.Key("status"), statusDetails(w))
			themeButton(c.Key("theme"), s.Appearance, s.SetAppearance)
		})
	})
}

// identity names the backend host, with its instance identity as a
// tooltip once known, above the connection line.
func identity(c *ui.Context, w *model.Window) {
	ui.Column(c).MinWidth(0).Shrink(1).Role(ui.RoleGroup).Label("Backend connection").Children(func() {
		host := theme.Strong(theme.Label(ui.Text(c, w.Hostname()))).SingleLine().MaxWidth(theme.ServerIdentityWidth)
		if instance, ok := w.Instance(); ok {
			kit.Tooltip(c, host, "Backend hostname: "+w.Hostname()+" · Identity: "+instance.InstanceID)
		}
		connection := w.Connection()
		if w.ConnectionUnavailable() {
			connection.State = connectionUnavailable
		}
		theme.Label(ui.Text(c, ConnectionLine(connection))).Role(ui.RoleStatus)
	})
}

// connectionUnavailable replaces the connection's state while status
// reads fail, as on the web.
const connectionUnavailable = "unavailable"

// ConnectionLine is the web header's connection summary:
// TRANSPORT · target · state.
func ConnectionLine(info endpoint.Info) string {
	return strings.ToUpper(info.Transport) + " · " + info.Target + " · " + info.State
}

// headerDivider is the thin rule between the identity and the actions.
func headerDivider(c *ui.Context) {
	t := theme.Current(c)
	ui.Box(c).Size(theme.Rule, theme.IconSize).MarginX(theme.Unit * 2).Background(t.Line).AccessHidden(true)
}

// Refresh button labels.
const (
	refreshLabel   = "Refresh"
	refreshPending = "Refreshing…"
)

// refreshButton requests capture; it is disabled while a refresh or a
// collection runs.
func refreshButton(c *ui.Context, w *model.Window) {
	pending := w.RefreshPending()
	label := refreshLabel
	if pending {
		label = refreshPending
	}
	b := kit.ButtonContent(c, kit.ButtonStyle{Variant: kit.Outline, Size: kit.SizeCompact, Disabled: pending}, func() {
		kit.Icon(c, icons.RotateCw())
		ui.Text(c, label).SingleLine()
	})
	if !pending && b.Clicked() {
		w.Refresh()
	}
}

// appearanceIcons are the web theme button's icon per choice.
var appearanceIcons = map[theme.Appearance]func() *ui.SVG{
	theme.AppearanceSystem: icons.Monitor,
	theme.AppearanceLight:  icons.Sun,
	theme.AppearanceDark:   icons.Moon,
}

// themeButton shows the current appearance and cycles it, as the web's:
// system, light, dark.
func themeButton(c *ui.Context, choice theme.Appearance, set func(theme.Appearance)) {
	name := "Theme: " + string(choice)
	b := kit.IconButton(c, appearanceIcons[choice](), name, kit.ButtonStyle{Variant: kit.Ghost}).Label(name + ". Change theme")
	if b.Clicked() {
		set(choice.Next())
	}
}
