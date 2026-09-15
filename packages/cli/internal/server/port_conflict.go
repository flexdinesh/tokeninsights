package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const portReleaseTimeout = 5 * time.Second
const portRetryInterval = 100 * time.Millisecond

type portProcess struct {
	pid  int
	name string
}

type listenFunc func(string, int) (net.Listener, error)
type findPortProcessesFunc func(context.Context, string, int) ([]portProcess, error)
type terminateProcessFunc func(portProcess) error

func acquireListener(ctx context.Context, options Options, output io.Writer) (net.Listener, error) {
	return acquireListenerWith(ctx, options, output, listen, findPortProcesses, terminateProcess)
}

func acquireListenerWith(ctx context.Context, options Options, output io.Writer, listen listenFunc, findProcesses findPortProcessesFunc, terminate terminateProcessFunc) (net.Listener, error) {
	listener, conflictErr := listen(options.Host, options.Port)
	if conflictErr == nil || !options.ResolvePortConflict || !errors.Is(conflictErr, syscall.EADDRINUSE) {
		return listener, conflictErr
	}
	terminal := newConsole(output)
	processes, err := findProcesses(ctx, options.Host, options.Port)
	if err != nil {
		_ = terminal.portBusy(options.Port, nil)
		return nil, fmt.Errorf("identify process using port %d: %w", options.Port, err)
	}
	if len(processes) == 0 {
		listener, err := listen(options.Host, options.Port)
		if err == nil {
			return listener, nil
		}
		if writeErr := terminal.portBusy(options.Port, nil); writeErr != nil {
			return nil, writeErr
		}
		return nil, err
	}
	if err := terminal.portBusy(options.Port, processes); err != nil {
		return nil, err
	}
	if options.Input == nil {
		return nil, conflictErr
	}
	confirmed, err := terminal.confirmTermination(options.Input)
	if err != nil {
		return nil, err
	}
	if !confirmed {
		return nil, conflictErr
	}
	for _, process := range processes {
		if err := terminate(process); err != nil {
			return nil, err
		}
	}
	return waitForListener(ctx, options.Host, options.Port, listen)
}

func findPortProcesses(ctx context.Context, host string, port int) ([]portProcess, error) {
	command := exec.CommandContext(ctx, "lsof", "-nP", "-Fpcn", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN")
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(output) == 0 {
			return []portProcess{}, nil
		}
		return nil, err
	}
	return parsePortProcesses(string(output), host), nil
}

func parsePortProcesses(output, host string) []portProcess {
	if host == "" {
		host = DefaultHost
	}
	processes := make([]portProcess, 0)
	current := portProcess{}
	matches := false
	flush := func() {
		if current.pid > 0 && current.pid != os.Getpid() && matches {
			processes = append(processes, current)
		}
	}
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			flush()
			pid, err := strconv.Atoi(line[1:])
			current = portProcess{}
			matches = false
			if err == nil {
				current = portProcess{pid: pid, name: "unknown"}
			}
		case 'c':
			if current.pid > 0 {
				current.name = line[1:]
			}
		case 'n':
			matches = matches || listenerMatchesHost(line[1:], host)
		}
	}
	flush()
	return processes
}

func listenerMatchesHost(address, host string) bool {
	addressHost, _, err := net.SplitHostPort(address)
	return err == nil && (host == "0.0.0.0" || addressHost == "*" || addressHost == host)
}

func terminateProcess(process portProcess) error {
	target, err := os.FindProcess(process.pid)
	if err != nil {
		return fmt.Errorf("find %s process %d: %w", process.name, process.pid, err)
	}
	if err := target.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("terminate %s process %d: %w", process.name, process.pid, err)
	}
	return nil
}

func waitForListener(ctx context.Context, host string, port int, listen listenFunc) (net.Listener, error) {
	waitCtx, cancel := context.WithTimeout(ctx, portReleaseTimeout)
	defer cancel()
	ticker := time.NewTicker(portRetryInterval)
	defer ticker.Stop()
	var lastErr error
	for {
		listener, err := listen(host, port)
		if err == nil {
			return listener, nil
		}
		lastErr = err
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("port %d was not released: %w", port, lastErr)
		case <-ticker.C:
		}
	}
}
