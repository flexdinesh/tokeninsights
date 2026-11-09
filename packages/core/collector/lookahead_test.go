package collector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/datastore"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
	"github.com/flexdinesh/tokeninsights/packages/core/rawcollectorstore"
)

func captureSessions(t *testing.T, path string, from, count int) {
	t.Helper()
	store, err := rawcollectorstore.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	capture, err := store.BeginCapture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = capture.Rollback() }()
	for i := from; i < from+count; i++ {
		data := json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, i))
		if _, err := capture.Record(t.Context(), evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: int64(i), Data: data}, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := capture.Commit(); err != nil {
		t.Fatal(err)
	}
}

// recordingAcceptance observes each request and can act around acceptance.
type recordingAcceptance struct {
	Delivery
	bodies [][]byte
	during func(call int)
	lose   func(call int) bool
}

func (d *recordingAcceptance) Submit(ctx context.Context, protocol int, body []byte) ([]byte, error) {
	d.bodies = append(d.bodies, bytes.Clone(body))
	call := len(d.bodies)
	if d.during != nil {
		d.during(call)
	}
	response, err := d.Delivery.Submit(ctx, protocol, body)
	if err == nil && d.lose != nil && d.lose(call) {
		return nil, errors.New("acknowledgement lost after acceptance")
	}
	return response, err
}

func lookaheadFixture(t *testing.T, records int) (string, Options, *recordingAcceptance) {
	t.Helper()
	root := t.TempDir()
	path, dataPath := filepath.Join(root, "collector.sqlite"), filepath.Join(root, "server.sqlite")
	captureSessions(t, path, 0, records)
	data, err := datastore.Open(t.Context(), dataPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	metadata, err := data.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	delivery := &recordingAcceptance{Delivery: DirectDelivery{Receiver: data}}
	options := Options{CollectorDBPath: path, ServerDBPath: dataPath, PublishOnly: true, Destination: &Destination{Identity: "http://local", DatabaseID: metadata.DatabaseID, DatasetID: metadata.DatasetID, Local: true, Transport: delivery}}
	return path, options, delivery
}

func decodedRange(t *testing.T, body []byte) (int64, int64) {
	t.Helper()
	batch, err := evidence.DecodeBatch(body)
	if err != nil {
		t.Fatal(err)
	}
	return batch.FromSequence, batch.ToSequence
}

func TestLookaheadDeliversEvidenceCapturedDuringAcceptance(t *testing.T) {
	const initial, appended = evidence.MaxEntries + 20, 7
	path, options, delivery := lookaheadFixture(t, initial)
	delivery.during = func(call int) {
		if call == 1 {
			captureSessions(t, path, initial, appended)
		}
	}
	result, err := Run(t.Context(), options)
	if err != nil || result.Pending != 0 || result.Accepted != initial+appended {
		t.Fatalf("delivery lost evidence: %+v %v", result, err)
	}
	next := int64(1)
	for _, body := range delivery.bodies {
		from, to := decodedRange(t, body)
		if from != next {
			t.Fatalf("delivery gap: from %d want %d", from, next)
		}
		next = to + 1
	}
	if next != initial+appended+1 {
		t.Fatalf("delivered through %d want %d", next-1, initial+appended)
	}
}

func TestLookaheadLostAcknowledgementReplaysIdenticalRequest(t *testing.T) {
	_, options, delivery := lookaheadFixture(t, 2*evidence.MaxEntries+5)
	delivery.lose = func(call int) bool { return call == 2 }
	first, err := Run(t.Context(), options)
	if err == nil || first.Accepted != evidence.MaxEntries || first.Pending == 0 {
		t.Fatalf("lost acknowledgement completed: %+v %v", first, err)
	}
	lost := delivery.bodies[1]
	delivery.lose = nil
	delivery.bodies = nil
	second, err := Run(t.Context(), options)
	if err != nil || second.Pending != 0 || second.Accepted != evidence.MaxEntries+5 {
		t.Fatalf("replay: %+v %v", second, err)
	}
	if !bytes.Equal(delivery.bodies[0], lost) {
		t.Fatal("replay changed request bytes")
	}
}

func TestLookaheadJoinsWhenAcceptanceIsCancelled(t *testing.T) {
	_, options, delivery := lookaheadFixture(t, 2*evidence.MaxEntries)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	delivery.during = func(call int) {
		if call == 1 {
			cancel()
		}
	}
	if _, err := Run(ctx, options); err == nil {
		t.Fatal("cancelled delivery succeeded")
	}
	delivery.during = nil
	cancelledRequest := delivery.bodies[0]
	delivery.bodies = nil
	result, err := Run(t.Context(), options)
	if err != nil || result.Pending != 0 || result.Accepted != 2*evidence.MaxEntries {
		t.Fatalf("recovery: %+v %v", result, err)
	}
	if !bytes.Equal(delivery.bodies[0], cancelledRequest) {
		t.Fatal("cancelled request not replayed identically")
	}
}
