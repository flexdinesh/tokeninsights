// check-gate is development tooling, separate from production storage locks.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const activeKey = "TOKENINSIGHTS_CHECK_GATE_ACTIVE"
const pollInterval = 100 * time.Millisecond
const shutdownGrace = 5 * time.Second

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) > 0 && args[0] == "--supervise" {
		return supervise(args[1:])
	}
	watchParent := len(args) > 0 && args[0] == "--watch-parent"
	if watchParent {
		args = args[1:]
	}
	if len(args) == 0 || os.Getenv(activeKey) != "" {
		fmt.Fprintln(os.Stderr, "check gate: command required; nested gates are forbidden")
		return 1
	}
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if watchParent {
		parent := os.NewFile(3, "launcher-liveness")
		if parent == nil {
			return 1
		}
		defer func() { _ = parent.Close() }()
		go func() {
			_, _ = io.Copy(io.Discard, parent)
			select {
			case signals <- syscall.SIGTERM:
			default:
			}
		}()
	}
	path, err := lockPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	lock, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = lock.Close() }()
	start := time.Now()
	fmt.Fprintln(os.Stderr, "check gate: waiting for repository checks")
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR) {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		select {
		case sig := <-signals:
			fmt.Fprintf(os.Stderr, "check gate: canceled after waiting %s\n", time.Since(start).Round(time.Millisecond))
			return signalCode(sig)
		case <-ticker.C:
		}
	}
	fmt.Fprintf(os.Stderr, "check gate: acquired; wait %s\n", time.Since(start).Round(time.Millisecond))
	reader, writer, err := os.Pipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	child := exec.Command(executable, append([]string{"--supervise"}, args...)...)
	child.ExtraFiles = []*os.File{lock, reader}
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_ = reader.Close()
	// The supervisor retains the same open file description. Never unlink the
	// lock file: a second inode would permit a second owner.
	_ = lock.Close()
	done := make(chan int, 1)
	go func() { done <- exitCode(child.Wait()) }()
	select {
	case code := <-done:
		return code
	case sig := <-signals:
		_ = writer.Close()
		<-done
		return signalCode(sig)
	}
}

func lockPath() (string, error) {
	output, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("resolve Git common directory: %w", err)
	}
	common, err := filepath.EvalSymlinks(strings.TrimSpace(string(output)))
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "tokeninsights-check.lock"), nil
}

func supervise(args []string) int {
	lock := os.NewFile(3, "check-lock")
	parent := os.NewFile(4, "parent-liveness")
	if lock == nil || parent == nil || len(args) == 0 {
		return 1
	}
	defer func() { _ = lock.Close() }()
	defer func() { _ = parent.Close() }()
	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	lostParent := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, parent)
		close(lostParent)
	}()
	child := exec.Command(args[0], args[1:]...)
	child.Env = append(os.Environ(), activeKey+"=1")
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Retain exclusion even if the supervisor itself crashes. Normal children
	// inherit this descriptor; detached workloads must manage their own cleanup.
	child.ExtraFiles = []*os.File{lock}
	start := time.Now()
	defer func() {
		fmt.Fprintf(os.Stderr, "check gate: execution %s\n", time.Since(start).Round(time.Millisecond))
	}()
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	done := make(chan int, 1)
	go func() { done <- exitCode(child.Wait()) }()
	var code int
	select {
	case code = <-done:
	case <-lostParent:
		code = 1
		stop(child, done)
	case sig := <-signals:
		code = signalCode(sig)
		stop(child, done)
	}
	// Also remove leftover same-group descendants after command failure/success.
	_ = unix.Kill(-child.Process.Pid, unix.SIGKILL)
	return code
}

func stop(child *exec.Cmd, done <-chan int) {
	_ = unix.Kill(-child.Process.Pid, unix.SIGTERM)
	deadline := time.Now().Add(shutdownGrace)
	select {
	case <-done:
	case <-time.After(shutdownGrace):
		_ = unix.Kill(-child.Process.Pid, unix.SIGKILL)
		<-done
	}
	// mise can exit before its children finish signal handlers (including Docker
	// cleanup). Keep admission closed until the group exits or grace expires.
	for time.Now().Before(deadline) {
		if errors.Is(unix.Kill(-child.Process.Pid, 0), unix.ESRCH) {
			return
		}
		time.Sleep(pollInterval)
	}
	_ = unix.Kill(-child.Process.Pid, unix.SIGKILL)
}

func signalCode(sig os.Signal) int {
	if value, ok := sig.(syscall.Signal); ok {
		return 128 + int(value)
	}
	return 1
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exit.ExitCode()
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}
