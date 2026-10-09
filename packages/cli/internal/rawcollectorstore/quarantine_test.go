package rawcollectorstore

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func TestQuarantineSurvivesReopenWithoutChangingCapturedEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	capture, err := store.BeginCapture(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := Checkpoint{SourceKey: evidence.Hash([]byte("source")), SourceID: "native", Lineage: "lineage", Format: "codex-jsonl", Offset: 123, Ordinal: 1, Prefix: evidence.Hash([]byte("prefix")), Context: json.RawMessage(`[]`), UpdatedAtMs: 1}
	if err := capture.SaveCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatal(err)
	}
	record := evidence.Record{Harness: "codex", Format: "codex-jsonl", SourceID: "native", Lineage: "lineage", Ordinal: 1, Data: json.RawMessage(`{"type":"session_meta","payload":{"id":"session"}}`)}
	if _, err := capture.Record(t.Context(), record, 1); err != nil {
		t.Fatal(err)
	}
	if err := capture.Commit(); err != nil {
		t.Fatal(err)
	}
	marker := Quarantine{SourceKey: evidence.Hash([]byte("source")), Format: "codex-jsonl", Signature: evidence.Hash([]byte("identity:size:mtime")), Code: "source_record_limit", ParserVersion: 1, Offset: 123, RecordedAtMs: 2}
	if err := store.SaveQuarantine(t.Context(), marker); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	got, found, err := store.GetQuarantine(t.Context(), marker.SourceKey)
	if err != nil || !found || got != marker {
		t.Fatal("quarantine did not survive restart", got, found, err)
	}
	state, found, err := store.Checkpoint(t.Context(), checkpoint.SourceKey, checkpoint.Format)
	if err != nil || !found || state.Offset != checkpoint.Offset || state.Ordinal != checkpoint.Ordinal || state.Prefix != checkpoint.Prefix || string(state.Context) != string(checkpoint.Context) {
		t.Fatal("quarantine altered continuity", state, found, err)
	}
	var count int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&count); err != nil || count != 1 {
		t.Fatal("quarantine discarded prior evidence", count, err)
	}
	marker.Signature = evidence.Hash([]byte("changed-file"))
	marker.ParserVersion++
	if err := store.SaveQuarantine(t.Context(), marker); err != nil {
		t.Fatal(err)
	}
	if got, found, err := store.GetQuarantine(t.Context(), marker.SourceKey); err != nil || !found || got != marker {
		t.Fatal("replacement failure was not saved", got, found, err)
	}
	for range 2 {
		if err := store.ClearQuarantine(t.Context(), marker.SourceKey); err != nil {
			t.Fatal(err)
		}
	}
	if _, found, err := store.GetQuarantine(t.Context(), marker.SourceKey); err != nil || found {
		t.Fatal("quarantine clear failed", found, err)
	}
}

func TestQuarantineRejectsUnboundedOrTransientFailureMetadata(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	valid := Quarantine{SourceKey: evidence.Hash([]byte("source")), Format: "codex-jsonl", Signature: evidence.Hash([]byte("metadata")), Code: "source_record_limit", ParserVersion: 1}
	for name, change := range map[string]func(*Quarantine){
		"empty source":      func(q *Quarantine) { q.SourceKey = "" },
		"long source":       func(q *Quarantine) { q.SourceKey = strings.Repeat("s", 257) },
		"private format":    func(q *Quarantine) { q.Format = "/home/private-session" },
		"private signature": func(q *Quarantine) { q.Signature = "/home/private-session" },
		"nonhex signature":  func(q *Quarantine) { q.Signature = strings.Repeat("z", 64) },
		"transient code":    func(q *Quarantine) { q.Code = "transport_failed" },
		"parser":            func(q *Quarantine) { q.ParserVersion = 0 },
		"offset":            func(q *Quarantine) { q.Offset = -1 },
		"time":              func(q *Quarantine) { q.RecordedAtMs = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			marker := valid
			change(&marker)
			if err := store.SaveQuarantine(t.Context(), marker); err == nil {
				t.Fatal("invalid quarantine accepted")
			}
		})
	}
	var count int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM evidence_quarantine").Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid markers persisted", count, err)
	}
}
