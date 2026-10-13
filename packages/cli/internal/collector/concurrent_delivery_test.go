package collector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
)

func TestBlockedRemoteDoesNotBlockCaptureOrAnotherDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	store, err := rawcollectorstore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := store.BeginCapture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, err = capture.Record(t.Context(), evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := capture.Commit(); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	started, unblock := make(chan struct{}), make(chan struct{})
	var submissions atomic.Int32
	var once sync.Once
	release := func() { once.Do(func() { close(unblock) }) }
	defer release()
	receiver := func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		batch, err := evidence.DecodeBatch(body)
		if err != nil {
			t.Error(err)
			return
		}
		_ = json.NewEncoder(w).Encode(evidence.Response{Receipt: evidence.Receipt{DatabaseID: batch.DatabaseID, DatasetID: batch.DatasetID, StreamID: batch.StreamID, BatchID: batch.BatchID, RequestHash: evidence.Hash(body), FromSequence: batch.FromSequence, ToSequence: batch.ToSequence, Accepted: int64(len(batch.Entries)), AcceptedAtMs: 1, InputRevision: 1}})
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if submissions.Add(1) == 1 {
			close(started)
		}
		select {
		case <-unblock:
			receiver(w, r)
		case <-r.Context().Done():
		}
	}))
	defer remote.Close()
	// Release the handler before httptest closes its active connections on failure.
	defer release()
	local := httptest.NewServer(http.HandlerFunc(receiver))
	defer local.Close()
	destination := func(target string, local bool) *Destination {
		return &Destination{Identity: target, DatabaseID: "database", DatasetID: "default", Local: local, Transport: HTTPDelivery{URL: target}}
	}
	done := make(chan error, 1)
	go func() {
		_, err := Run(t.Context(), Options{CollectorDBPath: path, PublishOnly: true, Destination: destination(remote.URL, false)})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("remote never submitted")
	}
	type outcome struct {
		result Result
		err    error
	}
	second := make(chan outcome, 1)
	go func() {
		result, err := Run(t.Context(), Options{CollectorDBPath: path, PublishOnly: true, Destination: destination(remote.URL, false)})
		second <- outcome{result: result, err: err}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err = captureRaw(ctx, Options{CollectorDBPath: path, SyncOptions: pipeline.SyncOptions{Harnesses: []pipeline.Harness{pipeline.HarnessPi}, SourceDir: t.TempDir(), Now: time.Now()}})
	if err != nil {
		t.Fatal("remote blocked capture", err)
	}
	result, err := Run(ctx, Options{CollectorDBPath: path, PublishOnly: true, ServerDBPath: filepath.Join(filepath.Dir(path), "local.sqlite"), Destination: destination(local.URL, true)})
	if err != nil || result.Accepted != 1 || result.Pending != 0 {
		t.Fatal("remote blocked local delivery", result, err)
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("remote failed to finish")
	}
	select {
	case outcome := <-second:
		if outcome.err != nil || outcome.result.Accepted != 0 {
			t.Fatal("concurrent destination replay", outcome.result, outcome.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second publisher failed to finish")
	}
	if submissions.Load() != 1 {
		t.Fatal("concurrent publishers submitted the same batch", submissions.Load())
	}
	result, err = Run(t.Context(), Options{CollectorDBPath: path, PublishOnly: true, Destination: destination(remote.URL, false)})
	if err != nil || result.Accepted != 0 || result.Pending != 0 {
		t.Fatal("remote lost acknowledgement", result, err)
	}
}
