package model

import "github.com/flexdinesh/tokeninsights/packages/core/dashboardview"

// UpdateSelection applies change to the query, as the web's
// updateSelection, and reads the usage of the result from its first
// page. It does nothing before the instance loads.
func (w *Window) UpdateSelection(change dashboardview.SelectionChange) {
	if w.instance == nil {
		return
	}
	w.query = w.query.WithSelection(change)
	w.syncReads()
}

// PeriodNavigation is the query's calendar navigation on the server
// calendar now; ok is false before the instance loads and where the web
// hides it: All time and custom dates.
func (w *Window) PeriodNavigation() (dashboardview.PeriodNavigation, bool) {
	if w.instance == nil {
		return dashboardview.PeriodNavigation{}, false
	}
	return dashboardview.NavigatePeriod(w.query, w.instance.Timezone, w.rt.Now())
}

// ShiftPeriod moves the query's calendar period one step in direction
// (dashboardview.PreviousPeriod or NextPeriod), never past the current
// period, and reads its usage.
func (w *Window) ShiftPeriod(direction int) {
	n, ok := w.PeriodNavigation()
	if !ok || (direction > 0 && n.AtCurrent()) {
		return
	}
	change, err := n.Shift(direction)
	if err != nil {
		return
	}
	w.UpdateSelection(change)
}

// ServerToday is the server calendar's date now (YYYY-MM-DD), or "" before
// the instance loads: the day custom date fields open on.
func (w *Window) ServerToday() string {
	if w.instance == nil {
		return ""
	}
	return dashboardview.CalendarRange(dashboardview.PeriodToday, w.instance.Timezone, w.rt.Now()).From
}
