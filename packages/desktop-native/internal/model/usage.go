package model

import (
	"context"
	"errors"
	"slices"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

// ErrDataChanged reports usage or facets answered from another snapshot
// than the status, again after the status was read once more: the web's
// "Data changed. Refreshing service status."
var ErrDataChanged = errors.New("read answered from another snapshot than the status")

// Usage is the usage a window shows: the first page of a usage read, with
// the summary, chart and last ingestion of the whole query, and the view
// it answered. Rows are the rows read so far: the first page and those
// LoadMoreRows appended; RowCount counts every row of the query.
type Usage struct {
	application.Usage
	// Tab and LocationGroup are the view the read answered, which a tab
	// switch leaves behind until its own read answers.
	Tab           dashboardview.Tab
	LocationGroup analytics.RepoGroup
	// Request is the read the rows answered: another request's rows are
	// other rows, in another order, where the same request's rows of
	// another revision are the same rows updated.
	Request analytics.Query
}

// SameRows reports whether request reads the rows u shows, in their order.
func (u Usage) SameRows(request analytics.Query) bool { return sameRequest(u.Request, request) }

// usageKey is one usage read: the request sent and the snapshot it must
// answer from, the web's usage query key.
type usageKey struct {
	request  analytics.Query
	identity string
	revision int64
}

func (k usageKey) equal(other usageKey) bool {
	return k.identity == other.identity && k.revision == other.revision && sameRequest(k.request, other.request)
}

// usageState is the window's usage read, the web's useAnalytics for the
// current query: one read at a time, numbered so a superseded answer is
// dropped.
type usageState struct {
	// seq numbers reads; an answer of an earlier read is stale.
	seq    uint64
	cancel context.CancelFunc
	// running is whether the latest read runs; requested is its key.
	running   bool
	requested *usageKey
	// data is the latest answer for the query's summary scope and the
	// snapshot identity, kept while other reads of that scope run or
	// fail; dataKey is the read it answered.
	data    *Usage
	dataKey usageKey
	err     error
	// awaitingStatus is whether an answer from another snapshot waits
	// for the status read it requested; staleRetried whether the read
	// was already repeated once for that, as the web's single retry.
	awaitingStatus, staleRetried bool
	// rows reads the pages of data after its first.
	rows rowsState
}

// usageWanted is the usage read the window should show, and whether it
// may read: the instance offers usage and the status permits analytics of
// its dataset.
func (w *Window) usageWanted() (usageKey, bool) {
	if w.instance == nil || !w.instance.Capabilities.Has(serverfeatures.Usage) {
		return usageKey{}, false
	}
	sync := w.SyncState()
	if !sync.AnalyticsEnabled {
		return usageKey{}, false
	}
	return usageKey{request: w.query.ToAnalytics(), identity: sync.Identity, revision: sync.Revision}, true
}

// syncUsage reads usage when the wanted read differs from the latest one:
// a new query, tab, snapshot identity or revision. A failed read stays
// failed until RetryUsage. It runs after each change that can change the
// wanted read.
func (w *Window) syncUsage() {
	u := &w.usage
	key, ok := w.usageWanted()
	if !ok || w.rt.Context.Err() != nil {
		return
	}
	if u.awaitingStatus {
		if w.sync.statusRunning {
			return
		}
		u.awaitingStatus = false
		if u.requested != nil && u.requested.equal(key) {
			if u.staleRetried {
				u.err = ErrDataChanged
				return
			}
			u.staleRetried = true
			w.readUsage(key)
			return
		}
	}
	if u.requested != nil && u.requested.equal(key) {
		return
	}
	u.staleRetried = false
	w.readUsage(key)
}

// readUsage starts the read of key, superseding any running one. Data of
// another summary scope goes, as the web shows no placeholder for it.
func (w *Window) readUsage(key usageKey) {
	u := &w.usage
	if u.cancel != nil {
		u.cancel()
	}
	u.seq++
	seq := u.seq
	ctx, cancel := context.WithCancel(w.rt.Context)
	u.cancel, u.running, u.requested, u.err = cancel, true, &key, nil
	w.dropRows()
	if u.data != nil && (u.dataKey.identity != key.identity || !sameSummaryScope(u.dataKey.request, key.request)) {
		u.data = nil
	}
	w.rt.Go(func() {
		usage, err := w.backend.Usage(ctx, key.request)
		w.rt.Update(func() { w.usageRead(seq, key, usage, err) })
	})
}

// usageRead applies the answer of read seq unless a later read superseded
// it. An answer from another snapshot identity or an older revision than
// the status reads status again, then the usage once more; an answer from
// a newer revision is shown and reads status again, as the web's.
func (w *Window) usageRead(seq uint64, key usageKey, usage application.Usage, err error) {
	u := &w.usage
	if seq != u.seq {
		return
	}
	u.running = false
	u.cancel()
	u.cancel = nil
	if err != nil {
		u.err = err
		return
	}
	if dashboardview.UsageIdentity(usage) != key.identity || usage.Revision < key.revision {
		u.awaitingStatus = true
		w.RefetchStatus()
		return
	}
	if usage.Revision > key.revision {
		w.RefetchStatus()
	}
	u.data = &Usage{Usage: usage, Tab: dashboardview.Tab(key.request.Tab), LocationGroup: key.request.LocationGroup, Request: key.request}
	u.dataKey, u.staleRetried = key, false
}

// usageSnapshotChanged drops usage of another snapshot identity and its
// running read, as the web removes its cached usage.
func (w *Window) usageSnapshotChanged(change SnapshotChange) {
	if !change.IdentityChanged {
		return
	}
	u := &w.usage
	if u.cancel != nil {
		u.cancel()
	}
	w.dropRows()
	w.usage = usageState{seq: u.seq + 1}
}

// Usage is the usage the window shows for the query's summary scope and
// snapshot identity, possibly answered for another tab or revision while
// that read runs; ok is false before one answered.
func (w *Window) Usage() (Usage, bool) {
	if w.usage.data == nil {
		return Usage{}, false
	}
	return *w.usage.data, true
}

// UsageError is why the latest usage read failed, or nil.
func (w *Window) UsageError() error { return w.usage.err }

// UsageFetching reports whether a usage read runs, or none answered or
// failed yet: the web's pending or fetching usage.
func (w *Window) UsageFetching() bool {
	u := &w.usage
	return u.running || u.awaitingStatus || (u.data == nil && u.err == nil)
}

// UsagePlaceholder reports whether the usage shown answered an earlier
// read than the latest, as the web's placeholder data.
func (w *Window) UsagePlaceholder() bool {
	u := &w.usage
	return u.data != nil && (u.requested == nil || !u.requested.equal(u.dataKey))
}

// UsageMatchesView reports whether the usage shown answered the selected
// tab and, on Repo, its grouping, so its chart and rows fit the view.
func (w *Window) UsageMatchesView() bool {
	u := &w.usage
	return u.data != nil && u.data.Tab == w.query.Tab &&
		(w.query.Tab != dashboardview.TabRepo || u.data.LocationGroup == w.query.LocationGroup)
}

// RetryUsage reads usage again, the ErrorBanner's Retry Request.
func (w *Window) RetryUsage() {
	key, ok := w.usageWanted()
	if !ok || w.rt.Context.Err() != nil {
		return
	}
	w.usage.awaitingStatus, w.usage.staleRetried = false, false
	w.readUsage(key)
}

// SameRequest reports whether two usage requests ask the same rows and
// chart: the same view, sort, page, bucket and summary scope.
func SameRequest(a, b analytics.Query) bool { return sameRequest(a, b) }

// sameRequest reports whether two usage requests ask the same.
func sameRequest(a, b analytics.Query) bool {
	return a.Tab == b.Tab && a.Sort == b.Sort && a.Direction == b.Direction && a.Page == b.Page &&
		a.PageSize == b.PageSize && a.Selection.Bucket == b.Selection.Bucket && sameSummaryScope(a, b)
}

// sameSummaryScope reports whether two requests share the web's summary
// scope: quality, period, bounds, dimension filters and Repo locations,
// whose usage totals are the same whatever the tab, sort, page or bucket.
func sameSummaryScope(a, b analytics.Query) bool {
	sa, sb := a.Selection, b.Selection
	return a.Quality == b.Quality && sa.Period == sb.Period && sa.From == sb.From && sa.To == sb.To &&
		slices.Equal(sa.Providers, sb.Providers) && slices.Equal(sa.Models, sb.Models) &&
		slices.Equal(sa.Harnesses, sb.Harnesses) && slices.Equal(sa.Sessions, sb.Sessions) &&
		a.LocationGroup == b.LocationGroup && slices.Equal(a.RepositoryKeys, b.RepositoryKeys) &&
		slices.Equal(a.DirectoryKeys, b.DirectoryKeys)
}
