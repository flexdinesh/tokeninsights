package dashboardview_test

import (
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// sydneyNow is 13 Nov 2026 22:30 UTC: already 14 Nov (a Saturday) in
// Sydney, so the server calendar decides the day.
var sydneyNow = time.Date(2026, time.November, 13, 22, 30, 0, 0, time.UTC)

const sydney = "Australia/Sydney"

func dateQuery(period, from, to string, mode dashboardview.DateMode) dashboardview.Query {
	q := dashboardview.InitialQuery(analytics.Selection{Period: period, From: from, To: to, Bucket: "day"}, dashboardview.TabTokens)
	q.DateMode = mode
	return q
}

func TestDateFilterLabelAndPreset(t *testing.T) {
	cases := []struct {
		query         dashboardview.Query
		label, preset string
		custom        bool
	}{
		{dateQuery("month", "", "", ""), "Monthly", dashboardview.PeriodMonth, false},
		{dateQuery("all", "", "", ""), "All time", dashboardview.PeriodAll, false},
		{dateQuery("yesterday", "", "", ""), "Daily", dashboardview.PeriodToday, false},
		{dateQuery("week", "2026-11-02", "2026-11-08", dashboardview.DateModeCalendar), "Weekly", dashboardview.PeriodWeek, false},
		{dateQuery("month", "2026-01-05", "2026-02-01", ""), "2026-01-05 → 2026-02-01", dashboardview.PeriodCustom, true},
		{dateQuery("month", "2026-01-05", "", ""), "2026-01-05 → Now", dashboardview.PeriodCustom, true},
		{dateQuery("month", "", "2026-02-01", ""), "Beginning → 2026-02-01", dashboardview.PeriodCustom, true},
	}
	for _, c := range cases {
		if got := dashboardview.DateFilterLabel(c.query); got != c.label {
			t.Errorf("DateFilterLabel(%+v) = %q, want %q", c.query.Selection, got, c.label)
		}
		if got := dashboardview.DatePreset(c.query); got != c.preset {
			t.Errorf("DatePreset(%+v) = %q, want %q", c.query.Selection, got, c.preset)
		}
		if got := c.query.CustomDates(); got != c.custom {
			t.Errorf("CustomDates(%+v) = %v", c.query.Selection, got)
		}
	}
}

func TestPresetAndCustomChanges(t *testing.T) {
	q := dateQuery("week", "2026-11-02", "2026-11-08", dashboardview.DateModeCalendar).WithPage(3)
	preset := q.WithSelection(dashboardview.PresetChange(dashboardview.PeriodYear))
	if preset.Period != "year" || preset.From != "" || preset.To != "" || preset.DateMode != "" || preset.Page != 1 {
		t.Fatalf("preset %+v", preset)
	}
	custom := q.WithSelection(dashboardview.CustomRangeChange("2026-01-01", ""))
	if custom.Period != "week" || custom.From != "2026-01-01" || custom.To != "" || custom.DateMode != "" || !custom.CustomDates() {
		t.Fatalf("custom %+v", custom)
	}
	for _, c := range []struct {
		from, to            string
		invalid, applicable bool
	}{
		{"", "", false, false},
		{"2026-01-01", "", false, true},
		{"", "2026-01-01", false, true},
		{"2026-01-01", "2026-01-01", false, true},
		{"2026-01-02", "2026-01-01", true, false},
	} {
		if got := dashboardview.CustomRangeOrderInvalid(c.from, c.to); got != c.invalid {
			t.Errorf("CustomRangeOrderInvalid(%q, %q) = %v", c.from, c.to, got)
		}
		if got := dashboardview.CustomRangeApplicable(c.from, c.to); got != c.applicable {
			t.Errorf("CustomRangeApplicable(%q, %q) = %v", c.from, c.to, got)
		}
	}
}

func TestNavigatePeriodHidesForCustomAndAllTime(t *testing.T) {
	for _, q := range []dashboardview.Query{
		dateQuery("all", "", "", ""),
		dateQuery("month", "2026-01-05", "", ""),
		dateQuery("month", "2026-01-01", "2026-01-31", ""),
	} {
		if _, ok := dashboardview.NavigatePeriod(q, sydney, sydneyNow); ok {
			t.Errorf("navigation shows for %+v", q.Selection)
		}
	}
}

func TestNavigatePeriodOnTheServerCalendar(t *testing.T) {
	cases := []struct {
		query     dashboardview.Query
		unit      string
		shown     dashboardview.DateRange
		label     string
		atCurrent bool
	}{
		{dateQuery("today", "", "", ""), "day", dashboardview.DateRange{From: "2026-11-14", To: "2026-11-14"}, "Nov 14, 2026", true},
		{dateQuery("yesterday", "", "", ""), "day", dashboardview.DateRange{From: "2026-11-13", To: "2026-11-13"}, "Nov 13, 2026", false},
		{dateQuery("week", "", "", ""), "week", dashboardview.DateRange{From: "2026-11-09", To: "2026-11-15"}, "Nov 9 – 15, 2026", true},
		{dateQuery("month", "", "", ""), "month", dashboardview.DateRange{From: "2026-11-01", To: "2026-11-30"}, "November 2026", true},
		{dateQuery("month", "2026-09-01", "2026-09-30", dashboardview.DateModeCalendar), "month", dashboardview.DateRange{From: "2026-09-01", To: "2026-09-30"}, "September 2026", false},
		{dateQuery("year", "", "", ""), "year", dashboardview.DateRange{From: "2026-01-01", To: "2026-12-31"}, "2026", true},
	}
	for _, c := range cases {
		n, ok := dashboardview.NavigatePeriod(c.query, sydney, sydneyNow)
		if !ok || n.Unit != c.unit || n.Range != c.shown || n.Label != c.label || n.AtCurrent() != c.atCurrent {
			t.Errorf("NavigatePeriod(%+v) = %+v (at current %v), %v", c.query.Selection, n, n.AtCurrent(), ok)
		}
	}
}

func TestPeriodShiftReturnsToDefaultBoundsAtTheCurrentPeriod(t *testing.T) {
	q := dateQuery("month", "", "", "")
	n, _ := dashboardview.NavigatePeriod(q, sydney, sydneyNow)
	change, err := n.Shift(dashboardview.PreviousPeriod)
	if err != nil {
		t.Fatal(err)
	}
	previous := q.WithSelection(change)
	if previous.Period != "month" || previous.From != "2026-10-01" || previous.To != "2026-10-31" || previous.DateMode != dashboardview.DateModeCalendar {
		t.Fatalf("previous %+v", previous)
	}
	n, _ = dashboardview.NavigatePeriod(previous, sydney, sydneyNow)
	if n.Label != "October 2026" || n.AtCurrent() {
		t.Fatalf("previous navigation %+v", n)
	}
	change, _ = n.Shift(dashboardview.NextPeriod)
	if back := previous.WithSelection(change); back.From != "" || back.To != "" || back.DateMode != "" || back.Period != "month" {
		t.Fatalf("back at the current period %+v", back)
	}

	// Yesterday navigates as Daily: forward reaches today.
	y := dateQuery("yesterday", "", "", "")
	n, _ = dashboardview.NavigatePeriod(y, sydney, sydneyNow)
	change, _ = n.Shift(dashboardview.NextPeriod)
	if today := y.WithSelection(change); today.Period != "today" || today.From != "" || today.DateMode != "" {
		t.Fatalf("yesterday forward %+v", today)
	}
	change, _ = n.Shift(dashboardview.PreviousPeriod)
	if before := y.WithSelection(change); before.Period != "today" || before.From != "2026-11-12" || before.To != "2026-11-12" || before.DateMode != dashboardview.DateModeCalendar {
		t.Fatalf("yesterday back %+v", before)
	}
}
