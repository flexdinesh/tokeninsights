package endpoint

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseAcceptsLocalAndSSHOnly(t *testing.T) {
	for value, want := range map[string]string{
		"":                       Local,
		"local":                  Local,
		"ssh://box":              "ssh://box",
		"ssh://dee@box":          "ssh://dee@box",
		"ssh://dee@box:2222":     "ssh://dee@box:2222",
		"ssh://box.example.com/": "ssh://box.example.com",
		"ssh://10.0.0.5:22":      "ssh://10.0.0.5:22",
	} {
		parsed, err := Parse(value)
		if err != nil || parsed.String() != want || parsed.IsLocal() != (want == Local) {
			t.Fatalf("Parse(%q) = %v, %v; want %s", value, parsed, err, want)
		}
	}
	for _, value := range []string{
		"remote", "tcp://box:8765", "http://box", "ssh://", "ssh://-oProxyCommand=evil",
		"ssh://-box", "ssh://dee:secret@box", "ssh://-l@box", "ssh://box/path", "ssh://box?x=1",
		"ssh://box#f", "ssh://box:0", "ssh://box:70000", " ssh://box", "ssh://box:abc", "ssh://dee;rm@box",
	} {
		if parsed, err := Parse(value); err == nil {
			t.Fatalf("Parse(%q) accepted %v", value, parsed)
		}
	}
}

func TestArgsPlaceDestinationAfterOptionTerminator(t *testing.T) {
	parsed, err := Parse("ssh://dee@box:2222")
	if err != nil {
		t.Fatal(err)
	}
	args := parsed.SSH.Args()
	terminator := slices.Index(args, "--")
	if terminator < 0 || !slices.Equal(args[terminator+1:], []string{"dee@box", DefaultCommand, "system", "dial-stdio"}) || !slices.Contains(args[:terminator], "2222") {
		t.Fatal("unsafe or incomplete ssh arguments", args)
	}
	parsed.SSH.Command = "/home/dee/.local/bin/tokeninsights"
	if args := parsed.SSH.Args(); args[len(args)-3] != parsed.SSH.Command {
		t.Fatal("remote.command not used", args)
	}
}

func TestValidateCommandRejectsControlCharacters(t *testing.T) {
	if err := ValidateCommand("/opt/ti/tokeninsights"); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"", " tokeninsights", "tokeninsights\nrm", "a\x00b"} {
		if ValidateCommand(command) == nil {
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
	dial := SSH{Host: "box"}.Dialer(executable)
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
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := dial(t.Context()); !errors.Is(err, ErrSessionClosed) {
		t.Fatal("second dial started another ssh session", err)
	}
}

func TestDialerReportsBoundedRemoteDiagnostics(t *testing.T) {
	executable := fakeSSH(t, `echo "sh: tokeninsights: not found" >&2; head -c 10000 /dev/zero | tr '\0' x >&2; exit 127`)
	conn, err := SSH{Host: "box"}.Dialer(executable)(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_, err = conn.Read(make([]byte, 1))
	var remote *RemoteError
	if !errors.As(err, &remote) || !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "remote.command") || len(remote.Stderr) > maxStderrBytes {
		t.Fatal("remote failure lost or unbounded diagnostics", err)
	}
}

func TestDialerRespectsCancelledContext(t *testing.T) {
	ctx, cancel := contextWithCancel(t)
	cancel()
	if _, err := (SSH{Host: "box"}).Dialer(fakeSSH(t, "exec cat"))(ctx); err == nil {
		t.Fatal("cancelled dial started ssh")
	}
}

func contextWithCancel(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithCancel(t.Context())
}
