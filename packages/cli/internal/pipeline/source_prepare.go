package pipeline

import (
	"context"
	"database/sql"
	"os"
	"runtime"
	"sync"
	"time"
)

const sourceProgressFlushInterval = 250 * time.Millisecond
const sourcePreparationWindowMultiplier = 2

// sourceState is an immutable snapshot of local continuity metadata for a job.
type sourceState struct {
	refresh    sourceRefreshState
	hasRefresh bool
	cursor     sourceCursorState
	hasCursor  bool
}

// preparedSource contains only metadata and facts, never transcript content.
// Workers prepare it; the single writer publishes it atomically.
type preparedSource struct {
	source          Source
	sourceInfo      os.FileInfo
	metadata        sourceRefreshMetadata
	hasMetadata     bool
	fingerprint     sourceFingerprint
	cursor          *sourceCursorState
	unchanged       bool
	facts           []RawTokenFact
	diagnostics     []Diagnostic
	status          string
	parseErr        error
	startedAtMs     int64
	minOccurredAtMs *int64
	maxOccurredAtMs *int64
	dependencies    []sourceDependency
}

type sourceDependency struct {
	source     Source
	metadata   sourceRefreshMetadata
	sourceInfo os.FileInfo
}

func prepareSource(ctx context.Context, adapter Adapter, source Source, options SyncOptions, state sourceState) preparedSource {
	var prepared preparedSource
	switch source.Harness {
	case HarnessPi:
		prepared = preparePiSource(ctx, source, options, state)
	case HarnessOpenCode:
		prepared = prepareOpenCodeSource(ctx, source, options, state)
	case HarnessCodex:
		if codex, ok := adapter.(*codexJSONLAdapter); ok {
			prepared = prepareCodexSource(ctx, codex, source, options, state)
		} else {
			prepared = prepareJSONLSource(ctx, adapter, source, options, state)
		}
	default:
		prepared = prepareJSONLSource(ctx, adapter, source, options, state)
	}
	prepared.source = source
	for _, fact := range prepared.facts {
		if fact.OccurredAtMs == nil {
			continue
		}
		value := *fact.OccurredAtMs
		if prepared.minOccurredAtMs == nil || value < *prepared.minOccurredAtMs {
			prepared.minOccurredAtMs = &value
		}
		if prepared.maxOccurredAtMs == nil || value > *prepared.maxOccurredAtMs {
			prepared.maxOccurredAtMs = &value
		}
	}
	return prepared
}

type sourceStateKey struct {
	harness  Harness
	kind, id string
}

func stateKeyFor(source Source) sourceStateKey {
	return sourceStateKey{harness: source.Harness, kind: source.Kind, id: source.ID}
}

func loadSourceStates(ctx context.Context, database *sql.DB, harness Harness) (map[sourceStateKey]sourceState, error) {
	states := map[sourceStateKey]sourceState{}
	if database == nil {
		return states, nil
	}
	rows, err := database.QueryContext(ctx, `SELECT r.source_kind, r.source_state_key,
		r.collector, r.parser, r.last_successful_refresh_at_ms, r.source_mtime_ms, r.source_size_bytes,
		c.id IS NOT NULL, COALESCE(c.collector, ''), COALESCE(c.parser, ''), COALESCE(c.cursor_kind, ''),
		COALESCE(c.byte_offset, 0), COALESCE(c.source_mtime_ms, 0), COALESCE(c.source_size_bytes, 0),
		COALESCE(c.prefix_hash, ''), COALESCE(c.boundary_hash, ''), COALESCE(c.location_fingerprint, '')
		FROM source_refresh_state r LEFT JOIN source_cursor_state c
		ON c.harness = r.harness AND c.source_kind = r.source_kind AND c.source_state_key = r.source_state_key
		WHERE r.harness = ?`, harness)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		key := sourceStateKey{harness: harness}
		state := sourceState{hasRefresh: true}
		r, c := &state.refresh, &state.cursor
		if err := rows.Scan(&key.kind, &key.id, &r.collector, &r.parser, &r.lastSuccessfulRefreshAtMs,
			&r.sourceMtimeMs, &r.sourceSizeBytes, &state.hasCursor, &c.collector, &c.parser, &c.kind,
			&c.offset, &c.mtimeMs, &c.sizeBytes, &c.prefixHash, &c.boundaryHash, &c.locationFingerprint); err != nil {
			return nil, err
		}
		states[key] = state
	}
	return states, rows.Err()
}

func sourceWorkerCount(options SyncOptions, count int) int {
	workers := options.workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	return max(1, min(workers, count))
}

type sourceTask struct {
	index       int
	source      Source
	startedAtMs int64
}

type sourceResult struct {
	index    int
	prepared preparedSource
}

// Prepared results are consumed in source order. Dispatch advances only as
// that ordered window drains, bounding queued and out-of-order results.
func prepareSources(ctx context.Context, sources []Source, options SyncOptions, adapter Adapter, states map[sourceStateKey]sourceState, start func(Source) error, consume func(preparedSource) error, flush func() error) error {
	if len(sources) == 0 {
		return nil
	}
	workerContext, cancel := context.WithCancel(ctx)
	workers := sourceWorkerCount(options, len(sources))
	window := workers * sourcePreparationWindowMultiplier
	jobs := make(chan sourceTask, window)
	results := make(chan sourceResult, window)
	var running sync.WaitGroup
	for range workers {
		running.Go(func() {
			for {
				select {
				case <-workerContext.Done():
					return
				case job, ok := <-jobs:
					if !ok {
						return
					}
					prepared := prepareSource(workerContext, adapter, job.source, options, states[stateKeyFor(job.source)])
					prepared.startedAtMs = job.startedAtMs
					select {
					case results <- sourceResult{index: job.index, prepared: prepared}:
					case <-workerContext.Done():
						return
					}
				}
			}
		})
	}
	defer func() {
		cancel()
		close(jobs)
		running.Wait()
	}()
	next := 0
	dispatch := func() error {
		source := sources[next]
		if start != nil {
			if err := start(source); err != nil {
				return err
			}
		}
		jobs <- sourceTask{index: next, source: source, startedAtMs: syncWallNow(options).UnixMilli()}
		next++
		return nil
	}
	for next < min(window, len(sources)) {
		if err := dispatch(); err != nil {
			return err
		}
	}
	ticker := time.NewTicker(sourceProgressFlushInterval)
	defer ticker.Stop()
	pending := make(map[int]preparedSource, window)
	for index := 0; index < len(sources); {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if flush != nil {
				if err := flush(); err != nil {
					return err
				}
			}
		case result := <-results:
			pending[result.index] = result.prepared
		}
		for {
			prepared, ok := pending[index]
			if !ok {
				break
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := consume(prepared); err != nil {
				return err
			}
			delete(pending, index)
			index++
			if next < len(sources) {
				if err := dispatch(); err != nil {
					return err
				}
			}
		}
	}
	if flush != nil {
		return flush()
	}
	return nil
}
