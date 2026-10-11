package cli

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func TestViewBackgroundCollectionUsesDirectDestination(t *testing.T) {
	root := t.TempDir()
	collectorPath, serverPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	bindings := make(chan collector.Options, 1)
	replaceViewCollector(t, func(_ context.Context, options collector.Options) (collector.Result, error) {
		if options.SyncOptions.CaptureProgress == nil {
			return collector.Result{}, errors.New("TUI detail observer missing")
		}
		options.SyncOptions.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSyncing})
		options.SyncOptions.CaptureProgress(pipeline.CaptureProgressEvent{Harness: pipeline.HarnessPi, Phase: pipeline.CaptureComplete,
			TotalKnown: true, Total: 2, Checked: 2, Captured: 1, Unchanged: 1})
		bindings <- options
		return collector.Result{}, nil
	})
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		var options collector.Options
		select {
		case options = <-bindings:
		case <-time.After(10 * time.Second):
			t.Fatal("background collection missing")
		}
		if options.CollectorDBPath != collectorPath || options.ServerDBPath != serverPath || !options.SyncOptions.FullRefresh || options.SyncOptions.SourceDir != root {
			t.Fatal("wrong role paths")
		}
		if options.Destination == nil || !options.Destination.Local || options.Destination.Transport == nil {
			t.Fatal("not direct ingestion")
		}
		if len(options.SyncOptions.Harnesses) != len(pipeline.SupportedHarnesses) {
			t.Fatal("missing harnesses/normalization")
		}
		dashboard := finishCollectionForTest(t, model)
		if dashboard.options.local == nil || dashboard.options.serverURL != "" || dashboard.refresh.result == nil || dashboard.refresh.result.Err != nil {
			t.Fatal("wrong runtime")
		}
		details := dashboard.options.local.Progress.Details()
		id := details.Collection.Attempts[0].AttemptID
		if capture := details.Captures[id]["pi"]; capture.Phase != collectorprogress.CaptureComplete || capture.Checked != 2 || capture.Captured != 1 || capture.Unchanged != 1 {
			t.Fatal("TUI composition lost source measurements", capture)
		}
		return dashboard, nil
	})
	defer restore()
	if err := Run(t.Context(), []string{"tui", "--collector-db-path", collectorPath, "--server-db-path", serverPath, "--full-refresh", "--source-dir", root}, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
}
