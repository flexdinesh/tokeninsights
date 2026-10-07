package server

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

func TestDuckDashboardTabsFiltersPaginationAndSeparateEstimates(t *testing.T) {
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	metadata, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	batch := evidence.Batch{ProtocolVersion: 2, ExtractorVersion: 1, DatabaseID: metadata.DatabaseID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 6}
	for i := 0; i < 6; i++ {
		record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: int64(i + 1), Data: json.RawMessage(fmt.Sprintf(`{"type":"message","id":"m-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"model-%d","usage":{"input":100,"output":20}}}`, i, i)), Context: []evidence.Context{{Ordinal: 0, Data: json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, i))}}, Location: &evidence.Location{DirectoryKey: "directory", DirectoryName: "project", RepositoryKey: "repository", RepositoryName: "repo", RepositorySource: "harness"}}
		batch.Entries = append(batch.Entries, evidence.Entry{Sequence: int64(i + 1), Record: record})
	}
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Accept(t.Context(), body); err != nil {
		t.Fatal(err)
	}
	for {
		worked, err := store.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	for _, tab := range []string{"tokens", "models", "providers", "harnesses", "sessions", "context", "repo"} {
		t.Run(tab, func(t *testing.T) {
			q := query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Tab: tab, Quality: "confirmed", Sort: "total", Direction: "desc", Page: 1, PageSize: 2, LocationGroup: db.RepoGroupRepository}
			if tab == "context" {
				q.Sort = "averageContext"
			}
			data, err := loadDataDashboard(t.Context(), store, q, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if data.Summary.TotalTokens != 720 || data.Summary.SessionCount != 6 || data.Pending != 0 || data.Generation != 1 || data.InputRevision != 1 {
				t.Fatalf("wrong summary/snapshot: %+v", data)
			}
			if len(data.Rows) > 2 {
				t.Fatal("unbounded page")
			}
			facets, err := loadDataFacets(t.Context(), store, q, "", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if len(facets.Models) != 6 || len(facets.Sessions) != 6 || facets.Revision != data.Revision {
				t.Fatalf("facets/snapshot %+v", facets)
			}
			q.Quality = "estimated"
			estimated, err := loadDataDashboard(t.Context(), store, q, time.Now())
			if err != nil || estimated.Summary.TotalTokens != 0 || estimated.RowCount != 0 {
				t.Fatalf("confirmed leaked into estimated: %+v %v", estimated, err)
			}
		})
	}
	q := query{Selection: viewer.Selection{Period: "all", Bucket: "day", Models: []string{"model-2"}}, Tab: "models", Quality: "confirmed", Sort: "name", Direction: "asc", Page: 999999999, PageSize: 2}
	data, err := loadDataDashboard(t.Context(), store, q, time.Now())
	if err != nil || data.Summary.TotalTokens != 120 || data.Page != 1 || data.Rows[0].Name != "model-2" {
		t.Fatalf("filter/page %+v %v", data, err)
	}
}

func TestDuckCalendarBucketsAndDayFilters(t *testing.T) {
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	// Test SQL calendar conversion independently of the machine's reporting zone.
	cases := []struct {
		zone      string
		timestamp string
		day       string
	}{
		{"@19800", "2026-01-01T20:00:00Z", "2026-01-02"},
		{"@-28800", "2026-01-02T02:00:00Z", "2026-01-01"},
		{"America/Los_Angeles", "2026-03-08T07:59:59Z", "2026-03-07"},
		{"America/Los_Angeles", "2026-03-08T10:00:00Z", "2026-03-08"},
		{"Australia/Sydney", "2026-10-03T16:00:00Z", "2026-10-04"},
	}
	for _, tc := range cases {
		at, err := time.Parse(time.RFC3339, tc.timestamp)
		if err != nil {
			t.Fatal(err)
		}
		expression, parameter := duckTimeExpression(tc.zone)
		var got string
		statement := "SELECT strftime(" + expression + ",'%Y-%m-%d') FROM (SELECT CAST(? AS BIGINT) AS occurred_at_ms)"
		if err := store.SQL().QueryRow(statement, parameter, at.UnixMilli()).Scan(&got); err != nil || got != tc.day {
			t.Fatal(tc, got, err)
		}
		where, args := duckWhere(db.Filter{DayFrom: tc.day, DayTo: tc.day}, tc.zone)
		var n int
		filterArgs := append([]interface{}{at.UnixMilli()}, args...)
		if err := store.SQL().QueryRow("SELECT COUNT(*) FROM (SELECT CAST(? AS BIGINT) AS occurred_at_ms,TRUE AS countable)"+where, filterArgs...).Scan(&n); err != nil || n != 1 {
			t.Fatal("inclusive calendar bounds", tc, n, err)
		}
	}
}
