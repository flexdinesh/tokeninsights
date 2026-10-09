package localruntime_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	api "github.com/flexdinesh/tokeninsights/packages/cli/internal/server/api"
)

func TestStartCollectionKeepsSavedUsageReadable(t *testing.T) {
	root := t.TempDir()
	collectorPath, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.duckdb")
	runtime, err := localruntime.Open(t.Context(), collectorPath, dataPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	pendingBatch(t, runtime.Store)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := runtime.WaitVisible(ctx); err != nil {
		t.Fatal(err)
	}
	started := make(chan collector.Options, 1)
	progress := make(chan pipeline.SyncProgressEvent, 1)
	release := make(chan struct{})
	expected := collector.Result{Accepted: 2, Batches: 1, PendingKnown: true}
	done := runtime.StartCollection(ctx, pipeline.SyncOptions{
		DBPath: filepath.Join(root, "wrong.sqlite"), Harnesses: []pipeline.Harness{pipeline.HarnessPi},
		Progress: func(event pipeline.SyncProgressEvent) { progress <- event },
	}, func(ctx context.Context, options collector.Options) (collector.Result, error) {
		options.SyncOptions.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressSynced})
		options.DeliveryProgress(collector.DeliveryProgress{Accepted: 1, Pending: 1, PendingKnown: true})
		started <- options
		select {
		case <-release:
			return expected, nil
		case <-ctx.Done():
			return collector.Result{}, ctx.Err()
		}
	})
	// Registration precedes both asynchronous collection and the first UI read.
	if attempts := runtime.Progress.Snapshot().Attempts; len(attempts) != 1 {
		t.Fatalf("startup progress missing: %+v", attempts)
	}
	var options collector.Options
	select {
	case options = <-started:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if options.CollectorDBPath != collectorPath || options.ServerDBPath != dataPath || options.SyncOptions.DBPath != collectorPath || options.Destination != runtime.Destination || !options.Destination.Local {
		t.Fatalf("incorrect direct bindings: %+v", options)
	}
	if event := <-progress; event.Harness != pipeline.HarnessPi || event.Status != pipeline.SyncProgressSynced {
		t.Fatalf("caller progress lost: %+v", event)
	}
	if attempts := runtime.Progress.Snapshot().Attempts; attempts[0].Stage != "submitting" || attempts[0].AcknowledgedEntries != 1 || attempts[0].Pending != 1 {
		t.Fatalf("runtime progress missing: %+v", attempts)
	}
	period := api.PeriodFilter("all")
	usage, err := runtime.Query.AllUsage(ctx, api.GetUsageParams{Period: &period})
	if err != nil || usage.Summary.Total != 120 || usage.FactCount == nil || *usage.FactCount != 1 {
		t.Fatalf("saved usage unavailable while collecting: %+v %v", usage, err)
	}
	close(release)
	select {
	case result := <-done:
		if result.Err != nil || result.Result.Accepted != expected.Accepted {
			t.Fatalf("terminal collection result lost: %+v", result)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if attempts := runtime.Progress.Snapshot().Attempts; attempts[0].Stage != "accepted" || attempts[0].AcknowledgedEntries != 2 {
		t.Fatalf("terminal progress missing: %+v", attempts)
	}
	if _, open := <-done; open {
		t.Fatal("result channel not closed")
	}
}

func TestCloseCancelsAndJoinsStartupCollectionBeforeClosingStorage(t *testing.T) {
	root := t.TempDir()
	runtime, err := localruntime.Open(t.Context(), filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	storageAvailable := make(chan error, 1)
	release := make(chan struct{})
	done := runtime.StartCollection(ctx, pipeline.SyncOptions{}, func(ctx context.Context, _ collector.Options) (collector.Result, error) {
		<-ctx.Done()
		_, err := runtime.Store.Metadata(context.Background())
		storageAvailable <- err
		<-release
		return collector.Result{}, ctx.Err()
	})
	closed := make(chan error, 1)
	go func() { closed <- runtime.Close() }()
	select {
	case err := <-storageAvailable:
		if err != nil {
			close(release)
			t.Fatalf("storage closed before collection joined: %v", err)
		}
	case <-ctx.Done():
		close(release)
		t.Fatal(ctx.Err())
	}
	select {
	case err := <-closed:
		close(release)
		t.Fatalf("Close returned before collection joined: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if result := <-done; !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("runtime cancellation lost: %+v", result)
	}
	result := <-runtime.StartCollection(ctx, pipeline.SyncOptions{}, func(context.Context, collector.Options) (collector.Result, error) {
		t.Error("collection started after runtime close")
		return collector.Result{}, nil
	})
	if !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("closed runtime accepted collection: %+v", result)
	}
}

func TestCallerCancellationKeepsRuntimeAvailable(t *testing.T) {
	root := t.TempDir()
	runtime, err := localruntime.Open(t.Context(), filepath.Join(root, "collector.sqlite"), filepath.Join(root, "data.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	ctx, cancel := context.WithCancel(t.Context())
	done := runtime.StartCollection(ctx, pipeline.SyncOptions{}, func(ctx context.Context, _ collector.Options) (collector.Result, error) {
		<-ctx.Done()
		return collector.Result{}, ctx.Err()
	})
	cancel()
	select {
	case result := <-done:
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("caller cancellation lost: %+v", result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("collection did not cancel")
	}
	if attempts := runtime.Progress.Snapshot().Attempts; len(attempts) != 1 || attempts[0].Stage != "interrupted" {
		t.Fatalf("cancelled progress not terminal: %+v", attempts)
	}
	if _, err := runtime.ProcessingStatus(t.Context()); err != nil {
		t.Fatalf("caller cancellation stopped runtime: %v", err)
	}
}
