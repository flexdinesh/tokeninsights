package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Section is what a SectionHeader shows. Count, when set, shows in a badge
// beside the title; HideDescription keeps the description for assistive
// technology only.
type Section struct {
	Title, Count, Description string
	HideDescription           bool
}

// SectionHeader builds the title row of an analytics or settings section,
// with the actions that actions builds wrapping at its end.
func SectionHeader(c *ui.Context, s Section, actions func()) ui.Element {
	t := theme.Current(c)
	row := ui.Row(c).Wrap().Justify(ui.SpaceBetween).GapX(theme.Unit * 4).GapY(theme.Unit * 2).PaddingY(theme.Unit * 2)
	return row.Children(func() {
		ui.Column(c).MinWidth(0).Children(func() {
			ui.Row(c).Gap(theme.Unit * 2).Children(func() {
				heading := theme.Title(ui.Text(c, s.Title)).TextColor(t.Ink).Role(ui.RoleHeading).Level(2)
				if s.HideDescription && s.Description != "" {
					heading.Description(s.Description)
				}
				if s.Count != "" {
					Badge(c, s.Count, Secondary)
				}
			})
			if s.Description != "" && !s.HideDescription {
				theme.Label(ui.Text(c, s.Description)).TextColor(t.InkSubtle).Margin(theme.Unit, 0, 0, 0)
			}
		})
		if actions != nil {
			ui.Row(c).MarginStart(ui.Auto).MinWidth(0).Wrap().Justify(ui.End).GapX(theme.Unit * 3).GapY(theme.Unit * 2).Children(actions)
		}
	})
}

// ControlLabel builds a caption beside a compact control, as "Sort" before
// the sort select.
func ControlLabel(c *ui.Context, label string, control func()) ui.Element {
	t := theme.Current(c)
	row := ui.Row(c).Gap(theme.Unit * 2).TextColor(t.InkSubtle)
	return row.Children(func() {
		theme.Label(ui.Text(c, label))
		control()
	})
}
