package model

import (
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// The excluded usage read is the web's excludedUsage: beside confirmed
// usage, one row of the usage excluded from the totals under the same
// filters (dashboardview.Query.ExcludedUsageQuery), whose summary the
// results notice and empty states tell of. It reads as usage does, keyed on
// its request and the status snapshot, once the usage shown answered
// without failing, and not while excluded usage is reviewed.

// excludedWanted is the excluded usage read the window should show, and
// whether it may read: usage may read, the query shows confirmed usage and
// its usage answered and did not fail, as the web's analytics.isSuccess.
func (w *Window) excludedWanted() (usageKey, bool) {
	key, ok := w.usageWanted()
	if !ok || w.query.Quality != dashboardview.QualityConfirmed || w.usage.data == nil || w.usage.err != nil {
		return usageKey{}, false
	}
	key.request = w.query.ExcludedUsageQuery().ToAnalytics()
	return key, true
}

// syncExcluded reads excluded usage when its wanted read differs from the
// latest one, as syncUsage does.
func (w *Window) syncExcluded() {
	key, ok := w.excludedWanted()
	w.syncUsageRead(&w.excluded, key, ok)
}

// SetQuality shows usage of quality, confirmed usage or the review of
// excluded usage, as the web's setQuality, and reads it.
func (w *Window) SetQuality(quality dashboardview.Quality) {
	if w.instance == nil || w.query.Quality == quality {
		return
	}
	w.query = w.query.WithQuality(quality)
	w.syncReads()
}

// ExcludedUsage is the summary of the usage excluded from the totals
// shown, as the web's hasExcludedUsage: the excluded read answered
// estimated usage of the revision the usage shown answered, and some
// session has it; ok is false otherwise.
func (w *Window) ExcludedUsage() (analytics.ViewerSummaryRow, bool) {
	e, d := w.excluded.data, w.usage.data
	if e == nil || d == nil || e.Quality != string(dashboardview.QualityEstimated) || e.Revision != d.Revision ||
		e.Summary.SessionCount <= 0 {
		return analytics.ViewerSummaryRow{}, false
	}
	return e.Summary, true
}

// ExcludedPending reports whether excluded usage has neither answered nor
// failed yet, the web's excludedUsage.isPending: the empty-usage message
// waits for it, as it says whether excluded usage exists.
func (w *Window) ExcludedPending() bool { return w.excluded.data == nil && w.excluded.err == nil }

// ExcludedError is why the latest excluded usage read failed, or nil.
func (w *Window) ExcludedError() error { return w.excluded.err }

// RetryExcluded reads excluded usage again, the notice's Retry excluded
// usage.
func (w *Window) RetryExcluded() {
	key, ok := w.excludedWanted()
	w.retryUsage(&w.excluded, key, ok)
}
