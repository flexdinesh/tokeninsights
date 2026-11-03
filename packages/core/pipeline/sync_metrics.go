package pipeline

import (
	"context"
	"sync/atomic"
)

// Optional job counters for benchmarks. JSONL bytes count actual reader traffic,
// including discovery and continuity verification; SQLite pages are not included.
type syncStats struct {
	bytesRead     atomic.Int64
	sourceParses  atomic.Int64
	writerCommits atomic.Int64
}

type syncStatsKey struct{}

func statsForSync(ctx context.Context) *syncStats {
	stats, _ := ctx.Value(syncStatsKey{}).(*syncStats)
	return stats
}

func recordSourceBytes(ctx context.Context, count int) {
	if stats := statsForSync(ctx); stats != nil {
		stats.bytesRead.Add(int64(count))
	}
}

func recordSourceParse(ctx context.Context) {
	if stats := statsForSync(ctx); stats != nil {
		stats.sourceParses.Add(1)
	}
}
