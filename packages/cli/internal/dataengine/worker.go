// Package dataengine coordinates durable processing through semantic storage
// operations. Storage owns transactions, dataset selection and revision fences;
// the engine owns interpretation, worker serialization and wakeup lifecycle.
package dataengine

import (
	"context"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
)

// Work is a consistent dataset-scoped connected component. Publication must
// fence its generation, membership and every scope revision in one transaction.
type Work struct {
	DatasetID            string
	Revision, Generation int64
	Root                 string
	// Bytes is the size of stored sanitized JSON, used for bounded admission.
	Bytes   int64
	Scopes  map[string]int64
	Records []evidence.Stored
}

type Backend interface {
	// LoadWork returns no work only when no eligible component exists. On a
	// failure it preserves dataset/root identity when already known for retry.
	LoadWork(context.Context) (Work, bool, error)
	// PublishProjection atomically fences and publishes. Stale work returns
	// false without mutation; the engine decides when to retry fresh work.
	PublishProjection(context.Context, Work, evidence.Projection) (bool, error)
	// RecordFailure keeps the component pending with durable bounded backoff.
	RecordFailure(context.Context, Work)
}

// ConcurrentBackend selects a complete component disjoint from all exclusions.
// Exclusions include active claims and briefly delayed stale components; only
// their dataset and scope membership are needed, not records or byte estimates.
// A positive byte limit excludes components larger than the remaining budget;
// zero allows a large component to run alone rather than starve indefinitely.
type ConcurrentBackend interface {
	Backend
	LoadWorkExcluding(context.Context, []Work, int64) (Work, bool, error)
}

type Worker struct {
	processing sync.Mutex
	wake       chan struct{}
}

func NewWorker() *Worker { return &Worker{wake: make(chan struct{}, 1)} }

func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Worker) ProcessNext(ctx context.Context, backend Backend) (worked bool, failure error) {
	if !w.processing.TryLock() {
		return false, nil
	}
	defer w.processing.Unlock()
	work, found, err := backend.LoadWork(ctx)
	defer func() {
		if failure != nil && ctx.Err() == nil && work.Root != "" {
			backend.RecordFailure(ctx, work)
		}
	}()
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	projection, err := processor.Process(ctx, work.Records)
	if err != nil {
		return false, err
	}
	if _, err := backend.PublishProjection(ctx, work, projection); err != nil {
		return false, err
	}
	// A fenced stale projection is work: retry with the latest component now.
	return true, nil
}

func (w *Worker) Run(ctx context.Context, backend Backend, report func(error)) {
	if concurrent, ok := backend.(ConcurrentBackend); ok {
		w.runConcurrent(ctx, concurrent, report)
		return
	}
	const retryPollInterval = time.Second
	ticker := time.NewTicker(retryPollInterval)
	defer ticker.Stop()
	for {
		for {
			worked, err := w.ProcessNext(ctx, backend)
			if err != nil {
				if ctx.Err() == nil && report != nil {
					report(err)
				}
				break
			}
			if !worked {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-w.wake:
		case <-ticker.C:
		}
	}
}

func (w *Worker) runConcurrent(ctx context.Context, backend ConcurrentBackend, report func(error)) {
	// One dispatcher owns claims and backend dataset selection. ProcessNext must
	// not load a component already being interpreted by the running dispatcher.
	w.processing.Lock()
	defer w.processing.Unlock()
	const workerCount = 2
	const maxInFlightBytes int64 = 32 << 20
	const retryPollInterval = time.Second
	ticker := time.NewTicker(retryPollInterval)
	defer ticker.Stop()
	type completion struct {
		work      Work
		err       error
		published bool
	}
	completed := make(chan completion, workerCount)
	var workers sync.WaitGroup
	defer workers.Wait()
	claims := make([]Work, 0, workerCount)
	var delayed staleComponents
	timer := time.NewTimer(staleRetryDelay)
	timer.Stop()
	defer timer.Stop()
	reportError := func(err error) {
		if err != nil && ctx.Err() == nil && report != nil {
			report(err)
		}
	}
	for ctx.Err() == nil {
		delayed.prune(time.Now())
		for len(claims) < workerCount && ctx.Err() == nil {
			var used int64
			for _, claim := range claims {
				used += claim.Bytes
			}
			limit := int64(0)
			if len(claims) > 0 {
				limit = maxInFlightBytes - used
				if limit <= 0 {
					break
				}
			}
			work, found, err := backend.LoadWorkExcluding(ctx, delayed.excluding(claims), limit)
			if err != nil {
				if ctx.Err() == nil && work.Root != "" {
					backend.RecordFailure(ctx, work)
				}
				reportError(err)
				// Failure recording can itself fail during disk/database errors.
				// Poll rather than spin selecting the same unacknowledged failure.
				break
			}
			if !found {
				break
			}
			claims = append(claims, work)
			workers.Go(func() {
				projection, err := processor.Process(ctx, work.Records)
				published := false
				if err == nil {
					published, err = backend.PublishProjection(ctx, work, projection)
				}
				if err != nil && ctx.Err() == nil {
					backend.RecordFailure(ctx, work)
				}
				completed <- completion{work: work, err: err, published: published}
			})
		}
		var eligible <-chan time.Time
		if deadline, ok := delayed.next(); ok {
			timer.Reset(time.Until(deadline))
			eligible = timer.C
		}
		select {
		case <-ctx.Done():
			return
		case result := <-completed:
			for index, claim := range claims {
				if claim.DatasetID == result.work.DatasetID && claim.Root == result.work.Root {
					copy(claims[index:], claims[index+1:])
					// Release records held by the unused backing-array slot.
					claims[len(claims)-1] = Work{}
					claims = claims[:len(claims)-1]
					break
				}
			}
			if result.err == nil && !result.published {
				delayed.add(result.work, time.Now())
			}
			reportError(result.err)
		case <-w.wake:
		case <-ticker.C:
		case <-eligible:
		}
	}
}
