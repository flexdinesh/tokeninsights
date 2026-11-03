package main

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

func TestDesktopURLHandoffPreservesLaunchCapturePolicy(t *testing.T) {
	root := t.TempDir()
	// Even a regression must never read the developer's real harness sources.
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
		session, err := connect(t.Context(), []string{"connect", "--json", "--url", keeper.URL})
		if err != nil {
			t.Fatal("desktop handoff rejected launch policy", err)
		}
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
	}
	assertAttempts(0)
	handoff()
	assertAttempts(0) // web --sync=false must remain saved-data only.
	if err := keeper.Capture(t.Context(), false, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// Whether the custom scan is still active or already done, handoff must
	// neither reject its source nor start a second scan of default sources.
	handoff()
	assertAttempts(1)
}
