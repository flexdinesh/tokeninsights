package cli

import (
	"bytes"
	"context"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
)

// The CLI tests exercise a real owner and Unix attachment without depending on
// a built adjacent executable. Real child startup is covered by localclient.
func useSharedBackendForTest(t *testing.T) {
	t.Helper()
	previous := connectLocalBackend
	t.Cleanup(func() { connectLocalBackend = previous })
	connectLocalBackend = func(ctx context.Context, options localclient.Options, prepare func() error) (*localclient.Session, error) {
		if prepare != nil {
			if err := prepare(); err != nil {
				return nil, err
			}
		}
		ownerCtx, cancel := context.WithCancel(t.Context())
		ready, finished := make(chan struct{}), make(chan error, 1)
		settings := localprotocol.Settings{CollectorDBPath: options.CollectorDBPath, ServerDBPath: options.ServerDBPath, AppDBPath: options.AppDBPath, Host: options.Host, Port: options.Port}
		go func() {
			finished <- localbackend.Serve(ownerCtx, settings, io.Discard, func(localprotocol.Descriptor) error { close(ready); return nil })
		}()
		select {
		case err := <-finished:
			cancel()
			return nil, err
		case <-ready:
		case <-ctx.Done():
			cancel()
			<-finished
			return nil, ctx.Err()
		}
		t.Cleanup(func() {
			cancel()
			if err := <-finished; err != nil {
				t.Error(err)
			}
		})
		return localclient.Connect(ctx, options)
	}
}

func TestWebClosingCommandPreservesAnotherViewerAndPrivateMaintenance(t *testing.T) {
	root := t.TempDir()
	settings := config.Defaults()
	settings.CollectorDBPath, settings.ServerDBPath, settings.Port = filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite"), 0
	useSharedBackendForTest(t)
	keeper, err := connectLocalBackend(t.Context(), localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, AppDBPath: settings.ApplicationPath(), Host: settings.Host, Port: settings.Port}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = keeper.Close() }()
	connectLocalBackend = func(ctx context.Context, options localclient.Options, prepare func() error) (*localclient.Session, error) {
		return localclient.ConnectPrepared(ctx, options, func() error { t.Error("attached command inspected owned storage"); return nil })
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	previousOpen := openDashboard
	t.Cleanup(func() { openDashboard = previousOpen })
	var stdout bytes.Buffer
	openDashboard = func(_ context.Context, _ browser.Mode, url string) (browser.Result, error) {
		if url != keeper.URL || !strings.Contains(ansi.Strip(stdout.String()), "Dashboard:  "+url) {
			t.Error("web did not attach to shared dashboard")
		}
		for _, path := range []string{"/api/v3/ingestion/batches", "/api/local/v1/capture", "/api/local/v1/reprocess"} {
			response, err := http.Post(url+path, "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if response.StatusCode < 400 {
				t.Error("public mutation enabled", path)
			}
		}
		cancel()
		return browser.Result{}, nil
	}
	invocation := commandInvocation{context: ctx, stdout: &stdout, stderr: io.Discard, configPath: localConfigForTest(t, settings)}
	if err := runWeb(invocation, []string{"--sync=false"}); err != nil {
		t.Fatal(err)
	}
	if _, err := keeper.Query.Instance(t.Context()); err != nil {
		t.Fatal("closing web stopped another viewer", err)
	}
	invocation.context = t.Context()
	if err := runData(invocation, []string{"reprocess"}); err != nil {
		t.Fatal("maintenance failed while viewer attached", err)
	}
}

func TestWebPassesLaunchCaptureSettings(t *testing.T) {
	root := t.TempDir()
	settings := config.Defaults()
	settings.CollectorDBPath, settings.ServerDBPath = filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	previous := connectLocalBackend
	t.Cleanup(func() { connectLocalBackend = previous })
	expected := errors.New("launch inspected")
	connectLocalBackend = func(_ context.Context, options localclient.Options, _ func() error) (*localclient.Session, error) {
		if !options.Capture || !options.FullRefresh || options.SourceDir != root {
			t.Error("launch capture controls lost", options)
		}
		return nil, expected
	}
	if err := runWeb(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: localConfigForTest(t, settings)}, []string{"--full-refresh", "--source-dir", root}); !errors.Is(err, expected) {
		t.Fatal(err)
	}
}

func TestConfigTokenStdinAndMaskedGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	var output bytes.Buffer
	invocation := commandInvocation{context: t.Context(), stdin: strings.NewReader("private-token\n"), stdout: &output, stderr: &output, configPath: path}
	if err := runConfig(invocation, []string{"set", "distributed.server-token"}); err != nil {
		t.Fatal(err)
	}
	if err := runConfig(invocation, []string{"get", "distributed.server-token"}); err != nil || output.String() != "configured\n" {
		t.Fatal("secret disclosure", err)
	}
	if err := runConfig(invocation, []string{"set", "distributed.server-token", "secret"}); !errors.Is(err, ErrUsage) {
		t.Fatal("token argument accepted")
	}
}

func TestWebReadyOutputRespectsColorEnvironment(t *testing.T) {
	const url = "http://127.0.0.1:43210"
	for _, test := range []struct {
		name     string
		force    string
		noColor  string
		cliColor string
		styled   bool
	}{
		{name: "redirected"},
		{name: "forced color", force: "1", styled: true},
		{name: "no color overrides force", force: "1", noColor: "1"},
		{name: "disabled color", cliColor: "0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CLICOLOR_FORCE", test.force)
			t.Setenv("NO_COLOR", test.noColor)
			t.Setenv("CLICOLOR", test.cliColor)
			var output bytes.Buffer
			printWebReady(&output, url, false)
			text := output.String()
			plain := ansi.Strip(text)
			if (text != plain) != test.styled {
				t.Fatalf("styled = %v, want %v: %q", text != plain, test.styled, text)
			}
			if !strings.Contains(plain, "TokenInsights  ready") || !strings.Contains(plain, "Dashboard:  "+url) || !strings.Contains(plain, "Press Ctrl+C to stop.") {
				t.Fatalf("missing ready information: %q", plain)
			}
		})
	}
}

// Commands parse their own flags once, so every flag form outranks the
// environment, including single-dash forms a separate pre-scan would miss.
func TestWebSingleDashFlagsOverrideInvalidEnvironment(t *testing.T) {
	root := t.TempDir()
	settings := config.Defaults()
	settings.CollectorDBPath, settings.ServerDBPath = filepath.Join(root, "env-collector.sqlite"), filepath.Join(root, "env-server.sqlite")
	path := localConfigForTest(t, settings)
	t.Setenv("TOKENINSIGHTS_PORT", "not-a-port")
	t.Setenv("TOKENINSIGHTS_COLLECTOR_DB_PATH", settings.CollectorDBPath)
	flagCollector, flagServer := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	useSharedBackendForTest(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	previousOpen := openDashboard
	t.Cleanup(func() { openDashboard = previousOpen })
	openDashboard = func(context.Context, browser.Mode, string) (browser.Result, error) {
		cancel()
		return browser.Result{}, nil
	}
	err := Run(ctx, []string{"--config-file", path, "web", "--sync=false", "-port", "0", "-collector-db-path", flagCollector, "-server-db-path", flagServer}, io.Discard, io.Discard, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(flagServer); err != nil {
		t.Fatalf("flag server database not used: %v", err)
	}
	assertViewMissingPath(t, settings.ServerDBPath)
}

func TestWebPortFlagBindsRequestedPort(t *testing.T) {
	useSharedBackendForTest(t)
	for _, form := range []string{"separate", "equals"} {
		t.Run(form, func(t *testing.T) {
			reservation, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			address := reservation.Addr().String()
			_, port, err := net.SplitHostPort(address)
			if err != nil {
				t.Fatal(err)
			}
			if err := reservation.Close(); err != nil {
				t.Fatal(err)
			}
			settings := config.Defaults()
			root := t.TempDir()
			settings.CollectorDBPath, settings.ServerDBPath, settings.Port = filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite"), 0
			path := localConfigForTest(t, settings)
			t.Setenv("TOKENINSIGHTS_PORT", "not-a-port")
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			previousOpen := openDashboard
			t.Cleanup(func() { openDashboard = previousOpen })
			var output bytes.Buffer
			openDashboard = func(_ context.Context, _ browser.Mode, url string) (browser.Result, error) {
				defer cancel()
				if url != "http://"+address || !strings.Contains(ansi.Strip(output.String()), "Dashboard:  "+url) {
					t.Fatalf("dashboard URL = %q, want http://%s; output = %q", url, address, output.String())
				}
				response, err := http.Get(url + "/api/v2/instance")
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = response.Body.Close() }()
				if response.StatusCode != http.StatusOK {
					t.Fatal(response.Status)
				}
				return browser.Result{}, nil
			}
			args := []string{"--sync=false", "--port", port}
			if form == "equals" {
				args = []string{"--sync=false", "--port=" + port}
			}
			if err := Run(ctx, append([]string{"--config-file", path, "web"}, args...), &output, io.Discard, time.Now()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWebRejectsInvalidPortBeforeOpeningStorage(t *testing.T) {
	for _, port := range []string{"-1", "65536", "not-a-port"} {
		t.Run(port, func(t *testing.T) {
			settings := config.Defaults()
			root := t.TempDir()
			settings.CollectorDBPath, settings.ServerDBPath = filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
			err := runWeb(commandInvocation{context: t.Context(), stdout: io.Discard, stderr: io.Discard, configPath: localConfigForTest(t, settings)}, []string{"--port", port})
			if err == nil {
				t.Fatalf("accepted invalid port %q", port)
			}
			assertViewMissingPath(t, settings.CollectorDBPath)
			assertViewMissingPath(t, settings.ServerDBPath)
		})
	}
}
