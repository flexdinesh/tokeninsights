package analytics

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

func TestDatasetAnalyticsIsolateCollidingIdentities(t *testing.T) {
	root, err := datastore.OpenKind(t.Context(), filepath.Join(t.TempDir(), "hosted.duckdb"), datastore.KindHosted)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	for _, dataset := range []string{"alice", "bob"} {
		if err := root.CreateDataset(t.Context(), dataset); err != nil {
			t.Fatal(err)
		}
	}
	alice, bob := root.ForDataset("alice"), root.ForDataset("bob")
	ingest := func(store *datastore.Store, input int) {
		t.Helper()
		metadata, err := store.Metadata(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, StreamID: "same-stream", BatchID: "same-batch", FromSequence: 1, ToSequence: 2}
		for i, session := range []string{"same-session", store.DatasetID() + "-session"} {
			record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "same-source", Lineage: "same-lineage", Ordinal: int64(i + 1), Data: json.RawMessage(fmt.Sprintf(`{"type":"message","id":"same-message-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"%s-provider","model":"%s-model-%d","usage":{"input":%d,"output":20}}}`, i, store.DatasetID(), store.DatasetID(), i, input)), Context: []evidence.Context{{Ordinal: 0, Data: json.RawMessage(fmt.Sprintf(`{"type":"session","id":"%s"}`, session))}}, Location: &evidence.Location{DirectoryKey: store.DatasetID() + "-directory", DirectoryName: store.DatasetID() + "-directory", RepositoryKey: store.DatasetID() + "-repo", RepositoryName: store.DatasetID() + "-repo", RepositorySource: "harness"}}
			batch.Entries = append(batch.Entries, evidence.Entry{Sequence: int64(i + 1), Record: record})
		}
		body, err := json.Marshal(batch)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Accept(t.Context(), body); err != nil {
			t.Fatal(err)
		}
		// Lost responses can replay the exact same batch without adding usage.
		if _, err := store.Accept(t.Context(), body); err != nil {
			t.Fatal(err)
		}
	}
	ingest(alice, 100)
	for {
		worked, err := root.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	ingest(bob, 900)
	aStatus, err := Status(t.Context(), alice)
	if err != nil {
		t.Fatal(err)
	}
	bStatus, err := Status(t.Context(), bob)
	if err != nil {
		t.Fatal(err)
	}
	if aStatus.Pending != 0 || bStatus.Pending == 0 || aStatus.Metadata.DatasetID != "alice" || bStatus.Metadata.DatasetID != "bob" {
		t.Fatalf("status leaked: %+v %+v", aStatus, bStatus)
	}
	for {
		worked, err := root.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	if _, err := root.SQL().ExecContext(t.Context(), "INSERT INTO processing.outcomes VALUES('bob','unusable','ambiguous','unusable_usage','',1,1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := root.SQL().ExecContext(t.Context(), "INSERT INTO analytics.estimates SELECT *,fact_id,'fixture' FROM analytics.facts WHERE dataset_id='bob'"); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		store                      *datastore.Store
		total, context, unresolved int64
	}{{alice, 240, 100, 0}, {bob, 1840, 900, 1}} {
		for _, tab := range []string{"tokens", "models", "providers", "harnesses", "sessions", "context", "repo", "directory"} {
			queryTab, group := tab, db.RepoGroupRepository
			if tab == "directory" {
				queryTab, group = "repo", db.RepoGroupDirectory
			}
			q := Query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Quality: "confirmed", Tab: queryTab, Sort: "total", Direction: "desc", Page: 999, PageSize: 1, LocationGroup: group}
			result, err := LoadDashboard(t.Context(), test.store, q, time.Now())
			if err != nil {
				t.Fatal(tab, err)
			}
			if result.DatasetID != test.store.DatasetID() || result.Summary.TotalTokens != test.total || result.Summary.SyncedSessions != 2 || result.Summary.SessionCount != 2 || result.Unresolved != test.unresolved || result.FactCount != 2 {
				t.Fatalf("%s dataset %s leaked: %+v", tab, test.store.DatasetID(), result)
			}
			if tab == "models" || tab == "sessions" || tab == "context" {
				if result.RowCount != 2 || result.Page != 2 || len(result.Rows) != 1 {
					t.Fatalf("pagination %s: %+v", tab, result)
				}
			}
			if tab != "context" {
				var chartTotal int64
				for _, row := range result.Chart {
					chartTotal += row.Total
				}
				if chartTotal != test.total {
					t.Fatalf("%s chart leaked: %+v", tab, result.Chart)
				}
			}
			if tab == "directory" && result.Rows[0].Name != test.store.DatasetID()+"-directory" {
				t.Fatalf("directory leaked: %+v", result.Rows)
			}
			if tab == "context" {
				for _, row := range result.Chart {
					if row.AverageContext != test.context || row.MedianContext != test.context || row.MaxContext != test.context {
						t.Fatalf("context leaked %+v", row)
					}
				}
			}
			facets, err := LoadFacets(t.Context(), test.store, q, "", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if facets.DatasetID != test.store.DatasetID() || len(facets.Models) != 2 || len(facets.Providers) != 1 || facets.Providers[0] != test.store.DatasetID()+"-provider" || len(facets.Sessions) != 2 {
				t.Fatalf("facets leaked %+v", facets)
			}
			if (tab == "repo" || tab == "directory") && (len(facets.Repositories) != 1 || facets.Repositories[0].Key != test.store.DatasetID()+"-repo" || len(facets.Directories) != 1 || facets.Directories[0].Key != test.store.DatasetID()+"-directory") {
				t.Fatalf("locations leaked %+v", facets)
			}
			searched, err := LoadFacets(t.Context(), test.store, q, "bob-session", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			wantedSessions := 0
			if test.store.DatasetID() == "bob" {
				wantedSessions = 1
			}
			if len(searched.Sessions) != wantedSessions || (wantedSessions == 1 && searched.Sessions[0] != "bob-session") {
				t.Fatalf("session search leaked: %+v", searched)
			}
			q.Selection.Providers = []string{"bob-provider"}
			filtered, err := LoadDashboard(t.Context(), alice, q, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if filtered.Summary.TotalTokens != 0 || filtered.RowCount != 0 || len(filtered.Chart) != 0 {
				t.Fatalf("cross-dataset filter leaked %+v", filtered)
			}
			q.Selection.Providers = nil
			q.Quality = "estimated"
			estimated, err := LoadDashboard(t.Context(), test.store, q, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			expected := int64(0)
			if test.store.DatasetID() == "bob" {
				expected = 1840
			}
			if estimated.Summary.TotalTokens != expected {
				t.Fatalf("estimates leaked %+v", estimated)
			}
			estimatedFacets, err := LoadFacets(t.Context(), test.store, q, "", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			expectedModels := 0
			if test.store.DatasetID() == "bob" {
				expectedModels = 2
			}
			if len(estimatedFacets.Models) != expectedModels || len(estimatedFacets.Sessions) != expectedModels {
				t.Fatalf("estimated facets leaked: %+v", estimatedFacets)
			}
		}
	}

	// Reprocessing one user's history must not change the other's generation or totals.
	if _, err := bob.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	for {
		worked, err := root.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			break
		}
	}
	for _, test := range []struct {
		store             *datastore.Store
		generation, total int64
	}{{alice, 1, 240}, {bob, 2, 1840}} {
		q := Query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Quality: "confirmed", Tab: "sessions", Sort: "date", Direction: "desc", Page: 1, PageSize: 10}
		result, err := LoadDashboard(t.Context(), test.store, q, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if result.Generation != test.generation || result.Pending != 0 || result.Summary.TotalTokens != test.total || result.Unresolved != 0 {
			t.Fatalf("generation isolation: %+v", result)
		}
	}
}
