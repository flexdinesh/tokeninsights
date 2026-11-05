// personal supervises a temporary TCP gateway for isolated network validation.
package main

import (
	"bufio"
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

func main() {
	command := exec.Command("/opt/tokeninsights/tokeninsights", "remote", "--listen=0.0.0.0:8767", "--advertise=personal:8767")
	stdout, err := command.StdoutPipe()
	if err != nil {
		panic(err)
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		panic(err)
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = command.Process.Signal(syscall.SIGTERM)
		})
	}
	defer stop()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			for _, field := range strings.Fields(scanner.Text()) {
				value := strings.Trim(field, "'\"")
				if strings.HasPrefix(value, "tokeninsights://connect/v1?") {
					select {
					case ready <- value:
					default:
					}
				}
			}
		}
	}()
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	var connection string
	select {
	case connection = <-ready:
	case err := <-done:
		panic(fmt.Sprintf("gateway exited before readiness: %v", err))
	case <-time.After(30 * time.Second):
		panic("gateway readiness timed out")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /connection", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Connection string `json:"connection"`
		}{Connection: connection})
	})
	mux.HandleFunc("POST /stop", func(w http.ResponseWriter, _ *http.Request) {
		stop()
		select {
		case <-done:
			w.WriteHeader(http.StatusNoContent)
		case <-time.After(20 * time.Second):
			http.Error(w, "gateway shutdown timed out", http.StatusGatewayTimeout)
		}
	})
	server := &http.Server{Addr: ":9876", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-interrupt
		stop()
		_ = server.Close()
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}
