package localruntime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

// Observer fans progress into an optional publisher. Failures cannot
// invalidate capture or delivery. Counts describe acknowledged work only.
type Observer struct {
	mu      sync.Mutex
	state   collectorprogress.Message
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	publish func(context.Context, collectorprogress.Message) error
}

// newObserver publishes through the command-owned progress registry.
func newObserver(ctx context.Context, publish func(context.Context, collectorprogress.Message) error) *Observer {
	o := &Observer{}
	if publish == nil {
		return o
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return o
	}
	o.ctx, o.cancel = context.WithCancel(ctx)
	o.publish = publish
	o.state = collectorprogress.Message{Operation: "begin", AttemptID: hex.EncodeToString(id[:]), Stage: "waiting", Harnesses: make(map[string]string)}
	if o.publish(o.ctx, o.state) != nil {
		o.cancel()
		o.publish = nil
		return o
	}
	o.done = make(chan struct{})
	go func() {
		defer close(o.done)
		ticker := time.NewTicker(collectorprogress.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-o.ctx.Done():
				return
			case <-ticker.C:
				o.mu.Lock()
				_ = o.publish(o.ctx, collectorprogress.Message{Operation: "heartbeat", AttemptID: o.state.AttemptID})
				o.mu.Unlock()
			}
		}
	}()
	return o
}

func (o *Observer) Collection(event pipeline.SyncProgressEvent) {
	if o.publish == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state.Operation, o.state.Stage = "update", "capturing"
	if event.Status == pipeline.SyncProgressWaiting {
		o.state.Stage = "waiting"
	}
	state := "running"
	switch event.Status {
	case pipeline.SyncProgressSynced:
		state = "complete"
	case pipeline.SyncProgressSkipped:
		state = "skipped"
	case pipeline.SyncProgressFailed:
		state = "failed"
	case pipeline.SyncProgressWaiting:
		state = "waiting"
	}
	if event.Harness != "" {
		o.state.Harnesses[string(event.Harness)] = state
	}
	_ = o.publish(o.ctx, o.state)
}

func (o *Observer) Delivery(progress collector.DeliveryProgress) {
	if o.publish == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state.Operation, o.state.Stage = "update", "submitting"
	o.state.AcknowledgedBatches = progress.Batches
	o.state.AcknowledgedEntries = progress.Accepted
	o.state.PendingKnown, o.state.Pending = progress.PendingKnown, progress.Pending
	_ = o.publish(o.ctx, o.state)
}

func (o *Observer) Finish(result collector.Result, err error) {
	if o.publish == nil {
		return
	}
	o.cancel()
	<-o.done
	o.mu.Lock()
	defer o.mu.Unlock()
	o.state.Operation, o.state.Stage = "finish", "accepted"
	o.state.AcknowledgedEntries, o.state.AcknowledgedBatches = result.Accepted, result.Batches
	o.state.PendingKnown, o.state.Pending = result.PendingKnown, result.Pending
	if err != nil || result.Collection.Quarantined > 0 {
		o.state.Stage, o.state.ErrorCode = "failed", "collection_failed"
		if result.DeliveryError != nil {
			o.state.ErrorCode = "submission_failed"
		}
		if errors.Is(err, context.Canceled) {
			o.state.Stage, o.state.ErrorCode = "interrupted", "cancelled"
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), collectorprogress.HeartbeatInterval)
	defer cancel()
	_ = o.publish(ctx, o.state)
}
