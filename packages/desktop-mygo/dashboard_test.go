package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

func TestExternalDashboardDoesNotResolveLocalStorage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "invalid-config.json")
	if err := os.WriteFile(path, []byte("invalid local settings"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_CONFIG_PATH", path)
	for _, target := range []string{"https://usage.example.com/tokens", "http://192.0.2.1:8766/"} {
		_, err := connectDashboard(t.Context(), target, nil)
		var external *externalTarget
		if !errors.As(err, &external) || external.URL != target {
			t.Fatalf("external target resolved local settings: %v", err)
		}
	}
}

// serveBackend runs a real backend and a keeper session that holds its dashboard.
func serveBackend(t *testing.T) *localclient.Session {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	settings := localprotocol.Settings{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite"), AppDBPath: filepath.Join(root, "app.sqlite")}
	ctx, cancel := context.WithCancel(t.Context())
	ready, finished := make(chan localprotocol.Descriptor, 1), make(chan error, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		finished <- localbackend.Serve(ctx, settings, io.Discard, func(d localprotocol.Descriptor) error { ready <- d; return nil })
	}()
	t.Cleanup(func() { cancel(); <-joined })
	select {
	case <-ready:
	case err := <-finished:
		t.Fatal(err)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	keeper, err := localclient.Connect(t.Context(), localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, AppDBPath: settings.AppDBPath, Host: "127.0.0.1", Port: 0, Dashboard: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = keeper.Close() })
	return keeper
}

func TestURLHandoffKeepsItsPageAndLaunchCapturePolicy(t *testing.T) {
	keeper := serveBackend(t)
	assertAttempts := func(want int) {
		t.Helper()
		progress, err := keeper.Progress(t.Context())
		if err != nil || len(progress.Collection.Attempts) != want {
			t.Fatalf("capture attempts = %d, want %d: %v", len(progress.Collection.Attempts), want, err)
		}
	}
	assertAttempts(0)
	target := keeper.URL + "/models?period=week"
	connected, err := connectDashboard(t.Context(), target, nil)
	if err != nil {
		t.Fatal("handoff rejected:", err)
	}
	if connected.URL != target {
		t.Fatalf("URL = %q, want the linked page %q", connected.URL, target)
	}
	if err := connected.Close(); err != nil {
		t.Fatal(err)
	}
	// A URL handed off by web inherits web's already-applied capture policy.
	assertAttempts(0)
}

func TestHandoffLeaseKeepsTheDashboardServing(t *testing.T) {
	keeper := serveBackend(t)
	connected, err := connectDashboard(t.Context(), keeper.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connected.Close() }()
	if err := keeper.Close(); err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(connected.URL + "/")
	if err != nil {
		t.Fatal("the window's lease lost the listener:", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "<title>TokenInsights</title>") {
		t.Fatalf("dashboard page: status %d, body %.200q", response.StatusCode, body)
	}
}

func TestPersonalTunnelWithoutPrivateOwnerUsesOriginalBrowserURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/instance" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"v2","serverKind":"personal","instanceId":"tunneled-personal","dataEpoch":"remote-db","datasetId":"default","dataReadiness":"ready","permissions":["read"],"capabilities":["usage","facets","web-dashboard"]}`))
	}))
	defer server.Close()
	target := server.URL + "/models?period=week"
	_, err := connectDashboard(t.Context(), target, nil)
	var external *externalTarget
	if !errors.As(err, &external) || external.URL != target {
		t.Fatalf("personal tunnel lost browser fallback: %v", err)
	}
}

func TestNavigationStaysOnTheDashboardOrigin(t *testing.T) {
	const origin = "http://127.0.0.1:8765"
	for raw, want := range map[string]navigation{
		"http://127.0.0.1:8765/models?period=week": navigateStay,
		"http://127.0.0.1:9999/":                   navigateBrowser,
		"http://localhost:8765/":                   navigateBrowser,
		"https://usage.example.com/":               navigateBrowser,
		"file:///etc/passwd":                       navigateDeny,
		"javascript:alert(1)":                      navigateDeny,
	} {
		if got := navigationFor(raw, origin); got != want {
			t.Errorf("navigationFor(%q) = %d, want %d", raw, got, want)
		}
	}
}
