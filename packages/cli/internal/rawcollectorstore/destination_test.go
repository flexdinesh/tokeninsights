package rawcollectorstore

import (
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
	body, err := json.Marshal(evidence.Response{Receipt: evidence.Receipt{DatabaseID: saved.Batch.DatabaseID, DatasetID: saved.Batch.DatasetID, StreamID: saved.Batch.StreamID, BatchID: saved.Batch.BatchID, RequestHash: evidence.Hash(saved.Request), FromSequence: saved.Batch.FromSequence, ToSequence: saved.Batch.ToSequence, Accepted: int64(len(saved.Batch.Entries)), AcceptedAtMs: 1, InputRevision: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Ack(t.Context(), id, body); err != nil {
		t.Fatal(err)
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
			id, err := store.ResolveDestination(t.Context(), "http://127.0.0.1:8765", scenario.database, scenario.dataset, scenario.local)
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
	if _, err := store.ResolveDestination(t.Context(), "https://remote.example", "database", "default", false); err == nil {
		t.Fatal("ambiguous remote database accepted")
	}
}
