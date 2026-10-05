package serverstore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestServerSchemaMatchesCheckedCopy(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "schema", "server.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if string(source) != Schema {
		t.Fatal("server embedded schema differs from source")
	}
}

func TestFreshServerDatabaseRoleAndDurableIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.sqlite")
	s, err := CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Metadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if m.DatabaseID == "" || m.OwnerID != "default" || m.Revision != 0 || m.LastIngestionAtMs != 0 {
		t.Fatalf("bad fresh metadata: %+v", m)
	}
	var count int
	if err := s.SQL().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND (name LIKE 'raw_%' OR name LIKE 'sync_%' OR name LIKE 'source_%' OR name='ingest_runs')`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("server contains %d producer tables", count)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	after, err := s.Metadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if after != m {
		t.Fatalf("reopen changed identity: before %+v after %+v", m, after)
	}
}

func TestServerRejectsLegacyCollectorAndFutureSchemaWithoutMutation(t *testing.T) {
	for _, application := range []int{0, 0x54495343, ApplicationID} {
		t.Run("role-"+strconv.Itoa(application), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "existing.sqlite")
			database, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec(`CREATE TABLE legacy_marker(value TEXT); INSERT INTO legacy_marker VALUES('synthetic-preserved'); PRAGMA user_version=999`); err != nil {
				t.Fatal(err)
			}
			if _, err := database.Exec("PRAGMA application_id=" + strconv.Itoa(application)); err != nil {
				t.Fatal(err)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if s, err := CreateIfMissing(path); err == nil {
				_ = s.Close()
				t.Fatal("incompatible existing DB accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("role/version rejection mutated existing DB")
			}
		})
	}
}
