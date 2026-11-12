package views

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
)

// filterOpen reports whether the popover of the filter labelled label
// shows.
func filterOpen(tt *ui.Tester, label string) bool {
	_, ok := tt.AccessByRole(ui.RoleList, label+filterSuffix)
	return ok
}

// openFilter opens the filter whose trigger is named trigger.
func openFilter(t *testing.T, tt *ui.Tester, trigger, label string) {
	t.Helper()
	press(t, tt, ui.RoleButton, trigger)
	if !filterOpen(tt, label) {
		t.Fatalf("%s filter did not open: %q", label, tt.Texts())
	}
}

// typeSearch types text into the focused search field.
func typeSearch(tt *ui.Tester, text string) {
	for _, r := range text {
		tt.Type(string(r))
	}
}

// optionChecked reports whether the option named name is checked.
func optionChecked(t *testing.T, tt *ui.Tester, name string) bool {
	t.Helper()
	a, ok := tt.AccessByRole(ui.RoleCheckBox, name)
	if !ok {
		t.Fatalf("no option %q in %q", name, tt.Texts())
	}
	return a.Checked
}

func TestFilterTriggersFollowTheDateFilter(t *testing.T) {
	tt := loaded().synced(windowWidth, windowHeight, false)
	date, _ := tt.Find("Monthly")
	previous := date
	for _, label := range []string{"Harness:", "Provider:", "Model:", "Session:"} {
		r, ok := tt.Find(label)
		if !ok || r.X <= previous.X || r.Y >= date.Y+date.H || r.Y+r.H <= date.Y {
			t.Fatalf("%s trigger at %v after %v, date %v", label, r, previous, date)
		}
		previous = r
	}
	for _, name := range []string{"Harness", "Provider", "Model", "Session"} {
		a, ok := tt.AccessByRole(ui.RoleButton, name)
		if !ok || a.Expanded {
			t.Fatalf("%s trigger %+v", name, a)
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleGroup, activeFiltersLabel); ok {
		t.Fatal("active filters without filters")
	}
}

func TestMultiSelectSearchesAndAppliesEachCheck(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	openFilter(t, tt, "Harness", "Harness")
	if !tt.Focused("Search harness") {
		t.Fatal("the search did not take the focus")
	}
	assertTexts(t, tt, "claude-code", "codex", "opencode", "pi", clearLabel, doneLabel)
	if tt.HasText(firstMatches) {
		t.Fatal("a local search notes server matches")
	}
	typeSearch(tt, "o")
	tt.Advance(settle)
	if tt.HasText("pi") || !tt.HasText("codex") || !tt.HasText("opencode") || !tt.HasText("claude-code") {
		t.Fatalf("search o shows %q", tt.Texts())
	}
	typeSearch(tt, "PE") // matching ignores case
	tt.Advance(settle)
	if tt.HasText("codex") || tt.HasText("claude-code") || !tt.HasText("opencode") {
		t.Fatalf("search oPE shows %q", tt.Texts())
	}
	typeSearch(tt, "z")
	tt.Advance(settle)
	assertTexts(t, tt, noValues)

	for range len("PEz") {
		tt.Key(0, ui.KeyBackspace)
	}
	tt.Advance(settle)
	press(t, tt, ui.RoleCheckBox, "codex")
	f.sync(tt)
	if !filterOpen(tt, "Harness") {
		t.Fatal("a check closed the popover")
	}
	if got := f.lastRequest().Selection.Harnesses; !slices.Equal(got, []string{"codex"}) {
		t.Fatalf("usage harnesses %v", got)
	}
	if got := f.backend.facetRequests[len(f.backend.facetRequests)-1].Query.Selection.Harnesses; !slices.Equal(got, []string{"codex"}) {
		t.Fatalf("facets harnesses %v", got)
	}
	if !optionChecked(t, tt, "codex") {
		t.Fatal("codex not checked")
	}
	if a, ok := tt.AccessByRole(ui.RoleButton, "Harness 1"); !ok || !a.Expanded {
		t.Fatalf("trigger with a choice %+v", a)
	}
	assertTexts(t, tt, "Harness:", "1")

	press(t, tt, ui.RoleButton, clearLabel)
	f.sync(tt)
	if len(f.lastRequest().Selection.Harnesses) != 0 || optionChecked(t, tt, "codex") || !filterOpen(tt, "Harness") {
		t.Fatalf("Clear: %v", f.lastRequest().Selection.Harnesses)
	}
	press(t, tt, ui.RoleButton, doneLabel)
	if filterOpen(tt, "Harness") {
		t.Fatal("Done kept the popover open")
	}

	// The search stays as the popover opens again, as the web's.
	openFilter(t, tt, "Harness", "Harness")
	if tt.HasText("pi") {
		t.Fatalf("the search was lost: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(settle)
	if filterOpen(tt, "Harness") {
		t.Fatal("Escape kept the popover open")
	}
}

func TestMultiSelectTogglesFromTheKeyboard(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	openFilter(t, tt, "Provider", "Provider")
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("anthropic") {
		t.Fatal("Tab did not reach the first option")
	}
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeySpace)
	f.sync(tt)
	if got := f.lastRequest().Selection.Providers; !slices.Equal(got, []string{"openai"}) {
		t.Fatalf("providers %v", got)
	}
}

func TestSelectedValuesNameTheTriggerAndShowAsChips(t *testing.T) {
	f := loaded()
	f.window.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionModels, []string{"gpt-5", "retired-model"}))
	f.window.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionHarnesses, []string{"pi"}))
	f.window.UpdateSelection(dashboardview.CustomRangeChange("2026-09-12", ""))
	tt := f.synced(windowWidth, windowHeight, false)
	assertTexts(t, tt, "gpt-5, retired-model", "2", clearAllLabel, customDatesLabel)
	if _, ok := tt.AccessByRole(ui.RoleButton, "Model 2"); !ok {
		t.Fatalf("model trigger in %q", tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleGroup, activeFiltersLabel); !ok {
		t.Fatal("no active filters")
	}
	// A selected value the facets no longer offer stays checkable.
	openFilter(t, tt, "Model 2", "Model")
	if !optionChecked(t, tt, "retired-model") || !optionChecked(t, tt, "gpt-5") || optionChecked(t, tt, "gpt-5-mini") {
		t.Fatal("selected options not checked")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(settle)

	press(t, tt, ui.RoleButton, "Remove model gpt-5")
	f.sync(tt)
	if got := f.lastRequest().Selection.Models; !slices.Equal(got, []string{"retired-model"}) {
		t.Fatalf("models after removing a chip %v", got)
	}
	press(t, tt, ui.RoleButton, customDatesLabel)
	f.sync(tt)
	if s := f.lastRequest().Selection; s.From != "" || s.To != "" || tt.HasText(customDatesLabel) {
		t.Fatalf("custom dates after removing their chip %+v", s)
	}
	press(t, tt, ui.RoleButton, clearAllLabel)
	f.sync(tt)
	if s := f.lastRequest().Selection; len(s.Models) != 0 || len(s.Harnesses) != 0 {
		t.Fatalf("Clear All left %+v", s)
	}
	if _, ok := tt.AccessByRole(ui.RoleGroup, activeFiltersLabel); ok {
		t.Fatal("active filters after Clear All")
	}
}

func TestSessionFilterSearchesTheBackendOnceTypingRests(t *testing.T) {
	f := loaded()
	tt := f.synced(windowWidth, windowHeight, false)
	openFilter(t, tt, "Session", "Session")
	assertTexts(t, tt, "mock-claude-session", "mock-pi-session", firstMatches)
	reads := len(f.backend.facetRequests)
	typeSearch(tt, "pi")
	f.sync(tt)
	assertTexts(t, tt, loadingValues)
	if len(f.backend.facetRequests) != reads {
		t.Fatal("the search read before typing rested")
	}
	tt.Advance(model.SessionSearchDelay)
	f.sync(tt)
	if got := f.backend.facetRequests[len(f.backend.facetRequests)-1]; got.Search != "pi" {
		t.Fatalf("search request %+v", got)
	}
	assertTexts(t, tt, "mock-pi-session", firstMatches)
	if tt.HasText("mock-claude-session") {
		t.Fatalf("matches %q", tt.Texts())
	}
	press(t, tt, ui.RoleCheckBox, "mock-pi-session")
	f.sync(tt)
	if got := f.lastRequest().Selection.Sessions; !slices.Equal(got, []string{"mock-pi-session"}) {
		t.Fatalf("sessions %v", got)
	}
}

func TestSessionSearchFailureRetries(t *testing.T) {
	f := loaded()
	f.backend.searchErr = errors.New("search unavailable")
	tt := f.synced(windowWidth, windowHeight, false)
	openFilter(t, tt, "Session", "Session")
	typeSearch(tt, "pi")
	tt.Advance(model.SessionSearchDelay)
	f.sync(tt)
	if _, ok := tt.AccessByRole(ui.RoleAlert, ""); !ok {
		t.Fatal("no search failure")
	}
	assertTexts(t, tt, sessionValuesFailed, "Retry session search")
	f.backend.searchErr = nil
	press(t, tt, ui.RoleButton, "Retry session search")
	f.sync(tt)
	assertTexts(t, tt, "mock-pi-session")
	if tt.HasText(sessionValuesFailed) {
		t.Fatal("retry kept the failure")
	}
}

func TestFacetsFailureShowsABannerThatRetries(t *testing.T) {
	f := loaded()
	f.backend.facetsErr = errors.New("facets unavailable")
	tt := f.synced(windowWidth, windowHeight, false)
	assertTexts(t, tt, facetsFailed, "Filters couldn’t load")
	f.backend.facetsErr = nil
	press(t, tt, ui.RoleButton, retryRequest)
	f.sync(tt)
	if tt.HasText(facetsFailed) {
		t.Fatal("Retry Request kept the failure")
	}
	assertTexts(t, tt, "Ready")
}

// TestFilterImages renders the filter toolbar with active filters, the
// harness popover and the session popover searching, in both appearances,
// and writes PNGs when TOKENINSIGHTS_NATIVE_GALLERY_DIR is set.
func TestFilterImages(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, shot := range []struct {
		name  string
		open  func(t *testing.T, f *fixture, tt *ui.Tester)
		texts []string
	}{
		{"filters-active", func(*testing.T, *fixture, *ui.Tester) {}, []string{clearAllLabel, customDatesLabel}},
		{"filters-harness", func(t *testing.T, _ *fixture, tt *ui.Tester) { openFilter(t, tt, "Harness 1", "Harness") }, []string{doneLabel, "opencode"}},
		{"filters-session", func(t *testing.T, f *fixture, tt *ui.Tester) {
			openFilter(t, tt, "Session", "Session")
			typeSearch(tt, "co")
			tt.Advance(model.SessionSearchDelay)
			f.sync(tt)
		}, []string{firstMatches, "mock-codex-session"}},
	} {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(shot.name+"/"+appearance, func(t *testing.T) {
				f := loaded()
				f.window.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionHarnesses, []string{"codex"}))
				f.window.UpdateSelection(dashboardview.DimensionChange(dashboardview.DimensionModels, []string{"gpt-5", "gpt-5-mini"}))
				f.window.UpdateSelection(dashboardview.CustomRangeChange("2026-09-12", "2026-09-14"))
				tt := f.synced(windowWidth, windowHeight, dark)
				shot.open(t, f, tt)
				tt.Advance(settle)
				assertTexts(t, tt, shot.texts...)
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, shot.name+"-"+appearance+".png"), tt)
			})
		}
	}
}
