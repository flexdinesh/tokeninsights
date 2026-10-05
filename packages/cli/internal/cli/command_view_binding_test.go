package cli

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

func TestViewExplicitSyncPreservesLocalAndRemoteDestinationBinding(t *testing.T) {
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
			previousSync := runViewSync
			runViewSync = func(_ commandInvocation, args []string) error {
				synced = true
				values := map[string]string{}
				for index := 0; index+1 < len(args); index += 2 {
					values[args[index]] = args[index+1]
				}
				if values["--collector-db-path"] != collectorPath || values["--server-db-path"] != serverPath {
					t.Fatalf("sync role paths=%v", values)
				}
				url, present := values["--server-url"]
				if present != remote || remote && url != queryServer.URL {
					t.Fatalf("sync changed destination binding: remote=%v args=%v", remote, values)
				}
				return nil
			}
			t.Cleanup(func() { runViewSync = previousSync })
			restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
				if !synced || model.options.serverURL != queryServer.URL {
					t.Fatal("viewer started before explicit collection or used wrong query URL")
				}
				return model, nil
			})
			defer restore()
			args := []string{"view", "--sync", "--collector-db-path", collectorPath, "--server-db-path", serverPath}
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
