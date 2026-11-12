package kit

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

// galleryDirEnv names the directory the gallery writes its PNGs to, one per
// section and appearance. Unset, the gallery only renders and asserts.
const galleryDirEnv = "TOKENINSIGHTS_NATIVE_GALLERY_DIR"

// galleryScale renders PNGs at a Retina scale for review.
const galleryScale = 2

// settle is longer than any kit transition or tooltip delay.
const settle = time.Second

func TestMain(m *testing.M) {
	if err := theme.RegisterFonts(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// gallery is the state the gallery's controls edit.
type gallery struct {
	clicks           int
	checkedPi        bool
	checkedCodex     bool
	sort, metric     string
	period           string
	name, password   string
	query            string
	popover, removed bool
}

func newGallery() *gallery {
	return &gallery{checkedPi: true, sort: "total", metric: "total", period: "daily", password: "short"}
}

var (
	sortOptions   = []Option[string]{{"total", "Total"}, {"date", "Date"}}
	metricOptions = []Option[string]{{"total", "Total"}, {"input", "Input"}, {"output", "Output"}}
	periodOptions = []Option[string]{{"daily", "Daily"}, {"monthly", "Monthly"}}
	variants      = []struct {
		name    string
		variant Variant
	}{{"primary", Primary}, {"outline", Outline}, {"secondary", Secondary}, {"ghost", Ghost}, {"quiet", Quiet}, {"link", Link}, {"destructive", Destructive}}
	sizes = []Size{SizeCompact, SizeDefault, SizeLarge}
)

// frame themes the window from the tester's appearance and lays a section
// out on the workspace surface.
func frame(c *ui.Context, build func()) {
	t := theme.Current(c)
	c.SetTheme(t.Widgets())
	ui.Column(c).Fill().Padding(theme.Gutter).Gap(theme.SectionGap).Background(t.Surface).TextColor(t.Ink).Children(build)
}

// section is one gallery page: what it builds, its size, what it must show,
// and the input that brings it to the state it shows.
type section struct {
	name          string
	width, height int
	texts         []string
	build         func(g *gallery, c *ui.Context)
	prepare       func(t *testing.T, tt *ui.Tester)
}

var sections = []section{
	{name: "buttons", width: 900, height: 300, texts: []string{"primary", "destructive", "Selected", "Disabled", "Harness:"}, build: buttonsSection},
	{name: "badges-chips", width: 600, height: 140, texts: []string{"60", "New", "Outline", "Error", "Model", "model-a"}, build: badgesSection},
	{name: "readouts", width: 900, height: 560, texts: []string{"Total tokens", "Total tokens: 1,234,567", "Sessions shown"}, build: readoutsSection},
	{name: "status", width: 600, height: 120, texts: []string{"Ready", "Loading", "Attention", "Synced", "Pending", "Failed"}, build: statusSection},
	{name: "alerts", width: 720, height: 220, texts: []string{"Processing usage", "Connection unavailable", "Retry"}, build: alertsSection},
	{name: "empty-skeleton", width: 720, height: 360, texts: []string{"No usage matches these filters", "Clear filters"}, build: emptySection},
	{name: "section-header", width: 900, height: 180, texts: []string{"Models", "1,204", "Sort", "Total"}, build: sectionHeaderSection},
	{name: "fields", width: 560, height: 520, texts: []string{"Key name", "Shown in the key list.", "Use 15–128 characters.", "Daily", "pi"}, build: fieldsSection},
	{name: "popover", width: 560, height: 360, texts: []string{"Harness", "Clear", "pi", "codex"}, build: popoverSection, prepare: openPopover},
	{name: "select-open", width: 560, height: 240, texts: []string{"Date"}, build: sectionHeaderSelect, prepare: openSelect},
	{name: "tooltip", width: 420, height: 160, texts: []string{"Columns"}, build: tooltipSection, prepare: restOnColumns},
	{name: "focus", width: 560, height: 200, texts: []string{"Focused"}, build: focusSection, prepare: tabTwice},
	{name: "typography", width: 720, height: 320, texts: []string{"Page heading", "Section heading"}, build: typographySection},
}

func buttonsSection(g *gallery, c *ui.Context) {
	for _, size := range sizes {
		ui.Row(c).Wrap().Gap(theme.Unit * 3).Children(func() {
			for _, v := range variants {
				if Button(c, v.name, ButtonStyle{Variant: v.variant, Size: size}).Clicked() {
					g.clicks++
				}
			}
		})
	}
	ui.Row(c).Wrap().Gap(theme.Unit * 3).Children(func() {
		Button(c, "Selected", ButtonStyle{Variant: Outline, Size: SizeCompact, Selected: true})
		Button(c, "Disabled", ButtonStyle{Variant: Outline, Size: SizeCompact, Disabled: true})
		IconButton(c, icons.Columns3(), "Columns", ButtonStyle{Variant: Outline, Size: SizeCompact})
		IconButton(c, icons.Activity(), "Activity", ButtonStyle{Variant: Outline})
		IconButton(c, icons.ChevronRight(), "Next month", ButtonStyle{Variant: Outline, Size: SizeCompact, Disabled: true})
		filterTrigger(c, "Harness:", "All", "", false)
		filterTrigger(c, "Model:", "model-a", "1", true)
	})
}

// filterTrigger is the web catalog's filter trigger, composed from the kit.
func filterTrigger(c *ui.Context, dimension, value, count string, selected bool) ui.Element {
	t := theme.Current(c)
	return ButtonContent(c, ButtonStyle{Variant: Outline, Size: SizeCompact, Selected: selected}, func() {
		ui.Text(c, dimension)
		ui.Text(c, value).TextColor(t.InkSubtle)
		if count != "" {
			Badge(c, count, Secondary)
		}
		icon(c, icons.ChevronDown())
	}).Gap(theme.Unit)
}

func badgesSection(g *gallery, c *ui.Context) {
	ui.Row(c).Gap(theme.Unit * 3).Children(func() {
		Badge(c, "60", Secondary)
		Badge(c, "New", Primary)
		Badge(c, "Outline", Outline)
		Badge(c, "Error", Destructive)
	})
	ui.Row(c).Wrap().Gap(theme.Unit * 2).Children(func() {
		if !g.removed && Chip(c, "Model", "model-a", "Remove model model-a").Clicked() {
			g.removed = true
		}
		Chip(c, "Harness", "pi", "Remove harness pi")
		Chip(c, "Session", "a-very-long-session-identifier-that-truncates-inside-the-chip", "Remove session")
	})
}

var galleryReadouts = []Readout{
	{"Total tokens", "1.2M", "Total tokens: 1,234,567", "Included in your usage total"},
	{"Input", "840K", "Input: 840,112", "Uncached prompt tokens"},
	{"Output", "92K", "Output: 92,003", "12K reasoning tokens reported"},
	{"Cache read", "301K", "Cache read: 301,450", "4K cache write tokens"},
	{"Sessions shown", "48", "Sessions shown: 48", "52 synced across all dates & harnesses"},
}

func readoutsSection(_ *gallery, c *ui.Context) {
	ReadoutStrip(c, "Filtered usage summary", galleryReadouts, false)
	ReadoutStrip(c, "Narrow usage summary", galleryReadouts, true).MaxWidth(theme.ReadoutNarrow / 2 * 1.2)
}

func statusSection(_ *gallery, c *ui.Context) {
	ui.Row(c).Wrap().Gap(theme.Unit * 4).Children(func() {
		for _, s := range []struct {
			state StatusState
			label string
		}{{StatusReady, "Ready"}, {StatusLoading, "Loading"}, {StatusAttention, "Attention"}} {
			ui.Row(c).Gap(theme.Unit * 2).Children(func() {
				StatusIcon(c, s.state)
				theme.Label(ui.Text(c, s.label))
			})
		}
		ui.Row(c).Gap(theme.Unit * 2).Children(func() {
			Spinner(c)
			theme.Label(ui.Text(c, "Spinner"))
		})
	})
	ui.Row(c).Wrap().Gap(theme.Unit * 4).Children(func() {
		for _, d := range []struct {
			tone  DotTone
			label string
		}{{DotSuccess, "Synced"}, {DotPending, "Pending"}, {DotDanger, "Failed"}} {
			ui.Row(c).Gap(theme.Unit * 2).Children(func() {
				StatusDot(c, d.tone)
				theme.Label(ui.Text(c, d.label))
			})
		}
	})
}

func alertsSection(g *gallery, c *ui.Context) {
	Alert(c, AlertContent{Title: "Processing usage", Description: "Saved usage stays visible while processing runs."}, nil)
	Alert(c, AlertContent{Destructive: true, Icon: icons.CircleAlert(), Title: "Connection unavailable", Description: "Retry once the server is reachable."}, func() {
		if Button(c, "Retry", ButtonStyle{Variant: Outline, Size: SizeCompact}).Clicked() {
			g.clicks++
		}
	})
}

func emptySection(_ *gallery, c *ui.Context) {
	Skeleton(c).Height(theme.Unit * 32)
	EmptyState(c, icons.SearchX(), "No usage matches these filters", "Try a wider date range or clear your filters.", func() {
		Button(c, "Clear filters", ButtonStyle{Variant: Outline})
	})
}

func sectionHeaderSection(g *gallery, c *ui.Context) {
	SectionHeader(c, Section{Title: "Models", Count: "1,204"}, func() {
		ControlLabel(c, "Sort", func() {
			Select(c.Key("sort"), &g.sort, sortOptions, SizeCompact).Label("Sort by")
		})
		IconButton(c, icons.Columns3(), "Columns", ButtonStyle{Variant: Outline, Size: SizeCompact})
	})
	SectionHeader(c, Section{Title: "Usage over time", Description: "Token usage across the selected range"}, nil)
}

// sectionHeaderSelect is the header alone, to show its sort menu open.
func sectionHeaderSelect(g *gallery, c *ui.Context) {
	SectionHeader(c, Section{Title: "Models", Count: "1,204"}, func() {
		ControlLabel(c, "Sort", func() {
			Select(c.Key("sort"), &g.sort, sortOptions, SizeCompact).Label("Sort by")
		})
	})
}

func fieldsSection(g *gallery, c *ui.Context) {
	Field(c, "Key name", func() { Input(c, &g.name).Placeholder("Work laptop") }).Description("Shown in the key list.")
	Field(c, "New password", func() { Input(c, &g.password).Password() }).Error("Use 15–128 characters.")
	SearchField(c.Key("search"), &g.query, "Search model")
	ui.Row(c).Gap(theme.Unit * 6).Children(func() {
		Select(c.Key("period-select"), &g.period, periodOptions, SizeDefault).Label("Period")
		Segmented(c.Key("metric"), "Chart metric", &g.metric, metricOptions, ButtonStyle{Variant: Quiet, Size: SizeCompact})
		Segmented(c.Key("period"), "Period", &g.period, periodOptions, ButtonStyle{Variant: Outline})
	})
	ui.Row(c).Gap(theme.Unit * 2).Children(func() {
		CheckboxOption(c.Key("pi"), &g.checkedPi, "pi")
		CheckboxOption(c.Key("codex"), &g.checkedCodex, "codex")
	})
}

func popoverSection(g *gallery, c *ui.Context) {
	ui.Row(c).Justify(ui.Center).Children(func() {
		anchor := filterTrigger(c, "Harness:", "pi", "1", true)
		if anchor.Clicked() {
			g.popover = !g.popover
		}
		Popover(c, anchor, &g.popover, func() {
			PopoverHeader(c, "Harness", func() {
				Button(c, "Clear", ButtonStyle{Variant: Link, Size: SizeCompact})
			})
			SearchField(c.Key("search"), &g.query, "Search harness")
			CheckboxOption(c.Key("pi"), &g.checkedPi, "pi")
			CheckboxOption(c.Key("codex"), &g.checkedCodex, "codex")
		})
	})
}

func tooltipSection(_ *gallery, c *ui.Context) {
	ui.Row(c).Justify(ui.Center).Padding(theme.Unit*12, 0, 0, 0).Children(func() {
		IconButton(c, icons.Columns3(), "Columns", ButtonStyle{Variant: Outline, Size: SizeCompact})
	})
}

func typographySection(_ *gallery, c *ui.Context) {
	t := theme.Current(c)
	theme.Page(ui.Text(c, "Page heading"))
	theme.Title(ui.Text(c, "Section heading"))
	theme.Sub(ui.Text(c, "Sub heading"))
	theme.View(ui.Text(c, "/ Models")).TextColor(t.InkSubtle)
	theme.Body(ui.Text(c, "Body text explains a view in a sentence or two."))
	theme.Label(ui.Text(c, "Label text for captions and metadata")).TextColor(t.InkMuted)
	theme.Readout(ui.Text(c, "1,234,567"))
}

func focusSection(g *gallery, c *ui.Context) {
	ui.Row(c).Gap(theme.Unit * 4).Children(func() {
		Button(c, "First", ButtonStyle{Variant: Outline})
		Button(c, "Focused", ButtonStyle{Variant: Primary})
		CheckboxOption(c.Key("pi"), &g.checkedPi, "pi")
	})
	SearchField(c.Key("search"), &g.query, "Search model")
}

func tabTwice(t *testing.T, tt *ui.Tester) {
	t.Helper()
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Focused") {
		t.Error("Tab did not reach the second button")
	}
}

func openPopover(t *testing.T, tt *ui.Tester) {
	t.Helper()
	click(t, tt, "Harness:")
}

func openSelect(t *testing.T, tt *ui.Tester) {
	t.Helper()
	click(t, tt, "Sort by")
}

func restOnColumns(t *testing.T, tt *ui.Tester) {
	t.Helper()
	r, ok := tt.Find("Columns")
	if !ok {
		t.Fatal("no Columns button")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
}

func click(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	if err := tt.Click(name); err != nil {
		t.Fatal(err)
	}
}

// TestGallery renders every section in both appearances, checks what each
// shows, and writes PNGs when TOKENINSIGHTS_NATIVE_GALLERY_DIR is set.
func TestGallery(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range sections {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(s.name+"/"+appearance, func(t *testing.T) {
				g := newGallery()
				tt := ui.NewTesterWith(func(c *ui.Context) { frame(c, func() { s.build(g, c) }) }, s.width, s.height, ui.TesterOptions{Dark: dark})
				if s.prepare != nil {
					s.prepare(t, tt)
				}
				tt.Advance(settle)
				for _, text := range s.texts {
					if !tt.HasText(text) {
						t.Errorf("missing %q in %q", text, tt.Texts())
					}
				}
				for _, a := range tt.AccessAll() {
					if (a.Role == ui.RoleButton || a.Role == ui.RoleCheckBox || a.Role == ui.RolePopUpButton) && a.Name == "" {
						t.Errorf("unnamed %v at %v", a.Role, a.Bounds)
					}
				}
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, s.name+"-"+appearance+".png"), tt)
			})
		}
	}
}

func writePNG(t *testing.T, path string, tt *ui.Tester) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
