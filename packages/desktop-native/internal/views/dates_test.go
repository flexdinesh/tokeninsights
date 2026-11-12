package views

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// The fixtures read at 13 Nov 2026 in Europe/London, in the month of the
// instance's default period.
const (
	monthlyTrigger = dateRangePrefix + "Monthly"
	currentMonth   = "November 2026"
)

// press clicks the element named name with role.
func press(t *testing.T, tt *ui.Tester, role ui.Role, name string) {
	t.Helper()
	a, ok := tt.AccessByRole(role, name)
	if !ok {
		t.Fatalf("no %q in %q", name, tt.Texts())
	}
	tt.ClickAt(a.Bounds.X+a.Bounds.W/2, a.Bounds.Y+a.Bounds.H/2)
	tt.Advance(settle)
}

// dateRangeOpen reports whether the date filter's popover shows.
func dateRangeOpen(tt *ui.Tester) bool {
	_, ok := tt.AccessByRole(ui.RoleRadioGroup, datePresetsLabel)
	return ok
}

// openDateRange opens the date filter showing trigger.
func openDateRange(t *testing.T, tt *ui.Tester, trigger string) {
	t.Helper()
	press(t, tt, ui.RoleButton, trigger)
	if !dateRangeOpen(tt) {
		t.Fatalf("date range did not open: %q", tt.Texts())
	}
}

// expectRequest checks the period and bounds of the latest usage read.
func (f *fixture) expectRequest(t *testing.T, period, from, to string) {
	t.Helper()
	if s := f.lastRequest().Selection; s.Period != period || s.From != from || s.To != to {
		t.Fatalf("usage request %+v, want period %q from %q to %q", s, period, from, to)
	}
}

func TestFilterToolbarSitsBetweenHeaderAndReadouts(t *testing.T) {
	tt := loaded().synced(windowWidth, windowHeight, false)
	if _, ok := tt.AccessByRole(ui.RoleRegion, dashboardLabel); !ok {
		t.Fatal("no filter toolbar")
	}
	assertTexts(t, tt, filtersLabel, "Monthly", currentMonth)
	header, _ := tt.Find("/ Tokens")
	caption, _ := tt.Find(filtersLabel)
	trigger, _ := tt.Find("Monthly")
	total, _ := tt.Find("Total tokens")
	month, _ := tt.Find(currentMonth)
	if !(header.Y < caption.Y && caption.Y < total.Y) || trigger.X <= caption.X || month.X <= trigger.X {
		t.Fatalf("toolbar layout: header %v caption %v trigger %v month %v total %v", header, caption, trigger, month, total)
	}
	if a, ok := tt.AccessByRole(ui.RoleButton, monthlyTrigger); !ok || a.Expanded {
		t.Fatalf("date trigger %+v", a)
	}

	// Without filter values the instance offers no toolbar; the period
	// navigation stays.
	f := newFixture()
	f.backend.instance.Capabilities = f.backend.instance.Capabilities[:1]
	f.window.Load()
	f.workers.run()
	tt = f.synced(windowWidth, windowHeight, false)
	if _, ok := tt.AccessByRole(ui.RoleRegion, dashboardLabel); ok {
		t.Fatal("toolbar without facets")
	}
	assertTexts(t, tt, currentMonth)
}

func TestDatePresetAppliesAtOnce(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	openDateRange(t, tt, monthlyTrigger)
	if a, _ := tt.AccessByRole(ui.RoleRadio, "Monthly"); !a.Checked {
		t.Fatalf("Monthly not chosen: %+v", a)
	}
	if tt.HasText(applyRangeLabel) {
		t.Fatal("custom bounds for a preset")
	}
	press(t, tt, ui.RoleRadio, "Yearly")
	f.sync(tt)
	f.expectRequest(t, "year", "", "")
	if dateRangeOpen(tt) {
		t.Fatal("a preset kept the popover open")
	}
	assertTexts(t, tt, "Yearly", "2026")

	// All time hides the navigation.
	openDateRange(t, tt, dateRangePrefix+"Yearly")
	press(t, tt, ui.RoleRadio, "All time")
	f.sync(tt)
	f.expectRequest(t, "all", "", "")
	if _, ok := tt.AccessByRole(ui.RoleNavigation, periodNavigationLabel); ok {
		t.Fatal("navigation for All time")
	}
}

func TestDatePresetChosenAgainReturnsToTheCurrentPeriod(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	press(t, tt, ui.RoleButton, previousPrefix+"month")
	f.sync(tt)
	f.expectRequest(t, "month", "2026-10-01", "2026-10-31")
	openDateRange(t, tt, monthlyTrigger)
	press(t, tt, ui.RoleRadio, "Monthly")
	f.sync(tt)
	f.expectRequest(t, "month", "", "")
	if dateRangeOpen(tt) {
		t.Fatal("choosing the preset again kept the popover open")
	}
	assertTexts(t, tt, currentMonth)
}

func TestDateFilterOpensAndClosesFromTheKeyboard(t *testing.T) {
	tt := loaded().synced(windowWidth, windowHeight, false)
	for range len(tt.AccessAll()) {
		if tt.Focused(monthlyTrigger) {
			break
		}
		tt.Key(0, ui.KeyTab)
	}
	if !tt.Focused(monthlyTrigger) {
		t.Fatal("Tab never reached the date filter")
	}
	for _, key := range []ui.Key{ui.KeyEnter, ui.KeySpace} {
		tt.Key(0, key)
		tt.Advance(settle)
		if !dateRangeOpen(tt) {
			t.Fatalf("%v did not open the date range", key)
		}
		if a, _ := tt.AccessByRole(ui.RoleButton, monthlyTrigger); !a.Expanded {
			t.Fatalf("open trigger not expanded: %+v", a)
		}
		tt.Key(0, ui.KeyTab) // into the popover
		tt.Key(0, ui.KeyEscape)
		tt.Advance(settle)
		if dateRangeOpen(tt) {
			t.Fatal("Escape kept the date range open")
		}
		if !tt.Focused(monthlyTrigger) {
			t.Fatal("closing did not return the focus to the trigger")
		}
	}
}

// chooseDay opens the date field named field and picks day, named as the
// calendar names it.
func chooseDay(t *testing.T, tt *ui.Tester, field, day string) {
	t.Helper()
	press(t, tt, ui.RolePopUpButton, field)
	press(t, tt, ui.RoleButton, day)
}

func TestCustomRangeValidatesAndApplies(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	openDateRange(t, tt, monthlyTrigger)
	press(t, tt, ui.RoleRadio, customLabel)
	if a, _ := tt.AccessByRole(ui.RoleRadio, customLabel); !a.Checked {
		t.Fatalf("Custom not chosen: %+v", a)
	}
	assertTexts(t, tt, fromLabel, toLabel, customHint, fromEmpty, toEmpty)
	if a, _ := tt.AccessByRole(ui.RoleButton, applyRangeLabel); !a.Disabled {
		t.Fatal("Apply range without bounds")
	}

	// Escape closes a field's calendar alone.
	press(t, tt, ui.RolePopUpButton, fromDateLabel)
	tt.Key(0, ui.KeyEscape)
	tt.Advance(settle)
	if a, _ := tt.AccessByRole(ui.RolePopUpButton, fromDateLabel); a.Expanded || !dateRangeOpen(tt) || !tt.Focused(fromDateLabel) {
		t.Fatalf("Escape in the calendar: field %+v, date range open %v", a, dateRangeOpen(tt))
	}

	chooseDay(t, tt, fromDateLabel, "November 20, 2026")
	chooseDay(t, tt, toDateLabel, "November 10, 2026")
	if !dateRangeOpen(tt) {
		t.Fatal("choosing a day closed the date range")
	}
	if alerts, ok := tt.AccessByRole(ui.RoleAlert, rangeOrderError); !ok {
		t.Fatalf("no order error: %+v", alerts)
	}
	if a, _ := tt.AccessByRole(ui.RoleButton, applyRangeLabel); !a.Disabled {
		t.Fatal("Apply range with bounds out of order")
	}
	if a, _ := tt.AccessByRole(ui.RolePopUpButton, fromDateLabel); a.Value != "11/20/2026" {
		t.Fatalf("from field %+v", a)
	}

	// Clearing To leaves an open end, which applies.
	press(t, tt, ui.RolePopUpButton, toDateLabel)
	press(t, tt, ui.RoleButton, "Clear")
	if tt.HasText(rangeOrderError) {
		t.Fatal("order error without a To date")
	}
	requests := len(f.backend.requests)
	press(t, tt, ui.RoleButton, applyRangeLabel)
	f.sync(tt)
	if len(f.backend.requests) != requests+1 {
		t.Fatal("Apply range read no usage")
	}
	f.expectRequest(t, "month", "2026-11-20", "")
	if dateRangeOpen(tt) {
		t.Fatal("Apply range kept the popover open")
	}
	trigger := dateRangePrefix + "2026-11-20 → Now"
	if _, ok := tt.AccessByRole(ui.RoleButton, trigger); !ok {
		t.Fatalf("no %q in %q", trigger, tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleNavigation, periodNavigationLabel); ok {
		t.Fatal("navigation for custom dates")
	}

	// Reopened, Custom edits the applied bounds; Today picks the server's day.
	openDateRange(t, tt, trigger)
	if a, _ := tt.AccessByRole(ui.RoleRadio, customLabel); !a.Checked {
		t.Fatal("custom range reopened on a preset")
	}
	press(t, tt, ui.RolePopUpButton, fromDateLabel)
	press(t, tt, ui.RoleButton, "Today")
	press(t, tt, ui.RoleButton, applyRangeLabel)
	f.sync(tt)
	f.expectRequest(t, "month", "2026-11-13", "")
}

func TestPeriodNavigationStepsAndStopsAtTheCurrentPeriod(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	next, ok := tt.AccessByRole(ui.RoleButton, nextPrefix+"month")
	if !ok || !next.Disabled {
		t.Fatalf("next at the current month: %+v", next)
	}
	if _, ok := tt.AccessByRole(ui.RoleStatus, currentMonth); !ok {
		t.Fatal("period label is not a status")
	}
	press(t, tt, ui.RoleButton, previousPrefix+"month")
	f.sync(tt)
	f.expectRequest(t, "month", "2026-10-01", "2026-10-31")
	assertTexts(t, tt, "October 2026", "Monthly")
	if next, _ := tt.AccessByRole(ui.RoleButton, nextPrefix+"month"); next.Disabled {
		t.Fatal("next disabled before the current month")
	}
	press(t, tt, ui.RoleButton, nextPrefix+"month")
	f.sync(tt)
	f.expectRequest(t, "month", "", "")
	assertTexts(t, tt, currentMonth)

	f.window.UpdateSelection(dashboardview.PresetChange(dashboardview.PeriodWeek))
	f.sync(tt)
	assertTexts(t, tt, "Nov 9 – 15, 2026")
	if _, ok := tt.AccessByRole(ui.RoleButton, previousPrefix+"week"); !ok {
		t.Fatalf("no previous week in %q", tt.Texts())
	}
}

// TestDateImages renders the filter row closed on Monthly with its
// navigation, the date popover on a preset, the custom bounds with the
// order error and a field's calendar, in both appearances, and writes
// PNGs when TOKENINSIGHTS_NATIVE_GALLERY_DIR is set.
func TestDateImages(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	shots := []struct {
		name    string
		prepare func(t *testing.T, tt *ui.Tester)
		texts   []string
	}{
		{"closed", func(*testing.T, *ui.Tester) {}, []string{"Monthly", currentMonth}},
		{"popover", func(t *testing.T, tt *ui.Tester) { openDateRange(t, tt, monthlyTrigger) }, []string{dateRangeTitle, "Weekly"}},
		{"custom-error", func(t *testing.T, tt *ui.Tester) {
			openDateRange(t, tt, monthlyTrigger)
			press(t, tt, ui.RoleRadio, customLabel)
			chooseDay(t, tt, fromDateLabel, "November 20, 2026")
			chooseDay(t, tt, toDateLabel, "November 10, 2026")
		}, []string{rangeOrderError, applyRangeLabel}},
		{"calendar", func(t *testing.T, tt *ui.Tester) {
			openDateRange(t, tt, monthlyTrigger)
			press(t, tt, ui.RoleRadio, customLabel)
			press(t, tt, ui.RolePopUpButton, fromDateLabel)
		}, []string{"Today", "Clear"}},
	}
	for _, shot := range shots {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(shot.name+"/"+appearance, func(t *testing.T) {
				tt := loaded().synced(windowWidth, windowHeight, dark)
				shot.prepare(t, tt)
				tt.Advance(settle)
				assertTexts(t, tt, shot.texts...)
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, "dates-"+shot.name+"-"+appearance+".png"), tt)
			})
		}
	}
}
