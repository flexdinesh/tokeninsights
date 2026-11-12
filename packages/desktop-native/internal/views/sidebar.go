package views

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// tabIcons are the web sidebar's Lucide icons per tab.
var tabIcons = map[dashboardview.Tab]func() *ui.SVG{
	dashboardview.TabTokens:    icons.Activity,
	dashboardview.TabModels:    icons.Box,
	dashboardview.TabProviders: icons.Layers3,
	dashboardview.TabHarnesses: icons.Command,
	dashboardview.TabSessions:  icons.ChartNoAxesCombined,
	dashboardview.TabContext:   icons.Gauge,
	dashboardview.TabRepo:      icons.GitBranch,
}

// Sidebar builds the navigation rail: the brand, a link per tab in
// navigation order and the workspace note at its foot. Tabs native views
// do not show yet stay visible but disabled (ADR 0030); the selected tab
// always stays enabled.
func Sidebar(c *ui.Context, w *model.Window) ui.Element {
	t := theme.Current(c)
	rail := ui.Column(c).Width(theme.SidebarWidth).Shrink(0).Gap(theme.Unit*2).Padding(0, theme.Unit*3, theme.Unit*4).
		Background(t.SurfaceRail).BorderWidth(0, theme.Rule, 0, 0).BorderColor(t.Line).
		Role(ui.RoleComplementary).Label("Dashboard navigation")
	return rail.Children(func() {
		ui.Row(c).MinHeight(theme.HeaderHeight).PaddingX(theme.Unit * 3).Children(func() { Brand(c) })
		ui.Column(c).Gap(theme.Unit).Role(ui.RoleNavigation).Label("Analytics views").Children(func() {
			for _, tab := range dashboardview.Tabs() {
				sidebarItem(c.Key(string(tab)), w, tab)
			}
		})
		ui.Spacer(c)
		ui.Column(c).PaddingX(theme.Unit * 3).TextColor(t.InkSubtle).Children(func() {
			theme.Label(ui.Text(c, "Personal workspace"))
			theme.Label(ui.Text(c, "Usage metadata only"))
		})
	})
}

// sidebarItem is one tab's link: quiet, with the selection tint while
// current.
func sidebarItem(c *ui.Context, w *model.Window, tab dashboardview.Tab) {
	selected := tab == w.Tab()
	enabled := selected || ported(tab)
	item := kit.ButtonContent(c, kit.ButtonStyle{Variant: kit.Quiet, Selected: selected, Disabled: !enabled}, func() {
		kit.Icon(c, tabIcons[tab]())
		ui.Text(c, dashboardview.TabLabel(tab)).SingleLine()
	}).Justify(ui.Start)
	if selected {
		item.Current(ui.CurrentPage)
	}
	if enabled && item.Clicked() {
		w.SelectTab(tab)
	}
}
