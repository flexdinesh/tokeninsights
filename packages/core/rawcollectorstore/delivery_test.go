package rawcollectorstore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/core/collectordb"
	"github.com/flexdinesh/tokeninsights/packages/core/evidence"
)

func appendSessions(t *testing.T, store *Store, from, count int) {
	t.Helper()
	tx, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	for i := from; i < from+count; i++ {
		data := json.RawMessage(fmt.Sprintf(`{"type":"session","id":"session-%d"}`, i))
		inserted, err := Record(t.Context(), tx, evidence.Record{Harness: "pi", Format: "pi-jsonl", SourceID: "source", Lineage: "lineage", Ordinal: int64(i), Data: data}, 1)
		if err != nil || !inserted {
			t.Fatal(inserted, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

// The scan-free count must equal the definitional count at every state.
func assertPendingMatchesCount(t *testing.T, store *Store, id string, want int64) {
	t.Helper()
	var oracle int64
	if err := store.DB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM evidence_outbox WHERE sequence > (SELECT acknowledged_sequence FROM evidence_destinations WHERE destination_id=?)", id).Scan(&oracle); err != nil {
		t.Fatal(err)
	}
	got, err := store.Pending(t.Context(), id)
	if err != nil || got != oracle || got != want {
		t.Fatalf("pending=%d oracle=%d want=%d err=%v", got, oracle, want, err)
	}
}

func acknowledge(t *testing.T, store *Store, id string, saved *SavedBatch) {
	t.Helper()
	body, err := json.Marshal(receiptFor(saved))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AckPrepared(t.Context(), id, saved, body); err != nil {
		t.Fatal(err)
	}
}

func TestPendingMatchesOutboxCountAcrossDeliveryAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	store, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	assertPendingMatchesCount(t, store, "missing", 0)
	a, err := store.ResolveDestination(t.Context(), "http://local", "database", "default")
	if err != nil {
		t.Fatal(err)
	}
	assertPendingMatchesCount(t, store, a, 0)
	appendSessions(t, store, 0, 300)
	assertPendingMatchesCount(t, store, a, 300)
	assertPendingMatchesCount(t, store, "missing", 0)
	first, err := store.Prepare(t.Context(), a)
	if err != nil || first == nil {
		t.Fatal(first, err)
	}
	assertPendingMatchesCount(t, store, a, 300)
	acknowledge(t, store, a, first)
	assertPendingMatchesCount(t, store, a, 300-int64(len(first.Batch.Entries)))
	b, err := store.ResolveDestination(t.Context(), "http://local", "replacement", "default")
	if err != nil {
		t.Fatal(err)
	}
	assertPendingMatchesCount(t, store, b, 300)
	appendSessions(t, store, 300, 5)
	assertPendingMatchesCount(t, store, a, 305-int64(len(first.Batch.Entries)))
	assertPendingMatchesCount(t, store, b, 305)
	for _, id := range []string{a, b} {
		for {
			saved, err := store.Prepare(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			if saved == nil {
				break
			}
			acknowledge(t, store, id, saved)
		}
		assertPendingMatchesCount(t, store, id, 0)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := collectordb.ResetAllLocked(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	store, err = Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	a, err = store.ResolveDestination(t.Context(), "http://local", "database", "default")
	if err != nil {
		t.Fatal(err)
	}
	appendSessions(t, store, 0, 3)
	assertPendingMatchesCount(t, store, a, 3)
}

func TestDraftPreparationEqualsFreshPreparation(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	id, err := store.ResolveDestination(t.Context(), "http://local", "database", "default")
	if err != nil {
		t.Fatal(err)
	}
	appendSessions(t, store, 0, evidence.MaxEntries+10)
	first, err := store.Prepare(t.Context(), id)
	if err != nil || first == nil || first.Batch.ToSequence != evidence.MaxEntries {
		t.Fatal(first, err)
	}
	// A draft never bypasses an unacknowledged batch.
	early, err := store.Draft(t.Context(), id, first.Batch.ToSequence)
	if err != nil || early == nil {
		t.Fatal(early, err)
	}
	retry, err := store.PrepareDraft(t.Context(), id, early)
	if err != nil || !bytes.Equal(retry.Request, first.Request) {
		t.Fatal("draft replaced pending request", err)
	}
	acknowledge(t, store, id, first)

	// Rows appended after an incomplete draft read are never skipped.
	appendSessions(t, store, evidence.MaxEntries+10, 2)
	stale, err := store.PrepareDraft(t.Context(), id, early)
	if err != nil || stale == nil || bytes.Equal(stale.Request, early.request) {
		t.Fatal("stale draft reused", err)
	}
	if stale.Batch.FromSequence != evidence.MaxEntries+1 || stale.Batch.ToSequence != evidence.MaxEntries+12 || len(stale.Batch.Entries) != 12 {
		t.Fatal("fresh preparation missed appended evidence", stale.Batch.FromSequence, stale.Batch.ToSequence)
	}
	acknowledge(t, store, id, stale)

	// A current draft is persisted byte for byte and replays identically.
	appendSessions(t, store, evidence.MaxEntries+12, 3)
	current, err := store.Draft(t.Context(), id, stale.Batch.ToSequence)
	if err != nil || current == nil {
		t.Fatal(current, err)
	}
	wrongCursor, err := store.Draft(t.Context(), id, stale.Batch.ToSequence-1)
	if err != nil || wrongCursor == nil {
		t.Fatal(wrongCursor, err)
	}
	tx, err := store.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	reuse, err := wrongCursor.matches(t.Context(), tx, current.batch, stale.Batch.ToSequence)
	_ = tx.Rollback()
	if err != nil || reuse {
		t.Fatal("draft for another cursor matched", err)
	}
	used, err := store.PrepareDraft(t.Context(), id, current)
	if err != nil || used == nil || !bytes.Equal(used.Request, current.request) {
		t.Fatal("current draft not used", err)
	}
	replayed, err := store.Prepare(t.Context(), id)
	if err != nil || !bytes.Equal(replayed.Request, used.Request) || replayed.Batch.FromSequence != used.Batch.FromSequence || replayed.Batch.ToSequence != used.Batch.ToSequence {
		t.Fatal("draft batch replay changed", err)
	}
	acknowledge(t, store, id, used)
	assertPendingMatchesCount(t, store, id, 0)
}

func TestAckPreparedDecodesStoredRequestUnlessIdentical(t *testing.T) {
	store, err := Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	id, err := store.ResolveDestination(t.Context(), "http://local", "database", "default")
	if err != nil {
		t.Fatal(err)
	}
	appendSessions(t, store, 0, 4)
	saved, err := store.Prepare(t.Context(), id)
	if err != nil || saved == nil {
		t.Fatal(saved, err)
	}
	// A prepared value that differs from stored bytes is not trusted.
	forged := &SavedBatch{Batch: saved.Batch, Request: append(bytes.Clone(saved.Request), ' ')}
	forged.Batch.ToSequence = 3
	forged.Batch.Entries = forged.Batch.Entries[:3]
	body, err := json.Marshal(receiptFor(forged))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AckPrepared(t.Context(), id, forged, body); err == nil {
		t.Fatal("receipt for unstored request acknowledged")
	}
	assertPendingMatchesCount(t, store, id, 4)
	body, err = json.Marshal(receiptFor(saved))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AckPrepared(t.Context(), id, forged, body); err != nil {
		t.Fatal("stored request not decoded", err)
	}
	assertPendingMatchesCount(t, store, id, 0)
}
