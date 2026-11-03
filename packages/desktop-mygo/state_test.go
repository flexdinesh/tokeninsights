package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSource struct {
	load   func(context.Context, filters) (snapshot, error)
	retry  func(context.Context) error
	closed chan struct{}
}

func (s *fakeSource) Load(ctx context.Context, f filters) (snapshot, error) { return s.load(ctx, f) }
func (s *fakeSource) Close() error                                          { close(s.closed); return nil }

func (s *fakeSource) Retry(ctx context.Context) error {
	if s.retry != nil {
		return s.retry(ctx)
	}
	return nil
}

func resultFrom(t *testing.T, worker *loader) loadResult {
	t.Helper()
	select {
	case result := <-worker.results:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("native loader did not respond")
		return loadResult{}
	}
}

func waitClosed(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("native session did not close")
	}
}

func TestWindowWorkerFiltersOnlyQueryAndRetryKeepsLease(t *testing.T) {
	var connections atomic.Int32
	var retries atomic.Int32
	first := make(chan struct{})
	connect := func(_ context.Context, _ string, capture bool) (dataSource, error) {
		if !capture {
			t.Error("launch/retry must request capture")
		}
		connections.Add(1)
		return &fakeSource{closed: first, retry: func(context.Context) error { retries.Add(1); return nil }, load: func(_ context.Context, f filters) (snapshot, error) { return snapshot{Range: f.Model}, nil }}, nil
	}
	worker := newLoader(connect, "")
	t.Cleanup(func() { worker.cancel(); waitClosed(t, worker.done) })
	worker.request(loadRequest{Generation: 1, Filters: filters{Period: "all"}})
	if result := resultFrom(t, worker); result.Err != nil {
		t.Fatal(result.Err)
	}
	worker.request(loadRequest{Generation: 2, Filters: filters{Period: "all", Model: "gpt"}})
	if result := resultFrom(t, worker); result.Err != nil || result.Data.Range != "gpt" {
		t.Fatal(result)
	}
	if connections.Load() != 1 {
		t.Fatal("filter reacquired a session and collected")
	}
	worker.request(loadRequest{Generation: 3, Filters: filters{Period: "all"}, Retry: true})
	if result := resultFrom(t, worker); result.Err != nil {
		t.Fatal(result.Err)
	}
	if connections.Load() != 1 || retries.Load() != 1 {
		t.Fatal("retry must capture once using the existing lease")
	}
	worker.cancel()
	waitClosed(t, worker.done)
	waitClosed(t, first)
	// Reopening is an independent window session, even after every old lease closed.
	reopenedClosed := make(chan struct{})
	reopened := newLoader(func(context.Context, string, bool) (dataSource, error) {
		return &fakeSource{closed: reopenedClosed, load: func(context.Context, filters) (snapshot, error) { return snapshot{}, nil }}, nil
	}, "")
	reopened.request(loadRequest{Generation: 1})
	resultFrom(t, reopened)
	reopened.cancel()
	waitClosed(t, reopened.done)
	waitClosed(t, reopenedClosed)
}

func TestCloseDuringLoadCancelsBeforeReleasingSession(t *testing.T) {
	started, cancelled, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	worker := newLoader(func(context.Context, string, bool) (dataSource, error) {
		return &fakeSource{closed: closed, load: func(ctx context.Context, _ filters) (snapshot, error) {
			close(started)
			<-ctx.Done()
			close(cancelled)
			return snapshot{}, ctx.Err()
		}}, nil
	}, "")
	worker.request(loadRequest{Generation: 1})
	waitClosed(t, started)
	worker.cancel()
	waitClosed(t, worker.done)
	waitClosed(t, cancelled)
	waitClosed(t, closed)
	select {
	case result := <-worker.results:
		t.Fatalf("closed window received %+v", result)
	default:
	}
}

func TestConnectFailureCanRetry(t *testing.T) {
	var attempts int
	closed := make(chan struct{})
	worker := newLoader(func(context.Context, string, bool) (dataSource, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("backend unavailable")
		}
		return &fakeSource{closed: closed, load: func(context.Context, filters) (snapshot, error) { return snapshot{Range: "recovered"}, nil }}, nil
	}, "")
	defer func() { worker.cancel(); waitClosed(t, worker.done) }()
	worker.request(loadRequest{Generation: 1})
	if result := resultFrom(t, worker); result.Err == nil {
		t.Fatal("connection error hidden")
	}
	worker.request(loadRequest{Generation: 2})
	if result := resultFrom(t, worker); result.Err != nil || result.Data.Range != "recovered" {
		t.Fatal(result)
	}
}

func TestCustomDatesValidateCalendarAndOrder(t *testing.T) {
	for _, f := range []filters{
		{From: "2026-02-30", To: "2026-03-01"},
		{From: "2026-04-01", To: "2026-03-01"},
		{From: "2026-03-01"},
	} {
		if f.validate() == nil {
			t.Fatalf("invalid dates accepted: %+v", f)
		}
	}
	if err := (filters{From: "2026-03-01", To: "2026-03-01"}).validate(); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsHoldIndependentLeases(t *testing.T) {
	closed := []chan struct{}{make(chan struct{}), make(chan struct{})}
	var connected atomic.Int32
	connect := func(context.Context, string, bool) (dataSource, error) {
		index := connected.Add(1) - 1
		return &fakeSource{closed: closed[index], load: func(context.Context, filters) (snapshot, error) { return snapshot{}, nil }}, nil
	}
	first, second := newLoader(connect, ""), newLoader(connect, "")
	defer func() { first.cancel(); second.cancel(); waitClosed(t, first.done); waitClosed(t, second.done) }()
	first.request(loadRequest{Generation: 1})
	resultFrom(t, first)
	second.request(loadRequest{Generation: 1})
	resultFrom(t, second)
	first.cancel()
	waitClosed(t, first.done)
	waitClosed(t, closed[0])
	select {
	case <-closed[1]:
		t.Fatal("closing one window released another window's lease")
	default:
	}
	second.cancel()
	waitClosed(t, second.done)
	waitClosed(t, closed[1])
}

func TestReconnectDoesNotCollectOnQueryRetry(t *testing.T) {
	var connected int
	firstClosed, secondClosed := make(chan struct{}), make(chan struct{})
	worker := newLoader(func(_ context.Context, _ string, capture bool) (dataSource, error) {
		connected++
		if connected == 1 {
			if !capture {
				t.Error("launch omitted capture")
			}
			return &fakeSource{closed: firstClosed, load: func(context.Context, filters) (snapshot, error) { return snapshot{}, errDisconnected }}, nil
		}
		if capture {
			t.Error("query retry collected")
		}
		return &fakeSource{closed: secondClosed, load: func(context.Context, filters) (snapshot, error) { return snapshot{}, nil }}, nil
	}, "")
	defer func() { worker.cancel(); waitClosed(t, worker.done) }()
	worker.request(loadRequest{Generation: 1})
	if result := resultFrom(t, worker); !errors.Is(result.Err, errDisconnected) {
		t.Fatal(result.Err)
	}
	waitClosed(t, firstClosed)
	worker.request(loadRequest{Generation: 2})
	if result := resultFrom(t, worker); result.Err != nil {
		t.Fatal(result.Err)
	}
}
