package localbackend

import (
	"context"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/collector"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
)

func TestCaptureCoalescesAndRetainsOneStrongerFollowup(t *testing.T) {
	root := t.TempDir()
	runtime, err := localruntime.OpenWithAppOptions(t.Context(), filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite"), filepath.Join(root, "app.sqlite"), localruntime.Options{CaptureDetails: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	started := make(chan bool, 3)
	release := make(chan struct{}, 3)
	c := &captures{ctx: t.Context(), runtime: runtime, run: func(ctx context.Context, options collector.Options) (collector.Result, error) {
		started <- options.SyncOptions.FullRefresh
		select {
		case <-ctx.Done():
			return collector.Result{}, ctx.Err()
		case <-release:
			return collector.Result{}, nil
		}
	}}
	first, err := c.request(localprotocol.CaptureRequest{SourceDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if <-started {
		t.Fatal("initial capture unexpectedly full")
	}
	for range 10 {
		ticket, err := c.request(localprotocol.CaptureRequest{SourceDir: root})
		if err != nil || ticket != first {
			t.Fatal("identical requests did not share completion", err)
		}
		if _, err := c.request(localprotocol.CaptureRequest{SourceDir: root, FullRefresh: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.request(localprotocol.CaptureRequest{SourceDir: filepath.Join(root, "different")}); !errors.Is(err, localprotocol.ErrCaptureBusy) {
		t.Fatal("different source silently merged", err)
	}
	release <- struct{}{}
	if !<-started {
		t.Fatal("stronger followup lost")
	}
	for range 10 {
		if _, err := c.request(localprotocol.CaptureRequest{SourceDir: root, FullRefresh: true}); err != nil {
			t.Fatal(err)
		}
	}
	release <- struct{}{}
	<-first.done
	select {
	case <-started:
		t.Fatal("unbounded repeated followups")
	default:
	}
	if first.result.ErrorCode != "" {
		t.Fatal("successful shared capture failed", first.result)
	}
	if first.result.AttemptID == "" {
		t.Fatal("shared result lost exact observer identity")
	}
	var found bool
	for _, attempt := range runtime.Progress.Snapshot().Attempts {
		if attempt.AttemptID == first.result.AttemptID {
			found = true
		}
	}
	if !found {
		t.Fatal("shared result does not identify an actual observer")
	}
}

func TestCaptureCancellationJoinsAndSanitizesError(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runtime, err := localruntime.Open(ctx, filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	started, finished := make(chan struct{}), make(chan struct{})
	c := &captures{ctx: ctx, runtime: runtime, run: func(ctx context.Context, _ collector.Options) (collector.Result, error) {
		close(started)
		defer close(finished)
		<-ctx.Done()
		return collector.Result{}, errors.New("private source path and prompt text")
	}}
	ticket, err := c.request(localprotocol.CaptureRequest{})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	c.join()
	select {
	case <-finished:
	default:
		t.Fatal("capture not joined")
	}
	if ticket.result.ErrorCode != "collection_failed" {
		t.Fatal("unsanitized collection failure", ticket.result)
	}
}
