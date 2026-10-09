package pipeline

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
)

const (
	rawPreparationWorkers = 4
	// Readers, native JSON decoding and sanitized record encoding share a
	// per-worker allowance. The record bound is independent of worker count.
	rawPreparationWorkerBytes = 4 * maxEvidenceLineBytes
	rawPreparationMemoryBytes = rawPreparationWorkers * rawPreparationWorkerBytes
	rawParserPolicyVersion    = 1
)

var (
	errSourceRecordLimit = errors.New("source_record_limit")
	errSourceQuarantined = errors.New("source_quarantined")
)

type recordLimitError struct{ Offset int64 }

func (e *recordLimitError) Error() string { return errSourceRecordLimit.Error() }
func (e *recordLimitError) Unwrap() error { return errSourceRecordLimit }

// captureSink permits native reading without owning a collector transaction.
// Its only records are sanitized evidence; checkpoints remain local metadata.
type captureSink interface {
	Checkpoint(context.Context, string, string) (rawcollectorstore.Checkpoint, bool, error)
	SaveCheckpoint(context.Context, rawcollectorstore.Checkpoint) error
	Record(context.Context, evidence.Record, int64) (bool, error)
}

type stagedRecord struct {
	Record       evidence.Record `json:"record"`
	RecordedAtMs int64           `json:"recordedAtMs"`
}

type preparedCapture struct {
	store         *rawcollectorstore.Store
	file          *os.File
	encoder       *json.Encoder
	baseline      rawcollectorstore.Checkpoint
	baselineFound bool
	checkpoint    rawcollectorstore.Checkpoint
	verify        func(context.Context) error
	quarantine    *rawcollectorstore.Quarantine
	quarantined   bool
}

func (p *preparedCapture) Checkpoint(ctx context.Context, key, format string) (rawcollectorstore.Checkpoint, bool, error) {
	state, found, err := p.store.Checkpoint(ctx, key, format)
	p.baseline, p.baselineFound = state, found
	return state, found, err
}
func (p *preparedCapture) SaveCheckpoint(_ context.Context, state rawcollectorstore.Checkpoint) error {
	p.checkpoint = state
	return nil
}
func (p *preparedCapture) Record(ctx context.Context, record evidence.Record, now int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := evidence.ValidateRecord(record); err != nil {
		return false, err
	}
	err := p.encoder.Encode(stagedRecord{Record: record, RecordedAtMs: now})
	return err == nil, err
}
func (p *preparedCapture) close() {
	if p.file != nil {
		path := p.file.Name()
		_ = p.file.Close()
		_ = os.Remove(path)
	}
}

func prepareRawSource(ctx context.Context, source Source, options SyncOptions, store *rawcollectorstore.Store, directory string) (*preparedCapture, error) {
	p := &preparedCapture{store: store}
	format := string(source.Harness) + "-jsonl"
	if source.Harness == HarnessOpenCode {
		format = "opencode-sqlite"
	}
	// SQLite snapshots may live in WAL; only JSONL deterministic record failures
	// currently qualify for quarantine with a file-signature retry policy.
	signature := ""
	if source.Harness != HarnessOpenCode {
		var err error
		signature, err = rawSourceSignature(source.Path)
		if err != nil {
			return p, err
		}
		marker, found, err := store.GetQuarantine(ctx, source.ID)
		if err != nil {
			return p, err
		}
		if found && !options.FullRefresh && marker.Signature == signature && marker.Format == format && marker.ParserVersion == rawParserPolicyVersion {
			p.quarantined = true
			return p, errSourceQuarantined
		}
	}
	file, err := os.CreateTemp(directory, "evidence-*")
	if err != nil {
		return p, err
	}
	p.file, p.encoder = file, json.NewEncoder(file)
	recordSourceParse(ctx)
	if source.Harness == HarnessOpenCode {
		_, err = parseSQLite(ctx, source, options, p)
	} else {
		_, err = parseJSONL(ctx, source, options, p)
	}
	var limit *recordLimitError
	if errors.As(err, &limit) && ctx.Err() == nil && signature != "" {
		current, statErr := rawSourceSignature(source.Path)
		if statErr == nil && current == signature {
			p.quarantine = &rawcollectorstore.Quarantine{SourceKey: source.ID, Format: format, Signature: signature, Code: errSourceRecordLimit.Error(), ParserVersion: rawParserPolicyVersion, Offset: limit.Offset, RecordedAtMs: time.Now().UnixMilli()}
		}
	}
	return p, err
}

func commitPreparedRaw(ctx context.Context, source Source, p *preparedCapture, store *rawcollectorstore.Store) (int, error) {
	tx, err := store.BeginCapture(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	current, found, err := tx.Checkpoint(ctx, source.ID, p.checkpoint.Format)
	if err != nil {
		return 0, err
	}
	if found != p.baselineFound || found && !reflect.DeepEqual(current, p.baseline) {
		return 0, errors.New("source_checkpoint_changed")
	}
	if _, err := p.file.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	decoder := json.NewDecoder(p.file)
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		var staged stagedRecord
		if err := decoder.Decode(&staged); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return 0, err
		}
		added, err := tx.Record(ctx, staged.Record, staged.RecordedAtMs)
		if err != nil {
			return 0, err
		}
		if added {
			count++
		}
	}
	if p.verify != nil {
		if err := p.verify(ctx); err != nil {
			return 0, err
		}
	}
	if err := tx.SaveCheckpoint(ctx, p.checkpoint); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if stats := statsForSync(ctx); stats != nil {
		stats.writerCommits.Add(1)
	}
	return count, store.ClearQuarantine(ctx, source.ID)
}

type rawHarnessState struct {
	harness                          Harness
	remaining, captured, quarantined int
	failed                           bool
}
type rawSourceJob struct {
	source       Source
	harnessIndex int
}
type rawSourceResult struct {
	prepared *preparedCapture
	err      error
}

func rawWorkerCount(options SyncOptions, count int) int {
	workers := options.workers
	if workers <= 0 {
		workers = rawPreparationWorkers
	}
	return max(1, min(workers, count, rawPreparationMemoryBytes/rawPreparationWorkerBytes))
}

func extractPrepared(ctx context.Context, options SyncOptions, store *rawcollectorstore.Store) (Summary, error) {
	var summary Summary
	harnesses := options.Harnesses
	if len(harnesses) == 0 {
		harnesses = SupportedHarnesses
	}
	summary.RequestedHarnesses = len(harnesses)
	if options.locationResolver == nil {
		options.locationResolver = &locationResolver{}
	}
	var jobs []rawSourceJob
	states := make([]rawHarnessState, len(harnesses))
	for i, h := range harnesses {
		states[i].harness = h
		if options.Progress != nil {
			options.Progress(SyncProgressEvent{Harness: h, Status: SyncProgressDiscovering})
		}
		adapter, ok := AdapterFor(h)
		if !ok {
			return summary, errors.New("unsupported_harness")
		}
		sources, err := adapter.Discover(ctx, DiscoverOptions{Sources: options.Sources, SourceDir: options.SourceDir, HarnessSubdirOnly: len(harnesses) > 1})
		if err != nil {
			summary.Failed++
			summary.Errors = append(summary.Errors, err)
			if options.Progress != nil {
				options.Progress(SyncProgressEvent{Harness: h, Status: SyncProgressFailed})
			}
			continue
		}
		if len(sources) == 0 {
			summary.Skipped++
			if options.Progress != nil {
				options.Progress(SyncProgressEvent{Harness: h, Status: SyncProgressSkipped})
			}
			continue
		}
		states[i].remaining = len(sources)
		if options.Progress != nil {
			options.Progress(SyncProgressEvent{Harness: h, Status: SyncProgressSyncing})
		}
		for _, source := range sources {
			source.ID = evidence.Tuple("source-file", source.Harness, source.ID, source.Path)
			jobs = append(jobs, rawSourceJob{source: source, harnessIndex: i})
		}
	}
	if len(jobs) == 0 {
		return summary, errors.Join(summary.Errors...)
	}
	directory, err := os.MkdirTemp("", "tokeninsights-capture-*")
	if err != nil {
		return summary, err
	}
	defer func() { _ = os.RemoveAll(directory) }()
	workers := rawWorkerCount(options, len(jobs))
	queue := make(chan int, workers)
	results := make([]chan rawSourceResult, len(jobs))
	for i := range results {
		results[i] = make(chan rawSourceResult, 1)
	}
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for index := range queue {
				p, err := prepareRawSource(ctx, jobs[index].source, options, store, directory)
				results[index] <- rawSourceResult{prepared: p, err: err}
			}
		})
	}
	// A source slot remains occupied until the single writer consumes it. This
	// bounds completed spools and active readers together, even for a slow source.
	issued := min(workers, len(jobs))
	for i := 0; i < issued; i++ {
		queue <- i
	}
	for completed := 0; completed < issued; completed++ {
		result := <-results[completed]
		job := jobs[completed]
		state := &states[job.harnessIndex]
		count := 0
		if result.err == nil {
			count, result.err = commitPreparedRaw(ctx, job.source, result.prepared, store)
		}
		if result.prepared.quarantine != nil {
			if saveErr := store.SaveQuarantine(ctx, *result.prepared.quarantine); saveErr != nil {
				result.err = errors.Join(result.err, saveErr)
			} else {
				result.prepared.quarantined = true
			}
		}
		result.prepared.close()
		if result.prepared.quarantined {
			state.quarantined++
			summary.Quarantined++
		}
		if result.err != nil {
			state.failed = true
			summary.Errors = append(summary.Errors, result.err)
		}
		state.captured += count
		summary.RawFacts += count
		state.remaining--
		if state.remaining == 0 {
			status := SyncProgressSynced
			if state.failed {
				summary.Failed++
				status = SyncProgressFailed
			} else if state.captured == 0 && !options.FullRefresh {
				summary.Skipped++
				status = SyncProgressSkipped
			} else {
				summary.Synced++
			}
			if options.Progress != nil {
				options.Progress(SyncProgressEvent{Harness: state.harness, Status: status, Quarantined: state.quarantined})
			}
		}
		if issued < len(jobs) && ctx.Err() == nil {
			queue <- issued
			issued++
		}
	}
	close(queue)
	wg.Wait()
	if issued < len(jobs) {
		for _, state := range states {
			if state.remaining > 0 {
				summary.Failed++
				if options.Progress != nil {
					options.Progress(SyncProgressEvent{Harness: state.harness, Status: SyncProgressFailed, Quarantined: state.quarantined})
				}
			}
		}
		summary.Errors = append(summary.Errors, ctx.Err())
	}
	return summary, errors.Join(summary.Errors...)
}

func copyPrefix(ctx context.Context, dst io.Writer, file *os.File, n int64) (int64, error) {
	return io.CopyN(dst, contextReader{ctx: ctx, reader: file}, n)
}
func verifyPreparedJSONL(ctx context.Context, path string, opened os.FileInfo, length int64, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	current, err := file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(opened, current) {
		return errors.New("source_replaced_during_capture")
	}
	hash := sha256.New()
	if _, err := copyPrefix(ctx, hash, file, length); err != nil {
		return err
	}
	if !bytes.Equal([]byte(hex.EncodeToString(hash.Sum(nil))), []byte(expected)) {
		return errors.New("source_changed_during_capture")
	}
	current, err = os.Stat(path)
	if err != nil {
		return err
	}
	if !os.SameFile(opened, current) {
		return errors.New("source_replaced_during_capture")
	}
	return nil
}
