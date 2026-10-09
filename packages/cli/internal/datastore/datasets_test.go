package datastore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"path/filepath"
	"testing"
)

func TestStoredReceiptCannotRedirectScopedProcessingRead(t *testing.T) {
	root, alice, bob := hostedStores(t)
	for _, store := range []*Store{alice, bob} {
		if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", piRecord("message", 100))); err != nil {
			t.Fatal(err)
		}
	}
	drain(t, root)
	var body string
	if err := root.SQL().QueryRow("SELECT receipt_json FROM ingestion.batches WHERE dataset_id='bob'").Scan(&body); err != nil {
		t.Fatal(err)
	}
	var receipt evidence.Receipt
	if err := json.Unmarshal([]byte(body), &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.DatasetID = "alice"
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := root.SQL().Exec("UPDATE ingestion.batches SET receipt_json=? WHERE dataset_id='bob'", string(encoded)); err != nil {
		t.Fatal(err)
	}
	if response, err := bob.Receipt(t.Context(), "stream", "batch"); err == nil || len(response.Processing.Items) != 0 {
		t.Fatal("corrupt receipt selected foreign processing", response, err)
	}
}

func hostedStores(t *testing.T) (*Store, *Store, *Store) {
	t.Helper()
	root, err := OpenKind(t.Context(), filepath.Join(t.TempDir(), "server.duckdb"), KindHosted)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	for _, dataset := range []string{"alice", "bob"} {
		if err := root.CreateDataset(t.Context(), dataset); err != nil {
			t.Fatal(err)
		}
	}
	return root, root.ForDataset("alice"), root.ForDataset("bob")
}

func TestDatasetsIsolateIdenticalNativeAndDeliveryIdentities(t *testing.T) {
	root, alice, bob := hostedStores(t)
	record := piRecord("message", 100)
	for _, store := range []*Store{alice, bob} {
		body := batchBody(t, store, "same-stream", "same-batch", record, record)
		before, err := store.Accept(t.Context(), evidence.ProtocolVersion, body)
		if err != nil {
			t.Fatal(err)
		}
		replay, err := store.Accept(t.Context(), evidence.ProtocolVersion, body)
		if err != nil || replay.Receipt != before.Receipt {
			t.Fatal(replay, err)
		}
	}
	// A separate collector within Alice's dataset must still deduplicate.
	if _, err := alice.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, alice, "other-stream", "batch", record)); err != nil {
		t.Fatal(err)
	}
	drain(t, root)
	for _, store := range []*Store{alice, bob} {
		if got := total(t, store, "analytics.confirmed"); got != 120 {
			t.Fatalf("dataset %s: total %d", store.DatasetID(), got)
		}
		response, err := store.Receipt(t.Context(), "same-stream", "same-batch")
		if err != nil || response.Receipt.DatasetID != store.DatasetID() || response.Processing.Pending != 0 || len(response.Processing.Items) != 2 {
			t.Fatal(response, err)
		}
		var count int
		if err := root.SQL().QueryRow("SELECT COUNT(*) FROM raw.evidence WHERE dataset_id=?", store.DatasetID()).Scan(&count); err != nil || count != 1 {
			t.Fatal(count, err)
		}
	}
	if _, err := bob.Receipt(t.Context(), "other-stream", "batch"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("foreign receipt exposed", err)
	}
	// A scoped handle rejects a different dataset binding before mutation.
	if _, err := bob.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, alice, "foreign", "batch", piRecord("other", 200))); err == nil {
		t.Fatal("foreign batch accepted")
	}
	var count int
	if err := root.SQL().QueryRow("SELECT COUNT(*) FROM ingestion.batches WHERE dataset_id='bob'").Scan(&count); err != nil || count != 1 {
		t.Fatal("foreign batch mutated Bob", count, err)
	}
}

func TestGenerationAndProjectionReplacementAreDatasetLocal(t *testing.T) {
	root, alice, bob := hostedStores(t)
	for _, store := range []*Store{alice, bob} {
		if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", piRecord("message", 100))); err != nil {
			t.Fatal(err)
		}
	}
	drain(t, root)
	bobBefore, err := bob.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	drain(t, root)
	bobAfter, err := bob.Metadata(t.Context())
	if err != nil || bobAfter != bobBefore {
		t.Fatal("Alice generation changed Bob", bobBefore, bobAfter, err)
	}
	aliceAfter, err := alice.Metadata(t.Context())
	if err != nil || aliceAfter.Generation != 2 {
		t.Fatal(aliceAfter, err)
	}
	// Conflicting evidence withdraws only Alice's countable projection.
	if _, err := alice.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, alice, "conflict", "batch", piRecord("message", 200))); err != nil {
		t.Fatal(err)
	}
	drain(t, root)
	if total(t, alice, "analytics.confirmed") != 0 || total(t, bob, "analytics.confirmed") != 120 {
		t.Fatal("projection replacement crossed dataset")
	}
	if total(t, bob, "analytics.estimated") != 0 {
		t.Fatal("Alice ambiguity exposed to Bob")
	}
}

func TestAncestorEvidenceNeverCrossesDatasets(t *testing.T) {
	root, alice, bob := hostedStores(t)
	if _, err := bob.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, bob, "child", "batch", codexRecord("child", "parent"))); err != nil {
		t.Fatal(err)
	}
	drain(t, root)
	before, err := bob.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := alice.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, alice, "parent", "batch", codexRecord("parent", ""))); err != nil {
		t.Fatal(err)
	}
	drain(t, root)
	after, err := bob.Metadata(t.Context())
	if err != nil || after != before {
		t.Fatal("foreign ancestor invalidated Bob", before, after, err)
	}
	if total(t, bob, "analytics.confirmed") != 0 || total(t, bob, "analytics.estimated") != 120 {
		t.Fatal("foreign parent attributed copied evidence")
	}
	if _, err := bob.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, bob, "parent", "batch", codexRecord("parent", ""))); err != nil {
		t.Fatal(err)
	}
	drain(t, root)
	if total(t, bob, "analytics.confirmed") != 120 || total(t, bob, "analytics.estimated") != 0 {
		t.Fatal("same-dataset parent did not resolve ambiguity")
	}
}

func TestWorkerRotatesDatasetsAndSkipsFailedScopes(t *testing.T) {
	root, alice, bob := hostedStores(t)
	for _, store := range []*Store{alice, bob} {
		if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", piRecord("message", 100))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := root.SQL().Exec("UPDATE raw.evidence SET record_json='invalid' WHERE dataset_id='alice'"); err != nil {
		t.Fatal(err)
	}
	if _, err := root.ProcessNext(t.Context()); err == nil {
		t.Fatal("corrupted Alice evidence ignored")
	}
	if worked, err := root.ProcessNext(t.Context()); err != nil || !worked {
		t.Fatal("Alice failure blocked Bob", worked, err)
	}
	if total(t, bob, "analytics.confirmed") != 120 {
		t.Fatal("Bob did not progress")
	}
}
