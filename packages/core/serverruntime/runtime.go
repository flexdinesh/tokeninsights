// Package serverruntime owns shared server workers and HTTP lifecycle. Local
// discovery, operator sockets and deployment policy remain composition concerns.
package serverruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	ShutdownTimeout   = 15 * time.Second
	readHeaderTimeout = 5 * time.Second
	requestTimeout    = 35 * time.Second
	idleTimeout       = 60 * time.Second
)

type Binding struct {
	Listener net.Listener
	Handler  http.Handler
	Health   bool
}

// Readiness checks initialized storage, not processing-queue emptiness.
type Readiness interface{ Ready(context.Context) error }

// Worker.Run returns only after all child work has joined. Storage stays owned
// by composition; implementations must stop on cancellation without discarding
// accepted evidence or pending work.
type Worker interface {
	Readiness
	Run(context.Context, func(error))
}

func health(store Readiness, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/readyz" {
			ctx, cancel := context.WithTimeout(r.Context(), readHeaderTimeout)
			defer cancel()
			if err := store.Ready(ctx); err != nil {
				http.Error(w, "not_ready", http.StatusServiceUnavailable)
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
}

func validate(store Readiness, bindings []Binding) error {
	if len(bindings) == 0 || store == nil {
		return errors.New("server requires listener")
	}
	for _, binding := range bindings {
		if binding.Listener == nil || binding.Handler == nil {
			return errors.New("invalid server listener")
		}
	}
	return nil
}

// Run joins processing and listener loops before returning, without closing the
// caller's store. HTTP requests drain within ShutdownTimeout; on expiry their
// connections close. Handlers must honor request cancellation.
func Run(parent context.Context, store Worker, log io.Writer, bindings []Binding, ready func() error) error {
	if err := validate(store, bindings); err != nil {
		return err
	}
	if log == nil {
		log = io.Discard
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		store.Run(ctx, func(err error) { _, _ = fmt.Fprintf(log, "processing: %v\n", err) })
	}()
	defer func() { cancel(); <-workerDone }()
	return serve(ctx, store, bindings, ready, ShutdownTimeout)
}

// Serve has Run's HTTP shutdown contract but starts no processing. Validation
// leaves resources untouched; once serving starts, all listeners close on exit.
// The composition owns storage, processing and cleanup on validation failure.
func Serve(parent context.Context, store Readiness, bindings []Binding, ready func() error) error {
	if err := validate(store, bindings); err != nil {
		return err
	}
	return serve(parent, store, bindings, ready, ShutdownTimeout)
}

func serve(parent context.Context, store Readiness, bindings []Binding, ready func() error, shutdownTimeout time.Duration) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	servers := make([]*http.Server, 0, len(bindings))
	failures := make(chan error, len(bindings))
	var listeners sync.WaitGroup
	for _, binding := range bindings {
		handler := binding.Handler
		if binding.Health {
			handler = health(store, handler)
		}
		httpServer := &http.Server{Handler: handler, BaseContext: func(net.Listener) context.Context { return ctx }, ReadHeaderTimeout: readHeaderTimeout, ReadTimeout: requestTimeout, WriteTimeout: requestTimeout, IdleTimeout: idleTimeout}
		servers = append(servers, httpServer)
		listeners.Go(func() { failures <- httpServer.Serve(binding.Listener) })
	}
	defer func() {
		for _, s := range servers {
			_ = s.Close()
		}
		listeners.Wait()
	}()
	var runErr error
	if ready != nil {
		runErr = ready()
	}
	if runErr == nil {
		select {
		case <-parent.Done():
		case runErr = <-failures:
		}
		if errors.Is(runErr, http.ErrServerClosed) {
			runErr = nil
		}
	}
	cancel()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	// Each Shutdown closes its listener before draining. Start them together so
	// a slow request on one listener cannot leave another accepting new work.
	var draining sync.WaitGroup
	shutdownErrors := make([]error, len(servers))
	for index, s := range servers {
		draining.Go(func() { shutdownErrors[index] = s.Shutdown(shutdown) })
	}
	draining.Wait()
	return errors.Join(runErr, errors.Join(shutdownErrors...))
}
