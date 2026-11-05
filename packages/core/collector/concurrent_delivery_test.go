package collector

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/core/rawcollectorstore"
)

type blockedAcceptance struct {
	Delivery
	started chan struct{}
	unblock chan struct{}
	calls   atomic.Int32
}

func (d *blockedAcceptance) Submit(ctx context.Context, protocol int, body []byte) ([]byte, error) {
	if d.calls.Add(1) == 1 {
		close(d.started)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-d.unblock:
		return d.Delivery.Submit(ctx, protocol, body)
	}
}

func TestBlockedLocalAcceptanceAllowsCaptureAndSerializesReplay(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	path, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	store, err := rawcollectorstore.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := store.BeginCapture(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = capture.Record(ctx, evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := capture.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := datastore.Open(ctx, dataPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = data.Close() }()
	metadata, err := data.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	delivery := &blockedAcceptance{Delivery: DirectDelivery{Receiver: data}, started: make(chan struct{}), unblock: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(delivery.unblock) }) }
	defer release()
	options := Options{CollectorDBPath: path, ServerDBPath: dataPath, PublishOnly: true, Destination: &Destination{Identity: "http://local", DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, Local: true, Transport: delivery}}
	type outcome struct {
		result Result
		err    error
	}
	first, second := make(chan outcome, 1), make(chan outcome, 1)
	var publishers sync.WaitGroup
	defer publishers.Wait()
	defer release()
	publish := func(done chan<- outcome) {
		defer publishers.Done()
		result, err := Run(ctx, options)
		done <- outcome{result, err}
	}
	publishers.Add(1)
	go publish(first)
	select {
	case <-delivery.started:
	case <-ctx.Done():
		t.Fatal("local acceptance never started")
	}
	publishers.Add(1)
	go publish(second)
	captureCtx, cancelCapture := context.WithTimeout(ctx, time.Second)
	defer cancelCapture()
	if _, err := captureRaw(captureCtx, Options{CollectorDBPath: path, SyncOptions: pipeline.SyncOptions{Harnesses: []pipeline.Harness{pipeline.HarnessPi}, SourceDir: t.TempDir(), Now: time.Now()}}); err != nil {
		t.Fatal("acceptance held capture lock", err)
	}
	release()
	for index, done := range []<-chan outcome{first, second} {
		select {
		case got := <-done:
			want := int64(1 - index)
			if got.err != nil || got.result.Accepted != want || got.result.Pending != 0 {
				t.Fatal("serialized acceptance lost receipt", got.result, got.err)
			}
		case <-ctx.Done():
			t.Fatal("local acceptance failed to join")
		}
	}
	if delivery.calls.Load() != 1 {
		t.Fatal("concurrent publishers resubmitted acknowledged batch", delivery.calls.Load())
	}
}
