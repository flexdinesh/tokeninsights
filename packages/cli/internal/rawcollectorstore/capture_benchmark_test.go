package rawcollectorstore

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

const captureBenchmarkRecords = 256

func captureBenchmarkRecord(index int) evidence.Record {
	return evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: int64(index + 1), Data: json.RawMessage(fmt.Sprintf(`{"type":"message","id":"message-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"model","usage":{"input":100,"output":20}}}`, index)), Context: []evidence.Context{{Ordinal: 0, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
}

func benchmarkCaptureRecords(b *testing.B, store *Store, records []evidence.Record) {
	b.Helper()
	capture, err := store.BeginCapture(b.Context())
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = capture.Rollback() }()
	for _, record := range records {
		if _, err := capture.Record(b.Context(), record, 1); err != nil {
			b.Fatal(err)
		}
	}
	if err := capture.Commit(); err != nil {
		b.Fatal(err)
	}
}

// Measures the current source-atomic writer; fixture/open/warm-up are excluded.
func BenchmarkCaptureRecords(b *testing.B) {
	for _, unchanged := range []bool{false, true} {
		name := "New"
		if unchanged {
			name = "Unchanged"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.StopTimer()
			records := make([]evidence.Record, captureBenchmarkRecords)
			for index := range records {
				records[index] = captureBenchmarkRecord(index)
			}
			for range b.N {
				store, err := Open(b.Context(), filepath.Join(b.TempDir(), "collector.sqlite"))
				if err != nil {
					b.Fatal(err)
				}
				if unchanged {
					benchmarkCaptureRecords(b, store, records)
				}
				b.StartTimer()
				benchmarkCaptureRecords(b, store, records)
				b.StopTimer()
				if err := store.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(captureBenchmarkRecords, "records/op")
		})
	}
}
