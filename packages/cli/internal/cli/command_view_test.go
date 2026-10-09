package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
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

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
)

func localViewOptions(t *testing.T, withUsage bool) tableOptions {
	t.Helper()
	root := t.TempDir()
	options := tableOptions{dbPath: filepath.Join(root, "server.duckdb"), collectorDBPath: filepath.Join(root, "collector.sqlite"), period: periodAllTime, bucket: bucketDay}
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
		if _, err := runtime.Store.Accept(t.Context(), body); err != nil {
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

func TestViewRejectsRemoteBeforeOpeningLocalDatabase(t *testing.T) {
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
	options, err := parseTableOptions([]string{"--server-url", "https://example.test", "--server-db-path", "server.sqlite", "--collector-db-path", "collector.sqlite", "--all-time", "--provider", "fixture-provider", "--model", "fixture-model", "--harness", "pi", "--session-id", "fixture-session"}, io.Discard, false, periodMonth)
	if err != nil {
		t.Fatal(err)
	}
	if options.serverURL != "https://example.test" || options.dbPath != "server.sqlite" || options.collectorDBPath != "collector.sqlite" || !options.syncBeforeView {
		t.Fatalf("wrong remote options: %+v", options)
	}
	expected := filters{providers: stringList{"fixture-provider"}, models: stringList{"fixture-model"}, harnesses: stringList{"pi"}, sessionIDs: stringList{"fixture-session"}}
	if !reflect.DeepEqual(options.filters, expected) {
		t.Fatalf("filters=%+v, want %+v", options.filters, expected)
	}
}

type viewRequestCounts struct{ gets, posts atomic.Int64 }

func newViewQueryServer(t *testing.T, withUsage bool) (*httptest.Server, *serverstore.Store, *viewRequestCounts) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "query-server.sqlite")
	store, err := serverstore.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	core := ingestion.NewCore(store)
	if withUsage {
		metadata, err := store.Metadata(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		occurred := int64(1767225600000)
		fact := publication.Fact{Harness: "pi", Session: publication.Session{Harness: "pi", NativeID: "fixture-view-session", FirstOccurredAtMs: occurred, LastOccurredAtMs: occurred}, Message: &publication.Message{NativeID: "fixture-view-request", OccurredAtMs: occurred}, OccurredAtMs: occurred, Provider: "fixture-provider", ProviderSource: "explicit", Model: "fixture-model", UsageScope: "message", Quality: "exact", Countable: true, InputTokens: 80, OutputTokens: 20, TotalTokens: 100}
		publication.SetIDs(&fact)
		batch := publication.Batch{ProtocolVersion: publication.ProtocolVersion, IdentityVersion: publication.IdentityVersion, SemanticsVersion: publication.SemanticsVersion, DatabaseID: metadata.DatabaseID, StreamID: "view-fixture-stream", BatchID: "view-fixture-batch", FromSequence: 1, ToSequence: 1, Hostname: "fixture-producer", Entries: []publication.Entry{{Sequence: 1, Fact: fact}}}
		body, err := publication.EncodeBatch(batch)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := core.Ingest(context.Background(), body); err != nil {
			t.Fatal(err)
		}
	}
	handler := legacyViewV2Adapter(server.NewHandler(context.Background(), path, core, io.Discard, "127.0.0.1"))
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

// These tests retain legacy ingestion/component oracles. Only their test read
// transport is adapted; production clients require the current descriptor.
func legacyViewV2Adapter(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/ingestion/capabilities" {
			clone := r.Clone(r.Context())
			url := *r.URL
			url.Path = "/api/v1/instance"
			clone.URL = &url
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, clone)
			var instance struct {
				DataEpoch string `json:"dataEpoch"`
			}
			if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &instance) != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(evidence.Capabilities{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: instance.DataEpoch, DatasetID: "default", Completion: "acceptance", MaxBodyBytes: evidence.MaxBodyBytes, MaxEntries: evidence.MaxEntries})
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v2/") {
			handler.ServeHTTP(w, r)
			return
		}
		clone := r.Clone(r.Context())
		url := *r.URL
		url.Path = strings.Replace(url.Path, "/api/v2/", "/api/v1/", 1)
		if url.Path == "/api/v1/status" {
			url.Path = "/api/v1/sync"
		}
		clone.URL = &url
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, clone)
		if recorder.Code != http.StatusOK {
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(recorder.Body.Bytes())
			return
		}
		var body map[string]any
		if json.Unmarshal(recorder.Body.Bytes(), &body) != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		body["datasetId"] = "default"
		if url.Path == "/api/v1/instance" {
			body["apiVersion"], body["serverKind"] = "v2", "personal"
			body["capabilities"] = []string{"usage", "facets", "web-dashboard", "raw-ingestion", "terminal-dashboard"}
			body["permissions"] = []string{"read", "ingest"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
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
