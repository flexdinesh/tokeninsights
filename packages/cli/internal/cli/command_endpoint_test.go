package cli

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/tui"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
)

func endpointConfigForTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	settings := config.Defaults()
	settings.CollectorDBPath, settings.ServerDBPath = filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	t.Setenv("TOKENINSIGHTS_ENDPOINT", "ssh://dee@box")
	return localConfigForTest(t, settings)
}

func TestDataRemoteEndpointUsesSSHAndNeverLocalStorage(t *testing.T) {
	path := endpointConfigForTest(t)
	previousLocal, previousRemote := connectLocalBackend, connectRemoteBackend
	t.Cleanup(func() { connectLocalBackend, connectRemoteBackend = previousLocal, previousRemote })
	connectLocalBackend = func(context.Context, localclient.Options, func() error) (*localclient.Session, error) {
		t.Error("remote endpoint attached local storage")
		return nil, errors.New("local")
	}
	expected := errors.New("remote attached")
	remoteCalls := 0
	connectRemoteBackend = func(_ context.Context, dial localclient.Dialer, options localclient.RemoteOptions) (*localclient.Session, error) {
		remoteCalls++
		if dial == nil || options.Capture {
			t.Error("maintenance requested remote capture or lacked a dialer")
		}
		return nil, expected
	}
	invocation := commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: path}
	if err := runData(invocation, []string{"wait"}); !errors.Is(err, expected) || remoteCalls != 1 {
		t.Fatal("data did not attach remote endpoint", err, remoteCalls)
	}
	if err := runData(invocation, []string{"wait", "--server-db-path", filepath.Join(t.TempDir(), "s.sqlite")}); err == nil || !strings.Contains(err.Error(), "rejects local database path flags") {
		t.Fatal("remote endpoint accepted local storage flag", err)
	}
	connectLocalBackend = func(context.Context, localclient.Options, func() error) (*localclient.Session, error) {
		return nil, expected
	}
	if err := runData(invocation, []string{"wait", "--endpoint=local"}); !errors.Is(err, expected) || remoteCalls != 1 {
		t.Fatal("--endpoint=local did not force local", err, remoteCalls)
	}
}

func TestWebRejectsRemoteEndpointWithoutFallback(t *testing.T) {
	path := endpointConfigForTest(t)
	previous := connectLocalBackend
	t.Cleanup(func() { connectLocalBackend = previous })
	connectLocalBackend = func(context.Context, localclient.Options, func() error) (*localclient.Session, error) {
		t.Error("remote web fell back to local")
		return nil, errors.New("local")
	}
	err := runWeb(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: path}, []string{"--sync=false"})
	if err == nil || !strings.Contains(err.Error(), "ssh://dee@box") {
		t.Fatal("web accepted remote endpoint", err)
	}
}

func TestTUIRemoteEndpointRejectsRelativeSourceBeforeConnecting(t *testing.T) {
	path := endpointConfigForTest(t)
	invocation := commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: path}
	err := tui.Run(tui.Command{Context: t.Context(), Args: []string{"--source-dir", "relative"}, Resolve: invocation.resolveLocalOverrides,
		Connect: func(context.Context, config.LocalSettings, func() error) (tui.Session, error) {
			t.Error("connected despite relative remote source")
			return nil, errors.New("connected")
		}, Stdout: io.Discard, Stderr: io.Discard})
	if !errors.Is(err, localclient.ErrRelativeRemoteSource) {
		t.Fatal("relative remote source accepted", err)
	}
}
