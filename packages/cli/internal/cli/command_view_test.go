package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
)

func localViewOptions(t *testing.T, withUsage bool) tableOptions {
	t.Helper()
	root := t.TempDir()
	options := tableOptions{dbPath: filepath.Join(root, "server.sqlite"), collectorDBPath: filepath.Join(root, "collector.sqlite"), period: periodAllTime, bucket: bucketDay}
	runtime, err := localruntime.Open(t.Context(), options.collectorDBPath, options.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	options.local = runtime
	t.Cleanup(func() { _ = runtime.Close() })
	if withUsage {
		metadata, err := runtime.Store.Metadata(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2,
			Data:    json.RawMessage(`{"type":"message","id":"request","message":{"role":"assistant","timestamp":1767225600000,"usage":{"input":80,"output":20,"totalTokens":100}}}`),
			Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
		body, err := json.Marshal(evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 1, Entries: []evidence.Entry{{Sequence: 1, Record: record}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.Store.Accept(t.Context(), evidence.ProtocolVersion, body); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Second*10)
		defer cancel()
		if err := runtime.WaitVisible(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return options
}

func TestTUIReadsSavedUsageDirectlyWithoutCollection(t *testing.T) {
	options := localViewOptions(t, true)
	if err := options.local.Close(); err != nil {
		t.Fatal(err)
	}
	replaceViewCollector(t, func(context.Context, collector.Options) (collector.Result, error) {
		t.Fatal("query-only collected")
		return collector.Result{}, nil
	})
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		if model.options.local == nil || model.options.serverURL != "" {
			t.Fatal("not direct query")
		}
		hostname, err := os.Hostname()
		if err != nil || !strings.Contains(model.statusline.View(1000), "hostname: "+hostname) {
			t.Fatal("TUI initial hostname unavailable", err)
		}
		loaded := model.loadDashboard()
		if loaded.err != nil {
			t.Fatal(loaded.err)
		}
		if loaded.hostname != hostname {
			t.Fatal("TUI query hostname changed", loaded.hostname)
		}
		if len(loaded.coverage) != 0 {
			t.Fatal("invented coverage")
		}
		if len(loaded.rows) != 1 || loaded.rows[0].totalValue != 100 || loaded.sessionCounts.Shown != 1 || loaded.sessionCounts.Synced != 1 {
			t.Fatalf("saved usage: %+v", loaded)
		}
		return model, nil
	})
	defer restore()
	if err := Run(t.Context(), []string{"tui", "--sync=false", "--all-time", "--server-db-path", options.dbPath, "--collector-db-path", options.collectorDBPath}, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
	assertViewMissingPath(t, options.collectorDBPath)
}

func TestViewRejectsRemoteFlagsBeforeOpeningLocalDatabase(t *testing.T) {
	invalidLocal := t.TempDir()
	err := Run(t.Context(), []string{"tui", "--sync=false", "--server-url", "https://remote.test", "--server-db-path", invalidLocal}, io.Discard, io.Discard, time.Now())
	if !errors.Is(err, ErrUsage) {
		t.Fatal(err)
	}
}

func TestViewQuitCancelsReadsWithoutReportingFailure(t *testing.T) {
	options := localViewOptions(t, false)
	_ = options.local.Close()
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		model.cancelSync()
		model.err = context.Canceled
		return model, nil
	})
	defer restore()
	if err := Run(t.Context(), []string{"tui", "--sync=false", "--server-db-path", options.dbPath, "--collector-db-path", options.collectorDBPath}, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestViewReturnsProgramAndQueryErrors(t *testing.T) {
	for _, programFailure := range []bool{true, false} {
		t.Run(fmt.Sprint(programFailure), func(t *testing.T) {
			options := localViewOptions(t, false)
			_ = options.local.Close()
			expected := errors.New("viewer failed")
			restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
				if programFailure {
					return model, expected
				}
				model.err = expected
				return model, nil
			})
			defer restore()
			if err := Run(t.Context(), []string{"tui", "--sync=false", "--server-db-path", options.dbPath, "--collector-db-path", options.collectorDBPath}, io.Discard, io.Discard, time.Now()); !errors.Is(err, expected) {
				t.Fatal(err)
			}
		})
	}
}

func TestTUIRejectsRemovedNoSyncFlagBeforeSideEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sqlite")
	err := Run(context.Background(), []string{"tui", "--sync", "--no-sync", "--server-db-path", path}, io.Discard, io.Discard, time.Now())
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("expected usage error, got %v", err)
	}
	assertViewMissingPath(t, path)
}

func TestViewParsesIndependentServerCollectorAndFilterOptions(t *testing.T) {
	options, err := parseTableOptions([]string{"--server-db-path", "server.sqlite", "--collector-db-path", "collector.sqlite", "--all-time", "--provider", "fixture-provider", "--model", "fixture-model", "--harness", "pi", "--session-id", "fixture-session"}, io.Discard, false, periodMonth)
	if err != nil {
		t.Fatal(err)
	}
	if options.dbPath != "server.sqlite" || options.collectorDBPath != "collector.sqlite" || !options.syncOnStart {
		t.Fatalf("wrong remote options: %+v", options)
	}
	expected := filters{providers: stringList{"fixture-provider"}, models: stringList{"fixture-model"}, harnesses: stringList{"pi"}, sessionIDs: stringList{"fixture-session"}}
	if !reflect.DeepEqual(options.filters, expected) {
		t.Fatalf("filters=%+v, want %+v", options.filters, expected)
	}
}

type viewRequestCounts struct{ gets, posts atomic.Int64 }

func newViewQueryServer(t *testing.T, withUsage bool) (*httptest.Server, *datastore.Store, *viewRequestCounts) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "query-server.sqlite")
	store, err := datastore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if withUsage {
		insertLoadRowsCanonicalTokenWithCounts(t, store.SQL(), 1767225600000, "pi", "fixture-view-session", "fixture-provider", "fixture-model", 80, 20, 0, 0, 0, 100)
	}
	handler := server.NewDataHandler(t.Context(), sqlanalytics.Source{Store: store}, io.Discard, "127.0.0.1", "", true)
	requests := &viewRequestCounts{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			requests.gets.Add(1)
		case http.MethodPost:
			requests.posts.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(remote.Close)
	return remote, store, requests
}

func isolateViewSources(t *testing.T, root string) {
	t.Helper()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "xdg"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "")
	t.Setenv("TOKENINSIGHTS_SERVER_TOKEN", "")
}

func assertViewMissingPath(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only remote view touched %s: %v", path, err)
	}
}

func TestViewOwnedDatabaseDoesNotLaunchSecondViewer(t *testing.T) {
	options := localViewOptions(t, false)
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		t.Fatal("second viewer started")
		return model, nil
	})
	defer restore()
	if err := Run(t.Context(), []string{"tui", "--sync=false", "--server-db-path", options.dbPath, "--collector-db-path", options.collectorDBPath}, io.Discard, io.Discard, time.Now()); !errors.Is(err, localruntime.ErrOwned) {
		t.Fatal(err)
	}
}
