package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// StatusState is the header status: ready, loading or needing attention.
type StatusState int

const (
	StatusReady StatusState = iota
	StatusLoading
	StatusAttention
)

// DotTone is a status dot's meaning.
type DotTone int

const (
	DotSuccess DotTone = iota
	DotPending
	DotDanger
)

// Spinner builds the rotating loader in the inherited ink. Rotation stops
// under reduced motion, as on the web. It is decorative: pair it with text,
// or name it with Label.
func Spinner(c *ui.Context) ui.Element {
	spin := icon(c, icons.LoaderCircle())
	if !c.Preferences().ReduceMotion {
		spin.Rotate(spin.Loop("spin", theme.DurationSpin, ui.Linear) * 360)
	}
	return spin
}

// StatusIcon builds the fixed-size icon of a status state: Activity in
// subtle ink when ready, the spinner in selected ink while loading, and
// CircleAlert in danger when attention is needed. Pair it with a label.
func StatusIcon(c *ui.Context, state StatusState) ui.Element {
	t := theme.Current(c)
	switch state {
	case StatusLoading:
		return Spinner(c).TextColor(t.SelectedInk)
	case StatusAttention:
		return icon(c, icons.CircleAlert()).TextColor(t.StatusDanger)
	}
	return icon(c, icons.Activity()).TextColor(t.InkSubtle)
}

// StatusDot builds the small circular status mark, the one round shape of
// the design. It is decorative: the adjacent text carries the meaning.
func StatusDot(c *ui.Context, tone DotTone) ui.Element {
	t := theme.Current(c)
	fill := t.StatusSuccess
	switch tone {
	case DotPending:
		fill = t.InkSubtle
	case DotDanger:
		fill = t.StatusDanger
	}
	return ui.Box(c).Size(theme.DotSize, theme.DotSize).Shrink(0).Radius(theme.DotSize / 2).Background(fill).AccessHidden(true)
}
