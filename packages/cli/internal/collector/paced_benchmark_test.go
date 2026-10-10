package collector_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

type pacedBackend struct {
	*measuredBackend
	first sync.Map
}

func (p *pacedBackend) PublishProjection(ctx context.Context, work dataengine.Work, projection evidence.Projection) (bool, error) {
	published, err := p.measuredBackend.PublishProjection(ctx, work, projection)
	if published && err == nil {
		p.first.LoadOrStore(work.DatasetID, time.Now())
	}
	return published, err
}

// BenchmarkPacedProcessing submits forty 256-message batches to one growing
// component and one message to another dataset during the stream. Capture,
// fixture creation and opening are untimed; acceptance and two real workers are
// timed. The ordinary ingestion matrix separately covers authenticated HTTP.
func BenchmarkPacedProcessing(b *testing.B) {
	const batches = 40
	const interval = 100 * time.Millisecond
	const coldBatch = 10
	b.ReportAllocs()
	b.StopTimer()
	metrics := make(map[string]float64)
	for range b.N {
		func() {
			store, err := datastore.OpenKind(b.Context(), filepath.Join(b.TempDir(), "data.sqlite"), datastore.KindHosted)
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			for _, dataset := range []string{"hot", "cold"} {
				if err := store.CreateDataset(b.Context(), dataset); err != nil {
					b.Fatal(err)
				}
			}
			hot, cold := store.ForDataset("hot"), store.ForDataset("cold")
			bodies := make([][]byte, batches)
			for index := range batches {
				var batch evidence.Batch
				if err := json.Unmarshal(acceptanceBody(b, hot, evidence.MaxEntries), &batch); err != nil {
					b.Fatal(err)
				}
				batch.StreamID = fmt.Sprint(index)
				for entry := range batch.Entries {
					ordinal := index*evidence.MaxEntries + entry
					batch.Entries[entry].Record.Ordinal = int64(ordinal + 2)
					batch.Entries[entry].Record.Data = json.RawMessage(fmt.Sprintf(`{"type":"message","id":"message-%d","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":100,"output":20}}}`, ordinal))
				}
				bodies[index], err = json.Marshal(batch)
				if err != nil {
					b.Fatal(err)
				}
			}
			coldBody := acceptanceBody(b, cold, 1)
			backend := &pacedBackend{measuredBackend: &measuredBackend{Store: store}}
			worker := dataengine.NewWorker()
			ctx, cancel := context.WithTimeout(b.Context(), time.Minute)
			joined := make(chan struct{})
			defer func() { cancel(); <-joined }()
			b.StartTimer()
			start := time.Now()
			go func() { defer close(joined); worker.Run(ctx, backend, func(err error) { b.Error(err) }) }()
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			var coldStart time.Time
			var submit time.Duration
			send := func(destination *datastore.Store, body []byte, count int64) {
				b.Helper()
				started := time.Now()
				response, err := (collector.DirectDelivery{Receiver: destination}).Submit(ctx, evidence.ProtocolVersion, body)
				submit += time.Since(started)
				if err != nil {
					b.Fatal(err)
				}
				var receipt evidence.Response
				if err := json.Unmarshal(response, &receipt); err != nil || receipt.Receipt.Accepted != count || receipt.Receipt.RequestHash != evidence.Hash(body) || receipt.Receipt.DatasetID != destination.DatasetID() {
					b.Fatal("receipt contract", receipt, err)
				}
				worker.Wake()
			}
			for index, body := range bodies {
				if index > 0 {
					select {
					case <-ticker.C:
					case <-ctx.Done():
						b.Fatal(ctx.Err())
					}
				}
				send(hot, body, evidence.MaxEntries)
				if index == coldBatch {
					coldStart = time.Now()
					send(cold, coldBody, 1)
				}
			}
			accepted := time.Now()
			for _, dataset := range []*datastore.Store{hot, cold} {
				if err := ingestionVisible(ctx, dataset); err != nil {
					b.Fatal(err)
				}
			}
			visible := time.Now()
			query := analytics.Query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Tab: "sessions", Quality: "confirmed", Sort: "total", Direction: "desc", Page: 1, PageSize: 200}
			dashboard, err := sqlanalytics.LoadDashboard(ctx, hot, query, time.Now())
			b.StopTimer()
			queried := time.Now()
			if err != nil || dashboard.Summary.TotalTokens != batches*evidence.MaxEntries*120 || dashboard.Summary.SessionCount != 1 {
				b.Fatal("dashboard totals", dashboard.Summary, err)
			}
			for _, dataset := range []*datastore.Store{hot, cold} {
				messages := int64(1)
				if dataset == hot {
					messages = batches * evidence.MaxEntries
				}
				var components [7]int64
				err := store.SQL().QueryRowContext(ctx, `SELECT COUNT(*),CAST(SUM(input_tokens) AS BIGINT),CAST(SUM(output_tokens) AS BIGINT),CAST(SUM(reasoning_tokens) AS BIGINT),CAST(SUM(cache_read_tokens) AS BIGINT),CAST(SUM(cache_write_tokens) AS BIGINT),CAST(SUM(total_tokens) AS BIGINT) FROM analytics_confirmed WHERE countable AND dataset_id=?`, dataset.DatasetID()).Scan(&components[0], &components[1], &components[2], &components[3], &components[4], &components[5], &components[6])
				if err != nil || components != [7]int64{messages, messages * 100, messages * 20, 0, 0, 0, messages * 120} {
					b.Fatal("component totals", components, err)
				}
			}
			// Visibility can be observed just before the worker records its metrics.
			cancel()
			<-joined
			hotFirst, hotOK := backend.first.Load("hot")
			coldFirst, coldOK := backend.first.Load("cold")
			if !hotOK || !coldOK {
				b.Fatal("missing publication")
			}
			hotTime, hotOK := hotFirst.(time.Time)
			coldTime, coldOK := coldFirst.(time.Time)
			if !hotOK || !coldOK || !coldTime.Before(accepted) {
				b.Fatal("independent dataset waited for global quiet")
			}
			metrics["first-publication-ns/op"] += float64(hotTime.Sub(start).Nanoseconds())
			metrics["independent-publication-ns/op"] += float64(coldTime.Sub(coldStart).Nanoseconds())
			metrics["acceptance-ns/op"] += float64(accepted.Sub(start).Nanoseconds())
			metrics["visibility-lag-ns/op"] += float64(visible.Sub(accepted).Nanoseconds())
			metrics["query-ns/op"] += float64(queried.Sub(visible).Nanoseconds())
			metrics["submit-ns/op"] += float64(submit.Nanoseconds())
			metrics["processed-records/op"] += float64(backend.records.Load())
			metrics["stale/op"] += float64(backend.stale.Load())
			metrics["published/op"] += float64(backend.published.Load())
		}()
	}
	for unit, value := range metrics {
		b.ReportMetric(value/float64(b.N), unit)
	}
}
