package cli

import (
	"bytes"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/tui"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
)

func TestBrowseOpensConfiguredURLWithoutCaptureAuthenticationOrStorage(t *testing.T) {
	root := t.TempDir()
	isolateViewSources(t, root)
	remote := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("browse contacted server") }))
	defer remote.Close()
	path := filepath.Join(root, "config.json")
	body := `{"collector":{"db-path":"missing/collector.sqlite"},"in-process":{"host":"invalid","port":-1},"distributed":{"server-url":"` + remote.URL + `/","server-token":"invalid token"}}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("TOKENINSIGHTS_SERVER_URL"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_ACCESS_TOKEN", "bad remote token")
	t.Setenv("TOKENINSIGHTS_PORT", "invalid")
	previous := openDashboard
	defer func() { openDashboard = previous }()
	var opened string
	openDashboard = func(url string) error { opened = url; return errors.New("unavailable") }
	var stdout, stderr bytes.Buffer
	if err := Run(t.Context(), []string{"--config-file", path, "browse"}, &stdout, &stderr, time.Now()); err != nil {
		t.Fatal(err)
	}
	if opened != remote.URL || stdout.String() != "Dashboard: "+remote.URL+"\n" || !strings.Contains(stderr.String(), "Could not open browser") {
		t.Fatal(opened, stdout.String(), stderr.String())
	}
	opened = ""
	stdout.Reset()
	stderr.Reset()
	if err := Run(t.Context(), []string{"--config-file", path, "browse", "--open=false"}, &stdout, &stderr, time.Now()); err != nil {
		t.Fatal(err)
	}
	if opened != "" || stderr.Len() != 0 {
		t.Fatal("print-only launched browser")
	}
	assertViewMissingPath(t, filepath.Join(root, "missing"))
	assertViewMissingPath(t, filepath.Join(root, "xdg", "tokeninsights"))
}
func TestBrowseAndSyncRejectMissingConfigurationBeforeSideEffects(t *testing.T) {
	root := t.TempDir()
	isolateViewSources(t, root)
	path := filepath.Join(root, "config.json")
	for _, args := range [][]string{{"browse"}, {"browse", "--server-url", "https://user:secret@example.test"}, {"sync", "--wait"}, {"sync", "--mode", "single-process"}, {"web", "--server-url", "https://example.test"}, {"tui", "--mode", "single-process"}} {
		if err := Run(t.Context(), append([]string{"--config-file", path}, args...), io.Discard, io.Discard, time.Now()); err == nil {
			t.Fatal("invalid command accepted", args)
		}
	}
	assertViewMissingPath(t, filepath.Join(root, "xdg", "tokeninsights"))
}
func TestTUIIgnoresDistributedCredentialsAndInvalidRemoteEnvironment(t *testing.T) {
	root := t.TempDir()
	isolateViewSources(t, root)
	collectorPath, serverPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	path := filepath.Join(root, "config.json")
	if err := config.Update(t.Context(), path, func(v *config.Values) error {
		if err := v.Set("distributed.server-url", "http://127.0.0.1:1", false); err != nil {
			return err
		}
		return v.Set("distributed.server-token", "fixture-token", false)
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TOKENINSIGHTS_SERVER_URL", "invalid-url")
	t.Setenv("TOKENINSIGHTS_ACCESS_TOKEN", "invalid token")
	var received tui.Command
	previous := runTerminalDashboard
	runTerminalDashboard = func(command tui.Command) error { received = command; return nil }
	t.Cleanup(func() { runTerminalDashboard = previous })
	args := []string{"--sync=false", "--all-time", "--server-db-path", serverPath, "--collector-db-path", collectorPath}
	if err := Run(t.Context(), append([]string{"--config-file", path, "tui"}, args...), io.Discard, io.Discard, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(received.Args, args) || received.Settings.ServerDBPath != serverPath || received.Settings.CollectorDBPath != collectorPath {
		t.Fatalf("tui received %+v", received)
	}
	assertViewMissingPath(t, collectorPath+".jobs.sqlite")
}
