package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		_, err := connectLocal(t.Context(), target, true)
		var external *externalTarget
		if !errors.As(err, &external) || external.URL != target {
			t.Fatalf("external target resolved local settings: %v", err)
		}
	}
}

func TestNativeURLHandoffPreservesLaunchCapturePolicy(t *testing.T) {
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
	keeper, err := localclient.Connect(t.Context(), localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, AppDBPath: settings.AppDBPath, Host: "127.0.0.1", Dashboard: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = keeper.Close() }()
	assertAttempts := func(want int) {
		t.Helper()
		progress, err := keeper.Progress(t.Context())
		if err != nil || len(progress.Collection.Attempts) != want {
			t.Fatalf("capture attempts = %d, want %d: %v", len(progress.Collection.Attempts), want, err)
		}
	}
	handoff := func() {
		t.Helper()
		// Native window startup requests capture for standalone launch. A URL
		// handed off by web must still inherit web's already-applied policy.
		source, err := connectLocal(t.Context(), keeper.URL, true)
		if err != nil {
			t.Fatal("native handoff rejected launch policy", err)
		}
		if err := source.Close(); err != nil {
			t.Fatal(err)
		}
	}
	assertAttempts(0)
	handoff()
	assertAttempts(0)
	if err := keeper.Capture(t.Context(), false, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	handoff()
	assertAttempts(1)
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
	_, err := connectLocal(t.Context(), target, true)
	var external *externalTarget
	if !errors.As(err, &external) || external.URL != target {
		t.Fatalf("personal tunnel lost browser fallback: %v", err)
	}
}
