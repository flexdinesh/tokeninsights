package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/application"
	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// sessionRows are count rows of the Sessions view, keyed by their order.
func sessionRows(count int) []analytics.Row {
	rows := make([]analytics.Row, count)
	for i := range rows {
		rows[i] = analytics.Row{Key: fmt.Sprintf("session-%03d", i), Name: fmt.Sprintf("session %d", i), Total: int64(count - i)}
	}
	return rows
}

// pageAt is page of rows read at revision of generation 1, as the
// backend answers it.
func pageAt(rows []analytics.Row, page int, revision int64) application.Usage {
	usage := usageAt(revision, 1)
	start := min((page-1)*TablePageSize, len(rows))
	usage.Rows = rows[start:min(start+TablePageSize, len(rows))]
	usage.RowCount, usage.Page, usage.PageSize = len(rows), page, TablePageSize
	return usage
}

// pagedSynced is a harness whose window read the first page of rows.
func pagedSynced(t *testing.T, rows []analytics.Row) *harness {
	h := newHarness(t)
	h.loaded(testInstance)
	h.window.Poll(at(0), false)
	h.answerReads(statusAt(1, 1), progressOf())
	if got := h.expectUsage(); got.Page != 1 || got.PageSize != TablePageSize {
		t.Fatalf("first usage read %+v", got)
	}
	h.answerUsage(pageAt(rows, 1, 1), nil)
	return h
}

func shownRows(w *Window) []analytics.Row {
	usage, _ := w.Usage()
	return usage.Rows
}

// expectPage waits for a page read and checks it reads page of the
// query's request.
func (h *harness) expectPage(page int) {
	h.t.Helper()
	request := h.expectUsage()
	query, _ := h.window.Query()
	want := query.ToAnalytics()
	want.Page = page
	if !sameRequest(request, want) {
		h.t.Fatalf("page read %+v, want %+v", request, want)
	}
}

func TestMoreRowsReadOnePageAtATime(t *testing.T) {
	rows := sessionRows(450)
	h := pagedSynced(t, rows)
	w := h.window
	if got := shownRows(w); len(got) != TablePageSize || !w.MoreRows() {
		t.Fatalf("first page: %d rows", len(got))
	}
	w.LoadMoreRows()
	h.expectPage(2)
	w.LoadMoreRows() // one page at a time
	h.noCall()
	if !w.RowsLoading() {
		t.Fatal("no page loading")
	}
	h.answerUsage(pageAt(rows, 2, 1), nil)
	if got := shownRows(w); len(got) != 2*TablePageSize || got[TablePageSize].Key != rows[TablePageSize].Key || w.RowsLoading() {
		t.Fatalf("after page 2: %d rows", len(got))
	}
	w.LoadMoreRows()
	h.expectPage(3)
	h.answerUsage(pageAt(rows, 3, 1), nil)
	if got := shownRows(w); len(got) != len(rows) || got[len(got)-1].Key != rows[len(rows)-1].Key || w.MoreRows() {
		t.Fatalf("after page 3: %d rows", len(got))
	}
	if usage, _ := w.Usage(); usage.RowCount != len(rows) || usage.Summary.TotalTokens != 647_000 {
		t.Fatalf("the pages changed the summary or count: %+v", usage.Summary)
	}
	w.LoadMoreRows() // every row read
	h.noCall()
}

func TestPagesOfSupersededReadsAreDropped(t *testing.T) {
	rows := sessionRows(450)
	h := pagedSynced(t, rows)
	w := h.window
	w.LoadMoreRows()
	h.expectPage(2)
	w.SortBy(dashboardview.SortTotal) // a new query reads its first page
	request := h.expectUsage()
	if request.Page != 1 || request.Sort != string(dashboardview.SortTotal) {
		t.Fatalf("sorted request %+v", request)
	}
	h.apply() // the page read, cancelled, answers late
	if w.RowsError() != nil || w.RowsLoading() || len(shownRows(w)) != TablePageSize {
		t.Fatalf("a superseded page applied: %v, %d rows", w.RowsError(), len(shownRows(w)))
	}
	w.LoadMoreRows() // the rows shown answer an earlier read
	h.noCall()
	reversed := sessionRows(450)
	reversed[0].Key = "session-sorted"
	h.answerUsage(pageAt(reversed, 1, 1), nil)
	if got := shownRows(w); len(got) != TablePageSize || got[0].Key != "session-sorted" {
		t.Fatalf("sorted rows %d", len(got))
	}

	// A new revision reads the first page again, dropping the page read
	// of the earlier revision.
	w.LoadMoreRows()
	h.expectPage(2)
	w.RefetchStatus()
	h.expectCall("status")
	h.answerStatus(statusAt(2, 1), nil)
	h.expectCalls("instance", "usage")
	if request := <-h.backend.requests; request.Page != 1 {
		t.Fatalf("revision request %+v", request)
	}
	h.apply() // the cancelled page read answers before the instance
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	if len(shownRows(w)) != TablePageSize || w.RowsError() != nil {
		t.Fatalf("a page of the earlier revision applied: %d rows", len(shownRows(w)))
	}
	h.answerUsage(pageAt(reversed, 1, 2), nil)
	if usage, _ := w.Usage(); usage.Revision != 2 || len(usage.Rows) != TablePageSize {
		t.Fatalf("revision 2 usage %d rows", len(usage.Rows))
	}
}

func TestPageOfAnotherRevisionRereadsStatus(t *testing.T) {
	rows := sessionRows(450)
	h := pagedSynced(t, rows)
	w := h.window
	w.LoadMoreRows()
	h.expectPage(2)
	h.answerUsage(pageAt(rows, 2, 2), nil) // published since the first page
	h.expectCall("status")
	if len(shownRows(w)) != TablePageSize || !errors.Is(w.RowsError(), ErrDataChanged) {
		t.Fatalf("a page of another revision: %d rows, %v", len(shownRows(w)), w.RowsError())
	}
	w.LoadMoreRows() // waits for the new revision
	h.noCall()
	h.answerStatus(statusAt(2, 1), nil)
	h.expectCalls("instance", "usage")
	<-h.backend.requests
	h.backend.instances <- instanceResult{instance: testInstance}
	h.apply()
	h.answerUsage(pageAt(rows, 1, 2), nil)
	if w.RowsError() != nil || len(shownRows(w)) != TablePageSize || !w.MoreRows() {
		t.Fatal("the new revision kept the page failure")
	}
}

func TestPageFailureKeepsRowsUntilRetry(t *testing.T) {
	rows := sessionRows(450)
	h := pagedSynced(t, rows)
	w := h.window
	w.LoadMoreRows()
	h.expectPage(2)
	h.answerUsage(application.Usage{}, errors.New("page unavailable"))
	if w.RowsError() == nil || len(shownRows(w)) != TablePageSize || w.UsageError() != nil {
		t.Fatalf("page failure: %v, %d rows", w.RowsError(), len(shownRows(w)))
	}
	assertStatus(t, w, "Ready", dashboardview.StatusReady)
	w.LoadMoreRows() // a failed page waits for its retry
	h.noCall()
	w.RetryRows()
	h.expectPage(2)
	if w.RowsError() != nil {
		t.Fatal("retry kept the failure")
	}
	h.answerUsage(pageAt(rows, 2, 1), nil)
	if len(shownRows(w)) != 2*TablePageSize {
		t.Fatalf("retried page: %d rows", len(shownRows(w)))
	}
}

func TestSortRereadsTheFirstPageSortedByTheBackend(t *testing.T) {
	rows := sessionRows(450)
	h := pagedSynced(t, rows)
	w := h.window
	w.LoadMoreRows()
	h.expectPage(2)
	h.answerUsage(pageAt(rows, 2, 1), nil)

	w.SortBy(dashboardview.SortTotal)
	request := h.expectUsage()
	if request.Sort != string(dashboardview.SortTotal) || request.Direction != string(dashboardview.Descending) || request.Page != 1 || request.PageSize != TablePageSize {
		t.Fatalf("sort request %+v", request)
	}
	if usage, _ := w.Usage(); len(usage.Rows) != 2*TablePageSize || !w.UsagePlaceholder() || usage.SameRows(request) {
		t.Fatal("the rows sorted before should show until the sorted rows answer")
	}
	h.answerUsage(pageAt(rows, 1, 1), nil)
	usage, _ := w.Usage()
	if len(usage.Rows) != TablePageSize || !usage.SameRows(request) {
		t.Fatalf("sorted rows %d", len(usage.Rows))
	}
	w.SortBy(dashboardview.SortTotal) // the current sort flips
	if request := h.expectUsage(); request.Sort != string(dashboardview.SortTotal) || request.Direction != string(dashboardview.Ascending) {
		t.Fatalf("flipped request %+v", request)
	}
}

func TestClearFiltersReadsWithoutThem(t *testing.T) {
	h := synced(t)
	w := h.window
	w.ClearFilters()
	request := h.expectUsage()
	if request.Selection.From != "" || request.Selection.To != "" || request.Selection.Period != testInstance.Defaults.Period {
		t.Fatalf("cleared request %+v", request)
	}
}
