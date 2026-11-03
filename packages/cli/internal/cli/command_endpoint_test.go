package cli

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/tui"
	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
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

func TestWebRemoteEndpointRejectsDesktopOpenAndRelativeSourceWithoutFallback(t *testing.T) {
	path := endpointConfigForTest(t)
	previousLocal, previousRemote := connectLocalBackend, connectRemoteBackend
	t.Cleanup(func() { connectLocalBackend, connectRemoteBackend = previousLocal, previousRemote })
	connectLocalBackend = func(context.Context, localclient.Options, func() error) (*localclient.Session, error) {
		t.Error("remote web fell back to local")
		return nil, errors.New("local")
	}
	connectRemoteBackend = func(context.Context, localclient.Dialer, localclient.RemoteOptions) (*localclient.Session, error) {
		t.Error("remote web connected despite invalid options")
		return nil, errors.New("remote")
	}
	invocation := commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: path}
	for _, mode := range []string{"mygo", "electron"} {
		if err := runWeb(invocation, []string{"--sync=false", "--open=" + mode}); err == nil || !strings.Contains(err.Error(), "opens only the browser") {
			t.Fatal("remote web accepted desktop open mode", mode, err)
		}
	}
	if err := runWeb(invocation, []string{"--source-dir", "relative", "--open=false"}); !errors.Is(err, localclient.ErrRelativeRemoteSource) {
		t.Fatal("remote web accepted relative source", err)
	}
}

func TestWebRemoteEndpointProxiesReadOnlyDashboard(t *testing.T) {
	path := endpointConfigForTest(t)
	root := t.TempDir()
	// The "remote" backend is a real owner reached through its private socket,
	// exactly what the SSH bridge splices.
	remote := localclient.Options{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite"), AppDBPath: filepath.Join(root, "app.sqlite")}
	useSharedBackendForTest(t)
	keeper, err := connectLocalBackend(t.Context(), remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = keeper.Close() }()
	previousRemote, previousOpen := connectRemoteBackend, openDashboard
	t.Cleanup(func() { connectRemoteBackend, openDashboard = previousRemote, previousOpen })
	socket, err := localprotocol.SocketPath(localprotocol.Settings{ServerDBPath: remote.ServerDBPath})
	if err != nil {
		t.Fatal(err)
	}
	connectRemoteBackend = func(ctx context.Context, dial localclient.Dialer, options localclient.RemoteOptions) (*localclient.Session, error) {
		if dial == nil || options.Capture {
			t.Error("remote web lost dialer or --sync=false")
		}
		// Like an SSH bridge: one HTTP/2 connection that cannot be redialed.
		used := false
		single := func(ctx context.Context) (net.Conn, error) {
			if used {
				return nil, errors.New("single-use bridge redialed")
			}
			used = true
			return localclient.SocketDialer(socket)(ctx)
		}
		return localclient.ConnectRemote(ctx, single, options)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	opened := false
	openDashboard = func(_ context.Context, mode browser.Mode, url string) (browser.Result, error) {
		defer cancel()
		opened = true
		if mode != browser.ModeBrowser || !strings.HasPrefix(url, "http://127.0.0.1:") {
			t.Error("remote web opened wrong viewer", mode, url)
		}
		get := func(target string, header http.Header) *http.Response {
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			for key, values := range header {
				request.Header[key] = values
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			return response
		}
		instance := get(url+"/api/v2/instance", nil)
		body, _ := io.ReadAll(instance.Body)
		_ = instance.Body.Close()
		if instance.StatusCode != http.StatusOK || !strings.Contains(string(body), keeper.InstanceID) {
			t.Error("proxied instance unavailable", instance.StatusCode)
		}
		page := get(url+"/", nil)
		_ = page.Body.Close()
		if page.StatusCode != http.StatusOK {
			t.Error("proxied dashboard page unavailable", page.StatusCode)
		}
		lease := get(url+"/api/local/v1/viewers", http.Header{"Origin": {url}})
		if lease.StatusCode != http.StatusOK || lease.Header.Get("Content-Type") != "text/event-stream" {
			t.Error("browser lease not proxied", lease.StatusCode)
		}
		_ = lease.Body.Close()
		for _, path := range []string{"/api/v3/ingestion/batches", "/api/local/v1/capture", "/api/local/v1/reprocess"} {
			response, err := http.Post(url+path, "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode < 400 {
				t.Error("proxy enabled mutation", path)
			}
		}
		descriptor := get(url+"/api/local/v1/descriptor", nil)
		_ = descriptor.Body.Close()
		if descriptor.StatusCode != http.StatusNotFound {
			t.Error("proxy exposed private descriptor", descriptor.StatusCode)
		}
		return browser.Result{}, nil
	}
	invocation := commandInvocation{context: ctx, stdout: io.Discard, stderr: io.Discard, configPath: path}
	if err := runWeb(invocation, []string{"--sync=false", "--port", "0", "--open=auto"}); err != nil {
		t.Fatal(err)
	}
	if !opened {
		t.Fatal("remote web never opened its dashboard")
	}
	if _, err := keeper.Query.Instance(t.Context()); err != nil {
		t.Fatal("closing remote web stopped the remote owner", err)
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
