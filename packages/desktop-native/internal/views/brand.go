package views

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// brandRect is one rounded bar of the Split T mark, in the mark's 32-unit
// view box (assets/brand).
type brandRect struct {
	x, y, w, h float32
	action     bool
}

const (
	brandViewBox float32 = 32
	brandRadius  float32 = 1.5
	// brandInset pulls the mark toward the rail edge, as the web's -ml-0.5,
	// so the bars' optical edge meets the nav items' text.
	brandInset float32 = 2
)

// brandBars are the mark's four bars: the T's two arms, its neck and the
// foot that carries the brand's single action color.
var brandBars = []brandRect{
	{x: 4, y: 4, w: 10, h: 6},
	{x: 16, y: 4, w: 12, h: 6},
	{x: 12, y: 12, w: 8, h: 6},
	{x: 12, y: 20, w: 8, h: 8, action: true},
}

// Brand builds the Split T mark beside the TokenInsights wordmark.
func Brand(c *ui.Context) ui.Element {
	t := theme.Current(c)
	return ui.Row(c).Gap(theme.BrandGap).Children(func() {
		ui.Box(c).Size(theme.BrandMark, theme.BrandMark).Shrink(0).Margin(0, 0, 0, -brandInset).AccessHidden(true).
			Draw(func(p *ui.Painter, r ui.Rect) {
				scale := r.W / brandViewBox
				for _, bar := range brandBars {
					fill := t.Ink
					if bar.action {
						fill = t.Action
					}
					p.Fill(ui.Rect{X: r.X + bar.x*scale, Y: r.Y + bar.y*scale, W: bar.w * scale, H: bar.h * scale}, fill, brandRadius*scale)
				}
			})
		theme.Brand(ui.Text(c, "TokenInsights")).SingleLine()
	})
}
