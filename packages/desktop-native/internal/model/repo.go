package model

import (
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// The Repo tab groups usage by location and filters it by repository and
// directory, as the web's: the grouping and the location filters belong to
// its usage, excluded usage and facets requests only
// (dashboardview.Query.ToAnalytics), and leaving the tab drops the
// location filters (dashboardview.Query.WithTab).

// SetLocationGroup groups Repo rows by group, as the web's
// setLocationGroup, and reads the usage of the grouping from its first
// page. It does nothing before the instance loads.
func (w *Window) SetLocationGroup(group analytics.RepoGroup) {
	if w.instance == nil || w.query.LocationGroup == group {
		return
	}
	w.query = w.query.WithLocationGroup(group)
	w.syncReads()
}

// UpdateLocations applies change to the Repo location filters, as the
// web's updateLocations, and reads the usage of the result from its first
// page. It does nothing before the instance loads.
func (w *Window) UpdateLocations(change dashboardview.LocationChange) {
	if w.instance == nil {
		return
	}
	w.query = w.query.WithLocations(change)
	w.syncReads()
}
