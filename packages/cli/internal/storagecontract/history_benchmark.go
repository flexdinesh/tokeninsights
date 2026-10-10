package storagecontract

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/dataengine"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

// BenchmarkFixture keeps physical opening and size accounting in adapters.
// SizeBytes measures adapter-defined footprint, not portable capacity.
type BenchmarkFixture struct {
	Tokens    Tokens
	SizeBytes func() (int64, error)
}

// BenchmarkSmoke selects bounded fixture sizes for executable smoke checks.
func BenchmarkSmoke() bool { return os.Getenv("TOKENINSIGHTS_BENCHMARK_SMOKE") == "1" }

type historyShape struct {
	name                                      string
	datasets, sessions, messages, generations int
}

// BenchmarkStorage seeds via real acceptance and processing. Smoke exercises
// every operation with small history; correctness has no timing threshold.
func BenchmarkStorage(b *testing.B, factory func(*testing.B) BenchmarkFixture) {
	shapes := []historyShape{
		{"History1K", 1, 10, 100, 1}, {"History10K", 1, 100, 100, 1},
		{"History100K", 1, 1000, 100, 1}, {"Tenants10", 10, 100, 100, 1},
		{"Retained3", 1, 100, 100, 3},
	}
	if BenchmarkSmoke() {
		shapes = []historyShape{{"Smoke", 2, 2, 10, 3}}
	}
	for _, shape := range shapes {
		b.Run(shape.name, func(b *testing.B) {
			f := factory(b)
			tokens := f.Tokens
			initial := int64(shape.sessions * shape.messages)
			for tenant := range shape.datasets {
				id := fmt.Sprintf("tenant-%d", tenant)
				if err := tokens.EnsureDataset(b.Context(), id); err != nil {
					b.Fatal(err)
				}
				d := tokens.Dataset(id)
				for start := 0; start < int(initial); start += evidence.MaxEntries {
					end := min(start+evidence.MaxEntries, int(initial))
					accept(b, d, batch(b, d, fmt.Sprintf("seed-%d", start), historyRecords(start, end, shape.sessions)...))
				}
				drainHistory(b, d)
				for generation := 1; generation < shape.generations; generation++ {
					if _, err := d.Reprocess(b.Context()); err != nil {
						b.Fatal(err)
					}
					drainHistory(b, d)
				}
				got := historyDashboard(b, d, query())
				checkHistory(b, got, initial, int64(shape.sessions), id)
				if got.DatasetID != id || got.Generation != int64(shape.generations) || got.Pending != 0 {
					b.Fatal("incomplete seed", got)
				}
			}
			size := func(b *testing.B) int64 {
				b.Helper()
				n, err := f.SizeBytes()
				if err != nil {
					b.Fatal(err)
				}
				return n
			}
			baselineBytes := size(b)
			current := initial
			for _, tab := range []string{"sessions", "context", "tokens"} {
				b.Run("Query-"+tab, func(b *testing.B) {
					d := tokens.Dataset("tenant-0")
					q := query()
					q.Tab = tab
					if tab == "context" {
						q.Sort = "averageContext"
					}
					b.ReportAllocs()
					for b.Loop() {
						checkHistory(b, historyDashboard(b, d, q), current, int64(shape.sessions), "tenant-0")
					}
					b.ReportMetric(float64(baselineBytes), "store-bytes")
				})
			}
			b.Run("Reopen", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					tokens = tokens.Reopen()
					b.StopTimer()
					got := historyDashboard(b, tokens.Dataset("tenant-0"), query())
					checkHistory(b, got, current, int64(shape.sessions), "tenant-0")
					if got.Pending != 0 || got.Generation != int64(shape.generations) {
						b.Fatal("reopen lost generation", got)
					}
					b.StartTimer()
				}
			})
			// Iterations grow history deliberately. Use fixed counts for comparisons.
			b.Run("Append", func(b *testing.B) {
				d := tokens.Dataset("tenant-0")
				beforeBytes := size(b)
				b.ReportAllocs()
				var acceptance, visibility time.Duration
				for b.Loop() {
					b.StopTimer()
					body := batch(b, d, fmt.Sprintf("append-%d", current), historyRecords(int(current), int(current)+1, shape.sessions)...)
					b.StartTimer()
					started := time.Now()
					receipt := accept(b, d, body)
					accepted := time.Now()
					drainHistory(b, d)
					current++
					checkHistory(b, historyDashboard(b, d, query()), current, int64(shape.sessions), "tenant-0")
					visibility += time.Since(accepted)
					acceptance += accepted.Sub(started)
					b.StopTimer()
					assertHistoryReceipt(b, d, receipt)
					b.StartTimer()
				}
				b.ReportMetric(float64(acceptance.Nanoseconds())/float64(b.N), "accept-ns/op")
				b.ReportMetric(float64(visibility.Nanoseconds())/float64(b.N), "visible-ns/op")
				b.ReportMetric(float64(size(b)-beforeBytes), "growth-bytes")
				b.ReportMetric(float64(size(b)), "store-bytes")
			})
			b.Run("QueriesDuringProcessing", func(b *testing.B) {
				d := tokens.Dataset("tenant-0")
				beforeBytes := size(b)
				ctx, cancel := context.WithTimeout(b.Context(), 5*time.Minute)
				worker := dataengine.NewWorker()
				done := make(chan struct{})
				failures := make(chan error, 1)
				go func() {
					defer close(done)
					worker.Run(ctx, d.Processing, func(err error) {
						select {
						case failures <- err:
						default:
						}
					})
				}()
				defer func() { cancel(); <-done }()
				var latencies []int64
				var acceptance, visibility time.Duration
				b.ReportAllocs()
				for b.Loop() {
					b.StopTimer()
					end := current + evidence.MaxEntries
					body := batch(b, d, fmt.Sprintf("concurrent-%d", current), historyRecords(int(current), int(end), shape.sessions)...)
					b.StartTimer()
					started := time.Now()
					receipt := accept(b, d, body)
					accepted := time.Now()
					worker.Wake()
					previous := current
					for {
						start := time.Now()
						got, err := d.Queries.Dashboard(ctx, query(), time.Now())
						latencies = append(latencies, time.Since(start).Nanoseconds())
						if err != nil {
							b.Fatal(err)
						}
						if got.FactCount < previous || got.FactCount > end {
							b.Fatal("nonmonotonic snapshot", got)
						}
						checkHistory(b, got, got.FactCount, int64(shape.sessions), "tenant-0")
						previous = got.FactCount
						select {
						case err := <-failures:
							b.Fatal(err)
						default:
						}
						if got.Pending == 0 && got.FactCount == end {
							break
						}
					}
					acceptance += accepted.Sub(started)
					visibility += time.Since(accepted)
					current = end
					b.StopTimer()
					assertHistoryReceipt(b, d, receipt)
					b.StartTimer()
				}
				slices.Sort(latencies)
				b.ReportMetric(float64(latencies[len(latencies)/2]), "query-p50-ns")
				b.ReportMetric(float64(latencies[(len(latencies)-1)*95/100]), "query-p95-ns")
				b.ReportMetric(float64(len(latencies))/float64(b.N), "queries/op")
				b.ReportMetric(float64(acceptance.Nanoseconds())/float64(b.N), "accept-ns/op")
				b.ReportMetric(float64(visibility.Nanoseconds())/float64(b.N), "visible-ns/op")
				b.ReportMetric(float64(size(b)-beforeBytes), "growth-bytes")
				b.ReportMetric(float64(size(b)), "store-bytes")
			})
			if shape.datasets > 1 {
				checkHistory(b, historyDashboard(b, tokens.Dataset("tenant-1"), query()), initial, int64(shape.sessions), "tenant-1")
			}
		})
	}
}

func historyRecords(start, end, sessions int) []evidence.Record {
	records := make([]evidence.Record, 0, end-start)
	const dayMilliseconds = int64(24 * time.Hour / time.Millisecond)
	const startMilliseconds int64 = 1700000000000
	const historyDays, models = 90, 4
	for i := start; i < end; i++ {
		r := record(fmt.Sprintf("message-%d", i))
		r.Ordinal = int64(i + 2)
		r.Context[0].Data = json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, i%sessions))
		r.Data = json.RawMessage(fmt.Sprintf(`{"type":"message","id":"message-%d","message":{"role":"assistant","timestamp":%d,"provider":"openai","model":"model-%d","usage":{"input":10,"output":7,"reasoning":2,"cacheRead":3,"cacheWrite":4,"totalTokens":24}}}`,
			i, startMilliseconds+int64(i%historyDays)*dayMilliseconds, i%models))
		records = append(records, r)
	}
	return records
}

func drainHistory(b *testing.B, d Dataset) {
	b.Helper()
	ctx, cancel := context.WithTimeout(b.Context(), 5*time.Minute)
	defer cancel()
	worker := dataengine.NewWorker()
	for {
		worked, err := worker.ProcessNext(ctx, d.Processing)
		if err != nil {
			b.Fatal(err)
		}
		if !worked {
			return
		}
	}
}

func historyDashboard(b *testing.B, d Dataset, q analytics.Query) analytics.Dashboard {
	b.Helper()
	got, err := d.Queries.Dashboard(b.Context(), q, time.Now())
	if err != nil {
		b.Fatal(err)
	}
	return got
}

func checkHistory(b *testing.B, got analytics.Dashboard, facts, sessions int64, datasetID string) {
	b.Helper()
	s := got.Summary
	if got.DatasetID != datasetID {
		b.Fatal("wrong dataset", got.DatasetID, datasetID)
	}
	if got.FactCount != facts || s.SessionCount != sessions ||
		s.InputTokens != facts*10 || s.OutputTokens != facts*5 || s.ReasoningTokens != facts*2 ||
		s.CacheReadTokens != facts*3 || s.CacheWriteTokens != facts*4 || s.TotalTokens != facts*24 {
		b.Fatal("history accounting changed", got)
	}
}

func assertHistoryReceipt(b *testing.B, d Dataset, receipt evidence.Receipt) {
	b.Helper()
	got, err := d.Receiver.Receipt(b.Context(), receipt.StreamID, receipt.BatchID)
	if err != nil || got.Receipt != receipt || got.Processing.Pending != 0 {
		b.Fatal("publication changed receipt or left pending work", got, err)
	}
}
