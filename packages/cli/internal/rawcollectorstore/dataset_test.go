package rawcollectorstore

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
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
	return evidence.Response{Receipt: evidence.Receipt{DatabaseID: b.DatabaseID, DatasetID: b.EffectiveDatasetID(), StreamID: b.StreamID, BatchID: b.BatchID, RequestHash: evidence.Hash(saved.Request), FromSequence: b.FromSequence, ToSequence: b.ToSequence, Accepted: int64(len(b.Entries)), AcceptedAtMs: 1, InputRevision: 1}}
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

func TestSchema17MigrationPreservesSavedV2RequestAndCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	database, _, err := db.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate exactly the old outbox definitions, retaining verified canonical state.
	for _, table := range []string{"evidence_batches", "evidence_destinations", "evidence_outbox", "evidence_sources", "evidence_state"} {
		if _, err := database.Exec("DROP TABLE " + table); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := os.ReadFile("../db/schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, old, _ := strings.Cut(string(schema), "-- Sanitized raw outbox.")
	old = strings.ReplaceAll(old, "  dataset_id TEXT NOT NULL DEFAULT 'default' CHECK (length(dataset_id) BETWEEN 1 AND 256),\n", "")
	old = strings.ReplaceAll(old, "  protocol_version INTEGER NOT NULL DEFAULT 3 CHECK (protocol_version IN (2, 3)),\n", "")
	old = strings.ReplaceAll(old, "database_id,dataset_id,protocol_version,first_sequence", "database_id,first_sequence")
	old = strings.ReplaceAll(old, "user_version = 18", "user_version = 17")
	if _, err := database.Exec("-- Sanitized raw outbox." + old); err != nil {
		t.Fatal(err)
	}
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"same-session"}`)}
	batch := evidence.Batch{ProtocolVersion: 2, ExtractorVersion: 1, DatabaseID: "database", StreamID: "unchanged-stream", BatchID: "unchanged-batch", FromSequence: 8, ToSequence: 8, Entries: []evidence.Entry{{Sequence: 8, Record: record}}}
	request, err := json.MarshalIndent(batch, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO evidence_state VALUES(1,'unchanged-stream',1,1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO evidence_destinations(destination_id,endpoint,database_id,acknowledged_sequence) VALUES('old-binding','https://shared','database',7)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("INSERT INTO evidence_batches(batch_id,destination_id,stream_id,database_id,first_sequence,last_sequence,request_hash,request_bytes) VALUES(?,?,?,?,?,?,?,?)", batch.BatchID, "old-binding", batch.StreamID, batch.DatabaseID, 8, 8, evidence.Hash(request), request); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	id, err := store.ResolveDestination(t.Context(), "https://shared", "database", "default", false)
	if err != nil || id != "old-binding" {
		t.Fatal("old binding lost", id, err)
	}
	retry, err := store.Prepare(t.Context(), id)
	if err != nil || retry == nil || !bytes.Equal(retry.Request, request) || retry.Batch.ProtocolVersion != 2 {
		t.Fatal("old bytes/version changed", retry, err)
	}
	body, _ := json.Marshal(receiptFor(retry))
	if err := store.Ack(t.Context(), id, body); err != nil {
		t.Fatal(err)
	}
	var cursor, protocol int
	var dataset string
	if err := store.DB.QueryRow("SELECT acknowledged_sequence,dataset_id FROM evidence_destinations WHERE destination_id=?", id).Scan(&cursor, &dataset); err != nil || cursor != 8 || dataset != "default" {
		t.Fatal(cursor, dataset, err)
	}
	if err := store.DB.QueryRow("SELECT protocol_version FROM evidence_batches WHERE batch_id=?", batch.BatchID).Scan(&protocol); err != nil || protocol != 2 {
		t.Fatal(protocol, err)
	}
}
