package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// AlertContent is what an Alert says. Destructive alerts take the danger
// treatment and are read out at once as alerts; Icon and Title are optional.
type AlertContent struct {
	Destructive bool
	Icon        *ui.SVG
	Title       string
	Description string
}

// Alert builds a framed message with an optional action, such as Retry,
// which action builds after the text. The row wraps on narrow widths.
func Alert(c *ui.Context, a AlertContent, action func()) ui.Element {
	t := theme.Current(c)
	box := ui.Row(c).Wrap().Gap(theme.Unit*3).Padding(theme.Unit*4).Border(theme.Rule, t.Line).Background(t.Surface).TextColor(t.Ink)
	if a.Destructive {
		box.Border(theme.Rule, t.StatusDanger).Background(t.StatusDangerSurface).TextColor(t.StatusDanger).Role(ui.RoleAlert)
	}
	theme.Body(box)
	return box.Children(func() {
		if a.Icon != nil {
			icon(c, a.Icon)
		}
		if a.Title != "" {
			theme.Strong(theme.Body(ui.Text(c, a.Title)))
		}
		theme.Body(ui.Text(c, a.Description)).Grow(1).Basis(0).MinWidth(0)
		if action != nil {
			action()
		}
	})
}

// EmptyState explains an empty result, centered, with an optional icon,
// description and recovery actions, which actions builds in a wrapping row.
func EmptyState(c *ui.Context, svg *ui.SVG, title, description string, actions func()) ui.Element {
	t := theme.Current(c)
	box := ui.Column(c).AlignItems(ui.Center).Padding(theme.Unit*12, theme.Unit*4).TextColor(t.InkSubtle)
	return box.Children(func() {
		if svg != nil {
			ui.Icon(c, svg).Size(theme.IconLarge, theme.IconLarge)
		}
		heading := theme.Sub(ui.Text(c, title)).TextColor(t.Ink).TextAlign(ui.Center).Role(ui.RoleHeading).Level(3).Margin(theme.Unit*3, 0, theme.Unit, 0)
		if svg == nil {
			heading.Margin(0, 0, theme.Unit, 0)
		}
		if description != "" {
			theme.Body(ui.Text(c, description)).TextColor(t.InkMuted).TextAlign(ui.Center).MaxWidth(theme.ProseWidth)
		}
		if actions != nil {
			ui.Row(c).Wrap().Justify(ui.Center).GapX(theme.Unit*4).GapY(theme.Unit*2).Margin(theme.Unit*3, 0, 0, 0).Children(actions)
		}
	})
}

// Skeleton builds a placeholder block that pulses while content loads; the
// caller sizes it to the content it stands for. It holds still under
// reduced motion.
func Skeleton(c *ui.Context) ui.Element {
	t := theme.Current(c)
	block := ui.Box(c).Background(t.SurfaceRaised).Border(theme.Rule, t.Line).AccessHidden(true)
	if !c.Preferences().ReduceMotion {
		block.Opacity(1 - (1-theme.PulseLow)*block.Loop("pulse", theme.DurationPulse, ui.Bounce(ui.EaseInOut)))
	}
	return block
}
