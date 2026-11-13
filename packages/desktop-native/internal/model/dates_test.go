package model

import (
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// expectSelection waits for the usage read of a selection change and
// checks its period and bounds.
func (h *harness) expectSelection(period, from, to string) analytics.Query {
	h.t.Helper()
	request := h.expectUsage()
	if s := request.Selection; s.Period != period || s.From != from || s.To != to || request.Page != 1 {
		h.t.Fatalf("usage request %+v, want period %q from %q to %q", request, period, from, to)
	}
	h.answerUsage(usageAt(1, 1), nil)
	return request
}

func TestSelectionBeforeTheInstanceDoesNothing(t *testing.T) {
	h := newHarness(t)
	w := h.window
	w.UpdateSelection(dashboardview.PresetChange(dashboardview.PeriodYear))
	w.ShiftPeriod(dashboardview.PreviousPeriod)
	if _, ok := w.PeriodNavigation(); ok || w.ServerToday() != "" {
		t.Fatal("dates before the instance loaded")
	}
	h.noCall()
}

func TestPresetChangeReadsUsageOfThePeriod(t *testing.T) {
	h := synced(t)
	w := h.window
	// testInstance's defaults are custom bounds: no navigation.
	if _, ok := w.PeriodNavigation(); ok {
		t.Fatal("navigation over custom bounds")
	}
	w.UpdateSelection(dashboardview.PresetChange(dashboardview.PeriodWeek))
	h.expectSelection("week", "", "")
	query, _ := w.Query()
	if query.DateMode != "" || query.CustomDates() || dashboardview.DateFilterLabel(query) != "Weekly" {
		t.Fatalf("query %+v", query)
	}
	// The same preset again reads nothing.
	w.UpdateSelection(dashboardview.PresetChange(dashboardview.PeriodWeek))
	h.noCall()
	if usage, ok := w.Usage(); !ok || w.UsageFetching() || usage.Request.Selection.Period != "week" {
		t.Fatalf("usage %+v, %v", usage, ok)
	}
}

func TestCustomRangeApplyReadsItsBounds(t *testing.T) {
	h := synced(t)
	w := h.window
	w.UpdateSelection(dashboardview.PresetChange(dashboardview.PeriodYear))
	h.expectSelection("year", "", "")
	w.UpdateSelection(dashboardview.CustomRangeChange("2026-03-01", ""))
	h.expectSelection("year", "2026-03-01", "")
	query, _ := w.Query()
	if !query.CustomDates() || dashboardview.DateFilterLabel(query) != "2026-03-01 → Now" {
		t.Fatalf("custom query %+v", query)
	}
	if _, ok := w.PeriodNavigation(); ok {
		t.Fatal("navigation over a custom range")
	}
}

func TestShiftPeriodStepsOnTheServerCalendar(t *testing.T) {
	h := synced(t)
	w := h.window
	if w.ServerToday() != "2026-11-13" {
		t.Fatalf("server today %q", w.ServerToday())
	}
	w.UpdateSelection(dashboardview.PresetChange(dashboardview.PeriodMonth))
	h.expectSelection("month", "", "")
	n, ok := w.PeriodNavigation()
	if !ok || n.Label != "November 2026" || !n.AtCurrent() || n.Unit != "month" {
		t.Fatalf("navigation %+v, %v", n, ok)
	}
	w.ShiftPeriod(dashboardview.NextPeriod) // at the current month
	h.noCall()

	w.ShiftPeriod(dashboardview.PreviousPeriod)
	h.expectSelection("month", "2026-10-01", "2026-10-31")
	query, _ := w.Query()
	if query.DateMode != dashboardview.DateModeCalendar || query.CustomDates() || dashboardview.DateFilterLabel(query) != "Monthly" {
		t.Fatalf("navigated query %+v", query)
	}
	if n, _ := w.PeriodNavigation(); n.Label != "October 2026" || n.AtCurrent() {
		t.Fatalf("navigation %+v", n)
	}
	w.ShiftPeriod(dashboardview.NextPeriod)
	h.expectSelection("month", "", "")
	if query, _ := w.Query(); query.DateMode != "" {
		t.Fatalf("current month kept the calendar mode: %+v", query)
	}
}
