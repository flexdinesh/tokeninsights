package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// PopoverOptions place a popover's panel below its anchor.
type PopoverOptions struct {
	// Width is the panel's width; zero is theme.PopoverWidth.
	Width float32
	// End lines the panel's end up with the anchor's end, as the web's
	// align="end"; Start lines their starts up, as align="start";
	// otherwise the panel centers on the anchor.
	End, Start bool
}

// Popover shows a panel below anchor, centered on it, while open is true;
// a press outside or Escape sets open to false. content builds the panel's
// children; the panel is 22rem wide with 0.75rem padding and fades and
// scales in. It returns the panel, invalid while closed.
func Popover(c *ui.Context, anchor ui.Element, open *bool, content func()) ui.Element {
	return PopoverWith(c, anchor, open, PopoverOptions{}, content)
}

// PopoverWith shows a popover as Popover does, placed and sized by o.
func PopoverWith(c *ui.Context, anchor ui.Element, open *bool, o PopoverOptions, content func()) ui.Element {
	t := theme.Current(c)
	width, at, self := theme.PopoverWidth, ui.AnchorBottom, ui.AnchorTop
	if o.Width != 0 {
		width = o.Width
	}
	switch {
	case o.End:
		at, self = ui.AnchorBottomEnd, ui.AnchorTopEnd
	case o.Start:
		at, self = ui.AnchorBottomStart, ui.AnchorTopStart
	}
	return ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		overlay(t, panel).AttachTo(anchor, at, self).AttachEdgeMargin(theme.PopoverEdge).
			Margin(theme.PopoverOffset, 0, 0, 0).Width(width).Padding(theme.Unit * 3).
			AttachedOrigin().Transition(ui.ElementTransition{
			Duration: theme.DurationFast,
			Enter:    &ui.Motion{Scale: popoverEnterScale, Flip: true},
			Exit:     &ui.Motion{Scale: popoverEnterScale, Flip: true},
		})
		content()
	})
}

// popoverEnterScale is where popovers scale in from, as the web's zoom-in-95.
const popoverEnterScale = 0.95

// PopoverHeader builds a popover's title row with an optional trailing
// action, such as a Clear link, which action builds.
func PopoverHeader(c *ui.Context, title string, action func()) ui.Element {
	row := ui.Row(c).MinHeight(theme.ControlCompact).Justify(ui.SpaceBetween).Gap(theme.Unit * 2)
	return row.Children(func() {
		theme.Sub(ui.Text(c, title)).Role(ui.RoleHeading).Level(3)
		if action != nil {
			action()
		}
	})
}

// Tooltip shows text by anchor while the pointer rests on it or the keyboard
// focus is on it, in the overlay look of chart tooltips, and describes anchor
// to assistive technology with it. It returns the tip, invalid while hidden.
func Tooltip(c *ui.Context, anchor ui.Element, text string) ui.Element {
	anchor.Description(text)
	return tip(c, anchor, text)
}

// tip shows text by anchor as Tooltip does, without describing anchor: for
// an icon button already named by the same text.
func tip(c *ui.Context, anchor ui.Element, text string) ui.Element {
	t := theme.Current(c)
	return ui.TooltipBase(c, anchor, func(box ui.Element) {
		overlay(t, box).AttachTo(anchor, ui.AnchorTop, ui.AnchorBottom).Margin(0, 0, theme.MenuOffset, 0).
			Padding(theme.Unit, theme.Unit*2).MaxWidth(theme.PopoverWidth)
		box.Children(func() { theme.Label(ui.Text(c, text)) })
	})
}
