// Package sshtransport owns system SSH stdio sessions.
package sshtransport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/flexdinesh/tokeninsights/packages/core/endpoint"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxStderrBytes = 4096
const exitGrace = 2 * time.Second

var ErrSessionClosed = errors.New("ssh session closed; relaunch to reconnect")

func commandPath(s endpoint.SSH) string {
	if s.Command == "" {
		return endpoint.DefaultCommand
	}
	return s.Command
}

// Args returns the ssh arguments starting the remote stdio bridge.
func Args(s endpoint.SSH) []string {
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
		command = endpoint.DefaultCommand
	}
	return append(args, "--", destination, command, "system", "dial-stdio")
}

// Dialer returns a single-use dialer: one ssh process carries the whole
// session, so its exit ends the session and a second dial reports closure.
func Dialer(s endpoint.SSH, executable string) func(context.Context) (net.Conn, error) {
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
		return start(s, executable)
	}
}

func start(s endpoint.SSH, executable string) (net.Conn, error) {
	if err := endpoint.ValidateCommand(commandPath(s)); err != nil {
		return nil, err
	}
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
	command := exec.Command(executable, Args(s)...)
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
	go func() { _ = command.Wait(); close(conn.exited) }()
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
	target  endpoint.SSH
	once    sync.Once
}

func (c *stdioConn) Read(p []byte) (int, error) {
	n, err := c.stdout.Read(p)
	if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
		select {
		case <-c.exited:
		case <-time.After(exitGrace):
		}
		return n, &RemoteError{Endpoint: endpoint.Endpoint{SSH: &c.target}.String(), Stderr: c.stderr.String(), Err: err}
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

func (c *stdioConn) LocalAddr() net.Addr { return stdioAddr("local") }
func (c *stdioConn) RemoteAddr() net.Addr {
	return stdioAddr(endpoint.Endpoint{SSH: &c.target}.String())
}
func (c *stdioConn) SetDeadline(t time.Time) error {
	return errors.Join(c.SetReadDeadline(t), c.SetWriteDeadline(t))
}
func (c *stdioConn) SetReadDeadline(t time.Time) error  { return c.stdout.SetReadDeadline(t) }
func (c *stdioConn) SetWriteDeadline(t time.Time) error { return c.stdin.SetWriteDeadline(t) }

type stdioAddr string

func (a stdioAddr) Network() string { return "ssh-stdio" }
func (a stdioAddr) String() string  { return string(a) }
