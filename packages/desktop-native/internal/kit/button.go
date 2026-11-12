package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Variant is a control's DESIGN.md look. Buttons take every variant; badges
// take Primary, Secondary, Outline and Destructive.
type Variant int

const (
	// Primary is filled with the action blue (the web's "default").
	Primary Variant = iota
	// Outline is the field edge on the workspace surface.
	Outline
	// Secondary is the raised surface with muted ink.
	Secondary
	// Ghost has no surface until hovered.
	Ghost
	// Quiet is muted ink without a surface until hovered.
	Quiet
	// Link is main ink without padding, blue and underlined when hovered.
	Link
	// Destructive is filled with the danger status.
	Destructive
)

// Size is a control's height and type: Default is 2rem body text, Compact
// 1.875rem label text for filters, chips and table controls, Large 2.75rem.
type Size int

const (
	SizeDefault Size = iota
	SizeCompact
	SizeLarge
)

// sizeMetrics are the height, horizontal padding and type of one Size.
type sizeMetrics struct {
	height, padX, font float32
	text               func(ui.Element) ui.Element
}

func (s Size) metrics() sizeMetrics {
	switch s {
	case SizeCompact:
		return sizeMetrics{theme.ControlCompact, theme.Unit * 2.5, theme.SizeLabel, theme.Label}
	case SizeLarge:
		return sizeMetrics{theme.ControlLarge, theme.Unit * 4, theme.SizeTitle, theme.Title}
	}
	return sizeMetrics{theme.ControlHeight, theme.Unit * 3, theme.SizeBody, theme.Body}
}

// ButtonStyle selects a button's look. Selected gives any variant the
// selection tint, kept while hovered. Disabled dims the button and turns off
// its hover; set it here rather than on the returned element so the look
// follows in the same frame.
type ButtonStyle struct {
	Variant  Variant
	Size     Size
	Selected bool
	Disabled bool
}

// controlColors are the fill, ink and edge of a control in one state. A zero
// ink inherits the surrounding text color, as the web's unstyled variants do.
type controlColors struct {
	background, ink, border ui.Color
	underline               bool
}

// buttonColors resolves DESIGN.md's button treatment for a variant, its
// selection and whether the pointer is over or pressing it. Pressing shares
// the hover look; the web has no separate active treatment.
func buttonColors(t *theme.Tokens, s ButtonStyle, hot bool) controlColors {
	if s.Selected {
		return controlColors{background: t.Selected, ink: t.SelectedInk}
	}
	neutralHover := controlColors{background: t.SurfaceHover, ink: t.Ink}
	switch s.Variant {
	case Outline:
		if hot {
			neutralHover.border = t.LineStrong
			return neutralHover
		}
		return controlColors{background: t.Surface, border: t.LineStrong}
	case Secondary:
		if hot {
			return controlColors{background: t.SurfaceRaised.Alpha(theme.HoverOpacityRaised), ink: t.InkMuted}
		}
		return controlColors{background: t.SurfaceRaised, ink: t.InkMuted}
	case Ghost:
		if hot {
			return neutralHover
		}
		return controlColors{}
	case Quiet:
		if hot {
			return neutralHover
		}
		return controlColors{ink: t.InkMuted}
	case Link:
		if hot {
			return controlColors{ink: t.InkHover, underline: true}
		}
		return controlColors{ink: t.Ink}
	case Destructive:
		if hot {
			return controlColors{background: t.StatusDanger.Alpha(theme.HoverOpacity), ink: t.StatusDangerInk}
		}
		return controlColors{background: t.StatusDanger, ink: t.StatusDangerInk}
	}
	if hot {
		return controlColors{background: t.Action.Alpha(theme.HoverOpacity), ink: t.ActionInk}
	}
	return controlColors{background: t.Action, ink: t.ActionInk}
}

// styleButton gives a ButtonBase or segment the look of s and reports
// whether its label should be underlined.
func styleButton(c *ui.Context, e ui.Element, s ButtonStyle) bool {
	m := s.Size.metrics()
	e.MinHeight(m.height).Gap(theme.Unit * 2).Justify(ui.Center).Shrink(0)
	m.text(e)
	theme.Medium(e)
	if s.Variant == Link {
		e.PaddingX(0)
	} else {
		e.PaddingX(m.padX)
	}
	if s.Disabled {
		e.Disabled(true)
	}
	focusRing(c, e, e.FocusVisible())
	hot := !s.Disabled && (e.Hovered() || e.Pressed())
	colors := buttonColors(theme.Current(c), s, hot)
	e.Background(fade(c, e, "bg", colors.background)).Border(theme.Rule, fade(c, e, "edge", colors.border))
	if colors.ink.A != 0 {
		e.TextColor(fade(c, e, "ink", colors.ink))
	}
	return colors.underline
}

// Button builds a push button showing label. Query Clicked or OnClick on the
// result.
func Button(c *ui.Context, label string, s ButtonStyle) ui.Element {
	b := ui.ButtonBase(c)
	underline := styleButton(c, b, s)
	b.Children(func() {
		m := s.Size.metrics()
		text := m.text(ui.Text(c, label).SingleLine())
		if underline {
			text.Underline().UnderlineOffset(theme.UnderlineOffset * m.font)
		}
	})
	return b
}

// ButtonContent builds a button whose content builds, as an icon before a
// label. A button without text needs a Label.
func ButtonContent(c *ui.Context, s ButtonStyle, content func()) ui.Element {
	b := ui.ButtonBase(c)
	styleButton(c, b, s)
	return b.Children(content)
}

// IconButton builds a square button showing only icon, named by label,
// which also shows as its tooltip.
func IconButton(c *ui.Context, svg *ui.SVG, label string, s ButtonStyle) ui.Element {
	b := ButtonContent(c, s, func() { icon(c, svg) })
	side := s.Size.metrics().height
	b.Size(side, side).Padding(0).Label(label)
	tip(c, b, label)
	return b
}
