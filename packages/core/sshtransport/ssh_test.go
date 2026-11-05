package sshtransport

import (
	"context"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestArgsPlaceDestinationAfterOptionTerminator(t *testing.T) {
	parsed, err := endpoint.Parse("ssh://dee@box:2222")
	if err != nil {
		t.Fatal(err)
	}
	args := Args(*parsed.SSH)
	terminator := slices.Index(args, "--")
	if terminator < 0 || !slices.Equal(args[terminator+1:], []string{"dee@box", endpoint.DefaultCommand, "system", "dial-stdio"}) || !slices.Contains(args[:terminator], "2222") {
		t.Fatal("unsafe or incomplete ssh arguments", args)
	}
	parsed.SSH.Command = "/home/dee/.local/bin/tokeninsights"
	if args := Args(*parsed.SSH); args[len(args)-3] != parsed.SSH.Command {
		t.Fatal("remote.command not used", args)
	}
}

func TestValidateCommandRejectsControlCharacters(t *testing.T) {
	if err := endpoint.ValidateCommand("/opt/ti/tokeninsights"); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"", " tokeninsights", "tokeninsights\nrm", "a\x00b"} {
		if endpoint.ValidateCommand(command) == nil {
			t.Fatalf("invalid remote command %q accepted", command)
		}
	}
}

func fakeSSH(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDialerCarriesOneStdioSessionAndRejectsRedial(t *testing.T) {
	executable := fakeSSH(t, "exec cat")
	dial := Dialer(endpoint.SSH{Host: "box"}, executable)
	conn, err := dial(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal("pipe deadlines unsupported", err)
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(conn, reply); err != nil || string(reply) != "ping" {
		t.Fatal("stdio round trip failed", string(reply), err)
	}
	if conn.RemoteAddr().String() != "ssh://box" {
		t.Fatal("remote address lost endpoint", conn.RemoteAddr())
	}
	if err := conn.SetWriteDeadline(time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = conn.Write([]byte("late"))
	var remote *RemoteError
	if !errors.Is(err, os.ErrDeadlineExceeded) || errors.As(err, &remote) {
		t.Fatal("write deadline misclassified as remote failure", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := dial(t.Context()); !errors.Is(err, ErrSessionClosed) {
		t.Fatal("second dial started another ssh session", err)
	}
}

func TestDialerReportsBoundedRemoteDiagnostics(t *testing.T) {
	executable := fakeSSH(t, `echo "sh: tokeninsights: not found" >&2; head -c 10000 /dev/zero | tr '\0' x >&2; exit 127`)
	conn, err := Dialer(endpoint.SSH{Host: "box"}, executable)(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_, err = conn.Read(make([]byte, 1))
	var remote *RemoteError
	if !errors.As(err, &remote) || !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "remote.command") || len(remote.Stderr) > maxStderrBytes {
		t.Fatal("remote failure lost or unbounded diagnostics", err)
	}
	// Read above joins the real process exit, making the write-side failure
	// deterministic rather than depending on which HTTP/2 goroutine wins.
	_, err = conn.Write([]byte("request after remote exit"))
	if !errors.As(err, &remote) || !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "remote.command") || len(remote.Stderr) > maxStderrBytes {
		t.Fatal("write-side remote failure lost or unbounded diagnostics", err)
	}
}

func TestDialerRespectsCancelledContext(t *testing.T) {
	ctx, cancel := contextWithCancel(t)
	cancel()
	if _, err := Dialer(endpoint.SSH{Host: "box"}, fakeSSH(t, "exec cat"))(ctx); err == nil {
		t.Fatal("cancelled dial started ssh")
	}
}

func contextWithCancel(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithCancel(t.Context())
}
