package cli

import (
	"context"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"io"
	"path/filepath"
	"testing"
	"time"
)

func TestViewStartupUsesDirectDestinationInsideLoadingScreen(t *testing.T) {
	root := t.TempDir()
	collectorPath, serverPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.duckdb")
	synced := false
	replaceViewCollector(t, func(_ context.Context, options collector.Options) (collector.Result, error) {
		synced = true
		if options.CollectorDBPath != collectorPath || options.ServerDBPath != serverPath {
			t.Fatal("wrong role paths")
		}
		if options.Destination == nil || !options.Destination.Local || options.Destination.Transport == nil || options.EnsureLocal != nil {
			t.Fatal("not direct ingestion")
		}
		if len(options.SyncOptions.Harnesses) != len(pipeline.SupportedHarnesses) || !options.SyncOptions.Normalize {
			t.Fatal("missing harnesses/normalization")
		}
		return collector.Result{}, nil
	})
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		if synced {
			t.Fatal("collected before loading screen")
		}
		dashboard, ok := driveStartupForTest(t, newStartupModel(model)).(interactiveModel)
		if !ok || !synced || dashboard.options.local == nil || dashboard.options.serverURL != "" {
			t.Fatal("wrong runtime")
		}
		return dashboard, nil
	})
	defer restore()
	if err := Run(t.Context(), []string{"tui", "--collector-db-path", collectorPath, "--server-db-path", serverPath}, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
}
