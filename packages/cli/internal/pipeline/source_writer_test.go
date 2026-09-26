package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func preparedWriterFixture(t *testing.T, count int) (*sql.DB, SyncOptions, []preparedSource) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	database, _, err := db.CreateIfMissing(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	options := SyncOptions{DBPath: dbPath, Harnesses: []Harness{HarnessPi}, Collector: "test", Parser: "test", Now: time.Unix(1700000000, 0)}
	var prepared []preparedSource
	var sources []Source
	for i := range count {
		id := fmt.Sprintf("session_%d", i)
		path := filepath.Join(t.TempDir(), "2026-01-01_"+id+".jsonl")
		writePiAssistantSession(t, path, id, "msg", 10, 5)
		source := (piJSONLAdapter{}).source(path, filepath.Dir(path))
		result := prepareSource(context.Background(), piJSONLAdapter{}, source, options, sourceState{})
		if result.parseErr != nil || result.cursor == nil || len(result.facts) != 1 {
			t.Fatalf("prepare fixture: %+v", result)
		}
		result.startedAtMs = options.Now.UnixMilli()
		result.facts[0].DedupeKey = "fixture:" + id
		prepared = append(prepared, result)
		sources = append(sources, source)
	}
	options.jobID, err = startSyncJob(context.Background(), database, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := recordDiscoveredSources(context.Background(), database, options, HarnessPi, sources); err != nil {
		t.Fatal(err)
	}
	return database, options, prepared
}

func TestPreparedWriterFailureRollsBackFactsMarkersAndDedupe(t *testing.T) {
	for _, failure := range []string{"marker", "status"} {
		t.Run(failure, func(t *testing.T) {
			database, options, prepared := preparedWriterFixture(t, 1)
			trigger := "CREATE TRIGGER fail_write BEFORE INSERT ON source_cursor_state BEGIN SELECT RAISE(ABORT, 'marker write failed'); END"
			if failure == "status" {
				trigger = "CREATE TRIGGER fail_write BEFORE UPDATE ON sync_sources WHEN NEW.status = 'ingested' BEGIN SELECT RAISE(ABORT, 'status write failed'); END"
			}
			if _, err := database.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			seen := map[string]int64{}
			summary, err := commitPreparedSources(context.Background(), database, options, prepared, seen)
			if err == nil || summary.RawFacts != 0 || summary.Observations != 0 || len(seen) != 0 {
				t.Fatalf("failed write escaped rollback: %+v, %v, seen=%v", summary, err, seen)
			}
			for _, table := range []string{"raw_token_usage", "raw_observations", "source_cursor_state", "source_refresh_state", "normalization_work_queue"} {
				assertCount(t, database, table, 0)
			}
			assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'failed' AND raw_fact_count = 0 AND observation_count = 0", 1)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM sync_sources WHERE status = 'failed'", 1)
			assertSQLCount(t, database, "SELECT checked_sources FROM sync_harnesses", 1)
			assertSQLCount(t, database, "SELECT failed_sources FROM sync_harnesses", 1)
		})
	}
}

func TestPreparedWriterUnchangedBatchCommitsCountsWithoutObservations(t *testing.T) {
	database, options, prepared := preparedWriterFixture(t, 2)
	for i := range prepared {
		prepared[i].facts, prepared[i].diagnostics = nil, nil
		prepared[i].unchanged, prepared[i].status = true, "unchanged"
	}
	summary, err := commitPreparedSources(context.Background(), database, options, prepared, map[string]int64{})
	if err != nil || summary.RawFacts != 0 || summary.Observations != 0 {
		t.Fatalf("unchanged batch: %+v, %v", summary, err)
	}
	assertCount(t, database, "raw_token_usage", 0)
	assertCount(t, database, "raw_observations", 0)
	assertCount(t, database, "source_cursor_state", 2)
	assertCount(t, database, "source_refresh_state", 2)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND observation_count = 0", 2)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM sync_sources WHERE status = 'unchanged'", 2)
	assertSQLCount(t, database, "SELECT checked_sources FROM sync_harnesses", 2)
	assertSQLCount(t, database, "SELECT failed_sources FROM sync_harnesses", 0)
}

func TestPreparedWriterBatchRollsBackWhenFailureAuditCannotCommit(t *testing.T) {
	database, options, prepared := preparedWriterFixture(t, 2)
	for i := range prepared {
		prepared[i].facts, prepared[i].diagnostics = nil, nil
		prepared[i].unchanged, prepared[i].status = true, "unchanged"
	}
	if _, err := database.Exec("CREATE TRIGGER fail_second_attempt BEFORE UPDATE ON sync_harnesses WHEN NEW.checked_sources = 2 BEGIN SELECT RAISE(ABORT, 'progress write failed'); END"); err != nil {
		t.Fatal(err)
	}
	seen := map[string]int64{}
	summary, err := commitPreparedSources(context.Background(), database, options, prepared, seen)
	if err == nil || summary.RawFacts != 0 || summary.Observations != 0 || len(seen) != 0 {
		t.Fatalf("failed batch escaped rollback: %+v, %v, %v", summary, err, seen)
	}
	assertCount(t, database, "ingest_runs", 0)
	assertCount(t, database, "source_cursor_state", 0)
	assertCount(t, database, "source_refresh_state", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM sync_sources WHERE status = 'pending'", 2)
	assertSQLCount(t, database, "SELECT checked_sources FROM sync_harnesses", 0)
}

func TestPreparedWriterMutationBeforeCommitDefersAndDeletesMarkers(t *testing.T) {
	database, options, prepared := preparedWriterFixture(t, 1)
	seen := map[string]int64{}
	if _, err := commitPreparedSources(context.Background(), database, options, prepared, seen); err != nil {
		t.Fatal(err)
	}
	assertCount(t, database, "source_cursor_state", 1)
	prepared[0].facts, prepared[0].diagnostics = nil, nil
	prepared[0].unchanged, prepared[0].status = true, "unchanged"
	file, err := os.OpenFile(prepared[0].source.Path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("\n")
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("mutate source: %v, %v", err, closeErr)
	}
	if _, err := commitPreparedSources(context.Background(), database, options, prepared, seen); err != nil {
		t.Fatal(err)
	}
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 1)
	assertCount(t, database, "source_cursor_state", 0)
	assertCount(t, database, "source_refresh_state", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM sync_sources WHERE status = 'deferred' AND min_occurred_at_ms IS NULL AND max_occurred_at_ms IS NULL", 1)
}

func TestPreparedWriterSameMetadataReplacementDefers(t *testing.T) {
	database, options, prepared := preparedWriterFixture(t, 1)
	path := prepared[0].source.Path
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := path + ".replacement"
	if err := os.WriteFile(replacement, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, err := commitPreparedSources(context.Background(), database, options, prepared, map[string]int64{}); err != nil {
		t.Fatal(err)
	}
	assertCount(t, database, "source_cursor_state", 0)
	assertCount(t, database, "source_refresh_state", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM sync_sources WHERE status = 'deferred'", 1)
}

func TestPreparedWriterCancelledBeforeCommitWritesNothing(t *testing.T) {
	database, options, prepared := preparedWriterFixture(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	seen := map[string]int64{}
	if _, err := commitPreparedSources(ctx, database, options, prepared, seen); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled commit: %v", err)
	}
	assertCount(t, database, "ingest_runs", 0)
	assertCount(t, database, "raw_token_usage", 0)
	assertCount(t, database, "source_cursor_state", 0)
	assertSQLCount(t, database, "SELECT checked_sources FROM sync_harnesses", 0)
	if len(seen) != 0 {
		t.Fatalf("cancelled write leaked dedupe: %v", seen)
	}
}

func TestPreparedWriterCancellationAfterFactWritesRollsBack(t *testing.T) {
	database, options, prepared := preparedWriterFixture(t, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Status publication calls the writer clock after facts and markers are
	// written. Cancellation there must roll back the enclosing transaction.
	options.Clock = func() time.Time {
		cancel()
		return options.Now
	}
	seen := map[string]int64{}
	if _, err := commitPreparedSources(ctx, database, options, prepared, seen); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled publication: %v", err)
	}
	for _, table := range []string{"ingest_runs", "raw_token_usage", "raw_observations", "source_cursor_state", "source_refresh_state", "normalization_work_queue"} {
		assertCount(t, database, table, 0)
	}
	assertSQLCount(t, database, "SELECT checked_sources FROM sync_harnesses", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM sync_sources WHERE status = 'pending'", 1)
	if len(seen) != 0 {
		t.Fatalf("cancelled transaction leaked dedupe: %v", seen)
	}
}
