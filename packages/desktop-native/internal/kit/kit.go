// Package kit is the native component library: the DESIGN.md primitives of
// packages/dashboard/src/ui built on MyGo's headless bases. Each component is
// an immediate-mode function that builds for one frame and returns its root
// element for further layout. Components read colors and metrics only from
// package theme; state lives in caller-owned values passed by pointer, and
// stateful components take their key from the caller's c.Key.
package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// fade returns the color e shows this frame on its way to target over
// DESIGN.md's state transition, or target at once under reduced motion.
func fade(c *ui.Context, e ui.Element, key string, target ui.Color) ui.Color {
	if c.Preferences().ReduceMotion {
		return target
	}
	return e.AnimateColor(key, target, ui.Animation{Duration: theme.DurationFast})
}

// Icon draws a Lucide icon at the inline icon size in the inherited ink,
// as inside ButtonContent. It is decorative: pair it with text.
func Icon(c *ui.Context, svg *ui.SVG) ui.Element { return icon(c, svg) }

// icon draws a Lucide icon at the inline icon size in the inherited ink.
func icon(c *ui.Context, svg *ui.SVG) ui.Element {
	return ui.Icon(c, svg).Size(theme.IconSize, theme.IconSize).Shrink(0)
}

// overlay gives a floating layer its DESIGN.md surface: overlay fill, fine
// rule and the overlay shadow.
func overlay(t *theme.Tokens, e ui.Element) ui.Element {
	return e.Background(t.SurfaceOverlay).Border(theme.Rule, t.Line).TextColor(t.Ink).
		Shadow(0, theme.ShadowY, theme.ShadowBlur, 0, t.Shadow)
}

// focusRing replaces the theme's ring, whose corners round outside the
// control, with DESIGN.md's square outline: theme.FocusRing wide,
// theme.FocusOffset outside e, in the focus color, while shown.
func focusRing(c *ui.Context, e ui.Element, shown bool) {
	e.FocusRing(false)
	if !shown {
		return
	}
	color := theme.Current(c).Focus
	e.DrawOver(func(p *ui.Painter, r ui.Rect) {
		out := theme.FocusOffset + theme.FocusRing/2 // strokes center on the rectangle's edge
		p.Stroke(ui.Rect{X: r.X - out, Y: r.Y - out, W: r.W + 2*out, H: r.H + 2*out}, color, 0, theme.FocusRing)
	})
}
