package collectorprogress

import (
	"context"
	"errors"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func TestDirectObserverTracksAcceptanceAndSanitizesFailures(t *testing.T) {
	for _, test := range []struct {
		name               string
		err, errorDelivery error
		stage, code        string
	}{
		{name: "accepted", stage: "accepted"},
		{name: "capture", err: errors.New("private source path"), stage: "failed", code: "collection_failed"},
		{name: "submission", err: errors.New("private destination"), errorDelivery: errors.New("private credential"), stage: "failed", code: "submission_failed"},
		{name: "cancelled", err: context.Canceled, stage: "interrupted", code: "cancelled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := New("instance")
			observer := NewObserver(t.Context(), func(_ context.Context, message Message) error { return registry.Apply(message) })
			observer.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSyncing})
			observer.Delivery(collector.DeliveryProgress{Accepted: 7, Batches: 2, PendingKnown: true, Pending: 3})
			progress := registry.Snapshot()
			if len(progress.Attempts) != 1 || progress.Attempts[0].Stage != "submitting" || progress.Attempts[0].Harnesses["pi"] != "running" || progress.Attempts[0].AcknowledgedEntries != 7 || progress.Attempts[0].Pending != 3 {
				t.Fatal("capture/acceptance progress lost", progress)
			}
			observer.Finish(collector.Result{Accepted: 7, Batches: 2, PendingKnown: true, DeliveryError: test.errorDelivery}, test.err)
			progress = registry.Snapshot()
			attempt := progress.Attempts[0]
			if attempt.Stage != test.stage || attempt.ErrorCode != test.code || attempt.AcknowledgedEntries != 7 || attempt.Pending != 0 {
				t.Fatal("terminal progress wrong", attempt)
			}
		})
	}
}
