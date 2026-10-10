package storagecontract

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

// BenchmarkQueries measures the same real ingestion/processing/query fixture on
// both engines. Database creation and the 600-session seed are untimed.
func BenchmarkQueries(b *testing.B, d Dataset) {
	const sessions = 600
	var records []evidence.Record
	for i := range sessions {
		r := record(fmt.Sprintf("benchmark-%d", i))
		r.Context[0].Data = json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, i))
		records = append(records, r)
	}
	// Acceptance batches obey the production wire limit.
	const batchSize = 100
	for start := 0; start < len(records); start += batchSize {
		accept(b, d, batch(b, d, fmt.Sprintf("seed-%d", start), records[start:start+batchSize]...))
	}
	worker := dataengine.NewWorker()
	for range sessions {
		worked, err := worker.ProcessNext(b.Context(), d.Processing)
		if err != nil || !worked {
			b.Fatal("seed processing", worked, err)
		}
	}
	for _, tab := range []string{"sessions", "context", "tokens"} {
		b.Run(tab, func(b *testing.B) {
			q := query()
			q.Tab = tab
			if tab == "context" {
				q.Sort = "averageContext"
			}
			b.ReportAllocs()
			for b.Loop() {
				got, err := d.Queries.Dashboard(b.Context(), q, time.Now())
				if err != nil || got.FactCount != sessions || got.Summary.TotalTokens != sessions*24 {
					b.Fatal("benchmark lost accounting", got, err)
				}
			}
		})
	}
}
