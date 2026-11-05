package localclient

import (
	"context"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/dashboardproxy"
	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
)

func testOptions(t *testing.T) Options {
	t.Helper()
	root := t.TempDir()
	return Options{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite")}
}

func serveForTest(t *testing.T, options Options) (localprotocol.Descriptor, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	ready, finished := make(chan localprotocol.Descriptor, 1), make(chan error, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		finished <- localbackend.Serve(ctx, localprotocol.Settings{CollectorDBPath: options.CollectorDBPath, ServerDBPath: options.ServerDBPath}, io.Discard, func(d localprotocol.Descriptor) error { ready <- d; return nil })
	}()
	t.Cleanup(func() { cancel(); <-joined })
	select {
	case d := <-ready:
		return d, finished
	case err := <-finished:
		t.Fatal(err)
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	return localprotocol.Descriptor{}, finished
}

func TestViewerStreamsReconnectAndPrivateMaintenance(t *testing.T) {
	options := testOptions(t)
	descriptor, finished := serveForTest(t, options)
	first, err := Connect(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	bridge, err := dashboardproxy.Start(t.Context(), first.PrivateRoundTripper(), dashboardproxy.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bridge.Close() }()
	response, err := http.Get(bridge.URL + "/api/local/v1/viewers")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatal("browser lease failed", err)
	}
	defer func() { _ = response.Body.Close() }()
	_ = first.Close()
	second, err := Connect(t.Context(), options)
	if err != nil {
		t.Fatal("reconnection failed", err)
	}
	defer func() { _ = second.Close() }()
	if second.InstanceID != descriptor.InstanceID {
		t.Fatal("browser lease lost backend")
	}
	if err := second.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := second.WaitVisible(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Progress(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := second.ProcessingStatus(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/local/v1/capture", "/api/local/v1/reprocess", "/api/v3/ingestion/batches"} {
		rejected, err := http.Post(bridge.URL+path, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		_ = rejected.Body.Close()
		if rejected.StatusCode < 400 {
			t.Fatal("public control enabled", path)
		}
	}
	_ = response.Body.Close()
	_ = bridge.Close()
	_ = second.Close()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("final viewer did not stop backend")
	}
	runtime, err := localruntime.Open(t.Context(), options.CollectorDBPath, options.ServerDBPath)
	if err != nil {
		t.Fatal("storage ownership survived shutdown", err)
	}
	_ = runtime.Close()
}
func TestSettingsMismatchNeverPreparesOrFallsBack(t *testing.T) {
	options := testOptions(t)
	serveForTest(t, options)
	session, err := Connect(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	changed := options
	changed.CollectorDBPath += ".other"
	_, err = ConnectPrepared(t.Context(), changed, func() error { t.Error("mismatch attempted preparation"); return nil })
	if !errors.Is(err, localprotocol.ErrSettingsMismatch) {
		t.Fatal("settings mismatch accepted", err)
	}
	if _, err := session.Query.Instance(t.Context()); err != nil {
		t.Fatal("mismatch damaged owner", err)
	}
}
func TestPreparationRunsOnlyForMissingUnownedBackend(t *testing.T) {
	options := testOptions(t)
	calls := 0
	_, err := ConnectPrepared(t.Context(), options, func() error { calls++; return errors.New("preparation stop") })
	if err == nil || calls != 1 {
		t.Fatal("preparation not run", calls, err)
	}
	serveForTest(t, options)
	session, err := ConnectPrepared(t.Context(), options, func() error { t.Error("attached viewer attempted recovery"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	_ = session.Close()
}
func TestPrivateSocketServesReadOnlyDashboardOverUnencryptedHTTP2(t *testing.T) {
	options := testOptions(t)
	descriptor, _ := serveForTest(t, options)
	socket, err := localprotocol.SocketPath(descriptor.Settings)
	if err != nil {
		t.Fatal(err)
	}
	session := newSession(SocketDialer(socket), true)
	defer session.transport.CloseIdleConnections()
	get := func(path, instance string) *http.Response {
		t.Helper()
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost"+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-TokenInsights-Instance", instance)
		response, err := session.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		return response
	}
	for _, path := range []string{"/api/v2/instance", "/"} {
		response := get(path, descriptor.InstanceID)
		if response.StatusCode != http.StatusOK || response.ProtoMajor != 2 {
			t.Fatal("private dashboard unavailable over h2c", path, response.StatusCode, response.Proto)
		}
	}
	if response := get("/api/v2/instance", "other"); response.StatusCode != http.StatusConflict {
		t.Fatal("private dashboard ignored instance pinning", response.StatusCode)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/api/v3/ingestion/batches", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-TokenInsights-Instance", descriptor.InstanceID)
	response, err := session.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode < 400 {
		t.Fatal("private dashboard mount enabled ingestion")
	}
	if _, err := session.descriptor(t.Context()); err != nil {
		t.Fatal("private dashboard bound a public listener", err)
	}
}

func TestRealProcessConcurrentStartupCanonicalAliasesAndFinalShutdown(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	options := testOptions(t)
	options.BackendPath = filepath.Join(t.TempDir(), "tokeninsights-backend")
	command := exec.CommandContext(ctx, "go", "build", "-o", options.BackendPath, "../../backend/cmd/tokeninsights-backend")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build backend: %v\n%s", err, output)
	}
	// The bridge requests capture. Its backend must only discover synthetic,
	// empty roots, never the test runner's installed harness artifacts.
	sourceHome := t.TempDir()
	t.Setenv("HOME", sourceHome)
	t.Setenv("XDG_DATA_HOME", filepath.Join(sourceHome, "data"))
	t.Setenv("CODEX_HOME", filepath.Join(sourceHome, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(sourceHome, "claude"))
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Dir(options.ServerDBPath), alias); err != nil {
		t.Fatal(err)
	}
	const clients = 8
	sessions, failures := make(chan *Session, clients), make(chan error, clients)
	var workers sync.WaitGroup
	for i := range clients {
		workers.Go(func() {
			selected := options
			if i%2 == 0 {
				selected.CollectorDBPath = filepath.Join(alias, "collector.sqlite")
				selected.ServerDBPath = filepath.Join(alias, "server.sqlite")
			}
			session, err := Connect(ctx, selected)
			if err != nil {
				failures <- err
				return
			}
			sessions <- session
		})
	}
	workers.Wait()
	close(sessions)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var attached []*Session
	for session := range sessions {
		attached = append(attached, session)
	}
	defer func() {
		for _, session := range attached {
			_ = session.Close()
		}
	}()
	if len(attached) != clients {
		t.Fatalf("attached %d clients, want %d", len(attached), clients)
	}
	for _, session := range attached {
		if session.InstanceID != attached[0].InstanceID {
			t.Fatal("multiple backend owners")
		}
	}
	socket, err := localprotocol.SocketPath(localprotocol.Settings{ServerDBPath: options.ServerDBPath})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(socket)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("control socket not private", err)
	}
	for _, session := range attached[:len(attached)-1] {
		_ = session.Close()
	}
	if _, err := attached[len(attached)-1].Query.Instance(ctx); err != nil {
		t.Fatal("one viewer stopped others", err)
	}
	_ = attached[len(attached)-1].Close()
	waitReleased(t, ctx, options.ServerDBPath)
	runtime, err := localruntime.Open(ctx, options.CollectorDBPath, options.ServerDBPath)
	if err != nil {
		t.Fatal("shutdown did not release usable storage", err)
	}
	_ = runtime.Close()
}
