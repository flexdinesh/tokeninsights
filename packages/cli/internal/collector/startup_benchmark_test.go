package collector_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/duckdb"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const (
	startupBenchmarkSources     = 8
	startupBenchmarkFacts       = 32
	startupBenchmarkTokens      = 120
	startupBenchmarkManySources = 240
)

func startupBenchmarkMessage(source, message int) string {
	return fmt.Sprintf("{\"type\":\"message\",\"id\":\"message-%d-%d\",\"message\":{\"role\":\"assistant\",\"timestamp\":1700000000000,\"provider\":\"openai\",\"model\":\"model\",\"usage\":{\"input\":100,\"output\":20}}}\n", source, message)
}

func startupBenchmarkRun(b *testing.B, runtime *localruntime.Runtime, options collector.Options, wantTokens int64, wantSessions int) (time.Duration, time.Duration, time.Duration) {
	b.Helper()
	started := time.Now()
	if _, err := collector.Run(b.Context(), options); err != nil {
		b.Fatal(err)
	}
	captured := time.Now()
	if err := runtime.WaitVisible(b.Context()); err != nil {
		b.Fatal(err)
	}
	visible := time.Now()
	query := analytics.Query{Selection: viewer.Selection{Period: "all", Bucket: "day"}, Tab: "sessions", Quality: "confirmed", Sort: "total", Direction: "desc", Page: 1, PageSize: 200}
	dashboard, err := duckdb.LoadDashboard(b.Context(), runtime.Store, query, time.Now())
	if err != nil || dashboard.Summary.TotalTokens != wantTokens || dashboard.Summary.SessionCount != int64(wantSessions) || dashboard.Pending != 0 {
		b.Fatalf("startup totals: %+v %v", dashboard, err)
	}
	return captured.Sub(started), visible.Sub(captured), time.Since(visible)
}

// Includes opening the local owner, capture, direct acceptance, concurrent
// processing/visibility and the first analytics query. Fixture setup, warm-up
// and owner shutdown are excluded. This synthetic Pi workload is not a claim
// about the duration of an interactive startup or a mixed-harness history.
func BenchmarkLocalStartup(b *testing.B) {
	for _, scenario := range []string{"FirstIngest", "Unchanged", "Append", "UnchangedManySources"} {
		b.Run(scenario, func(b *testing.B) {
			b.ReportAllocs()
			b.StopTimer()
			sourceCount, factCount := startupBenchmarkSources, startupBenchmarkFacts
			if scenario == "UnchangedManySources" {
				sourceCount, factCount = startupBenchmarkManySources, 1
			}
			var opened, captured, visible, queried time.Duration
			for range b.N {
				root := b.TempDir()
				sources := filepath.Join(root, "sources")
				if err := os.Mkdir(sources, 0o700); err != nil {
					b.Fatal(err)
				}
				for source := range sourceCount {
					var body strings.Builder
					fmt.Fprintf(&body, "{\"type\":\"session\",\"id\":\"session-%d\"}\n", source)
					for message := range factCount {
						body.WriteString(startupBenchmarkMessage(source, message))
					}
					if err := os.WriteFile(filepath.Join(sources, fmt.Sprintf("session-%d.jsonl", source)), []byte(body.String()), 0o600); err != nil {
						b.Fatal(err)
					}
				}
				collectorPath, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.duckdb")
				options := collector.Options{CollectorDBPath: collectorPath, ServerDBPath: dataPath, SyncOptions: pipeline.SyncOptions{SourceDir: sources, Harnesses: []pipeline.Harness{pipeline.HarnessPi}}}
				wantTokens := int64(sourceCount * factCount * startupBenchmarkTokens)
				if scenario != "FirstIngest" {
					warm, err := localruntime.Open(b.Context(), collectorPath, dataPath)
					if err != nil {
						b.Fatal(err)
					}
					options.Destination = warm.Destination
					startupBenchmarkRun(b, warm, options, wantTokens, sourceCount)
					if err := warm.Close(); err != nil {
						b.Fatal(err)
					}
				}
				if scenario == "Append" {
					path := filepath.Join(sources, "session-0.jsonl")
					file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
					if err != nil {
						b.Fatal(err)
					}
					_, writeErr := file.WriteString(startupBenchmarkMessage(0, startupBenchmarkFacts))
					closeErr := file.Close()
					if writeErr != nil || closeErr != nil {
						b.Fatal(writeErr, closeErr)
					}
					wantTokens += startupBenchmarkTokens
				}
				b.StartTimer()
				started := time.Now()
				runtime, err := localruntime.Open(b.Context(), collectorPath, dataPath)
				if err != nil {
					b.Fatal(err)
				}
				opened += time.Since(started)
				options.Destination = runtime.Destination
				capture, visibility, query := startupBenchmarkRun(b, runtime, options, wantTokens, sourceCount)
				captured += capture
				visible += visibility
				queried += query
				b.StopTimer()
				if err := runtime.Close(); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(opened.Nanoseconds())/float64(b.N), "open-ns/op")
			b.ReportMetric(float64(captured.Nanoseconds())/float64(b.N), "capture-accept-ns/op")
			b.ReportMetric(float64(visible.Nanoseconds())/float64(b.N), "visibility-ns/op")
			b.ReportMetric(float64(queried.Nanoseconds())/float64(b.N), "query-ns/op")
		})
	}
}
