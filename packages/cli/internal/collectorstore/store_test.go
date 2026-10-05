package collectorstore

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
	_ "modernc.org/sqlite"
)

func newStore(t *testing.T) (Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	database, _, err := db.CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return Store{DB: database}, path
}
func testFact(message string) publication.Fact {
	f := publication.Fact{Harness: "pi", Session: publication.Session{Harness: "pi", NativeID: "fixture-session", FirstOccurredAtMs: 10, LastOccurredAtMs: 10}, Message: &publication.Message{NativeID: message, OccurredAtMs: 10}, OccurredAtMs: 10, Provider: "fixture-provider", ProviderSource: "explicit", Model: "fixture-model", UsageScope: "message", Quality: "exact", Countable: true, InputTokens: 10, OutputTokens: 2, TotalTokens: 12}
	publication.SetIDs(&f)
	return f
}
func recordFact(t *testing.T, s Store, f publication.Fact) bool {
	t.Helper()
	ctx := context.Background()
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := Record(ctx, tx, f, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return changed
}
func count(t *testing.T, database *sql.DB, table string) int {
	t.Helper()
	var value int
	if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}
func receiptFor(saved *SavedBatch) []byte {
	body, _ := json.Marshal(publication.Receipt{DatabaseID: saved.Batch.DatabaseID, StreamID: saved.Batch.StreamID, BatchID: saved.Batch.BatchID, FromSequence: saved.Batch.FromSequence, ToSequence: saved.Batch.ToSequence, RequestHash: saved.RequestHash, Inserted: int64(len(saved.Batch.Entries)), CommittedAtMs: 200, Revision: 1})
	return body
}

func TestRecordAtomicRollbackAndNoop(t *testing.T) {
	s, path := newStore(t)
	ctx := context.Background()
	f := testFact("first")
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Record(ctx, tx, f, 100); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
	if count(t, s.DB, "publication_journal") != 0 {
		t.Fatal("rolled back journal visible")
	}
	if !recordFact(t, s, f) || recordFact(t, s, f) {
		t.Fatal("changed/noop mismatch")
	}
	if count(t, s.DB, "publication_journal") != 1 {
		t.Fatal("noop allocated sequence")
	}
	_ = s.DB.Close()
	database, err := db.OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if count(t, database, "publication_entities") != 1 {
		t.Fatal("reopen lost latest entity")
	}
}

func TestRecordEntityFailureRollsBackJournal(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	if _, err := s.DB.Exec("CREATE TRIGGER fail_entity BEFORE INSERT ON publication_entities BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Record(ctx, tx, testFact("first"), 100); err == nil {
		t.Fatal("expected entity failure")
	}
	_ = tx.Rollback()
	if count(t, s.DB, "publication_journal") != 0 {
		t.Fatal("failed entity retained journal prefix")
	}
}

func TestImmutableBatchAndIndependentDestinations(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	recordFact(t, s, testFact("first"))
	for _, id := range []string{"local", "other"} {
		if err := s.BindDestination(ctx, id, "http://127.0.0.1:1234", "server-one"); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.PrepareBatch(ctx, "local", "first-host", 100)
	if err != nil {
		t.Fatal(err)
	}
	recordFact(t, s, testFact("second"))
	retry, err := s.PrepareBatch(ctx, "local", "changed-host", 101)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Request, retry.Request) {
		t.Fatal("retry changed saved request")
	}
	if _, err := s.DB.Exec("UPDATE publication_batches SET request_bytes=? WHERE batch_id=?", []byte("{}"), first.Batch.BatchID); err == nil {
		t.Fatal("immutable request update accepted")
	}
	bad := receiptFor(first)
	var receipt publication.Receipt
	if err := json.Unmarshal(bad, &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.DatabaseID = "wrong-server"
	bad, _ = json.Marshal(receipt)
	if err := s.Ack(ctx, "local", bad, 200); err == nil {
		t.Fatal("bad receipt accepted")
	}
	if pending, err := s.Pending(ctx, "local"); err != nil || pending != 2 {
		t.Fatalf("bad receipt changed cursor %d %v", pending, err)
	}
	if err := s.Ack(ctx, "local", receiptFor(first), 200); err != nil {
		t.Fatal(err)
	}
	if pending, err := s.Pending(ctx, "other"); err != nil || pending != 2 {
		t.Fatalf("other cursor changed %d %v", pending, err)
	}
	next, err := s.PrepareBatch(ctx, "local", "host", 201)
	if err != nil {
		t.Fatal(err)
	}
	if next.Batch.FromSequence != 2 || len(next.Batch.Entries) != 1 {
		t.Fatal("ack skipped next snapshot")
	}
	if err := s.BindDestination(ctx, "local", "http://127.0.0.1:1234", "replacement"); err == nil {
		t.Fatal("silent server replacement accepted")
	}
}

func TestAcknowledgementFailureRollsBackReceiptAndCursor(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	recordFact(t, s, testFact("first"))
	if err := s.BindDestination(ctx, "local", "http://127.0.0.1:1234", "server"); err != nil {
		t.Fatal(err)
	}
	saved, err := s.PrepareBatch(ctx, "local", "host", 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec("CREATE TRIGGER fail_cursor BEFORE UPDATE ON publication_destinations BEGIN SELECT RAISE(ABORT,'injected'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.Ack(ctx, "local", receiptFor(saved), 200); err == nil {
		t.Fatal("expected cursor failure")
	}
	var acknowledged int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM publication_batches WHERE receipt_bytes IS NOT NULL").Scan(&acknowledged); err != nil || acknowledged != 0 {
		t.Fatalf("failed ack receipt visible %d %v", acknowledged, err)
	}
	retry, err := s.PrepareBatch(ctx, "local", "host", 201)
	if err != nil || !bytes.Equal(saved.Request, retry.Request) {
		t.Fatal("ack failure lost pending batch")
	}
}

func TestCollectorResetBoundaries(t *testing.T) {
	s, path := newStore(t)
	ctx := context.Background()
	recordFact(t, s, testFact("first"))
	var initial string
	if err := s.DB.QueryRow("SELECT stream_id FROM publication_state").Scan(&initial); err != nil {
		t.Fatal(err)
	}
	if err := db.ResetCanonical(ctx, s.DB); err != nil {
		t.Fatal(err)
	}
	if count(t, s.DB, "publication_journal") != 1 {
		t.Fatal("canonical reset lost journal")
	}
	if err := db.ResetAll(path); err != nil {
		t.Fatal(err)
	}
	var fresh string
	if err := s.DB.QueryRow("SELECT stream_id FROM publication_state").Scan(&fresh); err != nil {
		t.Fatal(err)
	}
	if initial == fresh || count(t, s.DB, "publication_journal") != 0 {
		t.Fatal("full collector reset did not create fresh stream")
	}
}

func TestBatchEntryBoundAndContiguity(t *testing.T) {
	s, _ := newStore(t)
	ctx := context.Background()
	for i := 0; i < publication.MaxEntries+1; i++ {
		recordFact(t, s, testFact(fmt.Sprintf("fixture-message-%d", i)))
	}
	if err := s.BindDestination(ctx, "local", "http://127.0.0.1:1234", "server"); err != nil {
		t.Fatal(err)
	}
	saved, err := s.PrepareBatch(ctx, "local", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Batch.Entries) != publication.MaxEntries || saved.Batch.FromSequence != 1 || saved.Batch.ToSequence != publication.MaxEntries {
		t.Fatal("batch did not stop at exact entry bound")
	}
	if err := s.Ack(ctx, "local", receiptFor(saved), 200); err != nil {
		t.Fatal(err)
	}
	next, err := s.PrepareBatch(ctx, "local", "", 201)
	if err != nil || len(next.Batch.Entries) != 1 || next.Batch.FromSequence != publication.MaxEntries+1 {
		t.Fatalf("batch suffix incorrect %v", err)
	}
}

func TestReferenceEnvelopeChangeJournalsWithoutChangingFactIdentity(t *testing.T) {
	s, _ := newStore(t)
	f := testFact("fixture-message")
	if !recordFact(t, s, f) {
		t.Fatal("initial fact not journaled")
	}
	initialHash := publication.PayloadHash(f)
	f.Session.FirstOccurredAtMs = 5
	f.Session.LastOccurredAtMs = 15
	f.Message.OccurredAtMs = 5
	if !recordFact(t, s, f) {
		t.Fatal("changed source envelope lost")
	}
	if recordFact(t, s, f) {
		t.Fatal("unchanged envelope grew journal")
	}
	if publication.PayloadHash(f) != initialHash || count(t, s.DB, "publication_entities") != 1 || count(t, s.DB, "publication_journal") != 2 {
		t.Fatal("reference change altered contribution identity")
	}
}
