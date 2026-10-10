package collector_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestionhttp"
)

// BenchmarkReceiverAcceptance measures adapters plus durable acceptance, without
// capture or workers. HTTP is the real handler over loopback; the ingestion matrix
// separately covers hosted authentication/admission and concurrent processing.
func BenchmarkReceiverAcceptance(b *testing.B) {
	for _, mode := range []string{"Direct", "HTTP"} {
		for _, operation := range []string{"New", "Replay"} {
			b.Run(mode+"/"+operation, func(b *testing.B) {
				b.ReportAllocs()
				b.StopTimer()
				for range b.N {
					func() {
						store := acceptanceStore(b)
						defer func() { _ = store.Close() }()
						body := acceptanceBody(b, store, evidence.MaxEntries)
						var delivery collector.Delivery = collector.DirectDelivery{Receiver: store}
						if mode == "HTTP" {
							remote := httptest.NewServer(ingestionhttp.Handler(store, ingestionhttp.IngestionPrefix, evidence.ProtocolVersion))
							defer remote.Close()
							delivery = collector.HTTPDelivery{URL: remote.URL, Client: remote.Client()}
						}
						var original evidence.Response
						if operation == "Replay" {
							response, err := delivery.Submit(b.Context(), evidence.ProtocolVersion, body)
							if err != nil {
								b.Fatal(err)
							}
							if err := json.Unmarshal(response, &original); err != nil {
								b.Fatal(err)
							}
						}
						b.StartTimer()
						response, err := delivery.Submit(b.Context(), evidence.ProtocolVersion, body)
						b.StopTimer()
						if err != nil {
							b.Fatal(err)
						}
						var received evidence.Response
						if err := json.Unmarshal(response, &received); err != nil || received.Receipt.Accepted != evidence.MaxEntries || received.Receipt.RequestHash != evidence.Hash(body) || received.Processing.Pending != evidence.MaxEntries {
							b.Fatal("acceptance contract", received, err)
						}
						if operation == "Replay" && received.Receipt != original.Receipt {
							b.Fatal("replay changed receipt")
						}
						var facts, batches int
						if err := store.SQL().QueryRow("SELECT (SELECT COUNT(*) FROM raw_evidence),(SELECT COUNT(*) FROM ingestion_batches)").Scan(&facts, &batches); err != nil || facts != evidence.MaxEntries || batches != 1 {
							b.Fatal("acceptance changed evidence or receipts", facts, batches, err)
						}
					}()
				}
			})
		}
	}
}
