package db

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectorContractRejectsWithoutMutation(t *testing.T) {
	for _, change := range []string{
		"PRAGMA user_version=19", "PRAGMA user_version=21", "PRAGMA application_id=0",
		"ALTER TABLE evidence_outbox ADD COLUMN unexpected TEXT",
		"DROP TRIGGER evidence_outbox_immutable_update",
		"CREATE TABLE obsolete(id INTEGER)",
		"CREATE TRIGGER unexpected BEFORE INSERT ON evidence_outbox BEGIN SELECT RAISE(ABORT, 'unexpected'); END",
	} {
		t.Run(change, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "collector.sqlite")
			database, created, err := CreateIfMissing(path)
			if err != nil || !created {
				t.Fatal(created, err)
			}
			if _, err := database.Exec(change); err != nil {
				t.Fatal(err)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, open := range []func(string) (*sql.DB, error){Open, OpenWritable} {
				if database, err := open(path); err == nil {
					_ = database.Close()
					t.Fatal("incompatible contract accepted")
				}
			}
			if database, _, err := CreateIfMissing(path); err == nil {
				_ = database.Close()
				t.Fatal("incompatible contract initialized")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("rejection mutated database", err)
			}
		})
	}
}

func TestCollectorCreationAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "collector.sqlite")
	for index := range 2 {
		database, created, err := CreateIfMissing(path)
		if err != nil || created != (index == 0) {
			t.Fatal(created, err)
		}
		if err := validate(t.Context(), database); err != nil {
			t.Fatal(err)
		}
		if err := database.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
