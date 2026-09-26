package pipeline

import (
	"context"
	"database/sql"
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

func withSyncStats(ctx context.Context, stats *syncStats) context.Context {
	if stats == nil {
		return ctx
	}
	return context.WithValue(ctx, syncStatsKey{}, stats)
}

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

func commitSyncTransaction(ctx context.Context, tx *sql.Tx) error {
	if err := tx.Commit(); err != nil {
		return err
	}
	if stats := statsForSync(ctx); stats != nil {
		stats.writerCommits.Add(1)
	}
	return nil
}
