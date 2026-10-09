package localruntime_test

import (
	"context"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collectorprogress"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestViewerConsumesQueuedRequestsInProcessAndReplaysWithoutInflation(t *testing.T) {
	root := t.TempDir()
	settings := config.Defaults()
	settings.CollectorDBPath = filepath.Join(root, "collector.sqlite")
	settings.ServerDBPath = filepath.Join(root, "server.duckdb")
	runtime, err := localruntime.OpenWithAppOptions(t.Context(), settings.CollectorDBPath, settings.ServerDBPath, filepath.Join(root, "app.sqlite"), localruntime.Options{CaptureDetails: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	source := filepath.Join(root, "sources", "pi", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(source), 0700); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session","id":"queued-session"}` + "\n" + `{"type":"message","id":"request","message":{"role":"assistant","timestamp":1767225600000,"usage":{"input":80,"output":20,"totalTokens":100}}}` + "\n"
	if err := os.WriteFile(source, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := syncjob.NewSpec(settings, []pipeline.Harness{pipeline.HarnessPi}, filepath.Join(root, "sources"), false, false)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := syncjob.Open(t.Context(), settings.CollectorDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = queue.Close() }()
	first, err := queue.Enqueue(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := queue.Enqueue(t.Context(), spec)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for _, job := range []syncjob.Job{first, second} {
		result, err := queue.Wait(ctx, job.ID)
		if err != nil || result.State != "accepted" {
			t.Fatal(result, err)
		}
	}
	progress := runtime.Progress.Snapshot()
	if progress.InstanceID != runtime.InstanceID || len(progress.Attempts) != 2 {
		t.Fatal("queued progress missing", progress)
	}
	var accepted int64
	details := runtime.Progress.Details()
	for _, attempt := range progress.Attempts {
		// Unchanged sources may skip capture on the replay job.
		if attempt.Stage != "accepted" || attempt.Harnesses["pi"] != "complete" && attempt.Harnesses["pi"] != "skipped" {
			t.Fatal("queued capture not finished", attempt)
		}
		accepted += attempt.AcknowledgedEntries
		capture := details.Captures[attempt.AttemptID]["pi"]
		if capture.Phase != collectorprogress.CaptureComplete || capture.Total != 1 || capture.Checked != 1 || capture.Captured+capture.Unchanged != 1 {
			t.Fatal("queued source measurements missing or mixed", attempt, capture)
		}
	}
	if accepted == 0 {
		t.Fatal("queued acceptance not reported", progress)
	}
	var total int64
	if err := runtime.Store.SQL().QueryRowContext(ctx, "SELECT COALESCE(SUM(total_tokens),0) FROM analytics.confirmed").Scan(&total); err != nil || total != 100 {
		t.Fatal("queued replay inflated/lost usage", total, err)
	}
}
