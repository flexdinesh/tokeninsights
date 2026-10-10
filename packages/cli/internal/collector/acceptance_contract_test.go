package collector_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestionhttp"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func acceptanceStore(t testing.TB) *datastore.Store {
	t.Helper()
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "data.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func acceptanceBody(t testing.TB, store *datastore.Store, entries int) []byte {
	t.Helper()
	caps, err := store.RawCapabilities(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion,
		DatabaseID: caps.DatabaseID, DatasetID: caps.DatasetID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: int64(entries)}
	for index := range entries {
		batch.Entries = append(batch.Entries, evidence.Entry{Sequence: int64(index + 1), Record: evidence.Record{
			Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: int64(index + 2),
			Data:    json.RawMessage(fmt.Sprintf(`{"type":"message","id":"message-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":100,"output":20}}}`, index)),
			Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}},
		}})
	}
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestAcceptanceValidationPreservesRejectionBoundaries(t *testing.T) {
	store := acceptanceStore(t)
	body := acceptanceBody(t, store, 1)
	change := func(old, replacement string) []byte {
		return bytes.Replace(body, []byte(old), []byte(replacement), 1)
	}
	before, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, code string
		body       []byte
		protocol   int
		status     int
	}{
		{"malformed", "invalid_request", []byte(`{`), 3, 400},
		{"unknown envelope", "invalid_request", change(`"streamId"`, `"private":"secret","streamId"`), 3, 400},
		{"duplicate envelope", "invalid_request", change(`"protocolVersion":3`, `"protocolVersion":3,"protocolVersion":3`), 3, 400},
		{"duplicate native key", "invalid_request", change(`"input":100`, `"input":100,"input":100`), 3, 400},
		{"private native field", "invalid_request", change(`"role":"assistant"`, `"role":"assistant","content":"secret"`), 3, 400},
		{"trailing value", "invalid_request", append(bytes.Clone(body), []byte(` {}`)...), 3, 400},
		{"invalid UTF-8", "invalid_request", change(`"source"`, "\"\xff\""), 3, 400},
		{"fractional sequence", "invalid_request", change(`"sequence":1`, `"sequence":1.5`), 3, 400},
		{"unsafe ordinal", "invalid_request", change(`"ordinal":2`, `"ordinal":9007199254740992`), 3, 400},
		{"missing dataset", "invalid_request", change(`"datasetId":"default"`, `"datasetId":""`), 3, 400},
		{"raw protocol", "incompatible", change(`"protocolVersion":3`, `"protocolVersion":2`), 3, 422},
		{"extractor", "incompatible", change(`"extractorVersion":1`, `"extractorVersion":2`), 3, 422},
		{"requested protocol", "incompatible", body, 2, 422},
		{"invalid before requested protocol", "invalid_request", []byte(`{`), 2, 400},
		{"body bound", "body_limit", make([]byte, evidence.MaxBodyBytes+1), 3, 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := (collector.DirectDelivery{Receiver: store}).Submit(t.Context(), test.protocol, test.body)
			var failure *collector.StageError
			if !errors.As(err, &failure) || failure.Stage != "validation" || failure.Code != test.code {
				t.Fatal("direct rejection", err)
			}
			request := httptest.NewRequest(http.MethodPost, ingestionhttp.IngestionPrefix+"batches", bytes.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			ingestionhttp.Handler(store, ingestionhttp.IngestionPrefix, test.protocol).ServeHTTP(response, request)
			var rejected publication.ErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &rejected); err != nil || response.Code != test.status || rejected.Stage != "validation" || rejected.Code != test.code {
				t.Fatal("HTTP rejection", response.Code, rejected, err)
			}
			after, err := store.Metadata(t.Context())
			var writes int
			countErr := store.SQL().QueryRow(`SELECT (SELECT COUNT(*) FROM raw_evidence)+(SELECT COUNT(*) FROM ingestion_batches)+(SELECT COUNT(*) FROM ingestion_items)+(SELECT COUNT(*) FROM ingestion_batch_items)+(SELECT COUNT(*) FROM processing_scopes)+(SELECT COUNT(*) FROM processing_dependencies)`).Scan(&writes)
			if err != nil || countErr != nil || writes != 0 || !reflect.DeepEqual(before, after) {
				t.Fatal("rejection mutated storage", writes, err, countErr)
			}
		})
	}
	// Exhaust real admission slots: malformed bytes must still fail at admission.
	for {
		release, allowed := store.AcquireAdmission()
		if !allowed {
			break
		}
		defer release()
	}
	direct := collector.DirectDelivery{Receiver: store}
	remote := httptest.NewServer(ingestionhttp.Handler(store, ingestionhttp.IngestionPrefix, evidence.ProtocolVersion))
	defer remote.Close()
	for _, delivery := range []collector.Delivery{direct, collector.HTTPDelivery{URL: remote.URL}} {
		_, err := delivery.Submit(t.Context(), evidence.ProtocolVersion, []byte(`{`))
		var failure *collector.StageError
		if !errors.As(err, &failure) || failure.Stage != "admission" || failure.Code != "busy" {
			t.Fatal("validation bypassed admission", err)
		}
	}
}

func TestAcceptancePreservesExactBytesAndLargeNativeNumbers(t *testing.T) {
	for _, mode := range []string{"Direct", "HTTP"} {
		t.Run(mode, func(t *testing.T) {
			store := acceptanceStore(t)
			var delivery collector.Delivery = collector.DirectDelivery{Receiver: store}
			if mode == "HTTP" {
				remote := httptest.NewServer(ingestionhttp.Handler(store, ingestionhttp.IngestionPrefix, evidence.ProtocolVersion))
				defer remote.Close()
				delivery = collector.HTTPDelivery{URL: remote.URL}
			}
			// Unsafe counters are accepted as raw evidence for later diagnostics, not rounded.
			body := append([]byte("\n "), acceptanceBody(t, store, 1)...)
			body = bytes.Replace(body, []byte(`"input":100`), []byte(`"input":9007199254740993`), 1)
			var first evidence.Response
			for attempt := range 2 {
				response, err := delivery.Submit(t.Context(), evidence.ProtocolVersion, body)
				if err != nil {
					t.Fatal(err)
				}
				var received evidence.Response
				if err := json.Unmarshal(response, &received); err != nil || received.Receipt.RequestHash != evidence.Hash(body) || received.Receipt.Accepted != 1 {
					t.Fatal("receipt changed bytes", received, err)
				}
				if attempt == 0 {
					first = received
				} else if first.Receipt != received.Receipt {
					t.Fatal("retry changed receipt")
				}
			}
			var saved []byte
			var record string
			if err := store.SQL().QueryRow("SELECT request_bytes FROM ingestion_batches").Scan(&saved); err != nil || !bytes.Equal(saved, body) {
				t.Fatal("request bytes changed", err)
			}
			if err := store.SQL().QueryRow("SELECT record_json FROM raw_evidence").Scan(&record); err != nil || !strings.Contains(record, "9007199254740993") {
				t.Fatal("native numeric precision lost", err)
			}
			_, err := delivery.Submit(t.Context(), evidence.ProtocolVersion, append(bytes.Clone(body), ' '))
			var failure *collector.StageError
			if !errors.As(err, &failure) || failure.Stage != "admission" || failure.Code != "batch_conflict" {
				t.Fatal("changed retry bytes accepted", err)
			}
		})
	}
}
