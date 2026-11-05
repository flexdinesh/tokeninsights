package localclient

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/core/serverownership"
	"github.com/flexdinesh/tokeninsights/packages/core/sshtransport"
)

// fakeSSH drops ssh options and the destination, then runs the remote command
// locally, recording its PID so tests can simulate an abrupt disconnect.
func fakeSSH(t *testing.T, pids string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh")
	script := "#!/bin/sh\nwhile [ \"$1\" != \"--\" ]; do shift; done\nshift 2\necho $$ >> '" + pids + "'\nexec \"$@\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// isolateRemote gives the "remote" machine private storage, configuration and
// empty harness roots, never the test runner's real sources.
func isolateRemote(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("TOKENINSIGHTS_CONFIG_PATH", filepath.Join(home, "config.json"))
	t.Setenv("CODEX_HOME", filepath.Join(home, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, "claude"))
	for _, key := range []string{"TOKENINSIGHTS_CONNECTION", "TOKENINSIGHTS_COLLECTOR_DB_PATH", "TOKENINSIGHTS_SERVER_DB_PATH"} {
		t.Setenv(key, "")
	}
	return filepath.Join(home, "data", "tokeninsights", "server.sqlite")
}

func buildRemoteCLI(t *testing.T, ctx context.Context) string {
	t.Helper()
	dir := t.TempDir()
	for output, pkg := range map[string]string{"tokeninsights": "../../cli/cmd/tokeninsights", "tokeninsights-backend": "../../backend/cmd/tokeninsights-backend"} {
		command := exec.CommandContext(ctx, "go", "build", "-o", filepath.Join(dir, output), pkg)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", output, err, out)
		}
	}
	return filepath.Join(dir, "tokeninsights")
}

func waitReleased(t *testing.T, ctx context.Context, server string) {
	t.Helper()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		owner, held, err := serverownership.Lifetime(server, false)
		if err != nil {
			t.Fatal(err)
		}
		if owner != nil {
			_ = owner.Close()
		}
		if !held {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("remote backend never shut down", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestSSHEndpointAttachesRemoteBackendAndReleasesOnDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	cli := buildRemoteCLI(t, ctx)
	server := isolateRemote(t)
	pids := filepath.Join(t.TempDir(), "pids")
	shim := fakeSSH(t, pids)
	remote := endpoint.SSH{Host: "box", Command: cli}
	source := t.TempDir()
	first, err := ConnectRemote(ctx, sshtransport.Dialer(remote, shim), RemoteOptions{Capture: true, SourceDir: source})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	instance, err := first.Query.Instance(ctx)
	if err != nil || instance.InstanceID != first.InstanceID || instance.Hostname == "" {
		t.Fatal("remote queries unavailable", instance, err)
	}
	if _, err := first.Collect(ctx, false, source); err != nil {
		t.Fatal("remote capture failed", err)
	}
	if err := first.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := ConnectRemote(ctx, sshtransport.Dialer(remote, shim), RemoteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if second.InstanceID != first.InstanceID {
		t.Fatal("second SSH session started another owner")
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Query.Status(ctx); err != nil {
		t.Fatal("closing one SSH viewer stopped another", err)
	}
	// Kill the first bridge abruptly: its lease must end and the session must
	// report disconnection instead of reconnecting.
	data, err := os.ReadFile(pids)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.Fields(string(data))[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.Done():
	case <-ctx.Done():
		t.Fatal("killed SSH bridge left session attached")
	}
	if _, err := first.Query.Instance(ctx); err == nil {
		t.Fatal("closed SSH session reconnected")
	}
	waitReleased(t, ctx, server)
	runtime, err := localruntime.Open(ctx, filepath.Join(filepath.Dir(server), "collector.sqlite"), server)
	if err != nil {
		t.Fatal("remote shutdown did not release usable storage", err)
	}
	_ = runtime.Close()
}

func TestSSHEndpointMissingRemoteCommandReportsDiagnostics(t *testing.T) {
	isolateRemote(t)
	shim := fakeSSH(t, filepath.Join(t.TempDir(), "pids"))
	missing := filepath.Join(t.TempDir(), "absent", "tokeninsights")
	_, err := ConnectRemote(t.Context(), sshtransport.Dialer(endpoint.SSH{Host: "box", Command: missing}, shim), RemoteOptions{})
	var remote *sshtransport.RemoteError
	if !errors.As(err, &remote) || !strings.Contains(err.Error(), missing) {
		t.Fatal("missing remote command lost diagnostics", err)
	}
}

func TestRemoteRejectsRelativeSourceAndIncompatibleProtocol(t *testing.T) {
	never := func(context.Context) (net.Conn, error) {
		t.Error("dialed before validation")
		return nil, errors.New("unexpected")
	}
	if _, err := ConnectRemote(t.Context(), never, RemoteOptions{Capture: true, SourceDir: "relative/dir"}); !errors.Is(err, ErrRelativeRemoteSource) {
		t.Fatal("relative remote source accepted", err)
	}
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{Protocols: protocols, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(localprotocol.Descriptor{ProtocolVersion: localprotocol.ProtocolVersion + 1, InstanceID: "future"})
	})}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()
	_, err = ConnectRemote(t.Context(), SocketDialer(socket), RemoteOptions{})
	if !errors.Is(err, localprotocol.ErrIncompatible) || !strings.Contains(err.Error(), "client protocol 3") {
		t.Fatal("incompatible remote accepted or unnamed", err)
	}
}
