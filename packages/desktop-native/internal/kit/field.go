package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Field labels the control that control builds, which the label names and
// focuses, as ui.Field does. Chain Description and Error on the result: the
// error marks the control invalid and draws its edge in danger.
func Field(c *ui.Context, label string, control func()) ui.Element {
	return ui.Field(c, label, control).Gap(theme.Unit)
}

// Input builds a single-line text input on the field surface with the
// strong field edge, editing value. The widget theme supplies the surface,
// edge, hover and invalid colors; this sets DESIGN.md's control metrics.
func Input(c *ui.Context, value *string) ui.Element {
	t := theme.Current(c)
	in := ui.TextInput(c, value).MinHeight(theme.ControlHeight).Padding(theme.Unit, theme.Unit*3).
		CaretColor(t.Action).PlaceholderColor(t.InkSubtle)
	focusRing(c, in, in.Focused())
	return theme.Body(in)
}

// SearchField builds a search input with a leading icon, editing query.
// The wrapper owns the field edge and draws the focus ring while the input
// has the focus. placeholder also names the input. The result is the
// wrapper; compare query to detect edits.
func SearchField(c *ui.Context, query *string, placeholder string) ui.Element {
	t := theme.Current(c)
	box := ui.Row(c).Gap(theme.Unit*2).MinHeight(theme.ControlHeight).PaddingX(theme.Unit*3).
		Background(t.SurfaceField).Border(theme.Rule, t.LineStrong).TextColor(t.InkSubtle)
	focusRing(c, box, box.FocusWithin())
	return box.Children(func() {
		icon(c, icons.Search())
		in := ui.TextInputBase(c, query).Grow(1).MinWidth(0).PaddingY(theme.Unit).FocusRing(false).
			Placeholder(placeholder).Label(placeholder).Search().TextColor(t.Ink).CaretColor(t.Action).PlaceholderColor(t.InkSubtle)
		theme.Body(in)
	})
}

// CheckboxOption builds a check box and its label as one compact row that
// toggles checked, as a filter option. The ring of the keyboard focus goes
// around the box.
func CheckboxOption(c *ui.Context, checked *bool, label string) ui.Element {
	t := theme.Current(c)
	row := ui.CheckboxBase(c, checked).Gap(theme.Unit * 2).MinHeight(theme.ControlCompact).PaddingX(theme.Unit)
	hover := ui.Transparent
	if row.Hovered() && !row.IsDisabled() {
		hover = t.SurfaceRaised
	}
	row.Background(fade(c, row, "bg", hover))
	ring := row.FocusVisible()
	row.FocusRing(false)
	theme.Body(row)
	return row.Children(func() {
		fill, edge := t.Surface, t.LineStrong
		if *checked {
			fill, edge = t.Action, t.Action
		}
		box := ui.Box(c).Size(theme.IconSize, theme.IconSize).Shrink(0).Center().
			Background(fade(c, row, "fill", fill)).Border(theme.Rule, fade(c, row, "edge", edge)).TextColor(t.ActionInk)
		focusRing(c, box, ring)
		box.Children(func() {
			if *checked {
				ui.Icon(c, icons.Check()).Size(theme.CheckSize, theme.CheckSize)
			}
		})
		theme.Body(ui.Text(c, label)).MinWidth(0).TextColor(t.Ink)
	})
}
