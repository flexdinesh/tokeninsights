package rawcollectorstore

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func TestCaptureCheckpointAndEvidenceCommitTogether(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}
	state := Checkpoint{SourceKey: "key", SourceID: "source", Lineage: "lineage", Format: "pi-jsonl", Offset: 20, Ordinal: 1, Prefix: "verified-prefix", Context: json.RawMessage(`{"records":[]}`), UpdatedAtMs: 1}
	for _, commit := range []bool{false, true} {
		capture, err := store.BeginCapture(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if inserted, err := capture.Record(t.Context(), record, 1); err != nil || !inserted {
			t.Fatal(inserted, err)
		}
		if err := capture.SaveCheckpoint(t.Context(), state); err != nil {
			t.Fatal(err)
		}
		if commit {
			err = capture.Commit()
		} else {
			err = capture.Rollback()
		}
		if err != nil {
			t.Fatal(err)
		}
		read, err := store.BeginCapture(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		got, found, err := read.Checkpoint(t.Context(), "key", "pi-jsonl")
		_ = read.Rollback()
		if err != nil || found != commit {
			t.Fatal(got, found, err)
		}
		if commit && (got.Offset != 20 || got.Ordinal != 1 || got.Prefix != "verified-prefix") {
			t.Fatal(got)
		}
		var count int
		if err := store.DB.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&count); err != nil || (count == 1) != commit {
			t.Fatal(count, err)
		}
	}
}
