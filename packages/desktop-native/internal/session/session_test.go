package session

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

func TestStandaloneNativeHoldsLocalLeaseAndCapturesOnce(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_DATA_HOME", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("TOKENINSIGHTS_CONFIG_PATH", filepath.Join(root, "config.json"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	settings := config.Defaults()
	ctx, cancel := context.WithCancel(t.Context())
	ready, finished := make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- localbackend.Serve(ctx, localprotocol.Settings{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath}, io.Discard, func(localprotocol.Descriptor) error { close(ready); return nil })
	}()
	select {
	case <-ready:
	case err := <-finished:
		cancel()
		t.Fatal(err)
	}
	defer func() {
		cancel()
		if err := <-finished; err != nil {
			t.Error(err)
		}
	}()
	connected, err := Connect(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connected.Close() }()
	progress, err := connected.Progress(t.Context())
	if err != nil || len(progress.Collection.Attempts) != 1 {
		t.Fatal("standalone launch capture", progress, err)
	}
	if connected.handoff != nil || connected.Info.Transport != "local" || connected.Info.Target != "local" || connected.Info.State != "connected" || connected.Info.Hostname == "" {
		t.Fatalf("standalone identity %+v", connected.Info)
	}
	if err := connected.Acknowledge(); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredOrForeignHandoffDoesNotOpenDefaultStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-config.json")
	if err := os.WriteFile(path, []byte("invalid settings must never be read"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_CONFIG_PATH", path)
	const id = "0123456789abcdef0123456789abcdef"
	if _, err := Connect(t.Context(), "tokeninsights-native://launch?id="+id); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatal("expired handoff resolved default configuration", err)
	}
	if _, err := Connect(t.Context(), "tokeninsights-desktop://launch?id="+id); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatal("native accepted a desktop launch link", err)
	}
}
