package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

func TestStandaloneDesktopHoldsLocalBridgeAndCapturesOnce(t *testing.T) {
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
	connected, err := connectDashboard(t.Context(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connected.Close() }()
	progress, err := connected.Session.Progress(t.Context())
	if err != nil || len(progress.Collection.Attempts) != 1 {
		t.Fatal("standalone launch capture", progress, err)
	}
	if connected.URL == "" || connected.handoff != nil {
		t.Fatal("standalone desktop did not hold bridge")
	}
}
func TestExpiredHandoffDoesNotOpenDefaultStorage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-config.json")
	if err := os.WriteFile(path, []byte("invalid settings must never be read"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_CONFIG_PATH", path)
	_, err := connectDashboard(t.Context(), "tokeninsights-mygo://launch?id=0123456789abcdef0123456789abcdef", nil)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatal("expired handoff resolved default configuration", err)
	}
	if _, err := desktoplaunch.ID("tokeninsights-mygo://open?url=http://localhost"); err == nil {
		t.Fatal("retired URL handoff accepted")
	}
}
func TestStandaloneArgumentsCannotSilentlySelectLocalData(t *testing.T) {
	for _, args := range [][]string{{"--endpoint=ssh://host"}, {"--connection", "ssh://host"}, {"tokeninsights-mygo://open?url=http://localhost"}} {
		if err := validateArguments(args); err == nil {
			t.Fatal("unsupported request fell through to standalone local capture", args)
		}
	}
	if err := validateArguments([]string{"tokeninsights-mygo://launch?id=0123456789abcdef0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}
}
func TestNavigationStaysOnDashboardOrigin(t *testing.T) {
	const origin = "http://127.0.0.1:8765"
	for raw, want := range map[string]navigation{
		"http://127.0.0.1:8765/models": navigateStay,
		"http://127.0.0.1:9999/":       navigateBrowser,
		"http://localhost:8765/":       navigateBrowser,
		"https://example.test/":        navigateBrowser,
		"file:///etc/passwd":           navigateDeny,
		"javascript:alert(1)":          navigateDeny,
	} {
		if got := navigationFor(raw, origin); got != want {
			t.Fatal(raw, got, want)
		}
	}
}
