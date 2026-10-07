package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
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
	for _, kind := range []serverfeatures.Kind{serverfeatures.Personal, serverfeatures.Hosted} {
		for _, queryOnly := range []bool{false, true} {
			t.Run(string(kind)+map[bool]string{true: "/query", false: "/sync"}[queryOnly], func(t *testing.T) {
				remote := descriptorServer(t, kind, []string{"usage", "facets", "web-dashboard", "raw-ingestion"})
				settings := config.Defaults()
				settings.ServerKind, settings.ServerURL = kind, remote.URL
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
					if options.Destination == nil || options.Destination.DatabaseID != "fixture-database" || options.Destination.DatasetID != "fixture-dataset" || options.Destination.Local || options.Token != settings.ServerToken {
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
	settings.ServerKind, settings.ServerURL, settings.ServerToken = serverfeatures.Hosted, remote.URL, "fixture-token"
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
	settings.ServerKind = serverfeatures.Hosted
	for _, args := range [][]string{nil, {"--sync=false"}} {
		if err := runView(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, args); !errors.Is(err, ErrUsage) {
			t.Fatal("hosted tui accepted", err)
		}
	}
	remote := descriptorServer(t, serverfeatures.Personal, []string{"usage", "facets"})
	settings.ServerKind, settings.ServerURL = serverfeatures.Personal, remote.URL
	if err := runView(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, settings: &settings}, []string{"--sync=false"}); err == nil || !strings.Contains(err.Error(), "terminal-dashboard") {
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
	settings.ServerKind, settings.ServerURL, settings.ServerToken = serverfeatures.Hosted, remote.URL, "fixture-token"
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
