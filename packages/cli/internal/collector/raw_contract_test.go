package collector_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestionhttp"
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

func rawGolden(t testing.TB, store *datastore.Store) []string {
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
	handler := ingestionhttp.Handler(store, ingestionhttp.IngestionPrefix, evidence.ProtocolVersion)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == ingestionhttp.IngestionPrefix+"batches" && lose {
			lose = false
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := store.Accept(r.Context(), evidence.ProtocolVersion, body); err != nil {
				t.Error(err)
			}
			http.Error(w, "lost acknowledgement", http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	metadata, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	options := collector.Options{CollectorDBPath: filepath.Join(root, "collector.sqlite"), Destination: &collector.Destination{Identity: httpServer.URL, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, Transport: collector.HTTPDelivery{URL: httpServer.URL}}, SyncOptions: pipeline.SyncOptions{SourceDir: acceptanceSources(t), Harnesses: pipeline.SupportedHarnesses}}
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
	if err := os.RemoveAll(options.SyncOptions.SourceDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(options.CollectorDBPath); err != nil {
		t.Fatal(err)
	}
	if result, err := collector.Run(t.Context(), options); err != nil || result.Batches != 0 {
		t.Fatalf("missing sources submitted changes: %+v %v", result, err)
	}
	if !reflect.DeepEqual(ids, rawGolden(t, store)) {
		t.Fatal("missing sources removed accepted history")
	}
}

func TestRawCollectorKeepsPrivateContentOutOfTransportAndStorage(t *testing.T) {
	sources := acceptanceSources(t)
	artifact := filepath.Join(sources, "pi", "project", "main.jsonl")
	body, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	private := []byte(`"role":"assistant","content":[{"type":"text","text":"SYNTHETIC_PRIVATE_MARKER"}],"toolOutput":"SYNTHETIC_TOOL_MARKER","request_headers":{"authorization":"SYNTHETIC_SECRET_MARKER"},`)
	updated := bytes.ReplaceAll(body, []byte(`"role":"assistant",`), private)
	if bytes.Equal(updated, body) {
		t.Fatal("privacy fixture did not include an assistant message")
	}
	if err := os.WriteFile(artifact, updated, 0o600); err != nil {
		t.Fatal(err)
	}
	assertPrivate := func(where string, body []byte) {
		for _, marker := range []string{"SYNTHETIC_PRIVATE_MARKER", "SYNTHETIC_TOOL_MARKER", "SYNTHETIC_SECRET_MARKER", sources} {
			if bytes.Contains(body, []byte(marker)) {
				t.Errorf("private source content reached %s", where)
			}
		}
	}
	root := t.TempDir()
	path := filepath.Join(root, "server.duckdb")
	store, err := datastore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	handler := ingestionhttp.Handler(store, ingestionhttp.IngestionPrefix, evidence.ProtocolVersion)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			assertPrivate("HTTP request", body)
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		handler.ServeHTTP(w, r)
	}))
	defer remote.Close()
	caps, err := store.RawCapabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	collectorPath := filepath.Join(root, "collector.sqlite")
	options := collector.Options{CollectorDBPath: collectorPath,
		Destination: &collector.Destination{Identity: remote.URL, DatabaseID: caps.DatabaseID, DatasetID: caps.DatasetID, Transport: collector.HTTPDelivery{URL: remote.URL}},
		SyncOptions: pipeline.SyncOptions{SourceDir: sources, Harnesses: pipeline.SupportedHarnesses}}
	if _, err := collector.Run(t.Context(), options); err != nil {
		t.Fatal(err)
	}
	rawDrain(t, store)
	rawGolden(t, store)
	if _, err := store.SQL().ExecContext(t.Context(), "CHECKPOINT"); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{collectorPath, collectorPath + "-wal", path, path + ".wal"} {
		body, err := os.ReadFile(file)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		assertPrivate(filepath.Base(file), body)
	}
}
