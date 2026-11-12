package kit

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/icons"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/theme"
)

const (
	testWidth  = 640
	testHeight = 400
)

func newTester(view func(c *ui.Context)) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) { frame(c, func() { view(c) }) }, testWidth, testHeight)
}

func center(t *testing.T, tt *ui.Tester, name string) (float32, float32) {
	t.Helper()
	r, ok := tt.Find(name)
	if !ok {
		t.Fatalf("no %q in %q", name, tt.Texts())
	}
	return r.X + r.W/2, r.Y + r.H/2
}

func TestButtonClicksUnlessDisabled(t *testing.T) {
	saves, deletes := 0, 0
	tt := newTester(func(c *ui.Context) {
		if Button(c, "Save", ButtonStyle{}).Clicked() {
			saves++
		}
		if IconButton(c, icons.X(), "Delete", ButtonStyle{Variant: Destructive, Disabled: true}).Clicked() {
			deletes++
		}
	})
	click(t, tt, "Save")
	_ = tt.Click("Delete")
	if saves != 1 || deletes != 0 {
		t.Fatalf("saves %d, deletes %d", saves, deletes)
	}
	if del, ok := tt.AccessByRole(ui.RoleButton, "Delete"); !ok || !del.Disabled {
		t.Fatalf("disabled icon button: %+v %v", del, ok)
	}
}

func TestChipRemovesByItsAccessibleName(t *testing.T) {
	removed := false
	tt := newTester(func(c *ui.Context) {
		if !removed && Chip(c, "Model", "model-a", "Remove model model-a").Clicked() {
			removed = true
		}
	})
	click(t, tt, "Remove model model-a")
	if !removed || tt.HasText("model-a") {
		t.Fatal("chip was not removed", tt.Texts())
	}
}

func TestCheckboxOptionTogglesByPointerAndSpace(t *testing.T) {
	checked := false
	tt := newTester(func(c *ui.Context) { CheckboxOption(c.Key("pi"), &checked, "pi") })
	click(t, tt, "pi")
	if box, _ := tt.AccessByRole(ui.RoleCheckBox, "pi"); !checked || !box.Checked {
		t.Fatalf("click left it %v, %+v", checked, box)
	}
	tt.Key(0, ui.KeySpace)
	if checked {
		t.Fatal("Space did not uncheck the focused option")
	}
}

func TestSelectChoosesByPointerAndKeyboard(t *testing.T) {
	sort := "total"
	changes := 0
	tt := newTester(func(c *ui.Context) {
		if Select(c.Key("sort"), &sort, sortOptions, SizeCompact).Label("Sort by").Changed() {
			changes++
		}
	})
	click(t, tt, "Sort by")
	if _, ok := tt.AccessByRole(ui.RoleOption, "Date"); !ok {
		t.Fatal("menu did not open", tt.Texts())
	}
	click(t, tt, "Date")
	if sort != "date" || changes != 1 {
		t.Fatalf("sort %q after %d changes", sort, changes)
	}
	if trigger, _ := tt.AccessByRole(ui.RolePopUpButton, "Sort by"); trigger.Value != "Date" || !tt.Focused("Sort by") {
		t.Fatalf("trigger after choosing: %+v", trigger)
	}
	tt.Key(0, ui.KeyDown) // opens on the choice
	tt.Key(0, ui.KeyUp)
	tt.Key(0, ui.KeyEnter)
	if sort != "total" {
		t.Fatalf("keyboard chose %q", sort)
	}
}

func TestSegmentedChoosesTypedValue(t *testing.T) {
	metric := "total"
	tt := newTester(func(c *ui.Context) {
		Segmented(c.Key("metric"), "Chart metric", &metric, metricOptions, ButtonStyle{Variant: Quiet, Size: SizeCompact})
	})
	if group, ok := tt.AccessByRole(ui.RoleRadioGroup, "Chart metric"); !ok {
		t.Fatalf("no radio group: %+v", group)
	}
	click(t, tt, "Output")
	if output, _ := tt.AccessByRole(ui.RoleRadio, "Output"); metric != "output" || !output.Checked {
		t.Fatalf("metric %q, %+v", metric, output)
	}
	tt.Key(0, ui.KeyLeft)
	if metric != "input" {
		t.Fatalf("arrow chose %q", metric)
	}
}

func TestSegmentedWithoutChoiceLeavesValue(t *testing.T) {
	metric := "unknown"
	tt := newTester(func(c *ui.Context) {
		Segmented(c.Key("metric"), "Chart metric", &metric, metricOptions, ButtonStyle{Variant: Quiet, Size: SizeCompact})
	})
	for _, o := range metricOptions {
		if a, _ := tt.AccessByRole(ui.RoleRadio, o.Label); a.Checked {
			t.Fatal(o.Label, "is checked without a matching value")
		}
	}
	if metric != "unknown" {
		t.Fatal("building changed the value to", metric)
	}
}

func TestPopoverOpensAndDismisses(t *testing.T) {
	open := false
	tt := newTester(func(c *ui.Context) {
		ui.Row(c).Justify(ui.Center).Children(func() {
			anchor := Button(c, "Open popover", ButtonStyle{Variant: Outline, Size: SizeCompact})
			if anchor.Clicked() {
				open = !open
			}
			Popover(c, anchor, &open, func() {
				PopoverHeader(c, "Model", func() { Button(c, "Clear", ButtonStyle{Variant: Link, Size: SizeCompact}) })
			})
		})
	})
	click(t, tt, "Open popover")
	tt.Advance(settle)
	if heading, ok := tt.AccessByRole(ui.RoleHeading, "Model"); !open || !ok || heading.Level != 3 {
		t.Fatalf("open %v, heading %+v", open, heading)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(settle)
	if open || tt.HasText("Model") {
		t.Fatal("Escape left the popover open")
	}
	click(t, tt, "Open popover")
	tt.Advance(settle)
	tt.ClickAt(testWidth-1, testHeight-1)
	tt.Advance(settle)
	if open {
		t.Fatal("a press outside left the popover open")
	}
}

func TestPopoverWithAlignsEndAndSizes(t *testing.T) {
	open := true
	tt := newTester(func(c *ui.Context) {
		ui.Row(c).Justify(ui.End).Padding(theme.Unit * 12).Children(func() {
			anchor := Button(c, "Status", ButtonStyle{Variant: Ghost})
			PopoverWith(c, anchor, &open, PopoverOptions{Width: theme.StatusPopoverWidth, End: true}, func() {
				ui.Text(c, "Details")
			}).Label("Status details")
		})
	})
	tt.Advance(settle)
	anchor, _ := tt.Find("Status")
	panel, ok := tt.Access("Status details")
	if !ok {
		t.Fatal("no panel")
	}
	r := panel.Bounds
	if r.W != theme.StatusPopoverWidth || r.Y <= anchor.Y {
		t.Fatalf("panel %+v below anchor %+v", r, anchor)
	}
	// The panel's end lines up with the button's, whose label is centered in it.
	if end := r.X + r.W; end < anchor.X+anchor.W || end > anchor.X+anchor.W+theme.ControlHeight {
		t.Fatalf("panel end %v, anchor text %+v", end, anchor)
	}
}

func TestTooltipShowsOnRestAndDescribes(t *testing.T) {
	tt := newTester(func(c *ui.Context) {
		ui.Row(c).Padding(theme.Unit * 12).Gap(theme.Unit * 12).Children(func() {
			IconButton(c, icons.Columns3(), "Columns", ButtonStyle{Variant: Outline})
			Tooltip(c, Button(c, "Refresh", ButtonStyle{Variant: Outline}), "Capture new usage")
		})
	})
	if _, ok := tt.AccessByRole(ui.RoleTooltip, ""); ok {
		t.Fatal("tip shows before the pointer rests")
	}
	tt.Move(center(t, tt, "Columns"))
	tt.Advance(settle)
	if _, ok := tt.AccessByRole(ui.RoleTooltip, ""); !ok {
		t.Fatal("tip did not show", tt.Texts())
	}
	if button, _ := tt.AccessByRole(ui.RoleButton, "Columns"); button.Description != "" {
		t.Fatalf("icon button repeats its name as description %q", button.Description)
	}
	if button, _ := tt.AccessByRole(ui.RoleButton, "Refresh"); button.Description != "Capture new usage" {
		t.Fatalf("description %q", button.Description)
	}
}

func TestSearchFieldEditsQuery(t *testing.T) {
	query := ""
	tt := newTester(func(c *ui.Context) { SearchField(c.Key("search"), &query, "Search model") })
	click(t, tt, "Search model")
	tt.Type("gpt")
	if query != "gpt" {
		t.Fatalf("query %q", query)
	}
	if field, _ := tt.Access("Search model"); !field.Search {
		t.Fatalf("not a search field: %+v", field)
	}
}

func TestStructureReachesAssistiveTechnology(t *testing.T) {
	tt := newTester(func(c *ui.Context) {
		SectionHeader(c, Section{Title: "Models", Count: "12", Description: "Hidden help", HideDescription: true}, nil)
		Alert(c, AlertContent{Destructive: true, Description: "Connection unavailable"}, nil)
		ReadoutStrip(c, "Filtered usage summary", galleryReadouts[:2], false)
	})
	if heading, _ := tt.AccessByRole(ui.RoleHeading, "Models"); heading.Level != 2 || heading.Description != "Hidden help" {
		t.Fatalf("heading %+v", heading)
	}
	if tt.HasText("Hidden help") {
		t.Fatal("hidden description shows")
	}
	if _, ok := tt.AccessByRole(ui.RoleAlert, ""); !ok {
		t.Fatal("destructive alert is no alert")
	}
	if _, ok := tt.AccessByRole(ui.RoleRegion, "Filtered usage summary"); !ok {
		t.Fatal("readout strip is no named region")
	}
	if _, ok := tt.Access("Input: 840,112"); !ok {
		t.Fatal("readout value lacks its full accessible value")
	}
}

func TestMotionStopsUnderReducedMotion(t *testing.T) {
	tt := ui.NewTesterWith(func(c *ui.Context) {
		frame(c, func() {
			Spinner(c)
			Skeleton(c).Height(theme.ControlHeight)
		})
	}, testWidth, testHeight, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})
	tt.Frame()
	if tt.FramePending() {
		t.Fatal("spinner or skeleton keeps animating under reduced motion")
	}
}

func TestButtonColorsFollowDesign(t *testing.T) {
	for _, dark := range []bool{false, true} {
		tk := theme.For(dark)
		for _, v := range variants {
			rest := buttonColors(tk, ButtonStyle{Variant: v.variant}, false)
			hot := buttonColors(tk, ButtonStyle{Variant: v.variant}, true)
			if rest == hot {
				t.Errorf("%s has no hover treatment", v.name)
			}
			selected := ButtonStyle{Variant: v.variant, Selected: true}
			if s, sh := buttonColors(tk, selected, false), buttonColors(tk, selected, true); s != sh || s.background != tk.Selected || s.ink != tk.SelectedInk {
				t.Errorf("%s selection is not the tint kept while hovered: %+v %+v", v.name, s, sh)
			}
		}
		if got := buttonColors(tk, ButtonStyle{Variant: Primary}, true).background; got != tk.Action.Alpha(theme.HoverOpacity) {
			t.Error("filled hover is not 90%", got)
		}
		if got := buttonColors(tk, ButtonStyle{Variant: Quiet}, true); got.background != tk.SurfaceHover || got.ink != tk.Ink {
			t.Error("quiet hover is not neutral", got)
		}
		if got := buttonColors(tk, ButtonStyle{Variant: Link}, true); !got.underline || got.ink != tk.InkHover || got.background.A != 0 {
			t.Error("link hover is not blue and underlined", got)
		}
	}
}

func TestReadoutRows(t *testing.T) {
	for _, test := range []struct {
		n      int
		narrow bool
		want   [][]int
	}{
		{0, false, nil},
		{5, false, [][]int{{0, 1, 2, 3, 4}}},
		{5, true, [][]int{{0}, {1, 2}, {3, 4}}},
		{4, true, [][]int{{0}, {1, 2}, {3}}},
		{1, true, [][]int{{0}}},
	} {
		got := readoutRows(test.n, test.narrow)
		if !slices.EqualFunc(got, test.want, slices.Equal[[]int]) {
			t.Errorf("%d narrow %v = %v, want %v", test.n, test.narrow, got, test.want)
		}
	}
}

func TestOptionLookup(t *testing.T) {
	if indexOf(metricOptions, "output") != 2 || indexOf(metricOptions, "none") != -1 {
		t.Fatal("indexOf")
	}
	if labelOf(sortOptions, "date") != "Date" || labelOf(sortOptions, "none") != "" {
		t.Fatal("labelOf")
	}
}
