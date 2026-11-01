// deployment-fixture is a test-only process supervisor for private Dagger services.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const executable = "/usr/local/bin/tokeninsights-server"
const stopTimeout = 20 * time.Second

type supervisor struct {
	mu    sync.Mutex
	child *exec.Cmd
	done  chan error
	log   *os.File
}

func (s *supervisor) stop() (int, error) {
	if s.child == nil {
		return 0, nil
	}
	defer func() { s.child = nil }()
	_ = s.child.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-s.done:
		return s.child.ProcessState.ExitCode(), err
	case <-time.After(stopTimeout):
		_ = s.child.Process.Kill()
		waitErr := <-s.done
		return s.child.ProcessState.ExitCode(), fmt.Errorf("SIGTERM timeout: %v", waitErr)
	}
}

func (s *supervisor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	result := struct {
		UID      int    `json:"uid"`
		ExitCode int    `json:"exitCode"`
		Output   string `json:"output"`
	}{UID: os.Getuid()}
	var err error
	if r.Method != http.MethodPost && (r.Method != http.MethodGet || r.URL.Path != "/logs") {
		http.Error(w, "method", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Path {
	case "/start":
		if s.child != nil {
			http.Error(w, "already started", http.StatusConflict)
			return
		}
		command := exec.Command(executable)
		command.Stdout, command.Stderr = s.log, s.log
		err = command.Start()
		if err == nil {
			s.child = command
			s.done = make(chan error, 1)
			go func() { s.done <- command.Wait() }()
		}
	case "/stop":
		result.ExitCode, err = s.stop()
	case "/health", "/reset":
		args := []string{"healthcheck"}
		var password string
		if r.URL.Path == "/reset" {
			var request struct {
				UserID   string `json:"userID"`
				Password string `json:"password"`
			}
			if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&request); err != nil {
				http.Error(w, "invalid request", 400)
				return
			}
			args = []string{"admin", "user", "reset-password", request.UserID}
			password = request.Password + "\n"
		}
		ctx, cancel := context.WithTimeout(r.Context(), stopTimeout)
		defer cancel()
		command := exec.CommandContext(ctx, executable, args...)
		command.Stdin = strings.NewReader(password)
		output, runErr := command.CombinedOutput()
		result.Output, err = string(output), runErr
	case "/logs":
		var output []byte
		output, err = os.ReadFile(s.log.Name())
		result.Output = string(output)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		logs, _ := os.ReadFile(s.log.Name())
		http.Error(w, fmt.Sprintf("%v\n%s\n%s", err, result.Output, logs), 500)
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

func run() error {
	if os.Getuid() != 10001 {
		return fmt.Errorf("fixture must inherit image UID 10001")
	}
	for path, value := range map[string]string{
		os.Getenv("TOKENINSIGHTS_BOOTSTRAP_PASSWORD_FILE"): "container initial password\n",
		os.Getenv("TOKENINSIGHTS_POSTGRES_DSN_FILE"):       os.Getenv("TOKENINSIGHTS_FIXTURE_POSTGRES_DSN") + "\n",
	} {
		if path != "" {
			if err := os.WriteFile(path, []byte(value), 0600); err != nil {
				return err
			}
		}
	}
	log, err := os.Create("/data/deployment-fixture.log")
	if err != nil {
		return err
	}
	defer func() { _ = log.Close() }()
	s := &supervisor{log: log}
	defer func() { s.mu.Lock(); defer s.mu.Unlock(); _, _ = s.stop() }()
	server := &http.Server{Addr: ":9876", Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
