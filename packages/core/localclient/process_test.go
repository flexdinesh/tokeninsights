package localclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/localbackend"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/core/serverownership"
)

func testOptions(t *testing.T) Options {
	t.Helper()
	root := t.TempDir()
	return Options{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite"), AppDBPath: filepath.Join(root, "app.sqlite"), Host: "127.0.0.1", Port: 0}
}

func serveForTest(t *testing.T, options Options) (localprotocol.Descriptor, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	ready, finished := make(chan localprotocol.Descriptor, 1), make(chan error, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		finished <- localbackend.Serve(ctx, localprotocol.Settings{CollectorDBPath: options.CollectorDBPath, ServerDBPath: options.ServerDBPath, AppDBPath: options.AppDBPath, Host: options.Host, Port: options.Port}, io.Discard, func(d localprotocol.Descriptor) error { ready <- d; return nil })
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
	response, err := http.Get(first.URL + "/api/local/v1/viewers")
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatal("browser lease failed", err)
	}
	defer func() { _ = response.Body.Close() }()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := Connect(t.Context(), options)
	if err != nil {
		t.Fatal("reconnection failed", err)
	}
	defer func() { _ = second.Close() }()
	if second.InstanceID != descriptor.InstanceID {
		t.Fatal("browser lease lost backend")
	}
	if err := second.Reprocess(t.Context()); err != nil {
		t.Fatal("owner maintenance failed", err)
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
		response, err := http.Post(second.URL+path, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode < 400 {
			t.Fatal("public control route enabled", path)
		}
	}
	_ = second.Close()
	_ = response.Body.Close()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("final viewer did not stop backend")
	}
	runtime, err := localruntime.OpenWithApp(t.Context(), options.CollectorDBPath, options.ServerDBPath, options.AppDBPath)
	if err != nil {
		t.Fatal("storage ownership survived shutdown", err)
	}
	_ = runtime.Close()
}

func TestSettingsMismatchNeverPreparesOrFallsBack(t *testing.T) {
	options := testOptions(t)
	_, _ = serveForTest(t, options)
	session, err := Connect(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	for _, changed := range []Options{
		func() Options { s := options; s.CollectorDBPath += ".other"; return s }(),
		func() Options { s := options; s.AppDBPath += ".other"; return s }(),
		func() Options { s := options; s.Port = 8765; return s }(),
	} {
		_, err := ConnectPrepared(t.Context(), changed, func() error { t.Error("mismatch attempted storage preparation"); return nil })
		if !errors.Is(err, localprotocol.ErrSettingsMismatch) {
			t.Fatal("settings mismatch accepted", err)
		}
	}
	if _, err := session.Query.Instance(t.Context()); err != nil {
		t.Fatal("mismatch damaged active owner", err)
	}
}

func TestPartialStartupFailurePreservesOccupiedPortAndReleasesOwnership(t *testing.T) {
	options := testOptions(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	address, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatal("not TCP")
	}
	options.Port = address.Port
	settings := localprotocol.Settings{CollectorDBPath: options.CollectorDBPath, ServerDBPath: options.ServerDBPath, AppDBPath: options.AppDBPath, Host: options.Host, Port: options.Port}
	if err := localbackend.Serve(t.Context(), settings, io.Discard, func(localprotocol.Descriptor) error { t.Error("ACK before listener ready"); return nil }); err == nil {
		t.Fatal("occupied port accepted")
	}
	if err := listener.Close(); err != nil {
		t.Fatal("another listener was stopped", err)
	}
	runtime, err := localruntime.OpenWithApp(t.Context(), options.CollectorDBPath, options.ServerDBPath, options.AppDBPath)
	if err != nil {
		t.Fatal("partial startup leaked owner", err)
	}
	_ = runtime.Close()
	socket, err := localprotocol.SocketPath(settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(socket); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partial startup left socket", err)
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
				selected.AppDBPath = filepath.Join(alias, "app.sqlite")
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
		if session.InstanceID != attached[0].InstanceID || session.URL != attached[0].URL {
			t.Fatal("multiple backend owners")
		}
	}
	bridge := exec.CommandContext(ctx, options.BackendPath, "connect", "--json", "--url", attached[0].URL)
	stdin, err := bridge.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = stdin.Close() }()
	stdout, err := bridge.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := bridge.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bridge.Process.Kill() }()
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		t.Fatal("JSON bridge did not ACK", scanner.Err())
	}
	var ack struct {
		URL             string `json:"url"`
		InstanceID      string `json:"instanceId"`
		ProtocolVersion int    `json:"protocolVersion"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &ack); err != nil || ack.URL != attached[0].URL || ack.InstanceID != attached[0].InstanceID || ack.ProtocolVersion != localprotocol.ProtocolVersion {
		t.Fatal("bridge attached wrong owner", ack, err)
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
	// A separate helper process owns its own lease after every Go session has
	// closed. Killing it simulates a desktop crash, with no graceful detach.
	response, err := http.Get(ack.URL + "/api/v2/instance")
	if err != nil {
		t.Fatal("bridge did not retain viewer", err)
	}
	_ = response.Body.Close()
	if err := bridge.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = bridge.Wait()
	if scanner.Scan() {
		t.Fatal("bridge emitted more than one stdout JSON line")
	}
	// Retry ownership acquisition, never mutate a live owner's socket. This
	// also proves shutdown eventually joins processing and releases storage.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		owner, held, err := serverownership.Lifetime(options.ServerDBPath, false)
		if err != nil {
			t.Fatal(err)
		}
		if owner != nil {
			_ = owner.Close()
		}
		if !held {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("child never shut down", ctx.Err())
		case <-ticker.C:
		}
	}
	runtime, err := localruntime.OpenWithApp(ctx, options.CollectorDBPath, options.ServerDBPath, options.AppDBPath)
	if err != nil {
		t.Fatal("shutdown did not release usable storage", err)
	}
	_ = runtime.Close()
}
