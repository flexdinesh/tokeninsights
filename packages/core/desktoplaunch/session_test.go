package desktoplaunch

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
)

func backendFixture(t *testing.T) Spec {
	t.Helper()
	root := t.TempDir()
	spec := Spec{Connection: "local", CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite")}
	ctx, cancel := context.WithCancel(t.Context())
	ready, finished := make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- localbackend.Serve(ctx, localprotocol.Settings{CollectorDBPath: spec.CollectorDBPath, ServerDBPath: spec.ServerDBPath}, io.Discard,
			func(localprotocol.Descriptor) error { close(ready); return nil })
	}()
	select {
	case <-ready:
	case err := <-finished:
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		if err := <-finished; err != nil {
			t.Error(err)
		}
	})
	return spec
}

func TestWindowOwnsExactSessionBridgeAndCapturePolicy(t *testing.T) {
	spec := backendFixture(t)
	first, err := Open(t.Context(), spec, "127.0.0.1", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	progress, err := first.Session.Progress(t.Context())
	if err != nil || len(progress.Collection.Attempts) != 0 {
		t.Fatal("saved-data launch captured", progress, err)
	}
	spec.Capture = true
	spec.SourceDir = t.TempDir()
	second, err := Open(t.Context(), spec, "127.0.0.1", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	if first.URL == second.URL || first.Session.InstanceID != second.Session.InstanceID {
		t.Fatal("windows did not receive distinct bridges on the same owner")
	}
	if err := second.Session.WaitVisible(t.Context()); err != nil {
		t.Fatal(err)
	}
	progress, err = second.Session.Progress(t.Context())
	if err != nil || len(progress.Collection.Attempts) != 1 {
		t.Fatal("handoff did not preserve one capture", progress, err)
	}
	response, err := http.Get(second.URL + "/api/viewer/connection")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var info endpoint.Info
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	hostname, err := os.Hostname()
	if err != nil || info.Transport != "local" || info.Target != "local" || info.State != "connected" || info.Hostname != hostname {
		t.Fatal(info, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Session.Query.Instance(t.Context()); err != nil {
		t.Fatal("closing one window stopped another", err)
	}
}

func TestOccupiedBridgePortReleasesOnlyFailedViewer(t *testing.T) {
	spec := backendFixture(t)
	keeper, err := Connect(t.Context(), spec, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = keeper.Close() }()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("unexpected listener address")
	}
	if _, err := Open(t.Context(), spec, "127.0.0.1", address.Port, nil); err == nil {
		t.Fatal("occupied bridge port selected fallback")
	}
	if _, err := keeper.Query.Instance(t.Context()); err != nil {
		t.Fatal("failed bridge closed another session", err)
	}
}

func TestExplicitRemoteFailureDoesNotPrepareLocalStorage(t *testing.T) {
	tcp := endpoint.Endpoint{TCP: &endpoint.TCP{Address: "127.0.0.1:1", Pin: "abababababababababababababababababababababababababababababababab", Token: "YWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWE"}}
	for _, target := range []string{"ssh://-unsafe", "tokeninsights://connect/v1?address=127.0.0.1:1#pin=bad&token=secret", tcp.ConnectionString()} {
		if _, err := Connect(t.Context(), Spec{Connection: target}, func() error { t.Fatal("remote failure prepared local stores"); return nil }); err == nil {
			t.Fatal("invalid target connected")
		}
	}
}

func TestNativeHandoffOwnsRequestedLiveSessionAfterLauncherReturns(t *testing.T) {
	spec := backendFixture(t)
	connected := make(chan *Dashboard, 1)
	failures := make(chan error, 1)
	err := launch(t.Context(), browser.DesktopApp, spec, func(ctx context.Context, raw string) error {
		go func() {
			handoff, err := Receive(ctx, browser.DesktopApp, raw)
			if err != nil {
				failures <- err
				return
			}
			defer func() { _ = handoff.Close() }()
			// The desktop owns a window-scoped context, independent of the finite CLI
			// rendezvous context, which is cancelled when Launch returns.
			dashboard, err := Open(t.Context(), handoff.Spec, "127.0.0.1", 0, nil)
			if err != nil {
				failures <- err
				return
			}
			if err := handoff.Acknowledge(browser.ModeDesktop); err != nil {
				_ = dashboard.Close()
				failures <- err
				return
			}
			connected <- dashboard
		}()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var dashboard *Dashboard
	select {
	case dashboard = <-connected:
	case err := <-failures:
		t.Fatal(err)
	}
	defer func() { _ = dashboard.Close() }()
	if _, err := dashboard.Session.Query.Instance(t.Context()); err != nil {
		t.Fatal("launcher exit released the desktop session", err)
	}
	progress, err := dashboard.Session.Progress(t.Context())
	if err != nil || len(progress.Collection.Attempts) != 0 {
		t.Fatal("handoff changed capture=false policy", progress, err)
	}
}

func TestExistingDesktopHandoffsKeepDistinctRequestedBackends(t *testing.T) {
	firstSpec, secondSpec := backendFixture(t), backendFixture(t)
	windows := make(chan *Dashboard, 2)
	opener := func(ctx context.Context, raw string) error {
		go func() {
			handoff, err := Receive(ctx, browser.DesktopApp, raw)
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = handoff.Close() }()
			dashboard, err := Open(t.Context(), handoff.Spec, "127.0.0.1", 0, nil)
			if err != nil {
				t.Error(err)
				return
			}
			if err := handoff.Acknowledge(browser.ModeDesktop); err != nil {
				_ = dashboard.Close()
				t.Error(err)
				return
			}
			windows <- dashboard
		}()
		return nil
	}
	for _, spec := range []Spec{firstSpec, secondSpec} {
		if err := launch(t.Context(), browser.DesktopApp, spec, opener); err != nil {
			t.Fatal(err)
		}
	}
	first, second := <-windows, <-windows
	defer func() { _ = first.Close(); _ = second.Close() }()
	if first.Session.InstanceID == second.Session.InstanceID || first.URL == second.URL {
		t.Fatal("second handoff reused the first window's backend or bridge")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Session.Query.Instance(t.Context()); err != nil {
		t.Fatal("closing first handoff disrupted the second target", err)
	}
}
