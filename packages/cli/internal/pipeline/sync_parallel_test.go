package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestSyncParallelPreservesSerialSemantics(t *testing.T) {
	root := t.TempDir()
	benchmarkSources(t, root, true)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	serial := SyncOptions{DBPath: filepath.Join(t.TempDir(), "serial.sqlite"), SourceDir: root, Harnesses: SupportedHarnesses, Normalize: true, Now: now, Clock: func() time.Time { return now }, workers: 1}
	parallel := serial
	parallel.DBPath = filepath.Join(t.TempDir(), "parallel.sqlite")
	parallel.workers = 4
	for _, stage := range []string{"first", "unchanged", "appended", "full-refresh"} {
		t.Run(stage, func(t *testing.T) {
			if stage == "appended" {
				benchmarkAppend(t, benchmarkSourcePath(root, HarnessPi, 0), benchmarkPiMessage(0, benchmarkFactCount))
				benchmarkAppend(t, benchmarkSourcePath(root, HarnessCodex, 0), codexReplayTurn("new-parent-turn"), codexReplayUsage(25, 10, (benchmarkFactCount+1)*10))
				benchmarkAppend(t, benchmarkSourcePath(root, HarnessClaudeCode, 0), benchmarkClaudeMessage(0, benchmarkFactCount))
			}
			serial.FullRefresh, parallel.FullRefresh = stage == "full-refresh", stage == "full-refresh"
			serial.stats, parallel.stats = &syncStats{}, &syncStats{}
			var serialPublished, parallelPublished []Harness
			serial.Progress = func(event SyncProgressEvent) {
				if event.Published {
					serialPublished = append(serialPublished, event.Harness)
				}
			}
			parallel.Progress = func(event SyncProgressEvent) {
				if event.Published {
					parallelPublished = append(parallelPublished, event.Harness)
				}
			}
			serialSummary := benchmarkSync(t, serial)
			parallelSummary := benchmarkSync(t, parallel)
			if !reflect.DeepEqual(serialSummary, parallelSummary) {
				t.Fatalf("summaries differ: serial=%+v parallel=%+v", serialSummary, parallelSummary)
			}
			if stage == "unchanged" && (serial.stats.sourceParses.Load() != 0 || parallel.stats.sourceParses.Load() != 0) {
				t.Fatalf("unchanged ancestry reparsed: serial=%d parallel=%d", serial.stats.sourceParses.Load(), parallel.stats.sourceParses.Load())
			}
			var wantPublished []Harness
			switch stage {
			case "first":
				wantPublished = SupportedHarnesses
			case "appended":
				wantPublished = []Harness{HarnessPi, HarnessCodex, HarnessClaudeCode}
			}
			if !reflect.DeepEqual(serialPublished, parallelPublished) || !reflect.DeepEqual(serialPublished, wantPublished) {
				t.Fatalf("publication order differs: serial=%+v parallel=%+v", serialPublished, parallelPublished)
			}
			serialDB := openTestDB(t, serial.DBPath)
			defer func() { _ = serialDB.Close() }()
			parallelDB := openTestDB(t, parallel.DBPath)
			defer func() { _ = parallelDB.Close() }()
			wantFacts := len(SupportedHarnesses) * benchmarkSourceCount * benchmarkFactCount
			wantFacts += benchmarkSourceCount * (benchmarkForkDepth - 1)
			wantTokens := benchmarkSourceCount*benchmarkFactCount*(12+12+12+10) + benchmarkSourceCount*(benchmarkForkDepth-1)*10
			if stage == "appended" || stage == "full-refresh" {
				wantFacts += 3
				wantTokens += 12 + 12 + 10
			}
			assertSQLCount(t, parallelDB, "SELECT COUNT(*) FROM canonical_token_usage", wantFacts)
			assertSQLCount(t, parallelDB, "SELECT SUM(total_tokens) FROM canonical_token_usage", wantTokens)
			assertEqualJSON(t, queryRawTokenUsage(t, parallelDB), queryRawTokenUsage(t, serialDB))
			assertEqualJSON(t, queryCanonicalTokenUsage(t, parallelDB), queryCanonicalTokenUsage(t, serialDB))
			assertEqualJSON(t, queryRawObservations(t, parallelDB), queryRawObservations(t, serialDB))
			assertEqualJSON(t, queryDiagnostics(t, parallelDB), queryDiagnostics(t, serialDB))
			serialStatus, err := db.ReadSyncStatus(context.Background(), serial.DBPath)
			if err != nil {
				t.Fatal(err)
			}
			parallelStatus, err := db.ReadSyncStatus(context.Background(), parallel.DBPath)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(serialStatus, parallelStatus) {
				t.Fatalf("status differs: serial=%+v parallel=%+v", serialStatus, parallelStatus)
			}
			filter := db.Filter{DayFrom: "2026-01-01", DayTo: "2026-01-02"}
			serialCoverage, err := db.ViewerDayCoverage(context.Background(), serialDB, filter, now)
			if err != nil {
				t.Fatal(err)
			}
			parallelCoverage, err := db.ViewerDayCoverage(context.Background(), parallelDB, filter, now)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(serialCoverage, parallelCoverage) {
				t.Fatalf("coverage differs: serial=%+v parallel=%+v", serialCoverage, parallelCoverage)
			}
		})
	}
}

type gatedSyncAdapter struct {
	parse func(context.Context, Source) error
}

func (gatedSyncAdapter) Harness() Harness { return "worker-test" }
func (gatedSyncAdapter) Discover(context.Context, DiscoverOptions) ([]Source, error) {
	return nil, nil
}
func (adapter gatedSyncAdapter) Parse(ctx context.Context, source Source, _ SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	return nil, nil, adapter.parse(ctx, source)
}

func workerTestSources(count int) []Source {
	sources := make([]Source, count)
	for index := range sources {
		sources[index] = Source{Harness: "worker-test", ID: fmt.Sprint(index)}
	}
	return sources
}

func TestSyncPreparationBoundsWorkersAndOutstandingResults(t *testing.T) {
	const workers = 3
	sources := workerTestSources(workers * 3)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered := make(chan string, len(sources))
	releaseFirst, releaseOthers := make(chan struct{}), make(chan struct{})
	var active, peak, dispatched atomic.Int64
	adapter := gatedSyncAdapter{parse: func(ctx context.Context, source Source) error {
		current := active.Add(1)
		for prior := peak.Load(); current > prior; prior = peak.Load() {
			if peak.CompareAndSwap(prior, current) {
				break
			}
		}
		defer active.Add(-1)
		entered <- source.ID
		release := releaseOthers
		if source.ID == sources[0].ID {
			release = releaseFirst
		}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	var consumed []string
	done := make(chan error, 1)
	go func() {
		done <- prepareSources(ctx, sources, defaultSyncOptions(SyncOptions{workers: workers}), adapter, nil,
			func(Source) error { dispatched.Add(1); return nil },
			func(prepared preparedSource) error { consumed = append(consumed, prepared.source.ID); return nil }, nil)
	}()
	for range workers {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("preparation did not use all available workers")
		}
	}
	if got := active.Load(); got != workers {
		t.Fatalf("active workers: got %d, want %d", got, workers)
	}
	close(releaseOthers)
	for range workers {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("out-of-order preparation stalled")
		}
	}
	if got := dispatched.Load(); got != workers*sourcePreparationWindowMultiplier {
		t.Fatalf("unbounded work behind blocked first source: dispatched %d", got)
	}
	select {
	case source := <-entered:
		t.Fatalf("prepared source %s beyond outstanding window", source)
	default:
	}
	close(releaseFirst)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(consumed) != len(sources) {
		t.Fatalf("consumed %d sources, want %d", len(consumed), len(sources))
	}
	for index, source := range sources {
		if consumed[index] != source.ID {
			t.Fatalf("source %d consumed out of order: %s", index, consumed[index])
		}
	}
	if active.Load() != 0 {
		t.Fatal("preparation returned with workers still running")
	}
	if got := peak.Load(); got != workers {
		t.Fatalf("worker bound: maximum active %d, want %d", got, workers)
	}
}

func TestSyncPreparationJoinsWorkersOnCancellationAndWriterFailure(t *testing.T) {
	for _, failure := range []string{"cancelled", "writer-failed"} {
		t.Run(failure, func(t *testing.T) {
			const workers = 3
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			sources := workerTestSources(workers * 3)
			entered := make(chan struct{}, len(sources))
			release := make(chan struct{})
			var active atomic.Int64
			adapter := gatedSyncAdapter{parse: func(ctx context.Context, _ Source) error {
				active.Add(1)
				defer active.Add(-1)
				entered <- struct{}{}
				select {
				case <-release:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}}
			writeFailure := errors.New("writer failed")
			done := make(chan error, 1)
			go func() {
				done <- prepareSources(ctx, sources, defaultSyncOptions(SyncOptions{workers: workers}), adapter, nil, nil,
					func(preparedSource) error { return writeFailure }, nil)
			}()
			for range workers {
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("preparation did not start workers")
				}
			}
			want := writeFailure
			if failure == "cancelled" {
				want = context.Canceled
				cancel()
			} else {
				close(release)
			}
			if err := <-done; !errors.Is(err, want) {
				t.Fatalf("preparation failure: got %v, want %v", err, want)
			}
			if active.Load() != 0 {
				t.Fatal("preparation returned with workers still running")
			}
		})
	}
}
