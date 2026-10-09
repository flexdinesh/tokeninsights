package rawcollectorstore

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func TestOutboxRetryAcknowledgementAndResetProtection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}
	for i := 0; i < 2; i++ {
		inserted, err := Record(t.Context(), tx, record, 1)
		if err != nil || inserted != (i == 0) {
			t.Fatal(inserted, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.Bind(t.Context(), "destination", "http://remote", "database"); err != nil {
		t.Fatal(err)
	}
	saved, err := store.Prepare(t.Context(), "destination")
	if err != nil || saved == nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.ResetAllLocked(t.Context(), path); err == nil {
		t.Fatal("reset deleted unaccepted evidence")
	}
	store, err = Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	retry, err := store.Prepare(t.Context(), "destination")
	if err != nil || retry == nil || !bytes.Equal(saved.Request, retry.Request) {
		t.Fatal("retry bytes changed", err)
	}
	response := evidence.Response{Receipt: evidence.Receipt{DatabaseID: "database", DatasetID: "default", StreamID: saved.Batch.StreamID, BatchID: saved.Batch.BatchID, RequestHash: evidence.Hash(saved.Request), FromSequence: 1, ToSequence: 1, Accepted: 1, AcceptedAtMs: 1, InputRevision: 1}}
	wrong := response
	wrong.Receipt.RequestHash = "wrong"
	body, _ := json.Marshal(wrong)
	if err := store.Ack(t.Context(), "destination", body); err == nil {
		t.Fatal("mismatched acknowledgement accepted")
	}
	if n, err := store.Pending(t.Context(), "destination"); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	body, _ = json.Marshal(response)
	if err := store.Ack(t.Context(), "destination", body); err != nil {
		t.Fatal(err)
	}
	response.Processing.Generation = 99
	body, _ = json.Marshal(response)
	if err := store.Ack(t.Context(), "destination", body); err != nil {
		t.Fatal("mutable status invalidated receipt", err)
	}
	if n, err := store.Pending(t.Context(), "destination"); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	if err := store.Bind(t.Context(), "destination", "http://remote", "replacement"); err == nil {
		t.Fatal("destination identity silently changed")
	}
}
