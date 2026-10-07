package datastore

import (
	"encoding/json"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"path/filepath"
	"testing"
)

func TestReprocessKeepsPublishedGenerationUntilComplete(t *testing.T) {
	store := testStore(t)
	first := piRecord("one", 100)
	second := piRecord("two", 200)
	second.Context[0].Data = json.RawMessage(`{"type":"session","id":"other"}`)
	if _, err := store.Accept(t.Context(), batchBody(t, store, "stream", "batch", first, second)); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	before, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	generation, err := store.Reprocess(t.Context())
	if err != nil || generation != 2 {
		t.Fatalf("reprocess: %d %v", generation, err)
	}
	status, err := store.Receipt(t.Context(), "stream", "batch")
	if err != nil || status.Processing.Generation != 2 || status.Processing.Pending != 2 {
		t.Fatalf("status %+v %v", status, err)
	}
	if worked, err := store.ProcessNext(t.Context()); err != nil || !worked {
		t.Fatal(worked, err)
	}
	partial, err := store.Metadata(t.Context())
	if err != nil || partial.Generation != 1 || partial.Revision != before.Revision || total(t, store, "analytics.confirmed") != 340 {
		t.Fatalf("partial generation published: %+v %v", partial, err)
	}
	drain(t, store)
	after, err := store.Metadata(t.Context())
	if err != nil || after.Generation != 2 || after.TargetGeneration != 2 || after.Revision != before.Revision+1 {
		t.Fatalf("activation %+v %v", after, err)
	}
	if got := total(t, store, "analytics.confirmed"); got != 340 {
		t.Fatalf("generation duplicated totals %d", got)
	}
	var retained int
	if err := store.SQL().QueryRow("SELECT COUNT(*) FROM analytics.facts WHERE generation=1").Scan(&retained); err != nil || retained != 2 {
		t.Fatalf("old generation lost: %d %v", retained, err)
	}
	status, err = store.Receipt(t.Context(), "stream", "batch")
	if err != nil || status.Processing.Pending != 0 || status.Receipt.InputRevision != 1 {
		t.Fatalf("reprocess changed acceptance %+v %v", status, err)
	}
}

func TestReprocessRestartIncludesEvidenceAcceptedDuringBuild(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.duckdb")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	first := piRecord("one", 100)
	second := piRecord("two", 200)
	second.Context[0].Data = json.RawMessage(`{"type":"session","id":"other"}`)
	if _, err := store.Accept(t.Context(), batchBody(t, store, "first", "batch", first, second)); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	if _, err := store.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ProcessNext(t.Context()); err != nil {
		t.Fatal(err)
	}
	third := piRecord("three", 300)
	if _, err := store.Accept(t.Context(), batchBody(t, store, "late", "batch", third)); err != nil {
		t.Fatal(err)
	}
	if total(t, store, "analytics.confirmed") != 340 {
		t.Fatal("building generation leaked partial data")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	drain(t, store)
	metadata, err := store.Metadata(t.Context())
	if err != nil || metadata.Generation != 2 || total(t, store, "analytics.confirmed") != 660 {
		t.Fatal("late input missing after restart", metadata, err)
	}
}

func TestProcessorUpgradeReplacesInterruptedOlderGeneration(t *testing.T) {
	store := testStore(t)
	if _, err := store.Accept(t.Context(), batchBody(t, store, "stream", "batch", piRecord("message", 100))); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	if _, err := store.Reprocess(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SQL().Exec("UPDATE analytics.generations SET processor_version=? WHERE generation=2", evidence.ProcessorVersion-1); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), store.path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	m, err := reopened.Metadata(t.Context())
	if err != nil || m.Generation != 1 || m.TargetGeneration != 3 || total(t, reopened, "analytics.confirmed") != 120 {
		t.Fatal("upgrade reused interrupted older rules", m, err)
	}
	var processor int
	if err := reopened.SQL().QueryRow("SELECT processor_version FROM analytics.generations WHERE generation=?", m.TargetGeneration).Scan(&processor); err != nil || processor != evidence.ProcessorVersion {
		t.Fatal("generation mislabels processor", processor, err)
	}
	drain(t, reopened)
	m, err = reopened.Metadata(t.Context())
	if err != nil || m.Generation != 3 || total(t, reopened, "analytics.confirmed") != 120 {
		t.Fatal("upgrade lost or mixed published history", m, err)
	}
}
