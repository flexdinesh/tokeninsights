package rawcollectorstore

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func destinationStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	tx, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, ordinal := range []int64{1, 2} {
		record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: ordinal, Data: json.RawMessage(`{"type":"session","id":"session"}`)}
		if _, err := Record(t.Context(), tx, record, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return store
}

func acknowledgeDestination(t *testing.T, store *Store, id string, saved *SavedBatch) {
	t.Helper()
	body, err := json.Marshal(evidence.Response{Receipt: evidence.Receipt{DatabaseID: saved.Batch.DatabaseID, DatasetID: saved.Batch.EffectiveDatasetID(), StreamID: saved.Batch.StreamID, BatchID: saved.Batch.BatchID, RequestHash: evidence.Hash(saved.Request), FromSequence: saved.Batch.FromSequence, ToSequence: saved.Batch.ToSequence, Accepted: int64(len(saved.Batch.Entries)), AcceptedAtMs: 1, InputRevision: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Ack(t.Context(), id, body); err != nil {
		t.Fatal(err)
	}
}

func TestPrivateAliasFinishesOriginalProtocolBeforeHighestCursor(t *testing.T) {
	store := destinationStore(t)
	const canonical = "http://127.0.0.1:8765"
	if err := store.Bind(t.Context(), "canonical", canonical, "database"); err != nil {
		t.Fatal(err)
	}
	complete, err := store.Prepare(t.Context(), "canonical")
	if err != nil || complete == nil {
		t.Fatal(err)
	}
	acknowledgeDestination(t, store, "canonical", complete)
	if err := store.Bind(t.Context(), "private", "http://local", "database"); err != nil {
		t.Fatal(err)
	}
	batch := complete.Batch
	batch.ProtocolVersion = evidence.LegacyProtocolVersion
	batch.DatasetID = ""
	batch.BatchID = "retained-protocol-two"
	batch.ToSequence = 1
	batch.Entries = batch.Entries[:1]
	original, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.ExecContext(t.Context(), `INSERT INTO evidence_batches(batch_id,destination_id,stream_id,database_id,dataset_id,protocol_version,first_sequence,last_sequence,request_hash,request_bytes) VALUES(?,?,?,?,?,?,?,?,?,?)`, batch.BatchID, "private", batch.StreamID, "database", "default", batch.ProtocolVersion, 1, 1, evidence.Hash(original), original); err != nil {
		t.Fatal(err)
	}
	id, err := store.ResolveDeliveryDestination(t.Context(), canonical, "database", "default", true)
	if err != nil || id != "private" {
		t.Fatal("pending alias abandoned", id, err)
	}
	saved, err := store.Prepare(t.Context(), id)
	if err != nil || saved == nil || !bytes.Equal(saved.Request, original) || saved.Batch.ProtocolVersion != evidence.LegacyProtocolVersion {
		t.Fatal("retained request rewritten", saved, err)
	}
	acknowledgeDestination(t, store, id, saved)
	id, err = store.ResolveDeliveryDestination(t.Context(), canonical, "database", "default", true)
	if err != nil || id != "canonical" {
		t.Fatal("highest verified cursor ignored", id, err)
	}
	if next, err := store.Prepare(t.Context(), id); err != nil || next != nil {
		t.Fatal("acknowledged evidence replayed", next, err)
	}
	var privateCursor int64
	if err := store.DB.QueryRowContext(t.Context(), "SELECT acknowledged_sequence FROM evidence_destinations WHERE destination_id='private'").Scan(&privateCursor); err != nil || privateCursor != 1 {
		t.Fatal("cursor copied between bindings", privateCursor, err)
	}
}

func TestDestinationAliasRequiresLocalDatabaseAndDataset(t *testing.T) {
	for _, scenario := range []struct {
		name, database, dataset string
		local                   bool
	}{
		{name: "remote", database: "database", dataset: "default"},
		{name: "replacement", database: "replacement", dataset: "default", local: true},
		{name: "account", database: "database", dataset: "other-user", local: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := destinationStore(t)
			if err := store.Bind(t.Context(), "private", "http://local", "database"); err != nil {
				t.Fatal(err)
			}
			saved, err := store.Prepare(t.Context(), "private")
			if err != nil || saved == nil {
				t.Fatal(err)
			}
			acknowledgeDestination(t, store, "private", saved)
			id, err := store.ResolveDeliveryDestination(t.Context(), "http://127.0.0.1:8765", scenario.database, scenario.dataset, scenario.local)
			if err != nil || id == "private" {
				t.Fatal("foreign alias reused", id, err)
			}
			if pending, err := store.Pending(t.Context(), id); err != nil || pending != 2 {
				t.Fatal("foreign cursor inherited", pending, err)
			}
		})
	}
}

func TestConflictingRemoteDestinationsRejectBeforeSelectingCursor(t *testing.T) {
	store := destinationStore(t)
	for _, database := range []string{"database", "replacement"} {
		if err := store.Bind(t.Context(), database, "https://remote.example", database); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ResolveDeliveryDestination(t.Context(), "https://remote.example", "database", "default", false); err == nil {
		t.Fatal("ambiguous remote database accepted")
	}
}

func TestBothLocalBindingsFinishRetainedRequests(t *testing.T) {
	store := destinationStore(t)
	const canonical = "http://127.0.0.1:8765"
	requests := make(map[string][]byte)
	for id, endpoint := range map[string]string{"canonical": canonical, "private": "http://local"} {
		if err := store.Bind(t.Context(), id, endpoint, "database"); err != nil {
			t.Fatal(err)
		}
		saved, err := store.Prepare(t.Context(), id)
		if err != nil || saved == nil {
			t.Fatal(err)
		}
		requests[id] = saved.Request
	}
	for range 2 {
		id, err := store.ResolveDeliveryDestination(t.Context(), canonical, "database", "default", true)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := store.Prepare(t.Context(), id)
		if err != nil || saved == nil || !bytes.Equal(saved.Request, requests[id]) {
			t.Fatal("pending request skipped or rewritten", saved, err)
		}
		acknowledgeDestination(t, store, id, saved)
		delete(requests, id)
	}
	if len(requests) != 0 {
		t.Fatal("pending binding abandoned")
	}
	id, err := store.ResolveDeliveryDestination(t.Context(), canonical, "database", "default", true)
	if err != nil || id != "canonical" {
		t.Fatal(id, err)
	}
	if saved, err := store.Prepare(t.Context(), id); err != nil || saved != nil {
		t.Fatal("history replayed after both receipts", saved, err)
	}
}

func TestRetainedSlashBindingReusedWithoutCursorReplay(t *testing.T) {
	store := destinationStore(t)
	const canonical = "https://remote.example/prefix"
	if err := store.Bind(t.Context(), "slash-binding", canonical+"/", "database"); err != nil {
		t.Fatal(err)
	}
	original, err := store.Prepare(t.Context(), "slash-binding")
	if err != nil || original == nil {
		t.Fatal(err)
	}
	id, err := store.ResolveDeliveryDestination(t.Context(), canonical, "database", "default", false)
	if err != nil || id != "slash-binding" {
		t.Fatal("slash binding abandoned", id, err)
	}
	retry, err := store.Prepare(t.Context(), id)
	if err != nil || retry == nil || !bytes.Equal(retry.Request, original.Request) {
		t.Fatal("slash normalization rewrote retained request", retry, err)
	}
	acknowledgeDestination(t, store, id, retry)
	id, err = store.ResolveDeliveryDestination(t.Context(), canonical, "database", "default", false)
	if err != nil || id != "slash-binding" {
		t.Fatal(id, err)
	}
	if pending, err := store.Pending(t.Context(), id); err != nil || pending != 0 {
		t.Fatal("slash normalization replayed cursor", pending, err)
	}
}
