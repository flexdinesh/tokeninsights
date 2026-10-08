package localruntime_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/analytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/datastore"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
)

func TestVisibilityReportsDurableFailureAfterRestartAndRecovers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "server.duckdb")
	store, err := datastore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	receipt := pendingBatch(t, store)
	var body string
	if err := store.SQL().QueryRow("SELECT record_json FROM raw.evidence").Scan(&body); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().Exec("UPDATE raw.evidence SET record_json='invalid'"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ProcessNext(t.Context()); err == nil {
		t.Fatal("corrupt evidence did not fail processing")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := localruntime.Open(t.Context(), filepath.Join(root, "collector.sqlite"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := runtime.WaitVisible(ctx); !errors.Is(err, localruntime.ErrProcessingFailed) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("durable failure mislabeled or hidden until deadline", err)
	}
	status, err := analytics.Status(t.Context(), runtime.Store)
	if err != nil || status.Pending != 1 || status.Failed != 1 {
		t.Fatal("failed scope missing from status", status, err)
	}
	after, err := runtime.Store.Receipt(t.Context(), "stream", "batch")
	if err != nil || after.Receipt != receipt || after.Processing.Pending != 1 {
		t.Fatal("visibility failure changed acceptance", after, err)
	}
	// Restore the deliberately corrupted fixture, then use normal generation recovery.
	if _, err := runtime.Store.SQL().Exec("UPDATE raw.evidence SET record_json=?", body); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Store.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WaitVisible(ctx); err != nil {
		t.Fatal("recovery retained obsolete processing failure", err)
	}
	status, err = analytics.Status(t.Context(), runtime.Store)
	if err != nil || status.Pending != 0 || status.Failed != 0 || status.Metadata.Generation != status.Metadata.TargetGeneration {
		t.Fatal("recovery did not become visible", status, err)
	}
	after, err = runtime.Store.Receipt(t.Context(), "stream", "batch")
	if err != nil || after.Receipt != receipt || after.Processing.Pending != 0 {
		t.Fatal("recovery changed receipt", after, err)
	}
}

func TestVisibilityDistinguishesSlowProcessingCancellationAndReadFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "server.duckdb")
	store, err := datastore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	pendingBatch(t, store)
	// Hold eligible work without recording a failure; no wall-clock race with a worker.
	if _, err := store.SQL().Exec("UPDATE processing.scopes SET retry_at_ms=?", time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := localruntime.Open(t.Context(), filepath.Join(root, "collector.sqlite"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if err := runtime.WaitVisible(ctx); !errors.Is(err, localruntime.ErrProcessingTimeout) || !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, localruntime.ErrProcessingFailed) {
		t.Fatal("slow processing misclassified", err)
	}
	ctx, cancel = context.WithCancel(t.Context())
	cancel()
	if err := runtime.WaitVisible(ctx); !errors.Is(err, context.Canceled) || errors.Is(err, localruntime.ErrProcessingTimeout) {
		t.Fatal("caller cancellation misclassified", err)
	}
	if err := runtime.Store.SQL().Close(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.WaitVisible(t.Context()); err == nil || errors.Is(err, localruntime.ErrProcessingFailed) || errors.Is(err, localruntime.ErrProcessingTimeout) {
		t.Fatal("storage query failure misclassified", err)
	}
}

func TestVisibilityAllowsDueFailureRetryWithoutRebuilding(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "server.duckdb")
	store, err := datastore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	receipt := pendingBatch(t, store)
	work, found, err := store.LoadWork(t.Context())
	if err != nil || !found {
		t.Fatal("missing retry work", found, err)
	}
	store.RecordFailure(t.Context(), work)
	if _, err := store.SQL().Exec("UPDATE processing.scopes SET retry_at_ms=0"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := localruntime.Open(t.Context(), filepath.Join(root, "collector.sqlite"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := runtime.WaitVisible(ctx); err != nil {
		t.Fatal("old failure prevented due retry", err)
	}
	status, err := analytics.Status(t.Context(), runtime.Store)
	if err != nil || status.Pending != 0 || status.Failed != 0 || status.FailedRetryAtMs != 0 || status.Metadata.Generation != 1 {
		t.Fatal("retry retained failure or rebuilt generation", status, err)
	}
	after, err := runtime.Store.Receipt(t.Context(), "stream", "batch")
	if err != nil || after.Receipt != receipt || after.Processing.Pending != 0 {
		t.Fatal("due retry changed accepted receipt", after, err)
	}
}
