package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// disclosureTurn is how far a disclosure's chevron turns as it opens: from
// pointing down to pointing up.
const disclosureTurn float32 = 180

// Disclosure builds the trigger of a disclosure of content shown below it
// while *open, on ui.CollapsibleBase, in the look of a compact Link
// button: label, then a chevron that turns to point up as it opens over
// DESIGN.md's disclosure duration, as the web's disclosure buttons. A
// click, Enter or Space toggles *open, and assistive technology reads an
// expanded or collapsed disclosure named label. Build the content with
// the result's Panel where it shows; it grows into view as it opens. Give
// it a key.
func Disclosure(c *ui.Context, open *bool, label string) ui.CollapsibleParts {
	p := ui.CollapsibleBase(c, open)
	trigger := p.Trigger
	underline := styleButton(c, trigger, ButtonStyle{Variant: Link, Size: SizeCompact})
	trigger.AlignItems(ui.Center).Gap(theme.Unit).Label(label)
	target := float32(0)
	if *open {
		target = 1
	}
	turn := trigger.AnimateTo("turn", target, ui.Animation{Duration: theme.DurationDisclosure})
	trigger.Children(func() {
		m := SizeCompact.metrics()
		text := m.text(ui.Text(c, label).SingleLine())
		if underline {
			text.Underline().UnderlineOffset(theme.UnderlineOffset * m.font)
		}
		icon(c, icons.ChevronDown()).Rotate(disclosureTurn * turn)
	})
	return p
}
