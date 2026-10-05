package cli

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

func TestViewStartupSyncPreservesLocalAndRemoteDestinationBinding(t *testing.T) {
	for _, remote := range []bool{false, true} {
		name := "local"
		if remote {
			name = "remote"
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv("TOKENINSIGHTS_SERVER_URL", "")
			queryServer, _, _ := newViewQueryServer(t, false)
			root := t.TempDir()
			collectorPath, serverPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
			ensured := 0
			replaceViewEnsure(t, func(_ context.Context, options service.Options) (service.State, error) {
				ensured++
				if remote {
					t.Fatal("remote viewer attempted local startup")
				}
				if options.DBPath != serverPath {
					t.Fatalf("local server path=%q", options.DBPath)
				}
				return service.State{Running: true, Record: &service.Record{URL: queryServer.URL}}, nil
			})
			synced := false
			replaceViewCollector(t, func(ctx context.Context, options collector.Options) (collector.Result, error) {
				synced = true
				if options.CollectorDBPath != collectorPath || options.ServerDBPath != serverPath {
					t.Fatal("wrong role paths")
				}
				if remote && options.ServerURL != queryServer.URL || !remote && options.ServerURL != "" {
					t.Fatal("destination binding changed")
				}
				if !remote {
					url, err := options.EnsureLocal(ctx)
					if err != nil || url != queryServer.URL {
						t.Fatal("local publication endpoint changed")
					}
				}
				if len(options.SyncOptions.Harnesses) != len(pipeline.SupportedHarnesses) || !options.SyncOptions.Normalize {
					t.Fatal("startup did not select all harnesses and normalization")
				}
				return collector.Result{}, nil
			})
			restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
				if synced {
					t.Fatal("sync ran before entering TUI")
				}
				final := driveStartupForTest(t, newStartupModel(model))
				dashboard, ok := final.(interactiveModel)
				if !ok || !synced || dashboard.options.serverURL != queryServer.URL {
					t.Fatal("startup failed to open intended dashboard")
				}
				return dashboard, nil
			})
			defer restore()
			args := []string{"tui", "--collector-db-path", collectorPath, "--server-db-path", serverPath}
			if remote {
				args = append(args, "--server-url", queryServer.URL)
			}
			if err := Run(t.Context(), args, io.Discard, io.Discard, time.Now()); err != nil {
				t.Fatal(err)
			}
			if remote && ensured != 0 || !remote && ensured != 1 {
				t.Fatalf("local startup calls=%d remote=%v", ensured, remote)
			}
		})
	}
}
