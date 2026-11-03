package main

import (
	"context"
	"errors"
	"time"
)

const refreshInterval = 2 * time.Second

var errDisconnected = errors.New("local backend disconnected; retry to reconnect")

// Presentation values and callbacks keep the native UI independent of HTTP,
// application queries, storage and backend startup details.
type filters struct{ Period, From, To, Provider, Model, Harness string }
type totals struct{ Total, Input, Output, Reasoning, CacheRead, CacheWrite, Sessions int64 }
type usageRow struct {
	Provider, Model, Harness string
	Tokens                   totals
}
type snapshot struct {
	Totals                        totals
	Rows                          []usageRow
	Providers, Models, Harnesses  []string
	Range, Collection, Processing string
	CollectionFailed              bool
}

type externalTarget struct{ URL string }

func (*externalTarget) Error() string { return "open this server in your browser" }

type dataSource interface {
	Load(context.Context, filters) (snapshot, error)
	Retry(context.Context) error
	Close() error
}

// capture is true for launch and explicit collection retry. Filter changes and
// periodic refresh borrow the existing session and only read saved data.
type connector func(ctx context.Context, target string, capture bool) (dataSource, error)
type loadRequest struct {
	Generation uint64
	Filters    filters
	Retry      bool
}
type loadResult struct {
	Generation uint64
	Data       snapshot
	Err        error
}

type loader struct {
	requests chan loadRequest
	results  chan loadResult
	cancel   context.CancelFunc
	done     chan struct{}
}

func newLoader(connect connector, target string) *loader {
	ctx, cancel := context.WithCancel(context.Background())
	l := &loader{requests: make(chan loadRequest, 1), results: make(chan loadResult, 1), cancel: cancel, done: make(chan struct{})}
	go l.run(ctx, connect, target)
	return l
}

func (l *loader) request(request loadRequest) {
	// A newer filter supersedes queued work, but never loses a collection retry.
	select {
	case previous := <-l.requests:
		request.Retry = request.Retry || previous.Retry
	default:
	}
	l.requests <- request
}

func (l *loader) run(ctx context.Context, connect connector, target string) {
	defer close(l.done)
	var source dataSource
	defer func() {
		if source != nil {
			_ = source.Close()
		}
	}()
	timer := time.NewTicker(refreshInterval)
	defer timer.Stop()
	var current loadRequest
	connected := false
	for {
		select {
		case <-ctx.Done():
			return
		case current = <-l.requests:
		case <-timer.C:
			if current.Generation == 0 || source == nil {
				continue
			}
		}
		if ctx.Err() != nil {
			return
		}
		var err error
		if source == nil {
			source, err = connect(ctx, target, current.Retry || !connected)
			if err == nil {
				connected = true
			}
		} else if current.Retry {
			err = source.Retry(ctx)
		}
		current.Retry = false
		var data snapshot
		if err == nil {
			data, err = source.Load(ctx, current.Filters)
		}
		if errors.Is(err, errDisconnected) && source != nil {
			_ = source.Close()
			source = nil
		}
		if ctx.Err() != nil {
			return
		}
		result := loadResult{Generation: current.Generation, Data: data, Err: err}
		select {
		case <-l.results:
		default:
		}
		l.results <- result
	}
}

func (f filters) validate() error {
	if f.Period == "custom" && (f.From == "" || f.To == "") {
		return errors.New("enter both dates as YYYY-MM-DD")
	}
	if f.From == "" && f.To == "" {
		return nil
	}
	if f.From == "" || f.To == "" {
		return errors.New("enter both dates as YYYY-MM-DD")
	}
	for _, value := range []string{f.From, f.To} {
		if _, err := time.Parse(time.DateOnly, value); err != nil {
			return errors.New("enter dates as YYYY-MM-DD")
		}
	}
	if f.From > f.To {
		return errors.New("from date must precede or equal To date")
	}
	return nil
}
