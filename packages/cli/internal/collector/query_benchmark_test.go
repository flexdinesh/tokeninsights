package collector_test

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/queryclient"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/viewer"
)

const queryBenchmarkSessions = 600

// Capture, acceptance and processing are setup. Timed reads exercise the TUI's
// complete-result contract and Web's first page over the same saved history.
func BenchmarkLocalSavedQueries(b *testing.B) {
	root := b.TempDir()
	sources := filepath.Join(root, "sources")
	if err := os.Mkdir(sources, 0o700); err != nil {
		b.Fatal(err)
	}
	now := time.Now()
	month := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, now.Location())
	dates := []time.Time{now, month, month.AddDate(0, -1, 0)}
	for i := range queryBenchmarkSessions {
		body := fmt.Sprintf("{\"type\":\"session\",\"id\":\"session-%d\"}\n{\"type\":\"message\",\"id\":\"message-%d\",\"message\":{\"role\":\"assistant\",\"timestamp\":%d,\"provider\":\"openai\",\"model\":\"model\",\"usage\":{\"input\":100,\"output\":20}}}\n", i, i, dates[i%len(dates)].UnixMilli())
		if err := os.WriteFile(filepath.Join(sources, fmt.Sprintf("session-%d.jsonl", i)), []byte(body), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	collectorPath, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.duckdb")
	runtime, err := localruntime.Open(b.Context(), collectorPath, dataPath)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = runtime.Close() })
	if _, err := collector.Run(b.Context(), collector.Options{CollectorDBPath: collectorPath, ServerDBPath: dataPath, Destination: runtime.Destination, SyncOptions: pipeline.SyncOptions{Harnesses: []pipeline.Harness{pipeline.HarnessPi}, SourceDir: sources}}); err != nil {
		b.Fatal(err)
	}
	if err := runtime.WaitVisible(b.Context()); err != nil {
		b.Fatal(err)
	}
	httpServer := httptest.NewServer(server.NewDataHandler(b.Context(), runtime.Store, nil, "127.0.0.1", runtime.InstanceID, false))
	b.Cleanup(httpServer.Close)
	httpClient, err := queryclient.New(httpServer.URL, httpServer.Client())
	if err != nil {
		b.Fatal(err)
	}
	for _, period := range []api.Period{"today", "month", "all"} {
		filter := (viewer.Selection{Period: string(period)}).Filter(now)
		var sessions int64
		for i := range queryBenchmarkSessions {
			at := dates[i%len(dates)]
			if (filter.Start.IsZero() || !at.Before(filter.Start)) && (filter.End.IsZero() || at.Before(filter.End)) {
				sessions++
			}
		}
		tab := api.UsageTab("sessions")
		params := api.GetUsageParams{Period: &period, Tab: &tab}
		for _, transport := range []string{"DirectAll", "HTTPPage"} {
			b.Run(string(period)+"/"+transport, func(b *testing.B) {
				b.ReportAllocs()
				for range b.N {
					var result api.UsageResponse
					var err error
					if transport == "DirectAll" {
						result, err = runtime.Query.AllUsage(b.Context(), params)
					} else {
						result, err = httpClient.Usage(b.Context(), params)
					}
					if err != nil || result.Summary.Total != sessions*startupBenchmarkTokens || result.Summary.Sessions != sessions || result.RowCount != sessions {
						b.Fatalf("saved query totals: %+v %v", result, err)
					}
					if transport == "DirectAll" && int64(len(result.Rows)) != sessions {
						b.Fatal("incomplete TUI results", len(result.Rows), sessions)
					}
				}
			})
		}
	}
}
