package datastore

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
)

func TestCurrentInspectionRejectsChangedContractsWithoutMutation(t *testing.T) {
	// A prior successful inspection must not cache the actual file's validity.
	warm := testStore(t)
	if err := warm.Close(); err != nil {
		t.Fatal(err)
	}
	if err := Inspect(t.Context(), warm.path); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ name, sql string }{
		{"extra table", "CREATE TABLE raw.obsolete (id INTEGER)"},
		{"view semantics", "CREATE OR REPLACE VIEW analytics.confirmed AS SELECT * FROM analytics.facts"},
		{"column", "ALTER TABLE raw.evidence ADD COLUMN unexpected VARCHAR"},
		{"constraint", "ALTER TABLE ingestion.instance ALTER COLUMN created_at_ms DROP NOT NULL"},
		{"index", "DROP INDEX raw.evidence_scope"},
		{"view", "DROP VIEW analytics.confirmed; CREATE VIEW analytics.confirmed AS SELECT dataset_id FROM analytics.facts"},
	} {
		t.Run(change.name, func(t *testing.T) {
			store := testStore(t)
			if _, err := store.Accept(t.Context(), evidence.ProtocolVersion, batchBody(t, store, "stream", "batch", piRecord("message", 100))); err != nil {
				t.Fatal(err)
			}
			drain(t, store)
			if _, err := store.SQL().ExecContext(t.Context(), change.sql); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := fileDigest(store.path)
			if err != nil {
				t.Fatal(err)
			}
			if err := Inspect(t.Context(), store.path); err == nil {
				t.Fatal("inspection accepted changed contract")
			}
			if opened, err := Open(t.Context(), store.path); err == nil {
				_ = opened.Close()
				t.Fatal("writable open accepted changed contract")
			}
			after, err := fileDigest(store.path)
			if err != nil || before != after {
				t.Fatal("rejection mutated saved evidence/receipts", err)
			}
		})
	}
}

func TestConcurrentCurrentInspection(t *testing.T) {
	for _, kind := range []string{KindPersonal, KindHosted} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "server.duckdb")
			store, err := OpenKind(t.Context(), path, kind)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			if err := InspectKind(t.Context(), path, kind); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func fileDigest(path string) ([32]byte, error) {
	body, err := os.ReadFile(path)
	return sha256.Sum256(body), err
}
