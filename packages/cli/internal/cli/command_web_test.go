package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverruntime"
)

func descriptorServer(t *testing.T, kind serverfeatures.Kind, capabilities []string) *httptest.Server {
	t.Helper()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if kind == serverfeatures.Hosted && r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("hosted preflight missing token")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v3/ingestion/capabilities" {
			_ = json.NewEncoder(w).Encode(evidence.Capabilities{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: "fixture-database", DatasetID: "fixture-dataset", Completion: "acceptance", MaxBodyBytes: evidence.MaxBodyBytes, MaxEntries: evidence.MaxEntries})
			return
		}
		if r.URL.Path != "/api/v2/instance" {
			t.Error("unexpected preflight route", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(api.InstanceResponseV2{ApiVersion: "v2", InstanceId: "fixture-instance", DataEpoch: "fixture-database", DataReadiness: "ready", DatasetId: "fixture-dataset", ServerKind: api.InstanceResponseV2ServerKind(kind), Capabilities: capabilities, Permissions: []api.InstanceResponseV2Permissions{"read", "ingest"}})
	}))
	t.Cleanup(remote.Close)
	return remote
}

func TestWebCompositionOrderingAndQueryOnly(t *testing.T) {
	for _, kind := range []serverfeatures.Kind{serverfeatures.Hosted} {
		for _, queryOnly := range []bool{false, true} {
			t.Run(string(kind)+map[bool]string{true: "/query", false: "/sync"}[queryOnly], func(t *testing.T) {
				remote := descriptorServer(t, kind, []string{"usage", "facets", "web-dashboard", "raw-ingestion"})
				settings := config.Defaults()
				settings.Mode, settings.ServerURL = config.Distributed, remote.URL
				if kind == serverfeatures.Hosted {
					settings.ServerToken = "fixture-token"
				}
				settings.CollectorDBPath = filepath.Join(t.TempDir(), "absent.sqlite")
				var events []string
				previousOpen, previousCollector := openDashboard, runWebCollector
				t.Cleanup(func() { openDashboard, runWebCollector = previousOpen, previousCollector })
				openDashboard = func(url string) error {
					events = append(events, "open")
					if url != remote.URL || strings.Contains(url, "fixture-token") {
						t.Fatal("wrong dashboard URL")
					}
					return errors.New("browser unavailable")
				}
				runWebCollector = func(_ context.Context, options collector.Options) (collector.Result, error) {
					events = append(events, "sync")
					if options.Destination == nil || options.Destination.DatabaseID != "fixture-database" || options.Destination.DatasetID != "fixture-dataset" || options.Destination.Local || options.Destination.Transport == nil {
						t.Fatal("unbound delivery")
					}
					return collector.Result{}, nil
				}
				var stdout, stderr bytes.Buffer
				invocation := commandInvocation{context: t.Context(), stdout: &stdout, stderr: &stderr, settings: &settings, now: time.Now()}
				var args []string
				if queryOnly {
					args = []string{"--sync=false"}
				}
				if err := runWeb(invocation, args); err != nil {
					t.Fatal(err)
				}
				want := []string{"open", "sync"}
				if kind == serverfeatures.Hosted {
					want = []string{"sync", "open"}
				}
				if queryOnly {
					want = []string{"open"}
				}
				if !reflect.DeepEqual(events, want) {
					t.Fatal(events, want)
				}
				assertViewMissingPath(t, settings.CollectorDBPath)
				if !strings.Contains(stdout.String(), remote.URL) || strings.Contains(stdout.String()+stderr.String(), "fixture-token") {
					t.Fatal("dashboard fallback/credential leak")
				}
			})
		}
	}
}

func TestWebSyncFailureStillOpensHostedSavedDashboard(t *testing.T) {
	remote := descriptorServer(t, serverfeatures.Hosted, []string{"usage", "facets", "web-dashboard", "raw-ingestion"})
	settings := config.Defaults()
	settings.Mode, settings.ServerURL, settings.ServerToken = config.Distributed, remote.URL, "fixture-token"
	previousOpen, previousCollector := openDashboard, runWebCollector
	t.Cleanup(func() { openDashboard, runWebCollector = previousOpen, previousCollector })
	expected := errors.New("delivery unavailable")
	opened := false
	openDashboard = func(string) error { opened = true; return nil }
	runWebCollector = func(context.Context, collector.Options) (collector.Result, error) {
		return collector.Result{DeliveryError: expected}, expected
	}
	err := runWeb(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, nil)
	if !errors.Is(err, expected) || !opened {
		t.Fatal("saved dashboard unavailable after sync error", err)
	}
}

func TestViewerCapabilitiesRejectBeforeCollection(t *testing.T) {
	settings := config.Defaults()
	settings.Mode = config.Distributed
	for _, args := range [][]string{nil, {"--sync=false"}} {
		if err := runView(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, args); !errors.Is(err, ErrUsage) {
			t.Fatal("hosted tui accepted", err)
		}
	}
	remote := descriptorServer(t, serverfeatures.Personal, []string{"usage", "facets"})
	settings.Mode, settings.ServerURL = config.Distributed, remote.URL
	if err := runView(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, []string{"--sync=false"}); err == nil || !strings.Contains(err.Error(), "single-process") {
		t.Fatal("disabled terminal enabled", err)
	}
	if err := runWeb(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, []string{"--host", "0.0.0.0"}); !errors.Is(err, ErrUsage) {
		t.Fatal("remote bind accepted", err)
	}
}

func TestConfigTokenStdinAndMaskedGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	var output bytes.Buffer
	invocation := commandInvocation{context: t.Context(), stdin: strings.NewReader("private-token\n"), stdout: &output, stderr: &output, configPath: path}
	if err := runConfig(invocation, []string{"set", "server-token"}); err != nil {
		t.Fatal(err)
	}
	if err := runConfig(invocation, []string{"get", "server-token"}); err != nil || output.String() != "configured\n" {
		t.Fatal("secret disclosure", err)
	}
	if err := runConfig(invocation, []string{"set", "server-token", "secret"}); !errors.Is(err, ErrUsage) {
		t.Fatal("token argument accepted")
	}
}

func TestHostedWebShowsCaptureAndAcceptanceBeforeOpening(t *testing.T) {
	remote := descriptorServer(t, serverfeatures.Hosted, []string{"usage", "facets", "web-dashboard", "raw-ingestion"})
	settings := config.Defaults()
	settings.Mode, settings.ServerURL, settings.ServerToken = config.Distributed, remote.URL, "fixture-token"
	previousOpen, previousCollector := openDashboard, runWebCollector
	t.Cleanup(func() { openDashboard, runWebCollector = previousOpen, previousCollector })
	var stdout, stderr bytes.Buffer
	opened := false
	openDashboard = func(string) error {
		opened = true
		if !strings.Contains(stderr.String(), "Accepted 6 entries in 2 batches; 0 pending.") {
			t.Fatal("browser opened before sync completion")
		}
		return nil
	}
	runWebCollector = func(_ context.Context, options collector.Options) (collector.Result, error) {
		options.SyncOptions.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSyncing})
		if opened || !strings.Contains(stderr.String(), "pi: reading sessions") {
			t.Fatal("capture progress delayed until completion")
		}
		options.SyncOptions.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSynced})
		options.DeliveryProgress(collector.DeliveryProgress{Batches: 1, Accepted: 3, PendingKnown: true, Pending: 3})
		if !strings.Contains(stderr.String(), "Accepted 3 entries in 1 batches; 3 pending.") {
			t.Fatal("acknowledgement not shown live")
		}
		options.DeliveryProgress(collector.DeliveryProgress{Batches: 2, Accepted: 6, PendingKnown: true})
		return collector.Result{Batches: 2, Accepted: 6, PendingKnown: true}, nil
	}
	if err := runWeb(commandInvocation{context: t.Context(), stdout: &stdout, stderr: &stderr, settings: &settings}, nil); err != nil {
		t.Fatal(err)
	}
	if !opened || strings.Contains(stdout.String()+stderr.String(), "fixture-token") || strings.Contains(stderr.String(), "%") {
		t.Fatal("invalid hosted progress", stderr.String())
	}
}

func TestLocalWebOwnsListenerUntilCancellationAndNeverAcceptsHTTPIngestion(t *testing.T) {
	settings := config.Defaults()
	root := t.TempDir()
	settings.ServerDBPath, settings.CollectorDBPath = filepath.Join(root, "server.duckdb"), filepath.Join(root, "collector.sqlite")
	settings.Host, settings.Port = "0.0.0.0", 0
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	previousOpen, previousCollector := openDashboard, runWebCollector
	t.Cleanup(func() { openDashboard, runWebCollector = previousOpen, previousCollector })
	started, finished := make(chan struct{}), make(chan struct{})
	runWebCollector = func(ctx context.Context, options collector.Options) (collector.Result, error) {
		if options.Destination == nil || options.Destination.Transport == nil || !options.Destination.Local {
			t.Error("web used HTTP ingestion")
			close(started)
			close(finished)
			return collector.Result{}, errors.New("web used HTTP ingestion")
		}
		options.SyncOptions.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSyncing})
		close(started)
		defer close(finished)
		<-ctx.Done()
		return collector.Result{}, ctx.Err()
	}
	var address string
	openDashboard = func(url string) error {
		if !strings.HasPrefix(url, "http://127.0.0.1:") {
			t.Fatal("wrong browser address/order", url)
		}
		select {
		case <-started:
		case <-time.After(10 * time.Second):
			t.Fatal("collection did not start")
		}
		select {
		case <-finished:
			t.Fatal("browser waited for collection")
		default:
		}
		address = url
		response, err := http.Get(url + "/api/v2/instance")
		if err != nil {
			t.Fatal(err)
		}
		var instance api.InstanceResponseV2
		if err := json.NewDecoder(response.Body).Decode(&instance); err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatal(response.Status)
		}
		hostname, err := os.Hostname()
		if err != nil || instance.Hostname != hostname {
			t.Fatal("local identity unavailable", instance.Hostname, err)
		}
		for _, capability := range []string{"usage", "facets", "web-dashboard", "collector-progress", "dashboard-reload"} {
			if !slices.Contains(instance.Capabilities, capability) {
				t.Fatal("local web capability missing", capability, instance.Capabilities)
			}
		}
		progress := readWebProgress(t, url)
		if progress.InstanceID != instance.InstanceId || len(progress.Attempts) != 1 || progress.Attempts[0].Stage != "capturing" || progress.Attempts[0].Harnesses["pi"] != "running" {
			t.Fatal("startup progress unavailable", progress)
		}
		for _, path := range []string{"/api/v3/ingestion/batches", "/api/v2/processing/reprocess", "/control/v1/collector-progress", "/api/v2/collector-progress"} {
			response, err = http.Post(url+path, "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode < 400 {
				t.Fatal("public mutation enabled", path)
			}
		}
		cancel()
		return nil
	}
	if err := runWeb(commandInvocation{context: ctx, stdout: io.Discard, stderr: io.Discard, settings: &settings}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("collection not joined")
	}
	client := &http.Client{Timeout: time.Second}
	if response, err := client.Get(address + "/api/v2/instance"); err == nil {
		_ = response.Body.Close()
		t.Fatal("listener survived command")
	}
	runtime, err := localruntime.Open(t.Context(), settings.CollectorDBPath, settings.ServerDBPath)
	if err != nil {
		t.Fatal("command retained database ownership", err)
	}
	_ = runtime.Close()
}

func readWebProgress(t *testing.T, url string) collectorprogress.Snapshot {
	t.Helper()
	response, err := http.Get(url + "/api/v2/collector-progress")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.Status)
	}
	var progress collectorprogress.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&progress); err != nil {
		t.Fatal(err)
	}
	return progress
}

func TestLocalWebCaptureFailureKeepsSavedDashboardAndShowsFailure(t *testing.T) {
	options := localViewOptions(t, true)
	_ = options.local.Close()
	settings := config.Defaults()
	settings.CollectorDBPath, settings.ServerDBPath, settings.Port = options.collectorDBPath, options.dbPath, 0
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	previousOpen, previousCollector := openDashboard, runWebCollector
	t.Cleanup(func() { openDashboard, runWebCollector = previousOpen, previousCollector })
	runWebCollector = func(_ context.Context, options collector.Options) (collector.Result, error) {
		options.SyncOptions.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressFailed})
		return collector.Result{}, errors.New("private source error")
	}
	openDashboard = func(url string) error {
		deadline := time.After(10 * time.Second)
		for {
			progress := readWebProgress(t, url)
			if len(progress.Attempts) == 1 && progress.Attempts[0].Stage == "failed" {
				if progress.Attempts[0].ErrorCode != "collection_failed" {
					t.Fatal("unsafe failure status", progress)
				}
				break
			}
			select {
			case <-deadline:
				t.Fatal("failure not published")
			case <-time.After(time.Millisecond):
			}
		}
		response, err := http.Get(url + "/api/v2/usage?period=all")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		var data api.UsageResponseV2
		if err := json.NewDecoder(response.Body).Decode(&data); err != nil || data.Summary.Total != 100 {
			t.Fatal("saved data lost after capture failure", data, err)
		}
		cancel()
		return nil
	}
	var stderr bytes.Buffer
	if err := runWeb(commandInvocation{context: ctx, stdout: io.Discard, stderr: &stderr, settings: &settings}, nil); err != nil || stderr.Len() != 0 {
		t.Fatal("terminal blocked or capture failure stopped dashboard", err, stderr.String())
	}
}

func TestLocalWebSavedOnlyHasNoStartupCollection(t *testing.T) {
	settings := config.Defaults()
	root := t.TempDir()
	settings.CollectorDBPath, settings.ServerDBPath, settings.Port = filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.duckdb"), 0
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	previousOpen, previousCollector := openDashboard, runWebCollector
	t.Cleanup(func() { openDashboard, runWebCollector = previousOpen, previousCollector })
	runWebCollector = func(context.Context, collector.Options) (collector.Result, error) {
		t.Error("saved-only web collected")
		return collector.Result{}, nil
	}
	openDashboard = func(url string) error {
		if progress := readWebProgress(t, url); len(progress.Attempts) != 0 {
			t.Fatal("saved-only web created startup attempt", progress)
		}
		cancel()
		return nil
	}
	if err := runWeb(commandInvocation{context: ctx, stdout: io.Discard, stderr: io.Discard, settings: &settings}, []string{"--sync=false"}); err != nil {
		t.Fatal(err)
	}
	assertViewMissingPath(t, settings.CollectorDBPath)
}

type failedWebListener struct {
	net.Listener
	err error
}

func (l failedWebListener) Accept() (net.Conn, error) { return nil, l.err }

func TestLocalWebHTTPFailureCancelsAndJoinsCollection(t *testing.T) {
	settings := config.Defaults()
	root := t.TempDir()
	settings.CollectorDBPath, settings.ServerDBPath, settings.Port = filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.duckdb"), 0
	previousCollector, previousServe := runWebCollector, serveLocalWeb
	t.Cleanup(func() { runWebCollector, serveLocalWeb = previousCollector, previousServe })
	finished := make(chan struct{})
	runWebCollector = func(ctx context.Context, _ collector.Options) (collector.Result, error) {
		defer close(finished)
		<-ctx.Done()
		return collector.Result{}, ctx.Err()
	}
	expected := errors.New("listener unavailable")
	serveLocalWeb = func(ctx context.Context, store serverruntime.Readiness, bindings []serverruntime.Binding, ready func() error) error {
		bindings[0].Listener = failedWebListener{Listener: bindings[0].Listener, err: expected}
		return serverruntime.Serve(ctx, store, bindings, ready)
	}
	err := runWeb(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, []string{"--open=false"})
	if !errors.Is(err, expected) {
		t.Fatal("HTTP failure lost", err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("HTTP failure left collector running")
	}
	runtime, err := localruntime.Open(t.Context(), settings.CollectorDBPath, settings.ServerDBPath)
	if err != nil {
		t.Fatal("ownership leaked", err)
	}
	_ = runtime.Close()
}
