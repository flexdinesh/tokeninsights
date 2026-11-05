package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

// Start real private owners inside this test process. Production attachment,
// bridge, query and capture protocols remain unchanged.
func useSharedBackendForTest(t *testing.T) {
	t.Helper()
	previousConnect, previousOpen := connectBackendSession, openViewerDashboard
	owners := make(map[string]bool)
	ensure := func(ctx context.Context, spec desktoplaunch.Spec, prepare func() error) error {
		if owners[spec.ServerDBPath] {
			return nil
		}
		if prepare != nil {
			if err := prepare(); err != nil {
				return err
			}
		}
		backendCtx, cancel := context.WithCancel(t.Context())
		ready, finished := make(chan struct{}), make(chan error, 1)
		go func() {
			finished <- localbackend.Serve(backendCtx, localprotocol.Settings{CollectorDBPath: spec.CollectorDBPath, ServerDBPath: spec.ServerDBPath}, io.Discard,
				func(localprotocol.Descriptor) error { close(ready); return nil })
		}()
		select {
		case err := <-finished:
			cancel()
			return err
		case <-ctx.Done():
			cancel()
			_ = <-finished
			return ctx.Err()
		case <-ready:
		}
		owners[spec.ServerDBPath] = true
		t.Cleanup(func() {
			cancel()
			if err := <-finished; err != nil {
				t.Error(err)
			}
		})
		return nil
	}
	connectBackendSession = func(ctx context.Context, spec desktoplaunch.Spec, prepare func() error) (*localclient.Session, error) {
		if err := ensure(ctx, spec, prepare); err != nil {
			return nil, err
		}
		return previousConnect(ctx, spec, func() error { t.Error("owned storage inspected"); return nil })
	}
	openViewerDashboard = func(ctx context.Context, spec desktoplaunch.Spec, host string, port int, prepare func() error) (*desktoplaunch.Dashboard, error) {
		if err := ensure(ctx, spec, prepare); err != nil {
			return nil, err
		}
		return previousOpen(ctx, spec, host, port, func() error { t.Error("owned storage inspected"); return nil })
	}
	t.Cleanup(func() { connectBackendSession = previousConnect; openViewerDashboard = previousOpen })
}

func TestWebClosingCommandPreservesAnotherViewerAndMaintenance(t *testing.T) {
	useSharedBackendForTest(t)
	root := t.TempDir()
	settings := config.Defaults()
	settings.CollectorDBPath, settings.ServerDBPath = filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	keeper, err := connectBackendSession(t.Context(), desktoplaunch.Specification(settings, false, false, ""), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = keeper.Close() }()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	previous := openDashboard
	t.Cleanup(func() { openDashboard = previous })
	var output bytes.Buffer
	openDashboard = func(_ context.Context, mode browser.Mode, url string) (browser.Result, error) {
		if mode != browser.ModeBrowser || !strings.Contains(output.String(), url) {
			t.Fatal("web did not print bridge URL", mode, output.String())
		}
		for _, path := range []string{"/api/v3/ingestion/batches", "/api/local/v1/reprocess", "/api/local/v1/capture"} {
			response, err := http.Post(url+path, "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode < 400 {
				t.Error("bridge exposed mutation", path)
			}
		}
		cancel()
		return browser.Result{}, nil
	}
	invocation := commandInvocation{context: ctx, stdout: &output, stderr: io.Discard, configPath: localConfigForTest(t, settings)}
	if err := runWeb(invocation, []string{"--capture=false"}); err != nil {
		t.Fatal(err)
	}
	if _, err := keeper.Query.Instance(t.Context()); err != nil {
		t.Fatal("web close stopped another viewer", err)
	}
	invocation.context = t.Context()
	if err := runData(invocation, []string{"reprocess"}); err != nil {
		t.Fatal(err)
	}
}

func TestWebPassesCapturePolicyAndOpenFalse(t *testing.T) {
	previous := openViewerDashboard
	t.Cleanup(func() { openViewerDashboard = previous })
	expected := errors.New("inspected")
	root := t.TempDir()
	openViewerDashboard = func(_ context.Context, spec desktoplaunch.Spec, _ string, _ int, _ func() error) (*desktoplaunch.Dashboard, error) {
		if !spec.Capture || !spec.FullRefresh || spec.SourceDir != root {
			t.Fatal("lost capture controls", spec)
		}
		return nil, expected
	}
	invocation := commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: writeConfigForTest(t, nil)}
	if err := runWeb(invocation, []string{"--full-refresh", "--source-dir", root, "--open=false"}); !errors.Is(err, expected) {
		t.Fatal(err)
	}
}

type cancelOnOutput struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancelOnOutput) Write(body []byte) (int, error) {
	count, err := w.Buffer.Write(body)
	if strings.Contains(w.String(), "Dashboard:") {
		w.cancel()
	}
	return count, err
}

func TestWebOpenFalseHoldsBridgeWithoutOpeningApplication(t *testing.T) {
	useSharedBackendForTest(t)
	root := t.TempDir()
	settings := config.Defaults()
	settings.CollectorDBPath, settings.ServerDBPath = filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output := &cancelOnOutput{cancel: cancel}
	previous := openDashboard
	t.Cleanup(func() { openDashboard = previous })
	openDashboard = func(context.Context, browser.Mode, string) (browser.Result, error) {
		t.Fatal("--open=false opened an application")
		return browser.Result{}, nil
	}
	err := runWeb(commandInvocation{context: ctx, stdout: output, stderr: io.Discard, configPath: localConfigForTest(t, settings)}, []string{"--capture=false", "--open=false"})
	if err != nil || !strings.Contains(output.String(), "Dashboard: http://127.0.0.1:") {
		t.Fatal("web did not print its live loopback bridge", err)
	}
}

func TestWebRejectsUnsafeBindingAndInvalidPortsBeforeStorage(t *testing.T) {
	previous := openViewerDashboard
	t.Cleanup(func() { openViewerDashboard = previous })
	openViewerDashboard = func(context.Context, desktoplaunch.Spec, string, int, func() error) (*desktoplaunch.Dashboard, error) {
		t.Fatal("invalid binding reached backend")
		return nil, nil
	}
	invocation := commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: writeConfigForTest(t, nil)}
	for _, args := range [][]string{{"--port=-1"}, {"--port=65536"}, {"--port=invalid"}, {"--host=0.0.0.0"}} {
		if err := runWeb(invocation, args); err == nil {
			t.Fatal("accepted", args)
		}
	}
}

func TestWebOccupiedPortDoesNotFallback(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	// Bridge bind rejection and lease cleanup are exercised by desktoplaunch's
	// real-backend test; this command preserves the exact explicit port.
	previous := openViewerDashboard
	t.Cleanup(func() { openViewerDashboard = previous })
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	expected := errors.New("address in use")
	openViewerDashboard = func(_ context.Context, _ desktoplaunch.Spec, _ string, selected int, _ func() error) (*desktoplaunch.Dashboard, error) {
		if selected == 0 {
			t.Fatal("port fallback")
		}
		return nil, expected
	}
	if err := runWeb(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: writeConfigForTest(t, nil)}, []string{"--port", port, "--open=false"}); !errors.Is(err, expected) {
		t.Fatal(err)
	}
}
