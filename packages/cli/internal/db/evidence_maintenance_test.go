package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func maintenanceSchema17(t *testing.T) (string, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	database, _, err := CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{TableEvidenceBatches, TableEvidenceDestinations, TableEvidenceOutbox, TableEvidenceSources, TableEvidenceState} {
		if _, err := database.Exec("DROP TABLE " + table); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := schemaFS.ReadFile(embeddedSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	_, old, _ := strings.Cut(string(schema), "-- Sanitized raw outbox.")
	old = strings.ReplaceAll(old, "  dataset_id TEXT NOT NULL DEFAULT 'default' CHECK (length(dataset_id) BETWEEN 1 AND 256),\n", "")
	old = strings.ReplaceAll(old, "  protocol_version INTEGER NOT NULL DEFAULT 3 CHECK (protocol_version IN (2, 3)),\n", "")
	old = strings.ReplaceAll(old, "database_id,dataset_id,protocol_version,first_sequence", "database_id,first_sequence")
	old = strings.ReplaceAll(old, "user_version = 18", "user_version = 17")
	if _, err := database.Exec("-- Sanitized raw outbox." + old); err != nil {
		t.Fatal(err)
	}
	return path, database
}

func TestResetAllSchema17UpgradeRespectsWriterLock(t *testing.T) {
	path, database := maintenanceSchema17(t)
	defer func() { _ = database.Close() }()
	release, err := AcquireWriterLock(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err = ResetAllContext(ctx, path)
	release()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("reset ignored writer lock", err)
	}
	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 17 {
		t.Fatal("blocked reset upgraded schema", version, err)
	}
	if err := ResetAllContext(t.Context(), path); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 18 {
		t.Fatal(version, err)
	}
}

func TestResetAllSchema17UpgradeProtectsPendingOutbox(t *testing.T) {
	path, database := maintenanceSchema17(t)
	defer func() { _ = database.Close() }()
	if _, err := database.Exec("INSERT INTO evidence_outbox(observation_key,harness,record_json,created_at_ms) VALUES('observation','pi','{\"harness\":\"pi\",\"format\":\"pi-jsonl\",\"sourceId\":\"source\",\"lineage\":\"lineage\",\"ordinal\":1,\"data\":{\"type\":\"session\",\"id\":\"session\"}}',1)"); err != nil {
		t.Fatal(err)
	}
	if err := ResetAllContext(t.Context(), path); err == nil {
		t.Fatal("reset discarded pending evidence")
	}
	var count, version int
	if err := database.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 18 {
		t.Fatal(version, err)
	}
}
