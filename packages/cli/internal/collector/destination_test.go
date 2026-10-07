package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
)

func TestInjectedDestinationRetryRotationAndAccountSwitch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	store, err := rawcollectorstore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := store.BeginCapture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, err = capture.Record(t.Context(), evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := capture.Commit(); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	var requests [][]byte
	var headers []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/ingestion/batches" || r.Method != http.MethodPost {
			t.Error("unexpected negotiation", r.URL.Path)
			w.WriteHeader(500)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		batch, err := evidence.DecodeBatch(body)
		if err != nil {
			t.Error(err)
			return
		}
		requests = append(requests, body)
		headers = append(headers, r.Header.Get("Authorization"))
		if len(requests) == 1 {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(evidence.Response{Receipt: evidence.Receipt{DatabaseID: batch.DatabaseID, DatasetID: batch.DatasetID, StreamID: batch.StreamID, BatchID: batch.BatchID, RequestHash: evidence.Hash(body), FromSequence: batch.FromSequence, ToSequence: batch.ToSequence, Accepted: int64(len(batch.Entries)), AcceptedAtMs: 1, InputRevision: 1}})
	}))
	defer server.Close()
	destination := Destination{URL: server.URL, DatabaseID: "database", DatasetID: "user-a", Client: server.Client()}
	options := Options{CollectorDBPath: path, PublishOnly: true, Destination: &destination, Token: "old-token", EnsureLocal: func(context.Context) (string, error) { t.Fatal("destination started service"); return "", nil }}
	if first, err := Run(t.Context(), options); err == nil || !first.PendingKnown || first.Pending != 1 {
		t.Fatal(first, err)
	}
	options.Token = "rotated-token"
	if second, err := Run(t.Context(), options); err != nil || second.Pending != 0 || second.Accepted != 1 {
		t.Fatal(second, err)
	}
	if len(requests) != 2 || !bytes.Equal(requests[0], requests[1]) {
		t.Fatal("lost receipt changed retry bytes")
	}
	if headers[0] != "Bearer old-token" || headers[1] != "Bearer rotated-token" {
		t.Fatal(headers)
	}
	if again, err := Run(t.Context(), options); err != nil || again.Accepted != 0 {
		t.Fatal(again, err)
	}
	destination.DatasetID = "user-b"
	options.Token = "second-user-token"
	if other, err := Run(t.Context(), options); err != nil || other.Accepted != 1 {
		t.Fatal(other, err)
	}
	if len(requests) != 3 {
		t.Fatal(len(requests))
	}
	foreign, err := evidence.DecodeBatch(requests[2])
	if err != nil || foreign.DatasetID != "user-b" || foreign.FromSequence != 1 {
		t.Fatal(foreign, err)
	}
}

func TestRetainedV2BatchPrecedesNewV3WithoutChangingBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	store, err := rawcollectorstore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}
	capture, err := store.BeginCapture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, ordinal := range []int64{1, 2} {
		record.Ordinal = ordinal
		if _, err := capture.Record(t.Context(), record, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := capture.Commit(); err != nil {
		t.Fatal(err)
	}
	record.Ordinal = 1
	var stream string
	if err := store.DB.QueryRow("SELECT stream_id FROM evidence_state WHERE id=1").Scan(&stream); err != nil {
		t.Fatal(err)
	}
	batch := evidence.Batch{ProtocolVersion: 2, ExtractorVersion: 1, DatabaseID: "database", StreamID: stream, BatchID: "saved-batch", FromSequence: 1, ToSequence: 1, Entries: []evidence.Entry{{Sequence: 1, Record: record}}}
	original, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var routes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routes = append(routes, r.URL.Path)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		got, err := evidence.DecodeBatch(body)
		if err != nil {
			t.Error(err)
			return
		}
		if len(routes) == 1 && !bytes.Equal(body, original) {
			t.Error("retained v2 request bytes changed")
		}
		if len(routes) == 2 && got.ProtocolVersion != 3 {
			t.Error("new request stayed on v2")
		}
		_ = json.NewEncoder(w).Encode(evidence.Response{Receipt: evidence.Receipt{DatabaseID: got.DatabaseID, DatasetID: got.EffectiveDatasetID(), StreamID: got.StreamID, BatchID: got.BatchID, RequestHash: evidence.Hash(body), FromSequence: got.FromSequence, ToSequence: got.ToSequence, Accepted: int64(len(got.Entries)), AcceptedAtMs: 1, InputRevision: 1}})
	}))
	defer server.Close()
	if err := store.Bind(t.Context(), "old-binding", server.URL, "database"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec("INSERT INTO evidence_batches(batch_id,destination_id,stream_id,database_id,dataset_id,protocol_version,first_sequence,last_sequence,request_hash,request_bytes) VALUES(?,?,?,?,?,?,?,?,?,?)", batch.BatchID, "old-binding", stream, "database", "default", 2, 1, 1, evidence.Hash(original), original); err != nil {
		t.Fatal(err)
	}
	options := Options{CollectorDBPath: path, PublishOnly: true, Destination: &Destination{URL: server.URL, DatabaseID: "database", DatasetID: "default", Client: server.Client()}}
	result, err := Run(t.Context(), options)
	if err != nil || result.Accepted != 2 || result.Batches != 2 || result.Pending != 0 {
		t.Fatal(result, err)
	}
	if len(routes) != 2 || routes[0] != "/api/v2/ingestion/batches" || routes[1] != "/api/v3/ingestion/batches" {
		t.Fatal(routes)
	}
}
