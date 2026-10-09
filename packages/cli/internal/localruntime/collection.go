package localruntime

import (
	"context"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

// CollectionResult reports capture/submission completion, not query visibility.
type CollectionResult struct {
	Result collector.Result
	Err    error
}

// StartCollection starts command-owned capture without blocking saved-data reads.
// The progress attempt is registered before return; Close cancels and joins it.
func (r *Runtime) StartCollection(ctx context.Context, options pipeline.SyncOptions, run func(context.Context, collector.Options) (collector.Result, error)) <-chan CollectionResult {
	done := make(chan CollectionResult, 1)
	r.lifecycleMu.Lock()
	if r.closing || r.ctx.Err() != nil || ctx.Err() != nil {
		err := ctx.Err()
		if err == nil {
			err = context.Canceled
		}
		r.lifecycleMu.Unlock()
		done <- CollectionResult{Err: err}
		close(done)
		return done
	}
	attempt, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.ctx, cancel)
	observer := r.Observe(attempt)
	options.CaptureProgress = observer.captureProgress()
	progress := options.Progress
	options.DBPath = r.collectorPath
	options.Progress = func(event pipeline.SyncProgressEvent) {
		observer.Collection(event)
		if progress != nil {
			progress(event)
		}
	}
	r.collections.Add(1)
	r.lifecycleMu.Unlock()
	go func() {
		defer r.collections.Done()
		defer close(done)
		defer cancel()
		defer stop()
		result, err := run(attempt, collector.Options{
			CollectorDBPath: r.collectorPath, ServerDBPath: r.dataPath,
			Destination: r.Destination, SyncOptions: options,
			DeliveryProgress: observer.Delivery,
		})
		observer.Finish(result, err)
		done <- CollectionResult{Result: result, Err: err}
	}()
	return done
}
