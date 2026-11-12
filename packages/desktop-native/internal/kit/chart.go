package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// ChartTip builds the panel of a chart's tooltip, as the web's chart
// tooltips: title over lines of values in label type, on the overlay
// surface with its rule and shadow. Place it at a chart's selection with
// chart.Anchor; it does not place itself. It takes no input, so the
// pointer under it stays over the chart.
func ChartTip(c *ui.Context, title string, lines []string) ui.Element {
	t := theme.Current(c)
	panel := overlay(t, ui.Column(c)).Padding(theme.Unit * 3).Gap(theme.Unit).Role(ui.RoleTooltip).IgnoreInput(true)
	return theme.Label(panel).Children(func() {
		ui.Text(c, title).NoWrap()
		for _, line := range lines {
			ui.Text(c, line).NoWrap()
		}
	})
}

// Swatch is the small square of a chart series' color beside its name in
// a legend. It is decorative: pair it with the name.
func Swatch(c *ui.Context, color ui.Color) ui.Element {
	return ui.Box(c).Size(theme.SwatchSize, theme.SwatchSize).Shrink(0).Background(color)
}

// LegendButton builds a chart legend item that acts, as the web's
// category chart labels: a link-styled compact button of the series'
// swatch, its name, which wraps, and its share, which stays whole. Its
// accessible name is the name and share. Query Clicked on the result.
func LegendButton(c *ui.Context, color ui.Color, name, share string) ui.Element {
	s := ButtonStyle{Variant: Link, Size: SizeCompact}
	b := ui.ButtonBase(c)
	underline := styleButton(c, b, s)
	b.Justify(ui.Start).Shrink(1).MinWidth(0).Label(name + " " + share)
	return b.Children(func() {
		m := s.Size.metrics()
		Swatch(c, color)
		style := func(text ui.Element) {
			m.text(text)
			if underline {
				text.Underline().UnderlineOffset(theme.UnderlineOffset * m.font)
			}
		}
		style(ui.Text(c, name).MinWidth(0))
		style(ui.Text(c, share).NoWrap().Shrink(0).FontFeatures("tnum"))
	})
}
