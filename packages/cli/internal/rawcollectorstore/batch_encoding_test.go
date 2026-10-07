package rawcollectorstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func encodingFixture(t testing.TB, large bool) (evidence.Batch, []evidence.Entry) {
	t.Helper()
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatabaseID: "database", DatasetID: "default", StreamID: "stream", BatchID: "batch", FromSequence: 98}
	var data json.RawMessage = []byte(`{"type":"session","id":"session"}`)
	if large {
		var err error
		data, err = json.Marshal(map[string]string{"type": "session", "id": strings.Repeat("<", evidence.MaxStringBytes), "timestamp": strings.Repeat("é", evidence.MaxStringBytes/2)})
		if err != nil {
			t.Fatal(err)
		}
	}
	entries := make([]evidence.Entry, evidence.MaxEntries+1)
	for i := range entries {
		record := evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: int64(i + 3), Data: data}
		if large {
			record.Context = []evidence.Context{{Ordinal: 0, Data: data}, {Ordinal: 1, Data: data}, {Ordinal: 2, Data: data}}
		}
		entries[i] = evidence.Entry{Sequence: int64(i) + batch.FromSequence, Record: record}
	}
	return batch, entries
}

func TestRawBatchEncodingBoundsAndEscaping(t *testing.T) {
	for _, large := range []bool{false, true} {
		t.Run(fmt.Sprint(large), func(t *testing.T) {
			batch, entries := encodingFixture(t, large)
			encodedBatch, body, err := encodeBatchPrefix(batch, entries)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := json.Marshal(encodedBatch)
			if err != nil || !bytes.Equal(expected, body) {
				t.Fatalf("wire encoding differs from standard encoding: %v", err)
			}
			if len(body) > evidence.MaxBodyBytes || encodedBatch.ToSequence != batch.FromSequence+int64(len(encodedBatch.Entries))-1 {
				t.Fatal("invalid body size or sequence range")
			}
			if !large && len(encodedBatch.Entries) != evidence.MaxEntries {
				t.Fatal("entry bound changed")
			}
			if large {
				if !bytes.Contains(body, []byte(`\u003c`)) || !bytes.Contains(body, []byte("é")) {
					t.Fatal("escaping or multibyte data changed")
				}
				next := encodedBatch
				next.Entries = append(next.Entries, entries[len(next.Entries)])
				next.ToSequence++
				tooLarge, err := json.Marshal(next)
				if err != nil || len(tooLarge) <= evidence.MaxBodyBytes {
					t.Fatalf("batch stopped before byte bound: %d %v", len(tooLarge), err)
				}
			}
		})
	}
}

func TestRawBatchRejectsGapInvalidAndOversizedRecords(t *testing.T) {
	for _, kind := range []string{"gap", "invalid", "oversized"} {
		t.Run(kind, func(t *testing.T) {
			batch, entries := encodingFixture(t, false)
			entries = entries[:2]
			switch kind {
			case "gap":
				entries[1].Sequence++
			case "invalid":
				entries[1].Record.Data = []byte(`{"type":"session","secret":"private"}`)
			case "oversized":
				entries[0].Record.Data = []byte(`{"type":"session","id":"` + strings.Repeat("x", evidence.MaxBodyBytes) + `"}`)
			}
			if _, _, err := encodeBatchPrefix(batch, entries); err == nil {
				t.Fatal("invalid record accepted")
			}
		})
	}
}

func TestRawByteBoundBatchRetryAndSuffix(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	_, entries := encodingFixture(t, true)
	tx, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, entry := range entries {
		if _, err := Record(t.Context(), tx, entry.Record, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := store.Bind(t.Context(), "destination", "http://remote", "database"); err != nil {
		t.Fatal(err)
	}
	first, err := store.Prepare(t.Context(), "destination")
	if err != nil || first == nil {
		t.Fatal(err)
	}
	if len(first.Batch.Entries) >= evidence.MaxEntries {
		t.Fatal("byte bound was not exercised")
	}
	retry, err := store.Prepare(t.Context(), "destination")
	if err != nil || retry == nil || !bytes.Equal(first.Request, retry.Request) || first.Batch.ProtocolVersion != retry.Batch.ProtocolVersion {
		t.Fatal("byte-bound retry changed saved request", err)
	}
	response := evidence.Response{Receipt: evidence.Receipt{DatabaseID: "database", DatasetID: "default", StreamID: first.Batch.StreamID, BatchID: first.Batch.BatchID, RequestHash: evidence.Hash(first.Request), FromSequence: first.Batch.FromSequence, ToSequence: first.Batch.ToSequence, Accepted: int64(len(first.Batch.Entries)), AcceptedAtMs: 1, InputRevision: 1}}
	ack, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Ack(t.Context(), "destination", ack); err != nil {
		t.Fatal(err)
	}
	next, err := store.Prepare(t.Context(), "destination")
	if err != nil || next == nil || next.Batch.FromSequence != first.Batch.ToSequence+1 || next.Batch.Entries[0].Record.Ordinal != entries[len(first.Batch.Entries)].Record.Ordinal {
		t.Fatal("byte bound skipped or repeated suffix", err)
	}
}

func BenchmarkRawBatchEncoding(b *testing.B) {
	batch, entries := encodingFixture(b, false)
	entries = entries[:evidence.MaxEntries]
	b.Run("growing_prefix", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			candidate := batch
			var body []byte
			for _, entry := range entries {
				candidate.Entries = append(candidate.Entries, entry)
				candidate.ToSequence = entry.Sequence
				var err error
				body, err = json.Marshal(candidate)
				if err != nil {
					b.Fatal(err)
				}
			}
			if _, err := evidence.DecodeBatch(body); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("encoded_entries", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := encodeBatchPrefix(batch, entries); err != nil {
				b.Fatal(err)
			}
		}
	})
}
