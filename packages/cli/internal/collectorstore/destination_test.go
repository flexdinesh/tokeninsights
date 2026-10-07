package collectorstore

import (
	"bytes"
	"testing"
)

func TestLegacyPrivateAliasPendingPrecedesCanonicalCursor(t *testing.T) {
	store, _ := newStore(t)
	const canonical = "http://127.0.0.1:8765"
	if err := store.BindDestination(t.Context(), "canonical", canonical, "database"); err != nil {
		t.Fatal(err)
	}
	if err := store.BindDestination(t.Context(), "private", "http://local", "database"); err != nil {
		t.Fatal(err)
	}
	recordFact(t, store, testFact("first"))
	original, err := store.PrepareBatch(t.Context(), "private", "original-host", 1)
	if err != nil || original == nil {
		t.Fatal(err)
	}
	recordFact(t, store, testFact("second"))
	complete, err := store.PrepareBatch(t.Context(), "canonical", "original-host", 1)
	if err != nil || complete == nil {
		t.Fatal(err)
	}
	if err := store.Ack(t.Context(), "canonical", receiptFor(complete), 2); err != nil {
		t.Fatal(err)
	}
	id, err := store.ResolveDeliveryDestination(t.Context(), canonical, "database", true)
	if err != nil || id != "private" {
		t.Fatal("retained legacy alias abandoned", id, err)
	}
	retry, err := store.PrepareBatch(t.Context(), id, "changed-host", 3)
	if err != nil || retry == nil || !bytes.Equal(retry.Request, original.Request) {
		t.Fatal("retained legacy bytes changed", retry, err)
	}
	if err := store.Ack(t.Context(), id, receiptFor(retry), 4); err != nil {
		t.Fatal(err)
	}
	id, err = store.ResolveDeliveryDestination(t.Context(), canonical, "database", true)
	if err != nil || id != "canonical" {
		t.Fatal("highest legacy cursor ignored", id, err)
	}
	if next, err := store.PrepareBatch(t.Context(), id, "host", 5); err != nil || next != nil {
		t.Fatal("legacy history replayed", next, err)
	}
	var cursor int64
	if err := store.DB.QueryRow("SELECT acknowledged_sequence FROM publication_destinations WHERE destination_id='private'").Scan(&cursor); err != nil || cursor != 1 {
		t.Fatal("legacy cursor copied", cursor, err)
	}
}

func TestLegacyAliasCannotCrossRemoteOrReplacementBinding(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(map[bool]string{false: "remote", true: "replacement"}[local], func(t *testing.T) {
			store, _ := newStore(t)
			if err := store.BindDestination(t.Context(), "private", "http://local", "database"); err != nil {
				t.Fatal(err)
			}
			recordFact(t, store, testFact("first"))
			saved, err := store.PrepareBatch(t.Context(), "private", "host", 1)
			if err != nil || saved == nil {
				t.Fatal(err)
			}
			if err := store.Ack(t.Context(), "private", receiptFor(saved), 2); err != nil {
				t.Fatal(err)
			}
			databaseID := "database"
			if local {
				databaseID = "replacement"
			}
			id, err := store.ResolveDeliveryDestination(t.Context(), "http://127.0.0.1:8765", databaseID, local)
			if err != nil || id == "private" {
				t.Fatal("foreign legacy alias reused", id, err)
			}
			if pending, err := store.Pending(t.Context(), id); err != nil || pending != 1 {
				t.Fatal("foreign legacy cursor inherited", pending, err)
			}
		})
	}
}

func TestLegacyConflictingRemoteDatabaseBindingsReject(t *testing.T) {
	store, _ := newStore(t)
	for _, database := range []string{"database", "replacement"} {
		if err := store.BindDestination(t.Context(), database, "https://remote.example", database); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.ResolveDeliveryDestination(t.Context(), "https://remote.example", "database", false); err == nil {
		t.Fatal("conflicting remote legacy bindings accepted")
	}
}

func TestLegacySlashBindingRetainsRequestAndCursor(t *testing.T) {
	store, _ := newStore(t)
	const canonical = "https://remote.example/prefix"
	if err := store.BindDestination(t.Context(), "slash-binding", canonical+"/", "database"); err != nil {
		t.Fatal(err)
	}
	recordFact(t, store, testFact("first"))
	original, err := store.PrepareBatch(t.Context(), "slash-binding", "host", 1)
	if err != nil || original == nil {
		t.Fatal(err)
	}
	id, err := store.ResolveDeliveryDestination(t.Context(), canonical, "database", false)
	if err != nil || id != "slash-binding" {
		t.Fatal("retained slash binding abandoned", id, err)
	}
	retry, err := store.PrepareBatch(t.Context(), id, "new-host", 2)
	if err != nil || retry == nil || !bytes.Equal(retry.Request, original.Request) {
		t.Fatal("retained request rewritten", retry, err)
	}
	if err := store.Ack(t.Context(), id, receiptFor(retry), 3); err != nil {
		t.Fatal(err)
	}
	id, err = store.ResolveDeliveryDestination(t.Context(), canonical, "database", false)
	if err != nil || id != "slash-binding" {
		t.Fatal(id, err)
	}
	if pending, err := store.Pending(t.Context(), id); err != nil || pending != 0 {
		t.Fatal("slash normalization replayed history", pending, err)
	}
}
