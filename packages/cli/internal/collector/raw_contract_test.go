package collector_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func rawDrain(t *testing.T, store *datastore.Store) {
	t.Helper()
	for {
		worked, err := store.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			return
		}
	}
}

func TestAllHarnessLegacyImportCoveragePreservesNativeIDs(t *testing.T) {
	for _, direct := range []bool{false, true} {
		name := "http"
		if direct {
			name = "direct"
		}
		t.Run(name, func(t *testing.T) {
			options, legacy, path := acceptanceSetup(t)
			oldServer := httptest.NewServer(ingestion.NewHandler(ingestion.NewCore(legacy)))
			options.ServerURL = oldServer.URL
			acceptanceRun(t, options)
			before := acceptanceAssert(t, legacy)
			sort.Strings(before)
			oldServer.Close()
			if err := legacy.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := datastore.Open(t.Context(), filepath.Join(filepath.Dir(path), "server.duckdb"))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			if !reflect.DeepEqual(before, rawGolden(t, store)) {
				t.Fatal("import changed native identity")
			}
			server := httptest.NewServer(store.Handler())
			defer server.Close()
			options.ServerURL = server.URL
			if direct {
				caps, err := store.RawCapabilities(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				options.ServerDBPath = filepath.Join(filepath.Dir(path), "server.duckdb")
				options.Destination = &collector.Destination{URL: "http://local", DatabaseID: caps.DatabaseID, DatasetID: caps.DatasetID, Local: true, Transport: collector.DirectDelivery{Receiver: store}}
			}
			if _, err := collector.Run(t.Context(), options); err != nil {
				t.Fatal(err)
			}
			rawDrain(t, store)
			if !reflect.DeepEqual(before, rawGolden(t, store)) {
				t.Fatal("coverage changed native identity")
			}
			var covered int
			if err := store.SQL().QueryRow("SELECT COUNT(*) FROM analytics.legacy_coverage").Scan(&covered); err != nil || covered != 12 {
				t.Fatal("native coverage incomplete", covered, err)
			}
		})
	}
}

func rawGolden(t *testing.T, store *datastore.Store) []string {
	t.Helper()
	var components [7]int64
	if err := store.SQL().QueryRow(`SELECT COUNT(*),CAST(SUM(input_tokens) AS BIGINT),CAST(SUM(output_tokens) AS BIGINT),CAST(SUM(reasoning_tokens) AS BIGINT),CAST(SUM(cache_read_tokens) AS BIGINT),CAST(SUM(cache_write_tokens) AS BIGINT),CAST(SUM(total_tokens) AS BIGINT) FROM analytics.confirmed WHERE countable`).Scan(&components[0], &components[1], &components[2], &components[3], &components[4], &components[5], &components[6]); err != nil {
		t.Fatal(err)
	}
	if components != [7]int64{12, 800, 148, 52, 96, 6, 1102} {
		rows, _ := store.SQL().Query("SELECT harness,session_native_id,message_native_id,total_tokens FROM analytics.confirmed ORDER BY 1,2,3")
		if rows != nil {
			for rows.Next() {
				var h, s, m string
				var n int64
				_ = rows.Scan(&h, &s, &m, &n)
				t.Log(h, s, m, n)
			}
			_ = rows.Close()
		}
		t.Fatalf("native component oracle changed: %v", components)
	}
	rows, err := store.SQL().Query("SELECT fact_id FROM analytics.confirmed ORDER BY fact_id")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestRawCollectorNativeGoldenRebuildAndLostAcknowledgement(t *testing.T) {
	root := t.TempDir()
	store, err := datastore.Open(t.Context(), filepath.Join(root, "server.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	lose := true
	handler := store.Handler()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == datastore.IngestionPrefix+"batches" && lose {
			lose = false
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := store.Accept(r.Context(), body); err != nil {
				t.Error(err)
			}
			http.Error(w, "lost acknowledgement", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	options := collector.Options{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerURL: httpServer.URL, SyncOptions: pipeline.SyncOptions{SourceDir: acceptanceSources(t), Harnesses: pipeline.SupportedHarnesses}}
	first, err := collector.Run(t.Context(), options)
	if err == nil || first.CollectionError != nil || !first.PendingKnown || first.Pending == 0 {
		t.Fatalf("lost receipt unexpectedly acknowledged %+v %v", first, err)
	}
	rawDrain(t, store)
	ids := rawGolden(t, store)
	second, err := collector.Run(t.Context(), options)
	if err != nil || second.Pending != 0 || second.Accepted == 0 {
		t.Fatalf("retry %+v %v", second, err)
	}
	rawDrain(t, store)
	if !reflect.DeepEqual(ids, rawGolden(t, store)) {
		t.Fatal("retry changed identities")
	}
	third, err := collector.Run(t.Context(), options)
	if err != nil || third.Accepted != 0 || third.Collection.RawFacts != 0 {
		t.Fatalf("repeat recaptured data %+v %v", third, err)
	}
	if err := os.Remove(options.CollectorDBPath); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := collector.Run(t.Context(), options)
	if err != nil || rebuilt.Accepted == 0 {
		t.Fatalf("rebuild %+v %v", rebuilt, err)
	}
	rawDrain(t, store)
	if !reflect.DeepEqual(ids, rawGolden(t, store)) {
		t.Fatal("collector deletion changed facts")
	}
	if _, err := store.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	rawDrain(t, store)
	if !reflect.DeepEqual(ids, rawGolden(t, store)) {
		t.Fatal("reprocessing changed facts")
	}
	var body string
	if err := store.SQL().QueryRow("SELECT record_json FROM raw.evidence LIMIT 1").Scan(&body); err != nil {
		t.Fatal(err)
	}
	var record evidence.Record
	if json.Unmarshal([]byte(body), &record) != nil {
		t.Fatal("raw evidence not readable")
	}
}
