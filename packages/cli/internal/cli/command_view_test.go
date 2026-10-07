package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

func TestTUIReadsSavedServerUsageWithoutCollection(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		remote, store, requests := newViewQueryServer(t, true)
		replaceViewEnsure(t, func(context.Context, service.Options) (service.State, error) {
			t.Fatal("remote viewer bootstrapped local server")
			return service.State{}, nil
		})
		root := t.TempDir()
		localPath := filepath.Join(root, "local-server.sqlite")
		collectorPath := filepath.Join(root, "collector.sqlite")
		isolateViewSources(t, root)
		restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
			if model.syncing || !model.loading {
				t.Fatalf("expected read loading without collection progress: syncing=%v loading=%v", model.syncing, model.loading)
			}
			if model.options.serverURL != remote.URL {
				t.Fatalf("server URL=%q", model.options.serverURL)
			}
			loaded := model.loadDashboard()
			if loaded.err != nil {
				t.Fatal(loaded.err)
			}
			if len(loaded.rows) != 1 || loaded.rows[0].totalValue != 100 {
				t.Fatalf("saved server facts=%+v", loaded.rows)
			}
			if len(loaded.coverage) != 0 {
				t.Fatalf("server query invented source coverage: %+v", loaded.coverage)
			}
			if loaded.sessionCounts.Shown != 1 || loaded.sessionCounts.Synced != 1 {
				t.Fatalf("session counts=%+v", loaded.sessionCounts)
			}
			return model, nil
		})
		defer restore()
		args := []string{"tui", "--sync=false", "--server-url", remote.URL, "--server-db-path", localPath, "--collector-db-path", collectorPath, "--all-time"}
		var stdout bytes.Buffer
		if err := Run(context.Background(), args, &stdout, io.Discard, time.Now()); err != nil {
			t.Fatal(err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("read-only viewer printed collection output: %q", stdout.String())
		}
		assertViewMissingPath(t, localPath)
		assertViewMissingPath(t, collectorPath)
		if requests.posts.Load() != 0 || requests.gets.Load() == 0 {
			t.Fatalf("requests GET=%d POST=%d", requests.gets.Load(), requests.posts.Load())
		}
		assertCLIQueryCount(t, store.SQL(), "SELECT COUNT(*) FROM ingestion_receipts", 1)
	})
}

func TestViewRemoteDoesNotOpenInvalidLocalDatabasePath(t *testing.T) {
	remote, _, _ := newViewQueryServer(t, false)
	invalidLocal := t.TempDir() // Opening a directory as SQLite would fail.
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		result := model.loadDashboard()
		return model, result.err
	})
	defer restore()
	if err := Run(context.Background(), []string{"tui", "--sync=false", "--server-url", remote.URL, "--server-db-path", invalidLocal}, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestViewQuitCancelsServerReadsWithoutReportingFailure(t *testing.T) {
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		model.cancelSync()
		model.err = context.Canceled
		return model, nil
	})
	defer restore()
	if err := Run(context.Background(), []string{"tui", "--sync=false", "--server-url", "http://127.0.0.1:1"}, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatalf("normal quit reported cancellation: %v", err)
	}
}

func TestViewReturnsProgramAndServerErrors(t *testing.T) {
	for _, programFailure := range []bool{true, false} {
		t.Run(map[bool]string{true: "program", false: "query"}[programFailure], func(t *testing.T) {
			expected := errors.New("viewer failed")
			restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
				if programFailure {
					return model, expected
				}
				model.err = expected
				return model, nil
			})
			defer restore()
			if err := Run(context.Background(), []string{"tui", "--sync=false", "--server-url", "http://127.0.0.1:1"}, io.Discard, io.Discard, time.Now()); !errors.Is(err, expected) {
				t.Fatalf("viewer error=%v", err)
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
	handler := server.NewHandler(context.Background(), path, core, io.Discard, "127.0.0.1")
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

func TestViewDefaultBootstrapsLocalQueryServerWithoutCollection(t *testing.T) {
	remote, _, requests := newViewQueryServer(t, true)
	root := t.TempDir()
	isolateViewSources(t, root)
	serverPath := filepath.Join(root, "server.sqlite")
	collectorPath := filepath.Join(root, "collector.sqlite")
	calls := 0
	replaceViewEnsure(t, func(_ context.Context, options service.Options) (service.State, error) {
		calls++
		if options.DBPath != serverPath {
			t.Fatalf("local server path=%q", options.DBPath)
		}
		return service.State{Running: true, Record: &service.Record{URL: remote.URL}}, nil
	})
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		result := model.loadDashboard()
		if result.err != nil || len(result.rows) != 1 || result.rows[0].totalValue != 100 {
			t.Fatalf("local saved query=%+v", result)
		}
		return model, nil
	})
	defer restore()
	if err := Run(context.Background(), []string{"tui", "--sync=false", "--server-db-path", serverPath, "--collector-db-path", collectorPath, "--all-time"}, io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || requests.posts.Load() != 0 {
		t.Fatalf("local ensures=%d query writes=%d", calls, requests.posts.Load())
	}
	assertViewMissingPath(t, collectorPath)
}

func TestViewLocalStartupFailureDoesNotLaunchViewer(t *testing.T) {
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "")
	expected := errors.New("server unavailable")
	replaceViewEnsure(t, func(context.Context, service.Options) (service.State, error) { return service.State{}, expected })
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		t.Fatal("viewer launched without local server")
		return model, nil
	})
	defer restore()
	if err := Run(context.Background(), []string{"tui", "--sync=false"}, io.Discard, io.Discard, time.Now()); !errors.Is(err, expected) {
		t.Fatalf("startup error=%v", err)
	}
}

func TestViewLocalStartupRequiresDiscoveryRecord(t *testing.T) {
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "")
	replaceViewEnsure(t, func(context.Context, service.Options) (service.State, error) {
		return service.State{Running: true}, nil
	})
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, _ io.Writer) (interactiveModel, error) {
		t.Fatal("viewer launched without query address")
		return model, nil
	})
	defer restore()
	if err := Run(context.Background(), []string{"tui", "--sync=false"}, io.Discard, io.Discard, time.Now()); err == nil {
		t.Fatal("missing record accepted")
	}
}

func replaceViewEnsure(t *testing.T, ensure func(context.Context, service.Options) (service.State, error)) {
	t.Helper()
	previous := ensureViewServer
	ensureViewServer = ensure
	t.Cleanup(func() { ensureViewServer = previous })
}

func newRawViewQueryServer(t *testing.T) (*httptest.Server, *datastore.Store, *viewRequestCounts) {
	t.Helper()
	store, err := datastore.Open(t.Context(), filepath.Join(t.TempDir(), "server.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	handler := server.NewDataHandler(t.Context(), store, io.Discard, "127.0.0.1", "", true)
	counts := &viewRequestCounts{}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			counts.posts.Add(1)
		} else {
			counts.gets.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(remote.Close)
	return remote, store, counts
}
