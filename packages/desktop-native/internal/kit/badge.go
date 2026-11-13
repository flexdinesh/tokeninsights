package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// badgeColors resolves a badge's look; variants other than Primary, Outline
// and Destructive show as Secondary, the web's default badge.
func badgeColors(t *theme.Tokens, v Variant) controlColors {
	switch v {
	case Primary:
		return controlColors{background: t.Action, ink: t.ActionInk}
	case Outline:
		return controlColors{border: t.Line, ink: t.Ink}
	case Destructive:
		return controlColors{background: t.StatusDanger, ink: t.StatusDangerInk}
	}
	return controlColors{background: t.SurfaceRaised, ink: t.InkMuted}
}

// Badge builds a short square label with tabular digits, as a count beside a
// section title or filter trigger. Assistive technology reads it with the
// element it is in.
func Badge(c *ui.Context, text string, v Variant) ui.Element {
	colors := badgeColors(theme.Current(c), v)
	b := ui.Row(c).Justify(ui.Center).Shrink(0).Padding(theme.Unit/2, theme.Unit*1.5).
		Background(colors.background).Border(theme.Rule, colors.border).TextColor(colors.ink)
	return b.Children(func() {
		theme.Medium(theme.Label(ui.Text(c, text).SingleLine().FontFeatures("tnum")))
	})
}

// Chip builds an active filter: its dimension, its value truncated, and a
// remove mark, in the selection treatment. The whole chip is the remove
// button, named by removeLabel, which keeps the full value; query Clicked to
// remove the filter.
func Chip(c *ui.Context, dimension, value, removeLabel string) ui.Element {
	t := theme.Current(c)
	chip := ButtonContent(c, ButtonStyle{Variant: Secondary, Size: SizeCompact, Selected: true}, func() {
		if dimension != "" {
			theme.Label(ui.Text(c, dimension).SingleLine()).TextColor(t.InkSubtle)
		}
		theme.Label(ui.Text(c, value).SingleLine()).MaxWidth(theme.ChipValueWidth).MinWidth(0).Shrink(1)
		icon(c, icons.X())
	})
	return chip.MaxWidthPercent(100).Label(removeLabel)
}
