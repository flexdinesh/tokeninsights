package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func TestTerminalProgressCoalescesWithoutLosingAcknowledgements(t *testing.T) {
	var output bytes.Buffer
	presenter := newTerminalSyncProgress(&output)
	presenter.now = func() time.Time { return time.Unix(1, 0) }
	for range 100 {
		presenter.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressDiscovering})
		presenter.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSyncing})
	}
	for i := int64(1); i <= 100; i++ {
		presenter.Delivery(collector.DeliveryProgress{Batches: i, Accepted: i, PendingKnown: true, Pending: 101 - i})
	}
	presenter.Collection(pipeline.SyncProgressEvent{Harness: pipeline.Harness("secret/path"), Status: pipeline.SyncProgressFailed})
	presenter.Finish(collector.Result{Batches: 100, Accepted: 100, PendingKnown: true, Pending: 1})
	want := "pi: finding sessions\npi: reading sessions\nSubmitting retained evidence...\nAccepted 1 entries in 1 batches; 100 pending.\nAccepted 100 entries in 100 batches; 1 pending.\n"
	if output.String() != want {
		t.Fatal(output.String())
	}
	presenter.Finish(collector.Result{Batches: 100, Accepted: 100, PendingKnown: true, Pending: 1})
	if output.String() != want {
		t.Fatal("duplicate completion printed")
	}
}

func TestTerminalProgressKeepsGlobalWaitAndHarnessStatesDistinct(t *testing.T) {
	var output bytes.Buffer
	presenter := newTerminalSyncProgress(&output)
	for range 10 {
		presenter.Collection(pipeline.SyncProgressEvent{Status: pipeline.SyncProgressWaiting})
		presenter.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessCodex, Status: pipeline.SyncProgressSkipped})
		presenter.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessClaudeCode, Status: pipeline.SyncProgressFailed})
		presenter.Delivery(collector.DeliveryProgress{PendingKnown: true})
	}
	if strings.Count(output.String(), "Waiting for another sync") != 1 || strings.Count(output.String(), "codex: no new usage") != 1 || strings.Count(output.String(), "claude-code: collection failed") != 1 || strings.Count(output.String(), "No new evidence to submit") != 1 {
		t.Fatal(output.String())
	}
}

func TestTerminalProgressReportsQuarantineAsIncomplete(t *testing.T) {
	var output bytes.Buffer
	presenter := newTerminalSyncProgress(&output)
	presenter.Collection(pipeline.SyncProgressEvent{Harness: pipeline.HarnessCodex, Status: pipeline.SyncProgressFailed, Quarantined: 2})
	if output.String() != "codex: incomplete; 2 files quarantined\n" {
		t.Fatal(output.String())
	}
	output.Reset()
	printSummary(&output, "sync", pipeline.Summary{Failed: 1, Quarantined: 2}, false)
	if !strings.Contains(output.String(), "failed=1") || !strings.Contains(output.String(), "quarantined=2") {
		t.Fatal("quarantined sources reported as complete:", output.String())
	}
}
