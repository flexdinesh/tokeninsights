package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// Readout is one static measurement of a ReadoutStrip. ValueLabel is the
// accessible full value, as the exact count behind an abbreviated Value.
type Readout struct {
	Label, Value, ValueLabel, Detail string
}

// ReadoutStrip builds a rule-separated strip of static readouts named label
// for assistive technology. The first readout is the total: wider than the
// others, and across both columns when narrow. Narrow strips, those
// narrower than theme.ReadoutNarrow, lay the readouts out in two columns
// without rules; the caller decides from the width it gives the strip
// (c.Size less its own chrome). Readouts pad to the page gutter, so place
// the strip without the gutter around it.
func ReadoutStrip(c *ui.Context, label string, readouts []Readout, narrow bool) ui.Element {
	t := theme.Current(c)
	strip := ui.Column(c).Role(ui.RoleRegion).Label(label).BorderWidth(0, 0, theme.Rule, 0).BorderColor(t.Line)
	return strip.Children(func() {
		for _, row := range readoutRows(len(readouts), narrow) {
			ui.Row(c).AlignItems(ui.Stretch).Children(func() {
				for _, i := range row {
					cell := readout(c, t, readouts[i])
					share := float32(1)
					if i == 0 && !narrow {
						share = theme.ReadoutPrimaryShare
					}
					cell.Basis(0).Grow(share)
					if i > 0 && !narrow {
						cell.BorderWidth(0, 0, 0, theme.Rule).BorderColor(t.Line)
					}
				}
				if narrow && len(row) == 1 && row[0] != 0 {
					ui.Box(c).Basis(0).Grow(1) // the empty second column
				}
			})
		}
	})
}

// readoutRows arranges n readouts: one row when wide; when narrow, the total
// alone, then the rest two by two.
func readoutRows(n int, narrow bool) [][]int {
	if n == 0 {
		return nil
	}
	if !narrow {
		row := make([]int, n)
		for i := range row {
			row[i] = i
		}
		return [][]int{row}
	}
	rows := [][]int{{0}}
	for i := 1; i < n; i += 2 {
		if i+1 < n {
			rows = append(rows, []int{i, i + 1})
		} else {
			rows = append(rows, []int{i})
		}
	}
	return rows
}

func readout(c *ui.Context, t *theme.Tokens, r Readout) ui.Element {
	cell := ui.Column(c).MinWidth(0).Gap(theme.Unit).Padding(theme.Unit*2, theme.Gutter)
	return cell.Children(func() {
		theme.Label(ui.Text(c, r.Label)).TextColor(t.InkMuted)
		theme.Readout(ui.Text(c, r.Value).SingleLine()).MinHeight(theme.ReadoutHeight).TextColor(t.Ink).Label(r.ValueLabel)
		if r.Detail != "" {
			theme.Label(ui.Text(c, r.Detail)).TextColor(t.InkSubtle)
		}
	})
}
