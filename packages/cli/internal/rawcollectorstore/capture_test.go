package rawcollectorstore

import (
	"encoding/json"
	"path/filepath"
	"reflect"
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

func TestCaptureRevisionsAndDuplicatesKeepContiguousDelivery(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	original := captureBenchmarkRecord(0)
	revised := original
	revised.Data = json.RawMessage(`{"type":"message","id":"message-0","message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"model","usage":{"input":200,"output":20}}}`)
	other := captureBenchmarkRecord(1)
	invalid := original
	invalid.Data = json.RawMessage(`{"type":"message","id":"message-0","message":{"content":"private"}}`)

	for index, records := range [][]evidence.Record{{original, original, invalid, revised}, {original, revised, other}} {
		capture, err := store.BeginCapture(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for recordIndex, record := range records {
			inserted, err := capture.Record(t.Context(), record, int64(index+1))
			if index == 0 && recordIndex == 2 {
				if err == nil || inserted {
					t.Fatal("private observation accepted", inserted, err)
				}
				continue
			}
			wantInserted := index == 0 && (recordIndex == 0 || recordIndex == 3) || index == 1 && recordIndex == 2
			if err != nil || inserted != wantInserted {
				t.Fatalf("capture %d record %d: inserted=%v want=%v error=%v", index, recordIndex, inserted, wantInserted, err)
			}
		}
		if err := capture.Commit(); err != nil {
			t.Fatal(err)
		}
		if _, err := capture.Record(t.Context(), other, 3); err == nil {
			t.Fatal("completed capture accepted another record")
		}
	}
	want := []evidence.Record{original, revised, other}
	rows, err := store.DB.QueryContext(t.Context(), "SELECT sequence,observation_key,record_json,created_at_ms FROM evidence_outbox ORDER BY sequence")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	index := 0
	for rows.Next() {
		var sequence, created int64
		var key, body string
		if err := rows.Scan(&sequence, &key, &body, &created); err != nil {
			t.Fatal(err)
		}
		if index >= len(want) || sequence != int64(index+1) {
			t.Fatal("duplicate/rejected observation changed delivery sequence", sequence, index)
		}
		var record evidence.Record
		if err := json.Unmarshal([]byte(body), &record); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(record, want[index]) || key != evidence.ObservationKey(want[index]) || key != evidence.Hash([]byte(body)) {
			t.Fatal("stored observation identity or snapshot changed", record, key)
		}
		if index < 2 && created != 1 || index == 2 && created != 2 {
			t.Fatal("replay changed observation capture time", created)
		}
		index++
	}
	if err := rows.Err(); err != nil || index != len(want) {
		t.Fatal("missing observations", index, err)
	}
}
