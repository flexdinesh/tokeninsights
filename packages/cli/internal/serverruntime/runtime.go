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
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
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

func Open(ctx context.Context, path string, options datastore.Options) (*datastore.Store, error) {
	release, err := db.AcquireWriterLock(ctx, path)
	if err != nil {
		return nil, err
	}
	defer release()
	return datastore.OpenWithOptions(ctx, path, options)
}

func health(store *datastore.Store, next http.Handler) http.Handler {
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

// Run joins all workers/listeners before returning. Its caller owns store and
// listener resources, including cleanup if startup or the ready callback fails.
func Run(parent context.Context, store *datastore.Store, log io.Writer, bindings []Binding, ready func() error) error {
	if len(bindings) == 0 || store == nil {
		return errors.New("server requires listener")
	}
	for _, binding := range bindings {
		if binding.Listener == nil || binding.Handler == nil {
			return errors.New("invalid server listener")
		}
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
	return Serve(ctx, store, bindings, ready)
}

// Serve owns HTTP listeners only; processing is owned by the composition.
func Serve(parent context.Context, store *datastore.Store, bindings []Binding, ready func() error) error {
	if len(bindings) == 0 || store == nil {
		return errors.New("server requires listener")
	}
	for _, binding := range bindings {
		if binding.Listener == nil || binding.Handler == nil {
			return errors.New("invalid server listener")
		}
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	servers := make([]*http.Server, 0, len(bindings))
	failures := make(chan error, len(bindings))
	for _, binding := range bindings {
		handler := binding.Handler
		if binding.Health {
			handler = health(store, handler)
		}
		httpServer := &http.Server{Handler: handler, BaseContext: func(net.Listener) context.Context { return ctx }, ReadHeaderTimeout: readHeaderTimeout, ReadTimeout: requestTimeout, WriteTimeout: requestTimeout, IdleTimeout: idleTimeout}
		servers = append(servers, httpServer)
		go func() { failures <- httpServer.Serve(binding.Listener) }()
	}
	defer func() {
		for _, s := range servers {
			_ = s.Close()
		}
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
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), ShutdownTimeout)
	defer cancelShutdown()
	for _, s := range servers {
		runErr = errors.Join(runErr, s.Shutdown(shutdown))
	}
	return runErr
}
