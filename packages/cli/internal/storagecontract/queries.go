package storagecontract

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
)

// These oracles enter through raw ingestion, so adapters cannot satisfy query
// parity by bypassing processing, session identity or active-generation views.
func queryTabsAndSessionPeaks(t *testing.T, store Tokens) {
	d := dataset(t, store, "query-contract")
	const smallerPrompt = 5
	peaks := []int{10, 30, 31, 50}
	for session, peak := range peaks {
		var records []evidence.Record
		for message, input := range []int{smallerPrompt, peak} {
			r := record(fmt.Sprintf("message-%d-%d", session, message))
			r.Data = json.RawMessage(fmt.Sprintf(`{"type":"message","id":"message-%d-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"model","usage":{"input":%d,"output":1}}}`, session, message, input))
			r.Context[0].Data = json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, session))
			records = append(records, r)
		}
		accept(t, d, batch(t, d, fmt.Sprintf("session-%d", session), records...))
	}
	drain(t, d)
	now := time.UnixMilli(1700000000000)
	for _, tab := range []string{"tokens", "models", "providers", "harnesses", "sessions", "context", "repo"} {
		t.Run(tab, func(t *testing.T) {
			q := query()
			q.Tab, q.PageSize = tab, 2
			q.LocationGroup = querymodel.RepoGroupRepository
			if tab == "context" {
				q.Sort = "averageContext"
			}
			first, err := d.Queries.Dashboard(t.Context(), q, now)
			if err != nil {
				t.Fatal(err)
			}
			if first.FactCount != 8 || first.Summary.InputTokens != 141 || first.Summary.OutputTokens != 8 || first.Summary.TotalTokens != 149 || first.Summary.SessionCount != 4 {
				t.Fatal("tab changed accounting", first)
			}
			wantRows := 1
			if tab == "sessions" {
				wantRows = 4
			}
			if first.RowCount != wantRows {
				t.Fatal("wrong grouping", first.Rows)
			}
			rows := append([]analytics.Row{}, first.Rows...)
			for page := 2; len(rows) < wantRows; page++ {
				q.Page = page
				next, err := d.Queries.Dashboard(t.Context(), q, now)
				if err != nil || len(next.Rows) == 0 || next.Summary != first.Summary || next.Revision != first.Revision || next.Generation != first.Generation {
					t.Fatal("page lost snapshot or progress", next, err)
				}
				rows = append(rows, next.Rows...)
			}
			all, err := d.Queries.AllDashboard(t.Context(), q, now, wantRows)
			if err != nil || !reflect.DeepEqual(all.Rows, rows) || all.Summary != first.Summary || all.Revision != first.Revision {
				t.Fatal("complete rows differ from pages", all, rows, err)
			}
			if tab == "context" {
				row := rows[0]
				if row.Sessions != 4 || row.AverageContext != 30 || row.MedianContext != 30 || row.MaxContext != 50 {
					t.Fatal("context must aggregate session peaks with floored even median", row)
				}
				q.Selection.Sessions = []string{"session-0", "session-1", "session-2"}
				odd, err := d.Queries.Dashboard(t.Context(), q, now)
				if err != nil || len(odd.Rows) != 1 {
					t.Fatal("filtered context unavailable", odd, err)
				}
				row = odd.Rows[0]
				if row.Sessions != 3 || row.AverageContext != 23 || row.MedianContext != 30 || row.MaxContext != 31 {
					t.Fatal("odd median/filter must use session peaks", row)
				}
			}
			if tab == "repo" && (rows[0].LocationKey != "unknown" || !rows[0].HasUnknownDirectory || len(rows[0].DirectoryNames) != 0) {
				t.Fatal("missing locations must remain visible", rows[0])
			}
		})
	}
}

func orderingAndBounds(t *testing.T, store Tokens) {
	d := dataset(t, store, "ordering")
	var records []evidence.Record
	for i, model := range []string{"z", "é", "a", "A"} {
		r := record(fmt.Sprintf("ordered-%d", i))
		r.Data = json.RawMessage(fmt.Sprintf(`{"type":"message","id":"ordered-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":%q,"usage":{"input":10,"output":0}}}`, i, model))
		records = append(records, r)
	}
	accept(t, d, batch(t, d, "ordered", records...))
	drain(t, d)
	q := query()
	q.Tab = "models"
	q.PageSize = 1
	for i, want := range []string{"A", "a", "z", "é"} {
		q.Page = i + 1
		got, err := d.Queries.Dashboard(t.Context(), q, time.Now())
		if err != nil || len(got.Rows) != 1 || got.Rows[0].Name != want {
			t.Fatal("unstable bytewise pagination", want, got, err)
		}
	}
	d = dataset(t, store, "bounds")
	records = nil
	for i := range 2 {
		r := record(fmt.Sprintf("bounded-%d", i))
		r.Data = json.RawMessage(fmt.Sprintf(`{"type":"message","id":"bounded-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"model","usage":{"input":%d,"output":0}}}`, i, publication.SafeInteger/2+1))
		records = append(records, r)
	}
	accept(t, d, batch(t, d, "bounded", records...))
	drain(t, d)
	if _, err := d.Queries.Dashboard(t.Context(), query(), time.Now()); err == nil || !strings.Contains(err.Error(), "aggregate_limit") {
		t.Fatal("unsafe aggregate accepted or misreported", err)
	}
}

func calendarQueries(t *testing.T, store Tokens) {
	t.Setenv("TZ", "/unavailable-test-zone")
	original := time.Local
	t.Cleanup(func() { time.Local = original })
	for index, tc := range []struct {
		zone, at, day, week, month, year string
		offset                           int
	}{
		{"America/Los_Angeles", "2026-03-08T07:59:59Z", "2026-03-07", "2026-03-02", "2026-03", "2026", 0},
		{"America/Los_Angeles", "2026-03-08T10:00:00Z", "2026-03-08", "2026-03-02", "2026-03", "2026", 0},
		{"Australia/Sydney", "2026-10-03T16:00:00Z", "2026-10-04", "2026-09-28", "2026-10", "2026", 0},
		{"fixed", "2026-01-01T20:00:00Z", "2026-01-02", "2025-12-29", "2026-01", "2026", 19800},
		{"fixed", "2026-01-01T02:00:00Z", "2025-12-31", "2025-12-29", "2025-12", "2025", -28800},
	} {
		t.Run(fmt.Sprintf("%d", index), func(t *testing.T) {
			var err error
			if tc.zone == "fixed" {
				time.Local = time.FixedZone("custom", tc.offset)
			} else {
				time.Local, err = time.LoadLocation(tc.zone)
				if err != nil {
					t.Fatal(err)
				}
			}
			at, err := time.Parse(time.RFC3339, tc.at)
			if err != nil {
				t.Fatal(err)
			}
			d := dataset(t, store, fmt.Sprintf("calendar-%d", index))
			r := record("calendar")
			r.Data = json.RawMessage(fmt.Sprintf(`{"type":"message","id":"calendar","message":{"role":"assistant","timestamp":%d,"provider":"openai","model":"model","usage":{"input":10,"output":1}}}`, at.UnixMilli()))
			accept(t, d, batch(t, d, "calendar", r))
			drain(t, d)
			for bucket, want := range map[string]string{"day": tc.day, "week": tc.week, "month": tc.month, "year": tc.year} {
				q := query()
				q.Tab = "tokens"
				q.Selection.Bucket = bucket
				q.Selection.From = tc.day
				q.Selection.To = tc.day
				got, err := d.Queries.Dashboard(t.Context(), q, at)
				if err != nil || len(got.Rows) != 1 || got.Rows[0].Name != want || got.Summary.TotalTokens != 11 {
					t.Fatal("calendar boundary", bucket, want, got, err)
				}
				q.Selection.From = "2027-01-01"
				q.Selection.To = "2027-01-01"
				empty, err := d.Queries.Dashboard(t.Context(), q, at)
				if err != nil || empty.FactCount != 0 || len(empty.Rows) != 0 {
					t.Fatal("day range leaked", empty, err)
				}
			}
		})
	}
}
