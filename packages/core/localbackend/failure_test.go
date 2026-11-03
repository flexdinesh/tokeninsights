package localbackend

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
	"github.com/flexdinesh/tokeninsights/packages/core/collector"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/localprotocol"
	"github.com/flexdinesh/tokeninsights/packages/core/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/core/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/core/queryclient"
)

func TestFailedSharedCaptureKeepsSavedPublicUsageAndSanitizedDetails(t *testing.T) {
	root := t.TempDir()
	settings := localprotocol.Settings{CollectorDBPath: filepath.Join(root, "collector.sqlite"), ServerDBPath: filepath.Join(root, "server.sqlite"), AppDBPath: filepath.Join(root, "app.sqlite")}
	runtime, err := localruntime.OpenWithApp(t.Context(), settings.CollectorDBPath, settings.ServerDBPath, settings.AppDBPath)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := runtime.Store.Metadata(t.Context())
	if err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2,
		Data:    json.RawMessage(`{"type":"message","id":"request","message":{"role":"assistant","timestamp":1767225600000,"usage":{"input":80,"output":20,"totalTokens":100}}}`),
		Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
	body, err := json.Marshal(evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, StreamID: "stream", BatchID: "batch", FromSequence: 1, ToSequence: 1, Entries: []evidence.Entry{{Sequence: 1, Record: record}}})
	if err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if _, err := runtime.Store.Accept(t.Context(), evidence.ProtocolVersion, body); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	visible, cancelVisible := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelVisible()
	if err := runtime.WaitVisible(visible); err != nil {
		_ = runtime.Close()
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	ready, finished := make(chan localprotocol.Descriptor, 1), make(chan error, 1)
	go func() {
		finished <- serve(ctx, settings, io.Discard, func(d localprotocol.Descriptor) error { ready <- d; return nil }, func(_ context.Context, options collector.Options) (collector.Result, error) {
			options.SyncOptions.Progress(pipeline.SyncProgressEvent{Harness: pipeline.HarnessPi, Status: pipeline.SyncProgressFailed})
			return collector.Result{Collection: pipeline.Summary{RawFacts: 3, Skipped: 1, Failed: 1}}, errors.New("private source path and prompt")
		})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-finished; err != nil {
			t.Error(err)
		}
	})
	<-ready
	session, err := localclient.Connect(t.Context(), localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, AppDBPath: settings.AppDBPath, Host: "127.0.0.1", Dashboard: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = session.Close() }()
	result, err := session.Collect(t.Context(), false, root)
	if err != nil || result.ErrorCode != "collection_failed" || result.AttemptID == "" || result.RawFacts != 3 || result.Skipped != 1 {
		t.Fatal("sanitized completion lost", result, err)
	}
	progress, err := session.Progress(t.Context())
	if err != nil || len(progress.Collection.Attempts) != 1 || progress.Collection.Attempts[0].AttemptID != result.AttemptID || progress.Collection.Attempts[0].Stage != "failed" || progress.Collection.Attempts[0].ErrorCode != "collection_failed" {
		t.Fatal("failure details unavailable", progress, err)
	}
	encoded, err := json.Marshal(struct {
		Result   localprotocol.CollectionResult
		Progress interface{}
	}{result, progress})
	if err != nil || strings.Contains(string(encoded), "private source") || strings.Contains(string(encoded), "prompt") {
		t.Fatal("capture leaked private failure", err)
	}
	public, err := queryclient.New(session.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := public.AllUsage(t.Context(), analytics.Query{Selection: analytics.Selection{Period: "all"}})
	if err != nil || usage.Summary.TotalTokens != 100 {
		t.Fatal("capture failure lost saved public usage", usage.Summary.TotalTokens, err)
	}
}
