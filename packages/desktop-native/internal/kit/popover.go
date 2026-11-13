package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Popover shows a panel below anchor, centered on it, while open is true;
// a press outside or Escape sets open to false. content builds the panel's
// children; the panel is 22rem wide with 0.75rem padding and fades and
// scales in. It returns the panel, invalid while closed.
func Popover(c *ui.Context, anchor ui.Element, open *bool, content func()) ui.Element {
	t := theme.Current(c)
	return ui.PopoverBase(c, anchor, open, func(panel ui.Element) {
		overlay(t, panel).AttachTo(anchor, ui.AnchorBottom, ui.AnchorTop).AttachEdgeMargin(theme.PopoverEdge).
			Margin(theme.PopoverOffset, 0, 0, 0).Width(theme.PopoverWidth).Padding(theme.Unit * 3).
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
