package views

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/kit"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Footer builds the page footer: a status dot beside the backend host,
// connection target and server timezone. The web's static harness list is
// omitted: the instance does not report its harnesses.
func Footer(c *ui.Context, w *model.Window) ui.Element {
	t := theme.Current(c)
	bar := ui.Row(c).Gap(theme.Unit*3).PaddingY(theme.Unit*3).BorderWidth(theme.Rule, 0, 0, 0).BorderColor(t.Line).
		TextColor(t.InkSubtle).Role(ui.RoleContentInfo)
	return bar.Children(func() {
		ui.Row(c).Gap(theme.Unit * 2).MinWidth(0).Shrink(1).Children(func() {
			kit.StatusDot(c, footerTone(w))
			theme.Label(ui.Text(c, FooterLine(w)))
		})
	})
}

// FooterLine is hostname · target, then the server timezone once the
// instance reported it.
func FooterLine(w *model.Window) string {
	line := w.Hostname() + " · " + w.Connection().Target
	if instance, ok := w.Instance(); ok {
		line += " · " + dashboardview.ServerZoneLabel(instance.Timezone)
	}
	return line
}

// footerTone is the dot's meaning until the status ladder arrives: danger
// after a failure, pending while loading or refreshing, success otherwise.
func footerTone(w *model.Window) kit.DotTone {
	_, loaded := w.Instance()
	switch {
	case w.InstanceError() != nil || w.RefreshError() != nil:
		return kit.DotDanger
	case !loaded || w.Refreshing():
		return kit.DotPending
	}
	return kit.DotSuccess
}
