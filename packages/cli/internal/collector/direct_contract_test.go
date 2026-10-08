package collector_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

type lostDirectReply struct {
	collector.Delivery
	lost bool
}

func (d *lostDirectReply) Submit(ctx context.Context, protocol int, body []byte) ([]byte, error) {
	response, err := d.Delivery.Submit(ctx, protocol, body)
	if err == nil && !d.lost {
		d.lost = true
		return nil, errors.New("interrupted after acceptance")
	}
	return response, err
}

func TestDirectCollectorGoldenReplayAndRebuild(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "server.duckdb")
	store, err := datastore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	transport := &lostDirectReply{Delivery: collector.DirectDelivery{Receiver: store}}
	caps, err := transport.Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	options := collector.Options{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: path,
		Destination: &collector.Destination{URL: "http://local", DatabaseID: caps.DatabaseID, DatasetID: caps.DatasetID, Local: true, Transport: transport},
		SyncOptions: pipeline.SyncOptions{SourceDir: acceptanceSources(t), Harnesses: pipeline.SupportedHarnesses}}
	first, err := collector.Run(t.Context(), options)
	if err == nil || first.CollectionError != nil || first.Pending == 0 {
		t.Fatalf("lost reply acknowledged: %+v %v", first, err)
	}
	rawDrain(t, store)
	ids := rawGolden(t, store)
	second, err := collector.Run(t.Context(), options)
	if err != nil || second.Pending != 0 || second.Accepted == 0 {
		t.Fatalf("retry: %+v %v", second, err)
	}
	rawDrain(t, store)
	if !reflect.DeepEqual(ids, rawGolden(t, store)) {
		t.Fatal("retry changed native identities")
	}
	if err := os.Remove(options.CollectorDBPath); err != nil {
		t.Fatal(err)
	}
	if _, err := collector.Run(t.Context(), options); err != nil {
		t.Fatal(err)
	}
	rawDrain(t, store)
	if !reflect.DeepEqual(ids, rawGolden(t, store)) {
		t.Fatal("collector rebuild changed native identities")
	}
	if _, err := store.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	rawDrain(t, store)
	if !reflect.DeepEqual(ids, rawGolden(t, store)) {
		t.Fatal("reprocessing changed native identities")
	}
}

func TestDirectAndHTTPShareAcceptanceReceiptsAndRejections(t *testing.T) {
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "data.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	remote := httptest.NewServer(store.Handler())
	defer remote.Close()
	direct := collector.DirectDelivery{Receiver: store}
	network := collector.HTTPDelivery{URL: remote.URL}
	caps, err := direct.Capabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	networkCaps, err := network.Capabilities(t.Context())
	if err != nil || networkCaps != caps {
		t.Fatalf("capabilities differ: %+v %v", networkCaps, err)
	}
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2,
		Data:    json.RawMessage(`{"type":"message","id":"message","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":100,"output":20}}}`),
		Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: caps.DatabaseID, DatasetID: caps.DatasetID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 2,
		Entries: []evidence.Entry{{Sequence: 1, Record: record}, {Sequence: 2, Record: record}}}
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	first, err := direct.Submit(t.Context(), evidence.ProtocolVersion, body)
	if err != nil {
		t.Fatal(err)
	}
	second, err := network.Submit(t.Context(), evidence.ProtocolVersion, body)
	if err != nil {
		t.Fatal(err)
	}
	var a, b evidence.Response
	if json.Unmarshal(first, &a) != nil || json.Unmarshal(second, &b) != nil || !reflect.DeepEqual(a, b) || a.Receipt.Accepted != 2 {
		t.Fatal("receipt/mappings differ")
	}
	rawDrain(t, store)
	left, err := direct.Receipt(t.Context(), "stream", "batch")
	if err != nil {
		t.Fatal(err)
	}
	right, err := network.Receipt(t.Context(), "stream", "batch")
	if err != nil || !reflect.DeepEqual(left, right) || left.Receipt != a.Receipt || left.Processing.Pending != 0 {
		t.Fatalf("terminal receipt differs: %+v %v", right, err)
	}
	var total int64
	if err := store.SQL().QueryRow("SELECT CAST(SUM(total_tokens) AS BIGINT) FROM analytics.confirmed").Scan(&total); err != nil || total != 120 {
		t.Fatalf("duplicates counted: %d %v", total, err)
	}
	batch.DatabaseID = "other-database"
	wrong, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		body     []byte
		protocol int
		code     string
	}{
		{"malformed", []byte(`{"private":"never persist"}`), 3, "invalid_request"},
		{"database", wrong, 3, "database_mismatch"},
		{"version", body, 2, "incompatible"},
		{"oversized", make([]byte, evidence.MaxBodyBytes+1), 3, "body_limit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, delivery := range []collector.Delivery{direct, network} {
				_, err := delivery.Submit(t.Context(), test.protocol, test.body)
				var stage *collector.StageError
				if !errors.As(err, &stage) || stage.Code != test.code {
					t.Fatalf("rejection differs: %v", err)
				}
			}
		})
	}
	// No HTTP client is consulted by the direct adapter, including capability
	// preflight. URL is a durable local identity, not an endpoint to contact.
	destination := &collector.Destination{URL: "http://local", Transport: direct, Client: &http.Client{Transport: rejectNetwork{t}}}
	if _, err := collector.NegotiateCapabilities(t.Context(), destination, ""); err != nil {
		t.Fatal(err)
	}
}

type rejectNetwork struct{ t *testing.T }

func (r rejectNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	r.t.Error("direct delivery used HTTP")
	return nil, errors.New("unexpected HTTP")
}
