package app

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	_ "modernc.org/sqlite"
)

func testController(t *testing.T) *Controller {
	t.Helper()
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	database, _, err := db.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	sources, err := pipeline.ResolveSources(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := New(context.Background(), path, sources, ID(), io.Discard)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := c.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return c
}

func awaitOperation(t *testing.T, c *Controller, id string) Operation {
	t.Helper()
	timeout := time.After(3 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		o, ok := c.Operation(id)
		if ok && o.State != "queued" && o.State != "running" {
			return o
		}
		select {
		case <-timeout:
			t.Fatal("operation did not finish")
			return Operation{}
		case <-ticker.C:
		}
	}
}

func TestRefreshCoalescesBeforeCaptureAndQueuesExactlyOneAfter(t *testing.T) {
	c := testController(t)
	started, capture, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	followup := make(chan struct{}, 1)
	calls := 0
	c.runner = func(ctx context.Context, a Action, progress func(pipeline.SyncProgressEvent), _ func(context.Context) error) (pipeline.Summary, error) {
		calls++
		if calls == 1 {
			close(started)
			select {
			case <-capture:
			case <-ctx.Done():
				return pipeline.Summary{}, ctx.Err()
			}
			progress(pipeline.SyncProgressEvent{Status: pipeline.SyncProgressDiscovering})
			select {
			case <-finish:
			case <-ctx.Done():
				return pipeline.Summary{}, ctx.Err()
			}
			return pipeline.Summary{}, errors.New("first fails")
		}
		followup <- struct{}{}
		return pipeline.Summary{}, nil
	}
	first, err := c.RequestRefresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	<-started
	joined, err := c.RequestRefresh(context.Background())
	if err != nil || joined.ID != first.ID {
		t.Fatal("early request not joined")
	}
	close(capture)
	// Observe capture without arbitrary sleeps.
	for {
		c.mu.Lock()
		captured := c.active.captured
		c.mu.Unlock()
		if captured {
			break
		}
		<-time.After(time.Millisecond)
	}
	second, err := c.RequestRefresh(context.Background())
	if err != nil || second.ID == first.ID {
		t.Fatal("late request lost")
	}
	for range 2000 {
		o, err := c.RequestRefresh(context.Background())
		if err != nil || o.ID != second.ID {
			t.Fatal("unbounded pending refresh")
		}
	}
	c.mu.Lock()
	queued := len(c.queue)
	c.mu.Unlock()
	if queued != 1 {
		t.Fatal("more than one followup")
	}
	close(finish)
	if result := awaitOperation(t, c, first.ID); result.State != "failed" {
		t.Fatal(result)
	}
	if result := awaitOperation(t, c, second.ID); result.State != "succeeded" {
		t.Fatal(result)
	}
	select {
	case <-followup:
	default:
		t.Fatal("followup skipped after failure")
	}
}

func TestConcurrentRefreshRequestsQueueAndCoalesce(t *testing.T) {
	c := testController(t)
	exclusiveStarted, finishExclusive := make(chan struct{}), make(chan struct{})
	refreshCaptured, finishRefresh := make(chan struct{}), make(chan struct{})
	var calls, active atomic.Int32
	c.runner = func(ctx context.Context, a Action, progress func(pipeline.SyncProgressEvent), _ func(context.Context) error) (pipeline.Summary, error) {
		running := active.Add(1)
		defer active.Add(-1)
		if running != 1 {
			return pipeline.Summary{}, errors.New("actions overlapped")
		}
		var finish <-chan struct{}
		switch calls.Add(1) {
		case 1:
			if a.Kind != "normalize" {
				return pipeline.Summary{}, errors.New("exclusive action did not run first")
			}
			finish = finishExclusive
			close(exclusiveStarted)
		case 2:
			progress(pipeline.SyncProgressEvent{Status: pipeline.SyncProgressDiscovering})
			finish = finishRefresh
			close(refreshCaptured)
		default:
			return pipeline.Summary{}, nil
		}
		select {
		case <-finish:
			return pipeline.Summary{}, nil
		case <-ctx.Done():
			return pipeline.Summary{}, ctx.Err()
		}
	}
	exclusive, err := c.Submit(context.Background(), ID(), Action{Kind: "normalize", Sources: c.sources})
	if err != nil {
		t.Fatal(err)
	}
	<-exclusiveStarted
	burst := func() Operation {
		t.Helper()
		const requests = 64
		type result struct {
			operation Operation
			err       error
		}
		start := make(chan struct{})
		results := make(chan result, requests)
		for range requests {
			go func() {
				<-start
				o, err := c.RequestRefresh(context.Background())
				results <- result{o, err}
			}()
		}
		close(start)
		var first Operation
		for range requests {
			r := <-results
			if r.err != nil || r.operation.State != "queued" {
				t.Fatal("refresh not queued", r.operation, r.err)
			}
			if first.ID == "" {
				first = r.operation
			} else if r.operation.ID != first.ID {
				t.Fatal("concurrent requests created duplicate work")
			}
		}
		return first
	}
	first := burst()
	if calls.Load() != 1 {
		t.Fatal("refresh ran before exclusive action completed")
	}
	close(finishExclusive)
	<-refreshCaptured
	second := burst()
	if first.ID == second.ID || calls.Load() != 2 {
		t.Fatal("post-capture requests did not queue one follow-up")
	}
	close(finishRefresh)
	for _, id := range []string{exclusive.ID, first.ID, second.ID} {
		if o := awaitOperation(t, c, id); o.State != "succeeded" {
			t.Fatal("queued action failed", o)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("duplicate refresh ran", calls.Load())
	}
}

func TestRefreshSurvivesRequesterCancellationAndRejectsSharedCancel(t *testing.T) {
	c := testController(t)
	started, finish := make(chan struct{}), make(chan struct{})
	c.runner = func(ctx context.Context, _ Action, _ func(pipeline.SyncProgressEvent), _ func(context.Context) error) (pipeline.Summary, error) {
		close(started)
		select {
		case <-finish:
			return pipeline.Summary{}, nil
		case <-ctx.Done():
			return pipeline.Summary{}, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	o, err := c.RequestRefresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	if err := c.CancelOperation(o.ID); err == nil {
		t.Fatal("shared cancel accepted")
	}
	close(finish)
	if result := awaitOperation(t, c, o.ID); result.State != "succeeded" {
		t.Fatal("request cancellation killed refresh")
	}
}

func TestResetCancelsQueuedRefreshAndBlocksNewRequests(t *testing.T) {
	c := testController(t)
	started, finish := make(chan struct{}), make(chan struct{})
	c.runner = func(ctx context.Context, a Action, progress func(pipeline.SyncProgressEvent), _ func(context.Context) error) (pipeline.Summary, error) {
		if a.Kind == "sync" {
			progress(pipeline.SyncProgressEvent{Status: pipeline.SyncProgressDiscovering})
			close(started)
			select {
			case <-finish:
			case <-ctx.Done():
				return pipeline.Summary{}, ctx.Err()
			}
		}
		return pipeline.Summary{}, nil
	}
	_, _ = c.RequestRefresh(context.Background())
	<-started
	pending, _ := c.RequestRefresh(context.Background())
	reset, err := c.Submit(context.Background(), ID(), Action{Kind: "reset-all"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.RequestRefresh(context.Background()); !errors.Is(err, ErrReset) {
		t.Fatal("refresh admitted across reset")
	}
	if o := awaitOperation(t, c, pending.ID); o.State != "cancelled" {
		t.Fatal("pending refresh survived reset")
	}
	close(finish)
	awaitOperation(t, c, reset.ID)
}

func TestResetDrainsReadersChangesEpochAndCancellationUnblocks(t *testing.T) {
	c := testController(t)
	epoch, release, err := c.ReadPermit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	resetStarted := make(chan struct{})
	c.runner = func(ctx context.Context, a Action, _ func(pipeline.SyncProgressEvent), before func(context.Context) error) (pipeline.Summary, error) {
		close(resetStarted)
		return pipeline.Summary{}, before(ctx)
	}
	o, err := c.Submit(context.Background(), ID(), Action{Kind: "reset-all"})
	if err != nil {
		t.Fatal(err)
	}
	<-resetStarted
	if active, _ := c.Operation(o.ID); active.State != "running" {
		t.Fatal("reset failed to wait for reader")
	}
	release()
	awaitOperation(t, c, o.ID)
	next, release, err := c.ReadPermit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
	if epoch == next {
		t.Fatal("epoch reused after destructive write")
	}
}

func TestExplicitActionOptionsAndFIFO(t *testing.T) {
	c := testController(t)
	started, finish := make(chan struct{}), make(chan struct{})
	received := make(chan Action, 3)
	c.runner = func(ctx context.Context, a Action, _ func(pipeline.SyncProgressEvent), _ func(context.Context) error) (pipeline.Summary, error) {
		received <- a
		if a.Kind == "sync" {
			close(started)
			select {
			case <-finish:
			case <-ctx.Done():
				return pipeline.Summary{}, ctx.Err()
			}
		}
		return pipeline.Summary{}, nil
	}
	a := Action{Kind: "sync", Sources: c.sources, Harnesses: []pipeline.Harness{pipeline.HarnessPi}, FullRefresh: true, Normalize: false, Now: time.Unix(123, 0)}
	first, err := c.Submit(context.Background(), ID(), a)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	second, err := c.Submit(context.Background(), ID(), Action{Kind: "normalize", Sources: c.sources})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := c.Submit(context.Background(), first.ID, a)
	if err != nil || duplicate.ID != first.ID {
		t.Fatal("duplicate operation")
	}
	changed := a
	changed.Normalize = true
	if _, err := c.Submit(context.Background(), first.ID, changed); err == nil {
		t.Fatal("request ID reused with a changed action")
	}
	close(finish)
	awaitOperation(t, c, second.ID)
	got := <-received
	if len(got.Harnesses) != 1 || got.Normalize || !got.FullRefresh || !got.Now.Equal(a.Now) {
		t.Fatal("explicit options changed", got)
	}
	if got := <-received; got.Kind != "normalize" {
		t.Fatal("FIFO lost")
	}
	if len(received) != 0 {
		t.Fatal("duplicate ran twice")
	}
}

func TestRefreshAliasesAndCompletedOutcomesRemainBounded(t *testing.T) {
	c := testController(t)
	started, finish := make(chan struct{}), make(chan struct{})
	c.runner = func(ctx context.Context, _ Action, _ func(pipeline.SyncProgressEvent), _ func(context.Context) error) (pipeline.Summary, error) {
		close(started)
		select {
		case <-finish:
			return pipeline.Summary{}, nil
		case <-ctx.Done():
			return pipeline.Summary{}, ctx.Err()
		}
	}
	id := ID()
	first, err := c.RequestRefreshID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	for range 3000 {
		joined, err := c.RequestRefresh(context.Background())
		if err != nil || joined.ID != first.ID {
			t.Fatal("duplicate work", err)
		}
	}
	c.mu.Lock()
	aliases, queued := len(c.aliases), len(c.queue)
	c.mu.Unlock()
	if aliases != 256 || queued != 0 {
		t.Fatal("unbounded requests", aliases, queued)
	}
	close(finish)
	awaitOperation(t, c, first.ID)
}
