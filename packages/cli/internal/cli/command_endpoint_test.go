package cli

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/tui"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
)

func TestMaintenanceRejectsTCPBeforeConnecting(t *testing.T) {
	previous := connectBackendSession
	t.Cleanup(func() { connectBackendSession = previous })
	connectBackendSession = func(context.Context, desktoplaunch.Spec, func() error) (*localclient.Session, error) {
		t.Fatal("TCP maintenance connected")
		return nil, nil
	}
	target := endpoint.Endpoint{TCP: &endpoint.TCP{Address: "127.0.0.1:43210", Pin: strings.Repeat("ab", 32), Token: "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE"}}
	invocation := commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: writeConfigForTest(t, nil)}
	if err := runData(invocation, []string{"wait", "--connection", target.ConnectionString()}); err == nil || !strings.Contains(err.Error(), "maintenance") {
		t.Fatal(err)
	}
}

func TestTUIExplicitRemoteRejectsRelativeSourceWithoutFallback(t *testing.T) {
	target, err := endpoint.Parse("ssh://host")
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Defaults()
	settings.Endpoint = target
	called := false
	err = tui.Run(tui.Command{Context: t.Context(), Args: []string{"--source-dir=relative"}, Now: time.Now(), Stdout: io.Discard, Stderr: io.Discard,
		Resolve: func(config.Overrides) (config.LocalSettings, error) { return settings, nil },
		Connect: func(context.Context, config.LocalSettings, func() error) (tui.Session, error) {
			called = true
			return nil, nil
		}})
	if err == nil || called {
		t.Fatal(called, err)
	}
}

func TestTUIInsideSSHStillSelectsLocal(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "192.0.2.1 123 192.0.2.2 22")
	path := writeConfigForTest(t, nil)
	settings, err := config.ResolveLocal(path, true, config.Overrides{})
	if err != nil || !settings.Endpoint.IsLocal() {
		t.Fatal(settings, err)
	}
	if _, err := config.ResolveLocal(path, true, config.Overrides{Connection: stringPointer("ssh://host"), ServerDBPath: stringPointer(filepath.Join(t.TempDir(), "unused.sqlite"))}); err == nil {
		t.Fatal("remote accepted local path")
	}
}
func stringPointer(value string) *string { return &value }
