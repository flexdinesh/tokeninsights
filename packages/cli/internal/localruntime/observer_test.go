package localruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

func TestDirectObserverTracksAcceptanceAndSanitizesFailures(t *testing.T) {
	for _, test := range []struct {
		name               string
		err, errorDelivery error
		stage, code        string
		quarantined        int
	}{
		{name: "accepted", stage: "accepted"},
		{name: "quarantined", quarantined: 2, stage: "failed", code: "collection_failed"},
		{name: "capture", err: errors.New("private source path"), stage: "failed", code: "collection_failed"},
		{name: "submission", err: errors.New("private destination"), errorDelivery: errors.New("private credential"), stage: "failed", code: "submission_failed"},
		{name: "cancelled", err: context.Canceled, stage: "interrupted", code: "cancelled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := collectorprogress.New("instance")
			observer := newObserver(t.Context(), func(_ context.Context, message collectorprogress.Message) error { return registry.Apply(message) })
			observer.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSyncing})
			observer.Delivery(collector.DeliveryProgress{Accepted: 7, Batches: 2, PendingKnown: true, Pending: 3})
			progress := registry.Snapshot()
			if len(progress.Attempts) != 1 || progress.Attempts[0].Stage != "submitting" || progress.Attempts[0].Harnesses["pi"] != "running" || progress.Attempts[0].AcknowledgedEntries != 7 || progress.Attempts[0].Pending != 3 {
				t.Fatal("capture/acceptance progress lost", progress)
			}
			observer.Finish(collector.Result{Collection: pipeline.Summary{Quarantined: test.quarantined}, Accepted: 7, Batches: 2, PendingKnown: true, DeliveryError: test.errorDelivery}, test.err)
			progress = registry.Snapshot()
			attempt := progress.Attempts[0]
			if attempt.Stage != test.stage || attempt.ErrorCode != test.code || attempt.AcknowledgedEntries != 7 || attempt.Pending != 0 {
				t.Fatal("terminal progress wrong", attempt)
			}
		})
	}
}

func TestCaptureDetailsRequireLocalOptInAndCollectorCapability(t *testing.T) {
	for _, test := range []struct {
		name             string
		enabled, capable bool
	}{
		{name: "tui", enabled: true, capable: true},
		{name: "web", capable: true},
		{name: "disabled", enabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy, err := serverfeatures.NewLocalViewer(test.capable)
			if err != nil {
				t.Fatal(err)
			}
			runtime := Runtime{Policy: policy, Progress: collectorprogress.New("instance"), captureDetails: test.enabled}
			observer := runtime.Observe(t.Context())
			observer.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessCodex, Status: pipeline.SyncProgressSyncing})
			callback := observer.captureProgress()
			if (callback != nil) != (test.enabled && test.capable) {
				t.Fatal("detail observer ignored composition policy")
			}
			if callback != nil {
				callback(pipeline.CaptureProgressEvent{Harness: pipeline.HarnessCodex, Phase: pipeline.CaptureDiscovering})
				callback(pipeline.CaptureProgressEvent{Harness: pipeline.HarnessCodex, Phase: pipeline.CaptureReading,
					TotalKnown: true, Total: 3, Checked: 1, Captured: 1, Active: 1})
			}
			observer.Finish(collector.Result{}, context.Canceled)
			details := runtime.Progress.Details()
			if test.enabled && test.capable {
				id := details.Collection.Attempts[0].AttemptID
				capture := details.Captures[id]["codex"]
				if capture.Phase != collectorprogress.CaptureInterrupted || capture.Total != 3 || capture.Checked != 1 || capture.Active != 0 {
					t.Fatal("observer lost or fabricated capture work", details)
				}
			} else if len(details.Captures) != 0 {
				t.Fatal("non-TUI composition measured detailed progress", details)
			}
			if !test.capable && len(details.Collection.Attempts) != 0 {
				t.Fatal("disabled collector capability published progress", details)
			}
		})
	}
}
