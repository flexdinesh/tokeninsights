package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/persistence/postgres/testdb"
)

func openTest(t *testing.T, dsn string) *Store {
	t.Helper()
	s, err := Open(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOwnershipLossFencesWritesAndAllowsReplacement(t *testing.T) {
	dsn := testdb.New(t)
	owner := openTest(t, dsn)
	if other, err := Open(t.Context(), dsn); err == nil {
		_ = other.Close()
		t.Fatal("second owner accepted")
	}
	owner.Writer.Lock()
	var pid int
	err := owner.connection.QueryRowContext(t.Context(), "SELECT pg_backend_pid()").Scan(&pid)
	owner.Writer.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	admin := testdb.Open(t, dsn)
	if _, err := admin.ExecContext(t.Context(), "SELECT pg_terminate_backend($1)", pid); err != nil {
		t.Fatal(err)
	}
	select {
	case <-owner.Context().Done():
	case <-time.After(10 * time.Second):
		t.Fatal("owner failed to stop after physical connection loss")
	}
	replacement := openTest(t, dsn)
	if err := replacement.Ready(t.Context()); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := owner.WriteTransaction(t.Context(), func(*sql.Tx) error { called = true; return nil }); err == nil || called {
		t.Fatal("lost owner reconnected or wrote", err)
	}
	if err := owner.Ready(t.Context()); err == nil {
		t.Fatal("lost owner remained ready")
	}
}

func TestConfigurationErrorsRedactSecrets(t *testing.T) {
	for _, dsn := range []string{"postgres://user:do-not-print@%invalid", "postgres://user:do-not-print@127.0.0.1:1/absent?sslmode=disable"} {
		_, err := Open(t.Context(), dsn)
		if err == nil || strings.Contains(err.Error(), "do-not-print") {
			t.Fatal("configuration leaked credentials or succeeded", err)
		}
	}
}

func TestSchemaAndPairRejectionNeverRepairs(t *testing.T) {
	for name, mutation := range map[string]string{
		"column":          "ALTER TABLE tokeninsights_data.raw_evidence ADD COLUMN unexpected TEXT",
		"constraint":      "ALTER TABLE tokeninsights_accounts.tokens DROP CONSTRAINT tokens_user_id_fkey",
		"index":           "DROP INDEX tokeninsights_data.pending_scopes",
		"view":            "CREATE OR REPLACE VIEW tokeninsights_data.analytics_confirmed AS SELECT f.* FROM tokeninsights_data.analytics_facts f WHERE false",
		"pair":            "UPDATE tokeninsights_accounts.application_metadata SET instance_id='wrong'",
		"version":         "ALTER TABLE tokeninsights_data.ingestion_instance DROP CONSTRAINT ingestion_instance_schema_version_check; UPDATE tokeninsights_data.ingestion_instance SET schema_version=999",
		"account-version": "ALTER TABLE tokeninsights_accounts.application_metadata DROP CONSTRAINT application_metadata_schema_version_check; UPDATE tokeninsights_accounts.application_metadata SET schema_version=1",
		"partial":         "DROP SCHEMA tokeninsights_accounts CASCADE",
	} {
		t.Run(name, func(t *testing.T) {
			dsn := testdb.New(t)
			owner := openTest(t, dsn)
			if err := owner.Close(); err != nil {
				t.Fatal(err)
			}
			db := testdb.Open(t, dsn)
			if _, err := db.ExecContext(t.Context(), mutation); err != nil {
				t.Fatal(err)
			}
			before, err := Inspect(t.Context(), db)
			if err != nil {
				t.Fatal(err)
			}
			if store, err := Open(t.Context(), dsn); err == nil {
				_ = store.Close()
				t.Fatal("incompatible storage accepted")
			}
			after, err := Inspect(t.Context(), db)
			if err != nil || !maps.Equal(before, after) {
				t.Fatal("rejection changed schema", err)
			}
			if name == "pair" || name == "version" || name == "account-version" {
				var value string
				query := "SELECT instance_id FROM tokeninsights_accounts.application_metadata"
				want := "wrong"
				if name == "version" {
					query = "SELECT schema_version::text FROM tokeninsights_data.ingestion_instance"
					want = "999"
				}
				if name == "account-version" {
					query = "SELECT schema_version::text FROM tokeninsights_accounts.application_metadata"
					want = "1"
				}
				if err := db.QueryRowContext(t.Context(), query).Scan(&value); err != nil || value != want {
					t.Fatal("rejection repaired metadata", value, err)
				}
			}
		})
	}
}

func TestWriteRollbackAndReadSnapshot(t *testing.T) {
	owner := openTest(t, testdb.New(t))
	snapshot, err := owner.Reader.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = snapshot.Rollback() }()
	var original string
	if err := snapshot.QueryRowContext(t.Context(), "SELECT database_id FROM ingestion_instance").Scan(&original); err != nil {
		t.Fatal(err)
	}
	err = owner.WriteTransaction(t.Context(), func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(t.Context(), "UPDATE ingestion_instance SET database_id='uncommitted'"); err != nil {
			return err
		}
		return context.Canceled
	})
	if err != context.Canceled {
		t.Fatal(err)
	}
	var current string
	if err := owner.Reader.QueryRowContext(t.Context(), "SELECT database_id FROM ingestion_instance").Scan(&current); err != nil || current != original {
		t.Fatal("partial transaction escaped", current, err)
	}
	if err := owner.WriteTransaction(t.Context(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(t.Context(), "UPDATE ingestion_instance SET database_id='committed'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.QueryRowContext(t.Context(), "SELECT database_id FROM ingestion_instance").Scan(&current); err != nil || current != original {
		t.Fatal("read snapshot changed", current, err)
	}
}

func TestAuthoritativeSchemasMatchCatalog(t *testing.T) {
	db, err := Connect(testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	data, err := os.ReadFile("../../../../../schema/data.postgres.sql")
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := os.ReadFile("../../../../../schema/app.postgres.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(t.Context(), string(data)+string(accounts)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	actual, err := Inspect(t.Context(), db)
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]string
	if err := json.Unmarshal(expectedContract, &expected); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(actual, expected) {
		t.Fatal("authoritative PostgreSQL schemas differ from catalog contract")
	}
}
