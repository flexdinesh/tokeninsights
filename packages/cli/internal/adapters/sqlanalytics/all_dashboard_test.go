package sqlanalytics

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/sqlutil"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

func TestAllDashboardMatchesPaginatedSnapshots(t *testing.T) {
	store := dashboardStore(t)
	if _, err := store.SQL().ExecContext(t.Context(), sqlutil.Bind("INSERT INTO analytics_estimates SELECT *,fact_id,'fixture' FROM analytics_facts")); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2023, 11, 16, 12, 0, 0, 0, time.Local)
	for _, quality := range []string{"confirmed", "estimated"} {
		for _, tab := range []string{"tokens", "models", "providers", "harnesses", "sessions", "context", "repo"} {
			for _, direction := range []string{"asc", "desc"} {
				t.Run(quality+"/"+tab+"/"+direction, func(t *testing.T) {
					q := analytics.Query{Selection: viewer.Selection{Period: "month", Bucket: "day"}, Quality: quality, Tab: tab, Sort: "total", Direction: direction, Page: 1, PageSize: 2, LocationGroup: querymodel.RepoGroupDirectory}
					if tab == "context" {
						q.Sort = "averageContext"
					}
					want, err := LoadDashboard(t.Context(), store, q, now)
					if err != nil {
						t.Fatal(err)
					}
					for len(want.Rows) < want.RowCount {
						q.Page++
						page, err := LoadDashboard(t.Context(), store, q, now)
						if err != nil {
							t.Fatal(err)
						}
						want.Rows = append(want.Rows, page.Rows...)
					}
					// Complete reads ignore a previously selected page, matching AllUsage.
					q.Page = 999
					got, err := LoadAllDashboard(t.Context(), store, q, now, 6)
					if err != nil || !reflect.DeepEqual(got, want) {
						t.Fatalf("complete read differs: got %+v, want %+v, error %v", got, want, err)
					}
					if got.Summary.TotalTokens != 720 || got.Summary.SessionCount != 6 {
						t.Fatal("usage lost", got.Summary)
					}
				})
			}
		}
	}
	q := analytics.Query{Selection: viewer.Selection{Period: "all", Bucket: "day", Models: []string{"model-2"}}, Quality: "confirmed", Tab: "models", Sort: "name", Direction: "asc", Page: 1, PageSize: 2}
	filtered, err := LoadAllDashboard(t.Context(), store, q, now, 1)
	if err != nil || filtered.Summary.TotalTokens != 120 || filtered.RowCount != 1 || len(filtered.Rows) != 1 || filtered.Rows[0].Name != "model-2" {
		t.Fatal("filter lost", filtered, err)
	}
	q.Selection.Period = "today"
	empty, err := LoadAllDashboard(t.Context(), store, q, now, 1)
	if err != nil || empty.Summary.TotalTokens != 0 || empty.RowCount != 0 || len(empty.Rows) != 0 || empty.Summary.SyncedSessions != 6 {
		t.Fatal("date filter lost", empty, err)
	}
}

func TestAllDashboardBoundsAndCancellation(t *testing.T) {
	store := dashboardStore(t)
	q := analytics.Query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Quality: "confirmed", Tab: "sessions", Sort: "total", Direction: "desc", Page: 1, PageSize: 2}
	for _, limit := range []int{0, -1, 5} {
		result, err := LoadAllDashboard(t.Context(), store, q, time.Now(), limit)
		if err == nil || len(result.Rows) != 0 {
			t.Fatal("invalid or exceeded row bound returned rows", limit, result, err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := LoadAllDashboard(ctx, store, q, time.Now(), 6)
	if err == nil || len(result.Rows) != 0 {
		t.Fatal("canceled read returned rows", result, err)
	}
}

func TestAllDashboardDuringAtomicPublication(t *testing.T) {
	store := dashboardStore(t)
	metadata, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	const publications = 12
	done := make(chan error, 1)
	go func() {
		for range publications {
			err := store.WriteTransaction(t.Context(), func(tx *sql.Tx) error {
				if _, err := tx.ExecContext(t.Context(), sqlutil.Bind("UPDATE analytics_facts SET input_tokens=input_tokens+1,total_tokens=total_tokens+1 WHERE dataset_id=?"), store.DatasetID()); err != nil {
					return err
				}
				_, err := tx.ExecContext(t.Context(), sqlutil.Bind("UPDATE ingestion_metadata SET revision=revision+1 WHERE dataset_id=?"), store.DatasetID())
				return err
			})
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	defer func() {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	q := analytics.Query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Quality: "confirmed", Tab: "sessions", Sort: "total", Direction: "desc", Page: 1, PageSize: 2}
	for range publications {
		result, err := LoadAllDashboard(t.Context(), store, q, time.Now(), 6)
		if err != nil {
			t.Fatal(err)
		}
		delta := result.Revision - metadata.Revision
		if delta < 0 || delta > publications || result.RowCount != 6 || len(result.Rows) != 6 || result.Summary.TotalTokens != 6*(120+delta) {
			t.Fatal("summary/revision mixed publications", result)
		}
		for _, row := range result.Rows {
			if row.Input != 100+delta || row.Total != 120+delta {
				t.Fatal("rows mixed publications", result.Revision, row)
			}
		}
	}
}
