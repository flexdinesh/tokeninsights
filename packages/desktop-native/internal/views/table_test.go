package views

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/desktop-native/internal/model"
)

// The web mocks' days of usage: 12 to 14 Sep 2026, 10:00 UTC.
var (
	dayOne   = time.Date(2026, time.September, 12, 10, 0, 0, 0, time.UTC).UnixMilli()
	dayTwo   = time.Date(2026, time.September, 13, 10, 0, 0, 0, time.UTC).UnixMilli()
	dayThree = time.Date(2026, time.September, 14, 10, 0, 0, 0, time.UTC).UnixMilli()
)

// usageRow is a row of total tokens split as the web mocks' summary
// splits its total: input, output, cache read, cache write, reasoning.
func usageRow(key, name string, sessions, total int64) analytics.Row {
	return analytics.Row{
		Key: key, Name: name, Sessions: sessions, Total: total,
		Input: total * 369 / 647, Output: total * 78 / 647, CacheRead: total * 178 / 647,
		CacheWrite: total * 9 / 647, Reasoning: total * 13 / 647,
	}
}

// sessionRow is a session row with its harness, provider, model and when
// it was last active.
func sessionRow(name, harness, provider, model string, date, total int64) analytics.Row {
	row := usageRow(harness+":"+name, name, 1, total)
	row.Harness, row.Provider, row.Model, row.Date, row.Context = harness, provider, model, date, total*3/4
	return row
}

// mockRows are the web mocks' rows of each tab but Repo.
func mockRows() map[dashboardview.Tab][]analytics.Row {
	tokens := []analytics.Row{
		{Key: "2026-09-12", Name: "2026-09-12", Date: dayOne, Sessions: 3, Input: 126_000, Output: 25_000, Reasoning: 4_000, CacheRead: 62_000, CacheWrite: 3_000, Total: 220_000},
		{Key: "2026-09-13", Name: "2026-09-13", Date: dayTwo, Sessions: 4, Input: 145_000, Output: 31_000, Reasoning: 6_000, CacheRead: 71_000, CacheWrite: 4_000, Total: 257_000},
		{Key: "2026-09-14", Name: "2026-09-14", Date: dayThree, Sessions: 3, Input: 98_000, Output: 22_000, Reasoning: 3_000, CacheRead: 45_000, CacheWrite: 2_000, Total: 170_000},
	}
	context := func(harness, provider, model string, average, median, maximum int64) analytics.Row {
		return analytics.Row{
			Key: harness + "\x00" + provider + "\x00" + model, Name: model, Harness: harness, Provider: provider, Model: model,
			Sessions: 1, AverageContext: average, MedianContext: median, MaxContext: maximum,
		}
	}
	return map[dashboardview.Tab][]analytics.Row{
		dashboardview.TabTokens: tokens,
		dashboardview.TabModels: {
			usageRow("gpt-5", "gpt-5", 2, 311_000), usageRow("claude-sonnet-4-5", "claude-sonnet-4-5", 1, 205_000),
			usageRow("gpt-5-mini", "gpt-5-mini", 1, 131_000),
		},
		dashboardview.TabProviders: {usageRow("openai", "openai", 3, 442_000), usageRow("anthropic", "anthropic", 1, 205_000)},
		dashboardview.TabHarnesses: {
			usageRow("codex", "codex", 1, 244_000), usageRow("claude-code", "claude-code", 1, 181_000),
			usageRow("opencode", "opencode", 1, 135_000), usageRow("pi", "pi", 1, 87_000),
		},
		dashboardview.TabSessions: {
			sessionRow("mock-codex-session", "codex", "openai", "gpt-5", dayThree, 244_000),
			sessionRow("mock-claude-session", "claude-code", "anthropic", "claude-sonnet-4-5", dayThree, 181_000),
			sessionRow("mock-opencode-session", "opencode", "openai", "gpt-5-mini", dayTwo, 135_000),
			sessionRow("mock-pi-session", "pi", "openai", "gpt-5", dayOne, 87_000),
		},
		dashboardview.TabContext: {
			context("codex", "openai", "gpt-5", 118_000, 121_000, 184_000),
			context("claude-code", "anthropic", "claude-sonnet-4-5", 92_000, 92_000, 133_000),
			context("opencode", "openai", "gpt-5-mini", 61_000, 61_000, 89_000),
		},
	}
}

// manySessions are count sessions across the harnesses, most recent first.
func manySessions(count int) []analytics.Row {
	harnesses := [][3]string{{"codex", "openai", "gpt-5"}, {"claude-code", "anthropic", "claude-sonnet-4-5"}, {"opencode", "openai", "gpt-5-mini"}, {"pi", "openai", "gpt-5"}}
	rows := make([]analytics.Row, count)
	for i := range rows {
		h := harnesses[i%len(harnesses)]
		date := dayThree - int64(i)*time.Hour.Milliseconds()
		rows[i] = sessionRow(fmt.Sprintf("session-%04d", i), h[0], h[1], h[2], date, int64(40_000+(i*7919)%260_000))
	}
	return rows
}

// onTab is a synced tester showing tab.
func (f *fixture) onTab(t *testing.T, tab dashboardview.Tab, dark bool) *ui.Tester {
	t.Helper()
	tt := f.synced(windowWidth, windowHeight, dark)
	if tab != f.window.Tab() {
		if err := tt.Click(dashboardview.TabLabel(tab)); err != nil {
			t.Fatal(err)
		}
		f.sync(tt)
	}
	return tt
}

// lastRequest is the latest usage request.
func (f *fixture) lastRequest() analytics.Query { return f.backend.requests[len(f.backend.requests)-1] }

func heading(t *testing.T, tt *ui.Tester, label string) ui.AccessInfo {
	t.Helper()
	a, ok := tt.AccessByRole(ui.RoleColumnHeader, label)
	if !ok {
		t.Fatalf("no %q heading in %q", label, tt.Texts())
	}
	return a
}

func TestTableShowsEachTabsColumnsAndRows(t *testing.T) {
	for tab, texts := range map[dashboardview.Tab][]string{
		dashboardview.TabTokens:   {"Tokens breakdown", "2026-09-12", "220K", "126K"},
		dashboardview.TabModels:   {"Models breakdown", "gpt-5", "311K"},
		dashboardview.TabSessions: {"Sessions breakdown", "mock-codex-session", "codex", "openai", "gpt-5", "Sep 14"},
		dashboardview.TabContext:  {"Context breakdown", "claude-code", "anthropic", "118K", "184K"},
	} {
		t.Run(string(tab), func(t *testing.T) {
			tt := loaded().onTab(t, tab, false)
			for _, column := range dashboardview.Columns(tab) {
				head := heading(t, tt, column.Label)
				if want := dashboardview.HeaderSort(tab, column.ID) == dashboardview.DefaultSort(tab); (head.Sort == ui.SortDescending) != want {
					t.Errorf("%s heading sorted %v", column.Label, head.Sort)
				}
			}
			assertTexts(t, tt, texts...)
			if _, ok := tt.AccessByRole(ui.RoleRegion, dashboardview.TabLabel(tab)+detailsSuffix); !ok {
				t.Error("no details region")
			}
		})
	}
}

func TestNumericCellsAlignAtTheirEnd(t *testing.T) {
	f := loaded()
	// Without chart rows, the chart's axis shows no period names.
	f.backend.charts = map[dashboardview.Tab][]analytics.Row{dashboardview.TabTokens: {}}
	tt := f.onTab(t, dashboardview.TabTokens, false)
	total := heading(t, tt, "Total")
	value, _ := tt.Find("220K")
	period := heading(t, tt, "Period")
	name, _ := tt.Find("2026-09-12")
	if end := total.Bounds.X + total.Bounds.W; value.X+value.W > end || value.X+value.W < end-24 {
		t.Errorf("total %v not at the end of %v", value, total.Bounds)
	}
	if name.X < period.Bounds.X || name.X > period.Bounds.X+24 {
		t.Errorf("period %v not at the start of %v", name, period.Bounds)
	}
}

// clickHeading clicks the middle of the column heading label.
func clickHeading(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	b := heading(t, tt, label).Bounds
	tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
}

func TestCellTooltipsNameExactValues(t *testing.T) {
	tt := loaded().onTab(t, dashboardview.TabSessions, false)
	for text, want := range map[string]string{
		"244K":               "244,000",
		"Sep 14, 2026":       "Sep 14, 2026, 11:00 · Europe/London",
		"mock-codex-session": "mock-codex-session",
	} {
		if got := described(tt, text); got != want {
			t.Errorf("%q described %q, want %q", text, got, want)
		}
	}
	r, _ := tt.Find("244K")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(settle)
	assertTexts(t, tt, "244,000")
}

// described is the description of the first element named name that has
// one, as a tooltip gives its anchor.
func described(tt *ui.Tester, name string) string {
	for _, a := range tt.AccessAll() {
		if a.Name == name && a.Description != "" {
			return a.Description
		}
	}
	return ""
}

// footerTexts are the texts from the results table's footer to the end
// of the page, in order.
func footerTexts(t *testing.T, tt *ui.Tester) []string {
	t.Helper()
	texts := tt.Texts()
	// The footer's Sessions is the last: the sidebar's and the
	// heading's come before it.
	start := -1
	for i, text := range texts {
		if text == sessionsShown {
			start = i
		}
	}
	if start < 0 {
		t.Fatalf("no footer in %q", texts)
	}
	return texts[start:]
}

func TestFooterSummarizesTheResults(t *testing.T) {
	f := loaded()
	tt := f.onTab(t, dashboardview.TabTokens, false)
	want := []string{sessionsShown, "4", shownSeparator, "5", syncedSuffix, summaryDot, "3", rowsSuffix, summaryDot, totalLabel, "647K", FooterLine(f.window)}
	if got := footerTexts(t, tt); !slices.Equal(got, want) {
		t.Errorf("footer %q, want %q", got, want)
	}
	if got := described(tt, "647K"); got != "647,000" {
		t.Errorf("total described %q", got)
	}
	if !slices.ContainsFunc(tt.AccessAll(), func(a ui.AccessInfo) bool { return a.Description == sessionsShownHint }) {
		t.Error("the session counts are not described")
	}
	f = loaded()
	tt = f.onTab(t, dashboardview.TabContext, false)
	want = []string{sessionsShown, "4", shownSeparator, "5", syncedSuffix, summaryDot, "3", rowsSuffix, FooterLine(f.window)}
	if got := footerTexts(t, tt); !slices.Equal(got, want) {
		t.Errorf("context footer %q, want %q", got, want)
	}
}

func TestHeadingsAndControlsSortThroughTheBackend(t *testing.T) {
	f := loaded()
	tt := f.onTab(t, dashboardview.TabTokens, false)
	// The chart's metric choice shows Total too: click the heading.
	clickHeading(t, tt, "Total")
	f.sync(tt)
	if got := f.lastRequest(); got.Sort != string(dashboardview.SortTotal) || got.Direction != string(dashboardview.Descending) || got.Page != 1 {
		t.Fatalf("heading sort %+v", got)
	}
	if heading(t, tt, "Total").Sort != ui.SortDescending || heading(t, tt, "Period").Sort != ui.SortNone {
		t.Fatal("headings did not follow the sort")
	}
	if err := tt.Click(sortAscendingLabel); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if got := f.lastRequest(); got.Sort != string(dashboardview.SortTotal) || got.Direction != string(dashboardview.Ascending) {
		t.Fatalf("direction toggle %+v", got)
	}
	if heading(t, tt, "Total").Sort != ui.SortAscending || !tt.HasText(sortDescendingLabel) {
		t.Fatal("ascending headings or toggle")
	}
	if err := tt.Click(sortByLabel); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Name"); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if got := f.lastRequest(); got.Sort != string(dashboardview.SortName) || got.Direction != string(dashboardview.Ascending) {
		t.Fatalf("sort select %+v", got)
	}
	// Tokens periods sort by date from their heading.
	if err := tt.Click("Period"); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if got := f.lastRequest(); got.Sort != string(dashboardview.SortDate) {
		t.Fatalf("period heading %+v", got)
	}
}

func TestColumnsPopoverHidesColumnsKeepingOne(t *testing.T) {
	tt := loaded().onTab(t, dashboardview.TabTokens, false)
	if err := tt.Click(columnsLabel); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.AccessByRole(ui.RoleCheckBox, "Period"); ok {
		t.Fatal("Name can be hidden")
	}
	toggleColumn(t, tt, "Input")
	if _, ok := tt.AccessByRole(ui.RoleColumnHeader, "Input"); ok {
		t.Fatal("hidden column shows")
	}
	if box, _ := tt.AccessByRole(ui.RoleCheckBox, "Input"); box.Checked {
		t.Fatal("hidden column checked")
	}
	heading(t, tt, "Output")
	toggleColumn(t, tt, "Input")
	heading(t, tt, "Input")

	// Context has no Name: its last column shown stays.
	f := loaded()
	tt = f.onTab(t, dashboardview.TabContext, false)
	if err := tt.Click(columnsLabel); err != nil {
		t.Fatal(err)
	}
	columns := dashboardview.Columns(dashboardview.TabContext)
	for _, column := range columns[:len(columns)-1] {
		toggleColumn(t, tt, column.Label)
	}
	last := columns[len(columns)-1].Label
	if box, _ := tt.AccessByRole(ui.RoleCheckBox, last); !box.Checked || !box.Disabled {
		t.Fatalf("last column %+v", box)
	}
	toggleColumn(t, tt, last)
	heading(t, tt, last)
	if _, ok := tt.AccessByRole(ui.RoleColumnHeader, columns[0].Label); ok {
		t.Fatal("a hidden column shows")
	}
}

// toggleColumn clicks the Columns popover's check box of label.
func toggleColumn(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	box, ok := tt.AccessByRole(ui.RoleCheckBox, label)
	if !ok {
		t.Fatalf("no %q check box in %q", label, tt.Texts())
	}
	tt.ClickAt(box.Bounds.X+box.Bounds.W/2, box.Bounds.Y+box.Bounds.H/2)
}

func TestEmptyResultsOfferClearFilters(t *testing.T) {
	f := loaded()
	f.backend.rows = nil
	f.window.UpdateSelection(dashboardview.CustomRangeChange("2026-11-01", "2026-11-30"))
	tt := f.onTab(t, dashboardview.TabTokens, false)
	assertTexts(t, tt, noMatches, includedScope+widerRange+orClearFilters+".")
	heading(t, tt, "Period")
	if err := tt.Click(clearFiltersLabel); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if got := f.lastRequest(); got.Selection.From != "" || got.Selection.To != "" {
		t.Fatalf("clear filters %+v", got)
	}
	assertTexts(t, tt, includedScope+widerRange+".")
	if tt.HasText(clearFiltersLabel) {
		t.Fatal("Clear filters without filters")
	}
}

// pageEnd scrolls the page to its end, past the readouts, which bring
// the table's whole scroll region into view.
const pageEnd = 2000

// scrollTable scrolls the table's rows down by delta where they show,
// and builds the frames that follow, with the pages they read.
func scrollTable(t *testing.T, f *fixture, tt *ui.Tester, delta float32) {
	t.Helper()
	table, ok := tt.AccessByRole(ui.RoleTable, dashboardview.TableTitle(dashboardview.TabSessions, dashboardview.QualityConfirmed))
	if !ok {
		t.Fatal("no table")
	}
	top, bottom := max(table.Bounds.Y, 0), min(table.Bounds.Y+table.Bounds.H, windowHeight)
	tt.Scroll(table.Bounds.X+table.Bounds.W/2, (top+bottom)/2, 0, delta)
	tt.Advance(settle) // the gesture ends
	f.sync(tt)
	tt.Frame()
}

// pageReads counts the usage reads of later pages.
func (f *fixture) pageReads() []int {
	pages := []int{}
	for _, request := range f.backend.requests {
		if request.Page > 1 {
			pages = append(pages, request.Page)
		}
	}
	return pages
}

func TestScrollingNearTheEndReadsTheNextPage(t *testing.T) {
	f := loaded()
	f.backend.rows[dashboardview.TabSessions] = manySessions(450)
	tt := f.onTab(t, dashboardview.TabSessions, false)
	assertTexts(t, tt, "450", "session-0000")
	scrollPage(tt, pageEnd)
	if pages := f.pageReads(); len(pages) != 0 {
		t.Fatalf("pages read before scrolling: %v", pages)
	}
	for range 40 {
		scrollTable(t, f, tt, 2000)
		if len(f.pageReads()) == 2 {
			break
		}
	}
	if pages := f.pageReads(); len(pages) != 2 || pages[0] != 2 || pages[1] != 3 {
		t.Fatalf("pages read %v", pages)
	}
	usage, _ := f.window.Usage()
	if len(usage.Rows) != 450 || f.window.MoreRows() {
		t.Fatalf("%d rows read", len(usage.Rows))
	}
	for range 40 {
		if tt.HasText("session-0449") {
			break
		}
		scrollTable(t, f, tt, 2000)
	}
	assertTexts(t, tt, "session-0449")
	if pages := f.pageReads(); len(pages) != 2 {
		t.Fatalf("pages read past the end %v", pages)
	}
}

func TestPageFailureShowsRetryKeepingRows(t *testing.T) {
	f := loaded()
	f.backend.rows[dashboardview.TabSessions] = manySessions(450)
	f.backend.pageErr = errors.New("backend closed the connection")
	tt := f.onTab(t, dashboardview.TabSessions, false)
	scrollPage(tt, pageEnd)
	for range 40 {
		scrollTable(t, f, tt, 2000)
		if f.window.RowsError() != nil {
			break
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleAlert, ""); !ok || !tt.HasText(rowsFailedPrefix+"backend closed the connection") {
		t.Fatalf("no page failure in %q", tt.Texts())
	}
	if usage, _ := f.window.Usage(); len(usage.Rows) != model.TablePageSize {
		t.Fatalf("the failure dropped rows: %d", len(usage.Rows))
	}
	reads := len(f.pageReads())
	scrollTable(t, f, tt, 2000)
	if len(f.pageReads()) != reads {
		t.Fatal("a failed page read again without Retry")
	}
	f.backend.pageErr = nil
	if err := tt.Click(retryRequest); err != nil {
		t.Fatal(err)
	}
	f.sync(tt)
	if usage, _ := f.window.Usage(); len(usage.Rows) <= model.TablePageSize || strings.Contains(strings.Join(tt.Texts(), " "), rowsFailedPrefix) {
		t.Fatalf("retry: %d rows", len(usage.Rows))
	}
}

// TestTableImages renders the Tokens and Models tables, the Sessions
// table of many rows scrolled down, and the Columns popover, in both
// appearances, and writes PNGs when TOKENINSIGHTS_NATIVE_GALLERY_DIR is
// set.
func TestTableImages(t *testing.T) {
	dir := os.Getenv(galleryDirEnv)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, shot := range []struct {
		name    string
		tab     dashboardview.Tab
		prepare func(t *testing.T, f *fixture, tt *ui.Tester)
	}{
		{"tokens", dashboardview.TabTokens, nil},
		{"models", dashboardview.TabModels, nil},
		{"sessions-scrolled", dashboardview.TabSessions, func(t *testing.T, f *fixture, tt *ui.Tester) {
			scrollPage(tt, pageEnd)
			scrollTable(t, f, tt, 1200)
		}},
		{"columns-popover", dashboardview.TabTokens, func(t *testing.T, _ *fixture, tt *ui.Tester) {
			if err := tt.Click(columnsLabel); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("Cache W"); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		for _, dark := range []bool{false, true} {
			appearance := map[bool]string{false: "light", true: "dark"}[dark]
			t.Run(shot.name+"/"+appearance, func(t *testing.T) {
				f := loaded()
				f.backend.rows[dashboardview.TabSessions] = manySessions(450)
				tt := f.onTab(t, shot.tab, dark)
				if shot.prepare != nil {
					shot.prepare(t, f, tt)
				}
				tt.Advance(settle)
				if dir == "" {
					return
				}
				tt.SetScale(galleryScale)
				tt.Frame()
				writePNG(t, filepath.Join(dir, "table-"+shot.name+"-"+appearance+".png"), tt)
			})
		}
	}
}

// scrollPage scrolls the page down by delta with the pointer over the
// readouts, above the table.
func scrollPage(tt *ui.Tester, delta float32) {
	r, _ := tt.Find("Total tokens")
	tt.Scroll(r.X, r.Y, 0, delta)
	tt.Frame()
}
