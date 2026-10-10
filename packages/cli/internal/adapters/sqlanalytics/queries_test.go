package sqlanalytics

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

func dashboardStore(t *testing.T) *datastore.Store {
	t.Helper()
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	metadata, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: 1, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 6}
	for i := 0; i < 6; i++ {
		record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: int64(i + 1), Data: json.RawMessage(fmt.Sprintf(`{"type":"message","id":"m-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"model-%d","usage":{"input":100,"output":20}}}`, i, i)), Context: []evidence.Context{{Ordinal: 0, Data: json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, i))}}, Location: &evidence.Location{DirectoryKey: "directory", DirectoryName: "project", RepositoryKey: "repository", RepositoryName: "repo", RepositorySource: "harness"}}
		batch.Entries = append(batch.Entries, evidence.Entry{Sequence: int64(i + 1), Record: record})
	}
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, body); err != nil {
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
	return store
}

func TestSQLDashboardTabsFiltersPaginationAndSeparateEstimates(t *testing.T) {
	store := dashboardStore(t)
	for _, tab := range []string{"tokens", "models", "providers", "harnesses", "sessions", "context", "repo"} {
		t.Run(tab, func(t *testing.T) {
			q := analytics.Query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Tab: tab, Quality: "confirmed", Sort: "total", Direction: "desc", Page: 1, PageSize: 2, LocationGroup: querymodel.RepoGroupRepository}
			if tab == "context" {
				q.Sort = "averageContext"
			}
			data, err := LoadDashboard(t.Context(), store, q, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if data.Summary.TotalTokens != 720 || data.Summary.SessionCount != 6 || data.Pending != 0 || data.Generation != 1 || data.InputRevision != 1 {
				t.Fatalf("wrong summary/snapshot: %+v", data)
			}
			if len(data.Rows) > 2 {
				t.Fatal("unbounded page")
			}
			facets, err := LoadFacets(t.Context(), store, q, "", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if len(facets.Models) != 6 || len(facets.Sessions) != 6 || facets.Revision != data.Revision {
				t.Fatalf("facets/snapshot %+v", facets)
			}
			q.Quality = "estimated"
			estimated, err := LoadDashboard(t.Context(), store, q, time.Now())
			if err != nil || estimated.Summary.TotalTokens != 0 || estimated.RowCount != 0 {
				t.Fatalf("confirmed leaked into estimated: %+v %v", estimated, err)
			}
		})
	}
	q := analytics.Query{Selection: viewer.Selection{Period: "all", Bucket: "day", Models: []string{"model-2"}}, Tab: "models", Quality: "confirmed", Sort: "name", Direction: "asc", Page: 999999999, PageSize: 2}
	data, err := LoadDashboard(t.Context(), store, q, time.Now())
	if err != nil || data.Summary.TotalTokens != 120 || data.Page != 1 || data.Rows[0].Name != "model-2" {
		t.Fatalf("filter/page %+v %v", data, err)
	}
}
