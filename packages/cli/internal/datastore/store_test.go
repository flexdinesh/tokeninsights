package datastore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/ingestionhttp"
)

func testStore(t testing.TB) *Store {
	t.Helper()
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "server.duckdb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
func piRecord(id string, input int64) evidence.Record {
	return evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: 2, Data: json.RawMessage(fmt.Sprintf(`{"type":"message","id":%q,"message":{"role":"assistant","timestamp":1700000000000,"provider":"openai","model":"gpt-5","usage":{"input":%d,"output":20}}}`, id, input)), Context: []evidence.Context{{Ordinal: 1, Data: json.RawMessage(`{"type":"session","id":"session"}`)}}}
}
func batchBody(t testing.TB, store *Store, stream, batch string, records ...evidence.Record) []byte {
	t.Helper()
	metadata, err := store.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	value := evidence.Batch{ProtocolVersion: evidence.ProtocolVersion, ExtractorVersion: 1, DatasetID: metadata.DatasetID, DatabaseID: metadata.DatabaseID, StreamID: stream, BatchID: batch, FromSequence: 1, ToSequence: int64(len(records))}
	for i, record := range records {
		value.Entries = append(value.Entries, evidence.Entry{Sequence: int64(i + 1), Record: record})
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
func drain(t *testing.T, store *Store) {
	t.Helper()
	for i := 0; i < 100; i++ {
		worked, err := store.ProcessNext(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !worked {
			return
		}
	}
	t.Fatal("worker failed to drain")
}
func total(t *testing.T, store *Store, table string) int64 {
	t.Helper()
	var total int64
	if err := store.SQL().QueryRowContext(t.Context(), "SELECT CAST(COALESCE(SUM(total_tokens),0) AS BIGINT) FROM "+table+" WHERE dataset_id=?", store.DatasetID()).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

func TestAcceptanceReplayAndConcurrentDuplicates(t *testing.T) {
	store := testStore(t)
	record := piRecord("message", 100)
	body := batchBody(t, store, "stream", "batch", record, record)
	response, err := store.Accept(t.Context(), evidence.ProtocolVersion, body)
	if err != nil {
		t.Fatal(err)
	}
	if response.Processing.Pending != 2 || response.Receipt.Accepted != 2 {
		t.Fatalf("missing item mappings: %+v", response)
	}
	if got := total(t, store, "analytics.confirmed"); got != 0 {
		t.Fatalf("acceptance performed processing: %d", got)
	}
	var group sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		duplicate := batchBody(t, store, fmt.Sprintf("copy-%d", i), "batch", record)
		group.Go(func() {
			_, err := store.Accept(context.Background(), evidence.ProtocolVersion, duplicate)
			if err != nil {
				failures <- err
			}
		})
	}
	group.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	drain(t, store)
	if got := total(t, store, "analytics.confirmed"); got != 120 {
		t.Fatalf("duplicates polluted totals: %d", got)
	}
	replay, err := store.Accept(t.Context(), evidence.ProtocolVersion, body)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Receipt != response.Receipt || replay.Processing.Pending != 0 || len(replay.Processing.Items) != 2 {
		t.Fatalf("receipt/status replay: %+v", replay)
	}
	request := httptest.NewRequest(http.MethodPost, ingestionhttp.IngestionPrefix+"batches", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	ingestionhttp.Handler(store, ingestionhttp.IngestionPrefix, evidence.ProtocolVersion).ServeHTTP(writer, request)
	if writer.Code != 200 {
		t.Fatalf("processed duplicate status: %d %s", writer.Code, writer.Body.String())
	}
	// A different Collector/batch also gets terminal status for identical evidence.
	lateBody := batchBody(t, store, "late-copy", "new-batch", record, record)
	lateRequest := httptest.NewRequest(http.MethodPost, ingestionhttp.IngestionPrefix+"batches", bytes.NewReader(lateBody))
	lateRequest.Header.Set("Content-Type", "application/json")
	lateWriter := httptest.NewRecorder()
	ingestionhttp.Handler(store, ingestionhttp.IngestionPrefix, evidence.ProtocolVersion).ServeHTTP(lateWriter, lateRequest)
	var late evidence.Response
	if lateWriter.Code != http.StatusOK || json.Unmarshal(lateWriter.Body.Bytes(), &late) != nil || late.Receipt.Accepted != 2 || late.Processing.Pending != 0 || len(late.Processing.Items) != 2 {
		t.Fatalf("already processed evidence lost status/mappings: %d %s", lateWriter.Code, lateWriter.Body.String())
	}
	changed := batchBody(t, store, "stream", "batch", piRecord("message", 200), record)
	_, err = store.Accept(t.Context(), evidence.ProtocolVersion, changed)
	var admission *AdmissionError
	if !errors.As(err, &admission) || admission.Code != "batch_conflict" {
		t.Fatalf("changed retry accepted: %v", err)
	}
	if got := total(t, store, "analytics.confirmed"); got != 120 {
		t.Fatal(got)
	}
}

func TestPendingAcceptanceSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.duckdb")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	body := batchBody(t, store, "stream", "batch", piRecord("message", 100))
	before, err := store.Accept(t.Context(), evidence.ProtocolVersion, body)
	if err != nil {
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
	after, err := store.Receipt(t.Context(), "stream", "batch")
	if err != nil || after.Receipt != before.Receipt || after.Processing.Pending != 1 {
		t.Fatalf("lost acceptance: %+v %v", after, err)
	}
	drain(t, store)
	if got := total(t, store, "analytics.confirmed"); got != 120 {
		t.Fatal(got)
	}
}

func TestNativeConflictWithdrawsConfirmedAndDeduplicatesEstimate(t *testing.T) {
	store := testStore(t)
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "first", "batch", piRecord("message", 100))); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "second", "batch", piRecord("message", 200))); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	if got := total(t, store, "analytics.confirmed"); got != 0 {
		t.Fatalf("conflict still confirmed: %d", got)
	}
	var count int
	if err := store.SQL().QueryRow("SELECT COUNT(*) FROM analytics.estimates").Scan(&count); err != nil || count != 1 {
		t.Fatalf("estimates %d %v", count, err)
	}
	response, err := store.Receipt(t.Context(), "first", "batch")
	if err != nil || response.Processing.Items[0].Disposition != "ambiguous" || response.Processing.Items[0].Code != "conflicting_native_identity" {
		t.Fatalf("missing ambiguity: %+v %v", response, err)
	}
}

func TestOverlappingBatchesBindSequenceAndKeepEveryMembership(t *testing.T) {
	store := testStore(t)
	first := piRecord("one", 100)
	if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "first", first, first)); err != nil {
		t.Fatal(err)
	}
	drain(t, store)
	var batch evidence.Batch
	if err := json.Unmarshal(batchBody(t, store, "stream", "overlap", first, piRecord("two", 200)), &batch); err != nil {
		t.Fatal(err)
	}
	batch.FromSequence, batch.ToSequence = 2, 3
	batch.Entries[0].Sequence, batch.Entries[1].Sequence = 2, 3
	body, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	response, err := store.Accept(t.Context(), evidence.ProtocolVersion, body)
	if err != nil || response.Receipt.Accepted != 2 || response.Processing.Pending != 2 {
		t.Fatal(response, err)
	}
	drain(t, store)
	if got := total(t, store, "analytics.confirmed"); got != 340 {
		t.Fatal("overlap inflated or skipped facts", got)
	}
	var mappings int
	if err := store.SQL().QueryRow("SELECT COUNT(*) FROM ingestion.batch_items").Scan(&mappings); err != nil || mappings != 4 {
		t.Fatal("overlap lost membership", mappings, err)
	}
	batch.BatchID = "changed-sequence"
	batch.Entries[0].Record = piRecord("different", 300)
	changed, err := json.Marshal(batch)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Accept(t.Context(), evidence.ProtocolVersion, changed)
	var failure *AdmissionError
	if !errors.As(err, &failure) || failure.Code != "sequence_conflict" {
		t.Fatal("sequence rebound", err)
	}
	if got := total(t, store, "analytics.confirmed"); got != 340 {
		t.Fatal("conflict mutated facts", got)
	}
	if err := store.SQL().QueryRow("SELECT COUNT(*) FROM ingestion.batch_items").Scan(&mappings); err != nil || mappings != 4 {
		t.Fatal("failed overlap partly committed", mappings, err)
	}
}
