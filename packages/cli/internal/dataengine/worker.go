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
	Scopes               map[string]int64
	Records              []evidence.Stored
}

type Backend interface {
	// LoadWork returns no work only when no eligible component exists. On a
	// failure it preserves dataset/root identity when already known for retry.
	LoadWork(context.Context) (Work, bool, error)
	// PublishProjection atomically fences and publishes. Stale work returns
	// false without mutation so a fresh component can be loaded immediately.
	PublishProjection(context.Context, Work, evidence.Projection) (bool, error)
	// RecordFailure keeps the component pending with durable bounded backoff.
	RecordFailure(context.Context, Work)
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
