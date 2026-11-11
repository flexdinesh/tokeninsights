// Package theme owns the native app's appearance: the DESIGN.md semantic color
// roles, type scale, control metrics and the bundled Geist font. Views read
// tokens from here and never write raw colors or sizes.
package theme

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Font is the registered interface family. Text falls back to the system font.
const Font = "Geist"

//go:embed fonts/geist-latin-variable.ttf
var geist []byte

// RegisterFonts registers the bundled Geist once per process before windows open.
func RegisterFonts() error { return ui.RegisterFont(geist, Font) }

// Type scale in DIPs, matching DESIGN.md typography at a 16 DIP root.
const (
	SizePage    float32 = 28
	SizeReadout float32 = 24
	SizeTitle   float32 = 16
	SizeBody    float32 = 14
	SizeLabel   float32 = 13
)

// Metrics in DIPs, matching DESIGN.md spacing, components and layout.
const (
	Unit           float32 = 4
	ControlHeight  float32 = 32
	ControlCompact float32 = 30
	ControlLarge   float32 = 44
	Rule           float32 = 1
	FocusRing      float32 = 2
	Gutter         float32 = 16
	SectionGap     float32 = 12
	SidebarWidth   float32 = 192
	HeaderHeight   float32 = 44
	ContentMax     float32 = 1600
)

// ChartColors is the number of categorical chart series colors.
const ChartColors = 12

// Tokens are the semantic color roles of one appearance.
type Tokens struct {
	Dark bool

	Surface, SurfaceRaised, SurfaceOverlay, SurfaceField ui.Color
	SurfaceRail, SurfaceHeading, SurfaceHover            ui.Color

	Ink, InkMuted, InkSubtle, InkHover ui.Color
	Line, LineStrong                   ui.Color

	Action, ActionInk, Selected, SelectedInk, Focus ui.Color

	StatusSuccess, StatusWarning, StatusDanger, StatusDangerInk, StatusDangerSurface ui.Color

	Chart [ChartColors]ui.Color
}

// role pairs one DESIGN.md color role with its light and dark values.
type role struct {
	name, light, dark string
	field             func(*Tokens) *ui.Color
}

// roles mirrors the DESIGN.md frontmatter; the guard test fails on drift.
var roles = []role{
	{"surface", "#ffffff", "#121316", func(t *Tokens) *ui.Color { return &t.Surface }},
	{"surface-raised", "#f7f7f7", "#17181c", func(t *Tokens) *ui.Color { return &t.SurfaceRaised }},
	{"surface-overlay", "#ffffff", "#17181c", func(t *Tokens) *ui.Color { return &t.SurfaceOverlay }},
	{"surface-field", "#ffffff", "#17181c", func(t *Tokens) *ui.Color { return &t.SurfaceField }},
	{"surface-rail", "#ffffff", "#141519", func(t *Tokens) *ui.Color { return &t.SurfaceRail }},
	{"surface-heading", "#ffffff", "#17181c", func(t *Tokens) *ui.Color { return &t.SurfaceHeading }},
	{"surface-hover", "#f7f7f7", "#17181c", func(t *Tokens) *ui.Color { return &t.SurfaceHover }},
	{"ink", "#22252b", "#ecedf0", func(t *Tokens) *ui.Color { return &t.Ink }},
	{"ink-muted", "#5f6269", "#a2a5ad", func(t *Tokens) *ui.Color { return &t.InkMuted }},
	{"ink-subtle", "#5f6269", "#a2a5ad", func(t *Tokens) *ui.Color { return &t.InkSubtle }},
	{"ink-hover", "#285bd4", "#82aaff", func(t *Tokens) *ui.Color { return &t.InkHover }},
	{"line", "#e6e6e8", "#2a2c31", func(t *Tokens) *ui.Color { return &t.Line }},
	{"line-strong", "#c9cacf", "#4a4c53", func(t *Tokens) *ui.Color { return &t.LineStrong }},
	{"action", "#285bd4", "#82aaff", func(t *Tokens) *ui.Color { return &t.Action }},
	{"action-ink", "#ffffff", "#121316", func(t *Tokens) *ui.Color { return &t.ActionInk }},
	{"selected", "#eef2fc", "#1f2738", func(t *Tokens) *ui.Color { return &t.Selected }},
	{"selected-ink", "#285bd4", "#82aaff", func(t *Tokens) *ui.Color { return &t.SelectedInk }},
	{"focus", "#285bd4", "#82aaff", func(t *Tokens) *ui.Color { return &t.Focus }},
	{"status-success", "oklch(0.49 0.14 155)", "oklch(0.72 0.16 155)", func(t *Tokens) *ui.Color { return &t.StatusSuccess }},
	{"status-warning", "oklch(0.55 0.14 75)", "oklch(0.78 0.15 80)", func(t *Tokens) *ui.Color { return &t.StatusWarning }},
	{"status-danger", "oklch(0.56 0.22 27)", "oklch(0.68 0.19 23)", func(t *Tokens) *ui.Color { return &t.StatusDanger }},
	{"status-danger-ink", "#ffffff", "#121316", func(t *Tokens) *ui.Color { return &t.StatusDangerInk }},
	{"status-danger-surface", "oklch(0.96 0.025 25)", "oklch(0.2 0.04 25)", func(t *Tokens) *ui.Color { return &t.StatusDangerSurface }},
	{"chart-1", "#285bd4", "#82aaff", func(t *Tokens) *ui.Color { return &t.Chart[0] }},
	{"chart-2", "#b45c7f", "#e59aba", func(t *Tokens) *ui.Color { return &t.Chart[1] }},
	{"chart-3", "oklch(0.67 0.16 65)", "oklch(0.76 0.15 70)", func(t *Tokens) *ui.Color { return &t.Chart[2] }},
	{"chart-4", "oklch(0.58 0.16 165)", "oklch(0.72 0.14 165)", func(t *Tokens) *ui.Color { return &t.Chart[3] }},
	{"chart-5", "oklch(0.6 0.2 25)", "oklch(0.72 0.18 25)", func(t *Tokens) *ui.Color { return &t.Chart[4] }},
	{"chart-6", "oklch(0.62 0.18 330)", "oklch(0.74 0.16 330)", func(t *Tokens) *ui.Color { return &t.Chart[5] }},
	{"chart-7", "oklch(0.55 0.14 200)", "oklch(0.72 0.13 200)", func(t *Tokens) *ui.Color { return &t.Chart[6] }},
	{"chart-8", "oklch(0.63 0.16 125)", "oklch(0.75 0.14 125)", func(t *Tokens) *ui.Color { return &t.Chart[7] }},
	{"chart-9", "oklch(0.61 0.15 90)", "oklch(0.78 0.14 90)", func(t *Tokens) *ui.Color { return &t.Chart[8] }},
	{"chart-10", "oklch(0.54 0.16 225)", "oklch(0.7 0.15 225)", func(t *Tokens) *ui.Color { return &t.Chart[9] }},
	{"chart-11", "oklch(0.58 0.15 355)", "oklch(0.73 0.14 355)", func(t *Tokens) *ui.Color { return &t.Chart[10] }},
	{"chart-12", "oklch(0.54 0.11 45)", "oklch(0.7 0.1 45)", func(t *Tokens) *ui.Color { return &t.Chart[11] }},
}

var light, dark = mustBuild(false), mustBuild(true)

// For returns the tokens of the light or dark appearance.
func For(isDark bool) *Tokens {
	if isDark {
		return &dark
	}
	return &light
}

func mustBuild(isDark bool) Tokens {
	tokens, err := build(isDark)
	if err != nil {
		panic(err)
	}
	return tokens
}

func build(isDark bool) (Tokens, error) {
	tokens := Tokens{Dark: isDark}
	for _, r := range roles {
		spec := r.light
		if isDark {
			spec = r.dark
		}
		value, err := parseColor(spec)
		if err != nil {
			return Tokens{}, fmt.Errorf("theme role %s: %w", r.name, err)
		}
		*r.field(&tokens) = value
	}
	return tokens, nil
}

// parseColor reads the two DESIGN.md color forms: #rrggbb and oklch(L C H).
func parseColor(spec string) (ui.Color, error) {
	if strings.HasPrefix(spec, "#") {
		if len(spec) != len("#rrggbb") {
			return ui.Color{}, fmt.Errorf("unsupported hex color %q", spec)
		}
		if _, err := strconv.ParseUint(spec[1:], 16, 32); err != nil {
			return ui.Color{}, fmt.Errorf("invalid hex color %q", spec)
		}
		return ui.Hex(spec), nil
	}
	inner, ok := strings.CutPrefix(spec, "oklch(")
	if !ok {
		return ui.Color{}, fmt.Errorf("unsupported color %q", spec)
	}
	inner, ok = strings.CutSuffix(inner, ")")
	parts := strings.Fields(inner)
	if !ok || len(parts) != 3 {
		return ui.Color{}, fmt.Errorf("invalid oklch color %q", spec)
	}
	var values [3]float32
	for i, part := range parts {
		value, err := strconv.ParseFloat(part, 32)
		if err != nil {
			return ui.Color{}, fmt.Errorf("invalid oklch color %q", spec)
		}
		values[i] = float32(value)
	}
	return ui.Oklch(values[0], values[1], values[2]), nil
}

// Widgets returns the MyGo widget theme for these tokens: square corners,
// Geist body text and the quiet blue for accent and focus.
func (t *Tokens) Widgets() *ui.Theme {
	base := ui.LightTheme()
	if t.Dark {
		base = ui.DarkTheme()
	}
	theme := *base
	theme.Background = t.Surface
	theme.Surface = t.SurfaceField
	theme.SurfaceHover = t.SurfaceHover
	theme.SurfacePressed = t.SurfaceRaised
	theme.Border = t.LineStrong
	theme.Text = t.Ink
	theme.TextMuted = t.InkMuted
	theme.Accent = t.Action
	theme.AccentHover = t.Action.Alpha(hoverOpacity)
	theme.AccentPressed = t.Action
	theme.AccentText = t.ActionInk
	theme.Danger = t.StatusDanger
	theme.Warning = t.StatusWarning
	theme.Success = t.StatusSuccess
	theme.Selection = t.Selected
	theme.Focus = t.Focus
	theme.Radius = 0
	theme.Spacing = Unit
	theme.FontSize = SizeBody
	theme.Font = Font
	return &theme
}

// hoverOpacity is DESIGN.md's filled-button hover: reduce to 90%.
const hoverOpacity = 0.9

// Letter spacing in em, per DESIGN.md typography.
const (
	trackingDisplay float32 = -0.035
	trackingTitle   float32 = -0.015
	weightStrong            = 600
	weightBrand             = 650
)

// Page styles page and startup headings.
func Page(e ui.Element) ui.Element {
	return e.FontSize(SizePage).FontWeight(weightStrong).LineHeight(1.15).LetterSpacing(trackingDisplay * SizePage)
}

// Readout styles usage readout values with tabular digits.
func Readout(e ui.Element) ui.Element {
	return e.FontSize(SizeReadout).FontWeight(weightStrong).LineHeight(1.15).LetterSpacing(trackingDisplay * SizeReadout).FontFeatures("tnum")
}

// Title styles section headings.
func Title(e ui.Element) ui.Element {
	return e.FontSize(SizeTitle).FontWeight(weightStrong).LineHeight(1.5).LetterSpacing(trackingTitle * SizeTitle)
}

// Body styles explanations, table cells and controls.
func Body(e ui.Element) ui.Element { return e.FontSize(SizeBody).LineHeight(1.4286) }

// Label styles captions, axes, compact controls and metadata.
func Label(e ui.Element) ui.Element { return e.FontSize(SizeLabel).LineHeight(1.3333) }

// Brand styles the TokenInsights wordmark.
func Brand(e ui.Element) ui.Element { return e.FontSize(SizeTitle).FontWeight(weightBrand) }
