package localruntime_test

import (
	"context"
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
	runtime, err := localruntime.Open(t.Context(), settings.CollectorDBPath, settings.ServerDBPath)
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
	var total int64
	if err := runtime.Store.SQL().QueryRowContext(ctx, "SELECT COALESCE(SUM(total_tokens),0) FROM analytics.confirmed").Scan(&total); err != nil || total != 100 {
		t.Fatal("queued replay inflated/lost usage", total, err)
	}
}
