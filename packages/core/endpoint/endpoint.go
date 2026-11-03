// Package endpoint selects where a personal backend runs: the local private
// socket or a remote machine reached through the system ssh client. It parses
// typed endpoints and dials SSH stdio bridges; it imports no storage, server or
// HTTP packages and never falls back from a remote endpoint to local.
package endpoint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Local is the reserved endpoint name for this machine's backend.
const Local = "local"

// DefaultCommand runs the remote CLI through the remote user's PATH.
const DefaultCommand = "tokeninsights"

// maxStderrBytes bounds ssh diagnostics retained for error messages.
const maxStderrBytes = 4096

// exitGrace bounds how long Close waits for ssh to exit after stdin closes.
const exitGrace = 2 * time.Second

var ErrSessionClosed = errors.New("ssh session closed; relaunch to reconnect")

// Endpoint is local when SSH is nil.
type Endpoint struct {
	SSH *SSH
}

// SSH identifies one remote backend. Command is the remote CLI; an empty
// Command uses DefaultCommand.
type SSH struct {
	User, Host string
	Port       int
	Command    string
}

func (e Endpoint) IsLocal() bool { return e.SSH == nil }

func (e Endpoint) String() string {
	if e.SSH == nil {
		return Local
	}
	u := url.URL{Scheme: "ssh", Host: e.SSH.Host}
	if e.SSH.Port != 0 {
		u.Host = net.JoinHostPort(e.SSH.Host, strconv.Itoa(e.SSH.Port))
	}
	if e.SSH.User != "" {
		u.User = url.User(e.SSH.User)
	}
	return u.String()
}

// Parse accepts "", "local" or ssh://[user@]host[:port]. Values that ssh could
// interpret as options, paths, queries and credentials are rejected.
func Parse(value string) (Endpoint, error) {
	if value == "" || value == Local {
		return Endpoint{}, nil
	}
	invalid := fmt.Errorf("invalid endpoint %q; use local or ssh://[user@]host[:port]", value)
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "ssh" || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.TrimSpace(value) != value {
		return Endpoint{}, invalid
	}
	host := u.Hostname()
	if host == "" || !safeToken(host) {
		return Endpoint{}, invalid
	}
	remote := &SSH{Host: host}
	if u.User != nil {
		if _, hasPassword := u.User.Password(); hasPassword {
			return Endpoint{}, invalid
		}
		remote.User = u.User.Username()
		if !safeToken(remote.User) {
			return Endpoint{}, invalid
		}
	}
	if port := u.Port(); port != "" {
		remote.Port, err = strconv.Atoi(port)
		if err != nil || remote.Port < 1 || remote.Port > 65535 {
			return Endpoint{}, invalid
		}
	}
	return Endpoint{SSH: remote}, nil
}

// safeToken rejects option-like and control-bearing values passed to ssh.
func safeToken(value string) bool {
	return value != "" && !strings.HasPrefix(value, "-") && !strings.ContainsAny(value, " \t\r\n\x00@/\\'\"`$;&|<>")
}

// ValidateCommand checks a remote command override. The remote shell receives
// it verbatim, so the user owns quoting; control characters are rejected.
func ValidateCommand(command string) error {
	if strings.TrimSpace(command) == "" || strings.TrimSpace(command) != command || strings.ContainsAny(command, "\r\n\x00") {
		return errors.New("invalid remote.command; use the remote tokeninsights executable path")
	}
	return nil
}

// Args returns the ssh arguments starting the remote stdio bridge.
func (s SSH) Args() []string {
	args := []string{"-T", "-o", "ServerAliveInterval=15"}
	if s.Port != 0 {
		args = append(args, "-p", strconv.Itoa(s.Port))
	}
	destination := s.Host
	if s.User != "" {
		destination = s.User + "@" + s.Host
	}
	command := s.Command
	if command == "" {
		command = DefaultCommand
	}
	return append(args, "--", destination, command, "system", "dial-stdio")
}

// Dialer returns a single-use dialer: one ssh process carries the whole
// session, so its exit ends the session and a second dial reports closure.
func (s SSH) Dialer(executable string) func(context.Context) (net.Conn, error) {
	var mu sync.Mutex
	used := false
	return func(ctx context.Context) (net.Conn, error) {
		mu.Lock()
		defer mu.Unlock()
		if used {
			return nil, ErrSessionClosed
		}
		used = true
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return s.start(executable)
	}
}

func (s SSH) start(executable string) (net.Conn, error) {
	if executable == "" {
		executable = "ssh"
	}
	// Pollable pipes support the deadlines HTTP transports may set.
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		return nil, err
	}
	conn := &stdioConn{stdin: stdinWriter, stdout: stdoutReader, exited: make(chan struct{}), target: s}
	command := exec.Command(executable, s.Args()...)
	command.Stdin, command.Stdout, command.Stderr = stdinReader, stdoutWriter, &conn.stderr
	err = command.Start()
	_ = stdinReader.Close()
	_ = stdoutWriter.Close()
	if err != nil {
		_ = stdinWriter.Close()
		_ = stdoutReader.Close()
		return nil, fmt.Errorf("cannot start ssh: %w", err)
	}
	conn.process = command.Process
	go func() { conn.waitErr = command.Wait(); close(conn.exited) }()
	return conn, nil
}

// boundedBuffer keeps the first maxStderrBytes of ssh diagnostics.
type boundedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if remaining := maxStderrBytes - b.buf.Len(); remaining > 0 {
		b.buf.Write(p[:min(len(p), remaining)])
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.buf.String())
}

// RemoteError reports a closed ssh session with its bounded diagnostics.
type RemoteError struct {
	Endpoint string
	Stderr   string
	Err      error
}

func (e *RemoteError) Error() string {
	message := fmt.Sprintf("remote backend %s disconnected", e.Endpoint)
	if e.Stderr != "" {
		message += ": " + e.Stderr
	}
	if strings.Contains(e.Stderr, "not found") || strings.Contains(e.Stderr, "No such file") {
		message += " (set remote.command to the remote tokeninsights path)"
	}
	return message
}

func (e *RemoteError) Unwrap() error { return e.Err }

type stdioConn struct {
	stdin   *os.File
	stdout  *os.File
	stderr  boundedBuffer
	process *os.Process
	exited  chan struct{}
	waitErr error
	target  SSH
	once    sync.Once
}

func (c *stdioConn) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		select {
		case <-c.exited:
		case <-time.After(exitGrace):
		}
		return n, &RemoteError{Endpoint: Endpoint{SSH: &c.target}.String(), Stderr: c.stderr.String(), Err: err}
	}
	return n, err
}

func (c *stdioConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }

// Close ends the session: closing stdin lets the remote bridge exit cleanly;
// ssh is killed if it outlives the grace period.
func (c *stdioConn) Close() error {
	c.once.Do(func() {
		_ = c.stdin.Close()
		go func() {
			select {
			case <-c.exited:
			case <-time.After(exitGrace):
				_ = c.process.Kill()
			}
			_ = c.stdout.Close()
		}()
	})
	return nil
}

func (c *stdioConn) LocalAddr() net.Addr  { return stdioAddr("local") }
func (c *stdioConn) RemoteAddr() net.Addr { return stdioAddr(Endpoint{SSH: &c.target}.String()) }
func (c *stdioConn) SetDeadline(t time.Time) error {
	return errors.Join(c.SetReadDeadline(t), c.SetWriteDeadline(t))
}
func (c *stdioConn) SetReadDeadline(t time.Time) error  { return c.stdout.SetReadDeadline(t) }
func (c *stdioConn) SetWriteDeadline(t time.Time) error { return c.stdin.SetWriteDeadline(t) }

type stdioAddr string

func (a stdioAddr) Network() string { return "ssh-stdio" }
func (a stdioAddr) String() string  { return string(a) }
