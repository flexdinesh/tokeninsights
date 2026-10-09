package rawcollectorstore

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func appendTestRecord(t *testing.T, store *Store) {
	t.Helper()
	tx, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = Record(t.Context(), tx, evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"same-session"}`)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func receiptFor(saved *SavedBatch) evidence.Response {
	b := saved.Batch
	return evidence.Response{Receipt: evidence.Receipt{DatabaseID: b.DatabaseID, DatasetID: b.DatasetID, StreamID: b.StreamID, BatchID: b.BatchID, RequestHash: evidence.Hash(saved.Request), FromSequence: b.FromSequence, ToSequence: b.ToSequence, Accepted: int64(len(b.Entries)), AcceptedAtMs: 1, InputRevision: 1}}
}

func TestDatasetSwitchRotationAndReceiptBinding(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	appendTestRecord(t, store)
	a, err := store.ResolveDestination(t.Context(), "https://shared", "database", "user-a", false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Prepare(t.Context(), a)
	if err != nil || first == nil {
		t.Fatal(first, err)
	}
	if first.Batch.DatasetID != "user-a" || first.Batch.ProtocolVersion != 3 {
		t.Fatal(first.Batch)
	}
	response := receiptFor(first)
	response.Receipt.DatasetID = "user-b"
	body, _ := json.Marshal(response)
	if err := store.Ack(t.Context(), a, body); err == nil {
		t.Fatal("foreign dataset acknowledged")
	}
	if n, err := store.Pending(t.Context(), a); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	response = receiptFor(first)
	body, _ = json.Marshal(response)
	if err := store.Ack(t.Context(), a, body); err != nil {
		t.Fatal(err)
	}
	rotated, err := store.ResolveDestination(t.Context(), "https://shared", "database", "user-a", false)
	if err != nil || rotated != a {
		t.Fatal("rotation changed cursor", rotated, err)
	}
	if n, err := store.Pending(t.Context(), rotated); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	b, err := store.ResolveDestination(t.Context(), "https://shared", "database", "user-b", false)
	if err != nil || b == a {
		t.Fatal("account switch reused binding", b, err)
	}
	second, err := store.Prepare(t.Context(), b)
	if err != nil || second == nil || second.Batch.DatasetID != "user-b" || second.Batch.FromSequence != 1 {
		t.Fatal(second, err)
	}
	if _, err := store.ResolveDestination(t.Context(), "https://shared", "replacement", "user-a", false); err == nil {
		t.Fatal("remote replacement inherited history")
	}
	for _, field := range []string{"dataset_id", "protocol_version"} {
		if _, err := store.DB.Exec("UPDATE evidence_batches SET "+field+"="+field+" WHERE batch_id=?", second.Batch.BatchID); err == nil {
			t.Fatal("request binding mutable", field)
		}
	}
}
