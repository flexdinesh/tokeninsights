package serverruntime

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
)

type observedListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *observedListener) Close() error {
	err := l.Listener.Close()
	l.once.Do(func() { close(l.closed) })
	return err
}

func listen(t *testing.T) *observedListener {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return &observedListener{Listener: l, closed: make(chan struct{})}
}

func await[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("lifecycle operation did not finish")
	}
	var zero T
	return zero
}

func openStore(t *testing.T) *datastore.Store {
	t.Helper()
	s, err := Open(t.Context(), filepath.Join(t.TempDir(), "data.duckdb"), datastore.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func request(t *testing.T, l net.Listener) <-chan error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 5 * time.Second}
		response, err := client.Get("http://" + l.Addr().String())
		if err == nil {
			_, err = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		done <- err
	}()
	return done
}

func TestShutdownStopsEveryListenerBeforeDrainingRequests(t *testing.T) {
	for _, cause := range []string{"cancellation", "listener failure"} {
		t.Run(cause, func(t *testing.T) { testDrainingRequests(t, cause) })
	}
}

func testDrainingRequests(t *testing.T, cause string) {
	t.Helper()
	s := openStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	release := make(chan struct{})
	releaseHandlers := sync.OnceFunc(func() { close(release) })
	entered := make(chan struct{}, 2)
	cancelled := make(chan error, 2)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-r.Context().Done()
		// Storage belongs to the composition and stays usable while draining.
		_, err := s.Metadata(context.Background())
		cancelled <- err
		<-release
		w.WriteHeader(http.StatusNoContent)
	})
	first, second := listen(t), listen(t)
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		done <- Serve(ctx, s, []Binding{{Listener: first, Handler: handler}, {Listener: second, Handler: handler}}, nil)
	}()
	defer func() { cancel(); releaseHandlers(); await(t, finished) }()
	requests := []<-chan error{request(t, first), request(t, second)}
	for range 2 {
		await(t, entered)
	}
	if cause == "listener failure" {
		_ = first.Close()
	} else {
		cancel()
	}
	for range 2 {
		if err := await(t, cancelled); err != nil {
			t.Fatal("storage closed before requests joined", err)
		}
	}
	await(t, first.closed)
	await(t, second.closed)
	select {
	case err := <-done:
		t.Fatal("returned before draining requests", err)
	default:
	}
	releaseHandlers()
	for _, result := range requests {
		if err := await(t, result); err != nil {
			t.Fatal(err)
		}
	}
	err := await(t, done)
	if cause == "listener failure" {
		if !errors.Is(err, net.ErrClosed) {
			t.Fatal("listener failure lost", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Metadata(t.Context()); err != nil {
		t.Fatal("Serve closed caller's store", err)
	}
}

func TestRunFailureClosesAllListenersAndLeavesStoreToCaller(t *testing.T) {
	for _, cause := range []string{"ready", "listener"} {
		t.Run(cause, func(t *testing.T) {
			s := openStore(t)
			first, second := listen(t), listen(t)
			startupErr := errors.New("startup failed")
			ready := func() error { return startupErr }
			if cause == "listener" {
				_ = first.Close()
				ready = nil
			}
			handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
			err := Run(t.Context(), s, nil, []Binding{{Listener: first, Handler: handler}, {Listener: second, Handler: handler}}, ready)
			if cause == "ready" && !errors.Is(err, startupErr) {
				t.Fatal("lost startup error", err)
			}
			if cause == "listener" && !errors.Is(err, net.ErrClosed) {
				t.Fatal("lost listener error", err)
			}
			await(t, first.closed)
			await(t, second.closed)
			if _, err := s.Metadata(t.Context()); err != nil {
				t.Fatal("Run closed caller's store", err)
			}
		})
	}
}

func TestInvalidBindingsRejectBeforeTakingResources(t *testing.T) {
	s := openStore(t)
	for _, run := range []func(context.Context, *datastore.Store, []Binding, func() error) error{
		Serve,
		func(ctx context.Context, s *datastore.Store, b []Binding, ready func() error) error {
			return Run(ctx, s, nil, b, ready)
		},
	} {
		l := listen(t)
		valid := Binding{Listener: l, Handler: http.NotFoundHandler()}
		for _, bindings := range [][]Binding{nil, {valid, {Listener: l}}, {valid, {Handler: http.NotFoundHandler()}}} {
			if err := run(t.Context(), s, bindings, func() error { t.Error("ready called for invalid bindings"); return nil }); err == nil {
				t.Fatal("invalid bindings accepted")
			}
		}
		if err := run(t.Context(), nil, []Binding{valid}, nil); err == nil {
			t.Fatal("nil store accepted")
		}
		select {
		case <-l.closed:
			t.Fatal("validation took listener ownership")
		default:
		}
	}
}

func TestShutdownDeadlineClosesActiveConnections(t *testing.T) {
	s := openStore(t)
	l := listen(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered, exited := make(chan struct{}), make(chan struct{})
	release := make(chan struct{})
	defer func() { close(release); await(t, exited) }()
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release // Deliberately ignores cancellation to exercise forced close.
		close(exited)
	})
	startupErr := errors.New("ready failed")
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, s, []Binding{{Listener: l, Handler: handler}}, func() error {
			<-entered
			return startupErr
		}, 25*time.Millisecond)
	}()
	result := request(t, l)
	if err := await(t, done); !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, startupErr) {
		t.Fatal("shutdown lost startup/deadline errors", err)
	}
	await(t, l.closed)
	if err := await(t, result); err == nil {
		t.Fatal("request survived forced connection close")
	}
	if _, err := s.Metadata(t.Context()); err != nil {
		t.Fatal("deadline closed caller's store", err)
	}
}
