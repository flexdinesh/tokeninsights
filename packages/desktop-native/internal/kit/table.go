package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// MyGo pads a table's cells by 1.5 and 2.5 spacing units, which TableCell
// and the headings cover to pad by DESIGN.md's 0.25rem and 0.75rem.
const (
	tableCellPadY = theme.Unit * 1.5
	tableCellPadX = theme.Unit * 2.5
)

// TableColumn is a column of a Table.
type TableColumn struct {
	// ID identifies the column among the others, for the widths the user
	// gives them (ui.ListState.Columns). Label heads the column and names
	// it.
	ID, Label string
	// Numeric aligns the heading and the cells at the end.
	Numeric bool
	// Width is the column's starting width; the user resizes it between
	// MinWidth and MaxWidth.
	Width, MinWidth, MaxWidth float32
	// Sorted is how the rows are sorted by the column.
	Sorted ui.SortDirection
}

// TableCellStyle places a cell's content: Numeric at the end, and
// LastRow without the rule under it, which the table's end draws.
type TableCellStyle struct {
	Numeric, LastRow bool
}

// Table builds a ui.Table of n rows under columns in DESIGN.md's look:
// fine rules, label headings on the heading surface and the neutral
// hover. Its rows are a list's, built only in view, cell building the
// content of each with TableCell. With sort, a click on a heading calls
// sort with the column's index: the heading shows the column's Sorted
// with the web's arrows, in the selected ink, and the app sorts the rows.
// The last column takes the room the others leave, at least its Width, so
// the rules and the hover span the table as the web's columns do; it has
// no edge to resize, as AppKit's last column has none. It sets
// s.HeaderCell. Size the result: it scrolls the rows past its height.
func Table(c *ui.Context, s *ui.ListState, columns []TableColumn, n int, cell func(row, col int), sort func(col int)) ui.Element {
	t := theme.Current(c)
	widgets := *c.Theme()
	widgets.Border = t.Line // the rule under the headings and the column edges
	specs := make([]ui.TableColumn, len(columns))
	for i, col := range columns {
		// The kit builds the headings, named by their labels.
		specs[i] = ui.TableColumn{ID: col.ID, Width: col.Width, MinWidth: col.MinWidth, MaxWidth: col.MaxWidth}
		if col.Numeric {
			specs[i].Align = ui.End
		}
	}
	if last := len(specs) - 1; last >= 0 {
		specs[last].MinWidth, specs[last].Width = max(specs[last].MinWidth, specs[last].Width), 0
	}
	s.HeaderCell = func(head ui.Element, col int) {
		var clicked func()
		if sort != nil {
			clicked = func() { sort(col) }
		}
		tableHeading(c, head, columns[col], clicked)
	}
	var table ui.Element
	ui.Themed(c, &widgets).Background(ui.Transparent).Children(func() {
		table = ui.Table(c, s, specs, n, cell)
		table.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: theme.TableRowHeight}, t.SurfaceHeading, 0)
			p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: theme.Rule}, t.Line, 0)
		})
	})
	return table
}

// TableCell builds a Table cell's content, which content builds, filling
// the cell with DESIGN.md's padding over MyGo's, centered on the row,
// with the rule under the row.
func TableCell(c *ui.Context, s TableCellStyle, content func()) ui.Element {
	t := theme.Current(c)
	box := tableCellBox(c, s.Numeric).PaddingY(theme.Unit)
	if !s.LastRow {
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - theme.Rule, W: r.W, H: theme.Rule}, t.Line, 0)
		})
	}
	return box.Children(content)
}

// tableCellBox is a row filling a table cell over MyGo's padding, padded
// by DESIGN.md's 0.75rem across, its content at the end when numeric.
func tableCellBox(c *ui.Context, numeric bool) ui.Element {
	box := ui.Row(c).Grow(1).AlignSelf(ui.Stretch).MinWidth(0).Margin(-tableCellPadY, -tableCellPadX).
		PaddingX(theme.Unit * 3).Gap(theme.Unit)
	if numeric {
		box.Justify(ui.End)
	}
	return box
}

// tableHeading builds a column's heading into its header cell head, as
// the web's TableHead: the label in medium label type and muted ink, and
// with sort, the arrows of its order; the sorted column and the hovered
// heading take the selected ink. A click calls sort.
func tableHeading(c *ui.Context, head ui.Element, col TableColumn, sort func()) {
	t := theme.Current(c)
	head.Label(col.Label)
	ink := t.InkMuted
	if sort != nil {
		head.Sort(col.Sorted).Cursor(ui.CursorPointer).OnClick(sort)
		if col.Sorted != ui.SortNone || head.Hovered() {
			ink = t.SelectedInk
		}
	}
	box := tableCellBox(c, col.Numeric).TextColor(fade(c, head, "ink", ink))
	box.Children(func() {
		theme.Medium(theme.Label(ui.Text(c, col.Label).SingleLine())).MinWidth(0)
		if sort == nil {
			return
		}
		switch col.Sorted {
		case ui.SortAscending:
			icon(c, icons.ArrowUp())
		case ui.SortDescending:
			icon(c, icons.ArrowDown())
		default:
			icon(c, icons.ArrowUpDown()).TextColor(t.InkSubtle)
		}
	})
}
