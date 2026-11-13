package model

import (
	"context"
	"slices"

	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// TablePageSize is how many rows one on-demand page reads: the backend's
// largest page.
//
// Native shows the table's rows in one virtualized list instead of the
// web's pager. Its query keeps the web's Page and PageSize: Page stays 1,
// as every query transition resets it and nothing pages it, and PageSize
// is TablePageSize. The usage read of the query is page 1, with the
// summary and chart; LoadMoreRows reads pages 2, 3, … of the same request
// and snapshot and appends their rows to Usage.Rows.
const TablePageSize = application.MaxPageSize

// rowsState is the on-demand page read after the rows shown: one at a
// time, numbered so an answer of an earlier one is dropped.
type rowsState struct {
	seq     uint64
	cancel  context.CancelFunc
	running bool
	err     error
}

// MoreRows reports whether the query has rows beyond those read.
func (w *Window) MoreRows() bool {
	d := w.usage.data
	return d != nil && len(d.Rows) < d.RowCount
}

// RowsLoading reports whether the next page of rows is being read.
func (w *Window) RowsLoading() bool { return w.usage.rows.running }

// RowsError is why the last page of rows failed to read, or nil. Rows
// read before stay; LoadMoreRows waits for RetryRows.
func (w *Window) RowsError() error { return w.usage.rows.err }

// LoadMoreRows reads the next page of the rows shown, as the table nears
// the end of those read. It does nothing while a page or usage read runs,
// after a page failed until RetryRows, when every row is read, or while
// the rows shown answered an earlier read than the latest.
func (w *Window) LoadMoreRows() {
	u := &w.usage
	if !w.MoreRows() || u.rows.running || u.rows.err != nil || w.UsagePlaceholder() || w.UsageFetching() ||
		w.rt.Context.Err() != nil {
		return
	}
	w.readRows()
}

// RetryRows reads the page that failed again.
func (w *Window) RetryRows() {
	w.usage.rows.err = nil
	w.LoadMoreRows()
}

// readRows reads the page after the rows shown, of the request and the
// snapshot they answered.
func (w *Window) readRows() {
	u, r := &w.usage, &w.usage.rows
	key, loaded := u.dataKey, len(u.data.Rows)
	size := key.request.PageSize
	if size < 1 || loaded%size != 0 {
		return // only full pages come before another
	}
	page := loaded/size + 1
	request := key.request
	request.Page = page
	r.seq++
	usageSeq, seq := u.seq, r.seq
	ctx, cancel := context.WithCancel(w.rt.Context)
	r.cancel, r.running, r.err = cancel, true, nil
	w.rt.Go(func() {
		usage, err := w.backend.Usage(ctx, request)
		w.rt.Update(func() { w.rowsRead(usageSeq, seq, page, usage, err) })
	})
}

// rowsRead appends the rows of page unless a later usage or page read
// superseded the read. A page of another snapshot, revision or row count
// than the rows shown is dropped and reads status again, whose new
// revision reads the query's first page again; the rows wait until then.
func (w *Window) rowsRead(usageSeq, seq uint64, page int, usage application.Usage, err error) {
	u, r := &w.usage, &w.usage.rows
	if usageSeq != u.seq || seq != r.seq {
		return
	}
	r.running = false
	r.cancel()
	r.cancel = nil
	d := u.data
	if d == nil || len(d.Rows) != (page-1)*u.dataKey.request.PageSize {
		return
	}
	if err != nil {
		r.err = err
		return
	}
	if dashboardview.UsageIdentity(usage) != u.dataKey.identity || usage.Revision != d.Revision ||
		usage.RowCount != d.RowCount || usage.Page != page {
		r.err = ErrDataChanged
		w.RefetchStatus()
		return
	}
	d.Rows = append(slices.Clip(d.Rows), usage.Rows...)
}

// dropRows cancels the page read and forgets its failure, as the rows it
// would extend go.
func (w *Window) dropRows() {
	r := &w.usage.rows
	if r.cancel != nil {
		r.cancel()
	}
	*r = rowsState{seq: r.seq + 1}
}
