package collector

import (
	"bytes"
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
	destination := Destination{Identity: server.URL + "/", DatabaseID: "database", DatasetID: "user-a", Transport: &HTTPDelivery{URL: server.URL, Token: "old-token", Client: server.Client()}}
	options := Options{CollectorDBPath: path, PublishOnly: true, Destination: &destination}
	if first, err := Run(t.Context(), options); err == nil || !first.PendingKnown || first.Pending != 1 {
		t.Fatal(first, err)
	}
	destination.Transport = HTTPDelivery{URL: server.URL, Token: "rotated-token", Client: server.Client()}
	destination.Identity = server.URL
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
	destination.Transport = HTTPDelivery{URL: server.URL, Token: "second-user-token", Client: server.Client()}
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
