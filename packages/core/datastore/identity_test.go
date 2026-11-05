package datastore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
)

func TestPersonalReceiptCannotRedirectProcessingRead(t *testing.T) {
	store := testStore(t)
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", piRecord("message", 100))); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	var body string
	if err := store.SQL().QueryRow("SELECT receipt_json FROM ingestion_batches").Scan(&body); err != nil {
		t.Fatal(err)
	}
	var receipt evidence.Receipt
	if err := json.Unmarshal([]byte(body), &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.DatasetID = "foreign"
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().Exec("UPDATE ingestion_batches SET receipt_json=?", string(encoded)); err != nil {
		t.Fatal(err)
	}
	if response, err := store.Receipt(t.Context(), "stream", "batch"); err == nil || len(response.Processing.Items) != 0 {
		t.Fatal("corrupt receipt selected processing", response, err)
	}
}

func TestHostedCreationRejectsBeforeFilesystemMutation(t *testing.T) {
	root := t.TempDir()
	if store, err := OpenKind(t.Context(), filepath.Join(root, "storage", "server.sqlite"), "hosted"); err == nil {
		_ = store.Close()
		t.Fatal("hosted storage created")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsupported kind mutated filesystem", entries, err)
	}
}
