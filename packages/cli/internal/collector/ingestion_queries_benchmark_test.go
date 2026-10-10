package collector_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

// Keep saved sessions queryable while adding 9,950 messages to them. This uses
// the real local runtime; no timing thresholds or synthetic query adapters.
func BenchmarkIngestionSavedQueries(b *testing.B) {
	const sessions, messages = 50, 200
	b.ReportAllocs()
	b.StopTimer()
	var queryTime, maxQuery time.Duration
	var queryCount int64
	for range b.N {
		func() {
			root := b.TempDir()
			sources := ingestionSources(b, sessions, 1)
			collectorPath, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
			runtime, err := localruntime.Open(b.Context(), collectorPath, dataPath)
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = runtime.Close() }()
			options := collector.Options{CollectorDBPath: collectorPath, ServerDBPath: dataPath, Destination: runtime.Destination,
				SyncOptions: pipeline.SyncOptions{SourceDir: sources, Harnesses: []pipeline.Harness{pipeline.HarnessPi}}}
			startupBenchmarkRun(b, runtime, options, sessions*startupBenchmarkTokens, sessions)
			for session := range sessions {
				file, err := os.OpenFile(filepath.Join(sources, fmt.Sprintf("session-%d.jsonl", session)), os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					b.Fatal(err)
				}
				for message := 1; message < messages; message++ {
					if _, err := file.WriteString(startupBenchmarkMessage(session, message)); err != nil {
						_ = file.Close()
						b.Fatal(err)
					}
				}
				if err := file.Close(); err != nil {
					b.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(b.Context(), 5*time.Minute)
			defer cancel()
			done := make(chan error, 1)
			b.StartTimer()
			go func() {
				_, err := collector.Run(ctx, options)
				if err == nil {
					err = runtime.WaitVisible(ctx)
				}
				done <- err
			}()
			query := analytics.Query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Tab: "sessions", Quality: "confirmed", Sort: "total", Direction: "desc", Page: 1, PageSize: 200}
			const want = sessions * messages * startupBenchmarkTokens
			previous := int64(sessions * startupBenchmarkTokens)
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				start := time.Now()
				dashboard, err := sqlanalytics.LoadDashboard(ctx, runtime.Store, query, time.Now())
				elapsed := time.Since(start)
				queryTime += elapsed
				maxQuery = max(maxQuery, elapsed)
				queryCount++
				if err != nil || dashboard.Summary.SessionCount != sessions || dashboard.Summary.TotalTokens < previous || dashboard.Summary.TotalTokens > want {
					cancel()
					<-done
					b.Fatalf("saved snapshot: %+v %v", dashboard, err)
				}
				previous = dashboard.Summary.TotalTokens
				select {
				case err := <-done:
					b.StopTimer()
					if err != nil {
						b.Fatal(err)
					}
					final, err := sqlanalytics.LoadDashboard(ctx, runtime.Store, query, time.Now())
					if err != nil || final.Summary.TotalTokens != want || final.Pending != 0 {
						b.Fatalf("final snapshot: %+v %v", final, err)
					}
					return
				case <-ticker.C:
				}
			}
		}()
	}
	b.ReportMetric(float64(queryTime.Nanoseconds())/float64(queryCount), "query-mean-ns/op")
	b.ReportMetric(float64(maxQuery.Nanoseconds()), "query-max-ns/op")
	b.ReportMetric(float64(queryCount)/float64(b.N), "queries/op")
}
