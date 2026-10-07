package datastore

import (
	"bytes"
	"encoding/json"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestion"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverstore"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyPartialCoveragePreservesHistoryReceiptsAndSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "server.sqlite")
	legacy, err := serverstore.CreateIfMissing(source)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := legacy.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	records := []evidence.Record{piRecord("one", 80), piRecord("two", 180), piRecord("three", 180)}
	raw := []evidence.Stored{}
	for _, record := range records {
		raw = append(raw, evidence.Stored{ID: evidence.EvidenceID(record), Scope: evidence.Scope(record), Record: record})
	}
	projection, err := pipeline.ProcessEvidence(t.Context(), raw)
	if err != nil || len(projection.Contributions) != 3 {
		t.Fatal(projection, err)
	}
	batch := publication.Batch{ProtocolVersion: 1, IdentityVersion: 1, SemanticsVersion: 1, DatabaseID: metadata.DatabaseID, StreamID: "legacy-stream", BatchID: "legacy-batch", FromSequence: 1, ToSequence: 3, Hostname: "fixture"}
	for i, contribution := range projection.Contributions {
		batch.Entries = append(batch.Entries, publication.Entry{Sequence: int64(i + 1), Fact: contribution.Fact})
	}
	body, err := publication.EncodeBatch(batch)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ingestion.NewCore(legacy).Ingest(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), filepath.Join(root, "server.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	imported, err := store.Metadata(t.Context())
	if err != nil || imported.DatabaseID != metadata.DatabaseID {
		t.Fatal(imported, err)
	}
	if replay, err := store.AcceptLegacy(t.Context(), body); err != nil || replay != receipt {
		t.Fatal("receipt lost", replay, err)
	}
	if got := total(t, store, "analytics.confirmed"); got != 500 {
		t.Fatal(got)
	}
	if _, err := store.Accept(t.Context(), batchBody(t, store, "raw-stream", "raw-batch", records[:2]...)); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	if got := total(t, store, "analytics.confirmed"); got != 500 {
		t.Fatal("partial rebuild erased or doubled history", got)
	}
	var covered int
	if err := store.SQL().QueryRow("SELECT COUNT(*) FROM analytics.legacy_coverage").Scan(&covered); err != nil || covered != 2 {
		t.Fatal(covered, err)
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("legacy source mutated", err)
	}
	// Same ID with conflicting counters cannot replace the proven baseline.
	conflicting := records[0]
	conflicting.Data = json.RawMessage(`{"type":"message","id":"one","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":90,"output":20}}}`)
	if _, err := store.Accept(t.Context(), batchBody(t, store, "conflict", "batch", conflicting)); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	if got := total(t, store, "analytics.confirmed"); got != 500 {
		t.Fatal("ambiguity erased legacy baseline", got)
	}
}

func TestSQLiteTargetRequiresImportWithoutMutation(t *testing.T) {
	source := filepath.Join(t.TempDir(), "custom.sqlite")
	legacy, err := serverstore.CreateIfMissing(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	store, err := Open(t.Context(), source)
	if store != nil {
		_ = store.Close()
		t.Fatal("SQLite opened as data store")
	}
	if err == nil || !strings.Contains(err.Error(), "requires import") {
		t.Fatal("missing import guidance", err)
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("SQLite target mutated", err)
	}
	target := filepath.Join(filepath.Dir(source), "custom.duckdb")
	imported, err := OpenWithLegacy(t.Context(), target, source)
	if err != nil {
		t.Fatal(err)
	}
	if err := imported.Close(); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(source)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("explicit import mutated source", err)
	}
}
