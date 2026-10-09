package datastore

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processor"
)

func TestLargeBatchPreservesDuplicatesAndProjection(t *testing.T) {
	store := testStore(t)
	const uniqueRecords = 190
	records := make([]evidence.Record, evidence.MaxEntries)
	for i := range records {
		records[i] = piRecord(fmt.Sprintf("message-%d", i%uniqueRecords), 100)
	}
	body := batchBody(t, store, "stream", "large", records...)
	accepted, err := store.Accept(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Receipt.Accepted != evidence.MaxEntries || accepted.Processing.Pending != evidence.MaxEntries {
		t.Fatalf("lost duplicate mappings: %+v", accepted)
	}
	var raw, mappings, scopes int64
	if err := store.SQL().QueryRowContext(t.Context(), "SELECT (SELECT COUNT(*) FROM raw.evidence),(SELECT COUNT(*) FROM ingestion.batch_items),(SELECT COUNT(*) FROM processing.scopes)").Scan(&raw, &mappings, &scopes); err != nil {
		t.Fatal(err)
	}
	if raw != uniqueRecords || mappings != evidence.MaxEntries || scopes != 1 {
		t.Fatalf("unexpected storage counts: %d %d %d", raw, mappings, scopes)
	}
	drain(t, store)
	if got := total(t, store, "analytics.confirmed"); got != uniqueRecords*120 {
		t.Fatalf("wrong usage total: %d", got)
	}
	var facts, provenance, outcomes int64
	if err := store.SQL().QueryRowContext(t.Context(), "SELECT (SELECT COUNT(*) FROM analytics.facts),(SELECT COUNT(*) FROM analytics.provenance),(SELECT COUNT(*) FROM processing.outcomes)").Scan(&facts, &provenance, &outcomes); err != nil {
		t.Fatal(err)
	}
	if facts != uniqueRecords || provenance != uniqueRecords || outcomes != uniqueRecords {
		t.Fatalf("lost projection rows: %d %d %d", facts, provenance, outcomes)
	}
	replay, err := store.Accept(t.Context(), body)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Receipt != accepted.Receipt || replay.Processing.Pending != 0 || len(replay.Processing.Items) != evidence.MaxEntries {
		t.Fatalf("receipt changed or mappings missing: %+v", replay)
	}
	// Overlapping sequences remain immutable, while new sequences in the same batch succeed.
	var next evidence.Batch
	if err := json.Unmarshal(body, &next); err != nil {
		t.Fatal(err)
	}
	next.BatchID = "overlap"
	next.FromSequence = 129
	next.ToSequence = 384
	for i := range next.Entries {
		sequence := next.FromSequence + int64(i)
		next.Entries[i].Sequence = sequence
		if sequence <= evidence.MaxEntries {
			next.Entries[i].Record = records[sequence-1]
		} else {
			next.Entries[i].Record = records[i]
		}
	}
	overlap, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	response, err := store.Accept(t.Context(), overlap)
	if err != nil {
		t.Fatal(err)
	}
	if response.Receipt.InputRevision != accepted.Receipt.InputRevision || response.Processing.Pending != 0 || len(response.Processing.Items) != evidence.MaxEntries {
		t.Fatalf("duplicate overlap changed revision: %+v", response)
	}
}

func TestLargeBatchSequenceConflictRollsBack(t *testing.T) {
	store := testStore(t)
	initial := piRecord("original", 100)
	body := batchBody(t, store, "stream", "initial", initial)
	var batch evidence.Batch
	if err := json.Unmarshal(body, &batch); err != nil {
		t.Fatal(err)
	}
	batch.FromSequence = 200
	batch.ToSequence = 200
	batch.Entries[0].Sequence = 200
	encoded, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Accept(t.Context(), encoded)
	if err != nil {
		t.Fatal(err)
	}
	records := make([]evidence.Record, evidence.MaxEntries)
	for i := range records {
		records[i] = piRecord(fmt.Sprintf("new-%d", i), 100)
	}
	_, err = store.Accept(t.Context(), batchBody(t, store, "stream", "conflict", records...))
	var admission *AdmissionError
	if !errors.As(err, &admission) || admission.Code != "sequence_conflict" {
		t.Fatalf("expected immutable sequence conflict: %v", err)
	}
	var raw, mappings, batches int64
	if err := store.SQL().QueryRowContext(t.Context(), "SELECT (SELECT COUNT(*) FROM raw.evidence),(SELECT COUNT(*) FROM ingestion.batch_items),(SELECT COUNT(*) FROM ingestion.batches)").Scan(&raw, &mappings, &batches); err != nil {
		t.Fatal(err)
	}
	if raw != 1 || mappings != 1 || batches != 1 {
		t.Fatalf("conflicting batch leaked rows: %d %d %d", raw, mappings, batches)
	}
	metadata, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.InputRevision != first.Receipt.InputRevision {
		t.Fatalf("conflict advanced revision: %+v", metadata)
	}
}

func benchmarkBatch(b *testing.B, store *Store) []byte {
	b.Helper()
	metadata, err := store.Metadata(b.Context())
	if err != nil {
		b.Fatal(err)
	}
	batch := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: evidence.ExtractorVersion, DatasetID: metadata.DatasetID, DatabaseID: metadata.DatabaseID, StreamID: "benchmark-stream", BatchID: "benchmark-batch", FromSequence: 1, ToSequence: evidence.MaxEntries}
	for i := range evidence.MaxEntries {
		batch.Entries = append(batch.Entries, evidence.Entry{Sequence: int64(i + 1), Record: piRecord(fmt.Sprintf("message-%d", i), 100)})
	}
	body, err := json.Marshal(batch)
	if err != nil {
		b.Fatal(err)
	}
	return body
}

func BenchmarkAcceptance256(b *testing.B) {
	b.ReportAllocs()
	b.StopTimer()
	for range b.N {
		store, err := Open(b.Context(), filepath.Join(b.TempDir(), "server.duckdb"))
		if err != nil {
			b.Fatal(err)
		}
		body := benchmarkBatch(b, store)
		b.StartTimer()
		response, err := store.Accept(b.Context(), body)
		b.StopTimer()
		if err != nil || response.Receipt.Accepted != evidence.MaxEntries {
			_ = store.Close()
			b.Fatalf("acceptance: %+v %v", response, err)
		}
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPublication256(b *testing.B) {
	b.ReportAllocs()
	b.StopTimer()
	for range b.N {
		store, err := Open(b.Context(), filepath.Join(b.TempDir(), "server.duckdb"))
		if err != nil {
			b.Fatal(err)
		}
		if _, err := store.Accept(b.Context(), benchmarkBatch(b, store)); err != nil {
			_ = store.Close()
			b.Fatal(err)
		}
		work, found, err := store.LoadWork(b.Context())
		if err != nil || !found {
			_ = store.Close()
			b.Fatal("load work", err)
		}
		projection, err := processor.Process(b.Context(), work.Records)
		if err != nil {
			_ = store.Close()
			b.Fatal(err)
		}
		b.StartTimer()
		published, err := store.PublishProjection(b.Context(), work, projection)
		b.StopTimer()
		if err != nil || !published {
			_ = store.Close()
			b.Fatal("publication", err)
		}
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
