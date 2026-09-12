package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRebuildSourceKey = "test-scope-fingerprint"

func markLifecyclePending(t *testing.T, database *sql.DB) {
	t.Helper()
	if _, err := database.Exec("UPDATE database_lifecycle SET rebuild_pending = 1, rebuild_source_key = ?", testRebuildSourceKey); err != nil {
		t.Fatal(err)
	}
}

func execLifecycleSQL(t *testing.T, database *sql.DB, statement string) {
	t.Helper()
	if _, err := database.Exec(statement); err != nil {
		t.Fatal(err)
	}
}

func inspectLifecycle(t *testing.T, path string) Compatibility {
	t.Helper()
	state, err := InspectCompatibility(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func recoverLifecycle(t *testing.T, path string) {
	t.Helper()
	release, err := AcquireWriterLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := ResetForRecovery(context.Background(), path, testRebuildSourceKey); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleMissingAndFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "usage.sqlite")
	if got := inspectLifecycle(t, path); got != (Compatibility{}) {
		t.Fatalf("missing compatibility: %+v", got)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspection created directory: %v", err)
	}
	database, created, err := CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if !created {
		t.Fatal("fresh database not created")
	}
	if got := inspectLifecycle(t, path); got != (Compatibility{Exists: true}) {
		t.Fatalf("fresh compatibility: %+v", got)
	}
	assertDBCount(t, database, TableDatabaseLifecycle, 1)
	var generation int
	if err := database.QueryRow("SELECT data_generation FROM database_lifecycle WHERE id = 1").Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if generation != CurrentDataGeneration {
		t.Fatalf("generation = %d", generation)
	}
	if _, err := database.Exec("INSERT INTO database_lifecycle VALUES (2, 1, 0, NULL, 0)"); err == nil {
		t.Fatal("lifecycle accepted a second row")
	}
}

func TestRecoveryLegacySchemas(t *testing.T) {
	for version := 2; version < SupportedSchemaVersion; version++ {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "legacy.sqlite")
			database, err := openSQLite(path, false)
			if err != nil {
				t.Fatal(err)
			}
			execLifecycleSQL(t, database, legacySchema(t, version))
			_ = database.Close()
			if got := inspectLifecycle(t, path); got != (Compatibility{Exists: true, ResetRequired: true}) {
				t.Fatalf("legacy compatibility: %+v", got)
			}
			for _, open := range []func(string) (*sql.DB, error){Open, OpenWritable} {
				if database, err := open(path); !errors.Is(err, ErrRecoveryRequired) {
					if database != nil {
						_ = database.Close()
					}
					t.Fatalf("legacy open error = %v", err)
				}
			}
			if database, _, err := CreateIfMissing(path); !errors.Is(err, ErrRecoveryRequired) {
				if database != nil {
					_ = database.Close()
				}
				t.Fatalf("legacy create error = %v", err)
			}
			recoverLifecycle(t, path)
			if got := inspectLifecycle(t, path); got != (Compatibility{Exists: true, RebuildPending: true, RebuildSourceKey: testRebuildSourceKey}) {
				t.Fatalf("recovered compatibility: %+v", got)
			}
		})
	}
}

func TestRecoveryGenerationAndResume(t *testing.T) {
	database, path := newTestDB(t)
	defer func() { _ = database.Close() }()
	insertCanonicalToken(t, database, 1000, "codex", "old", "openai", "gpt", 10, 2, 0, 0, 0, 12)
	execLifecycleSQL(t, database, "UPDATE database_lifecycle SET data_generation = 0")
	if !inspectLifecycle(t, path).ResetRequired {
		t.Fatal("old data generation accepted")
	}
	recoverLifecycle(t, path)
	assertDBCount(t, database, TableCanonicalTokenUsage, 0)
	assertDBCount(t, database, TableRawTokenUsage, 0)
	if opened, err := Open(path); !errors.Is(err, ErrRebuildPending) {
		if opened != nil {
			_ = opened.Close()
		}
		t.Fatalf("pending analytics error = %v", err)
	}
	prepared, created, err := CreateIfMissing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = prepared.Close() }()
	if created {
		t.Fatal("pending database recreated")
	}
	insertCanonicalToken(t, prepared, 2000, "codex", "resumed", "openai", "gpt", 7, 1, 0, 0, 0, 8)
	execLifecycleSQL(t, prepared, "INSERT INTO normalization_work_queue (raw_fact_id, domain, enqueued_at_ms) SELECT id, 'token_usage', 2000 FROM raw_token_usage")
	// A post-commit retry must preserve partial imports and pending work.
	recoverLifecycle(t, path)
	assertDBCount(t, prepared, TableRawTokenUsage, 1)
	assertDBCount(t, prepared, TableNormalizationWorkQueue, 1)
	if err := CompleteRecovery(context.Background(), prepared); !errors.Is(err, ErrRebuildPending) {
		t.Fatalf("completion with pending work = %v", err)
	}
	if !inspectLifecycle(t, path).RebuildPending {
		t.Fatal("failed completion published analytics")
	}
	execLifecycleSQL(t, prepared, "DELETE FROM normalization_work_queue")
	if err := CompleteRecovery(context.Background(), prepared); err != nil {
		t.Fatal(err)
	}
	if got := inspectLifecycle(t, path); got != (Compatibility{Exists: true}) {
		t.Fatalf("completed compatibility: %+v", got)
	}
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	assertDBCount(t, reader, TableCanonicalTokenUsage, 1)
	recoverLifecycle(t, path)
	assertDBCount(t, reader, TableCanonicalTokenUsage, 1)
}

func TestRecoveryRefusesUnsafeFilesWithoutChangingContents(t *testing.T) {
	for _, kind := range []string{"newer-schema", "newer-generation", "foreign", "foreign-current-version", "foreign-extra-table", "corrupt", "missing-lifecycle", "missing-singleton"} {
		t.Run(kind, func(t *testing.T) {
			database, path := newTestDB(t)
			switch kind {
			case "newer-schema":
				execLifecycleSQL(t, database, fmt.Sprintf("PRAGMA user_version = %d", SupportedSchemaVersion+1))
			case "newer-generation":
				execLifecycleSQL(t, database, fmt.Sprintf("UPDATE database_lifecycle SET data_generation = %d", CurrentDataGeneration+1))
			case "foreign", "foreign-current-version":
				_ = database.Close()
				path = filepath.Join(t.TempDir(), "foreign.sqlite")
				var err error
				database, err = openSQLite(path, false)
				if err != nil {
					t.Fatal(err)
				}
				execLifecycleSQL(t, database, "CREATE TABLE notes (id INTEGER PRIMARY KEY, title TEXT); INSERT INTO notes VALUES (1, 'keep'); PRAGMA user_version = 2")
				if kind == "foreign-current-version" {
					execLifecycleSQL(t, database, fmt.Sprintf("PRAGMA user_version = %d", SupportedSchemaVersion))
				}
			case "foreign-extra-table":
				execLifecycleSQL(t, database, "CREATE TABLE notes (id INTEGER PRIMARY KEY); UPDATE database_lifecycle SET data_generation = 0")
			case "missing-lifecycle":
				execLifecycleSQL(t, database, "DROP TABLE database_lifecycle")
			case "missing-singleton":
				execLifecycleSQL(t, database, "DELETE FROM database_lifecycle")
			}
			_ = database.Close()
			if kind == "corrupt" {
				if err := os.WriteFile(path, []byte("not a SQLite database"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := InspectCompatibility(context.Background(), path); err == nil {
				t.Fatal("unsafe database accepted by inspection")
			}
			for _, open := range []func(string) (*sql.DB, error){Open, OpenWritable} {
				if database, err := open(path); err == nil {
					_ = database.Close()
					t.Fatal("unsafe database opened")
				}
			}
			if database, _, err := CreateIfMissing(path); err == nil {
				_ = database.Close()
				t.Fatal("unsafe database accepted by create")
			}
			release, err := AcquireWriterLock(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			if err := ResetForRecovery(context.Background(), path, testRebuildSourceKey); err == nil {
				t.Fatal("unsafe automatic reset accepted")
			}
			release()
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("unsafe file changed")
			}
		})
	}
}

func TestResetRollbackPreservesOldDataAndLifecycle(t *testing.T) {
	for _, failDDL := range []bool{false, true} {
		t.Run(fmt.Sprintf("ddl-error-%t", failDDL), func(t *testing.T) {
			database, path := newTestDB(t)
			defer func() { _ = database.Close() }()
			insertCanonicalToken(t, database, 1000, "codex", "old", "openai", "gpt", 10, 2, 0, 0, 0, 12)
			execLifecycleSQL(t, database, "UPDATE database_lifecycle SET data_generation = 0, updated_at_ms = 123")
			conn, err := database.Conn(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			if _, err := conn.ExecContext(context.Background(), "PRAGMA foreign_keys = OFF"); err != nil {
				t.Fatal(err)
			}
			tx, err := conn.BeginTx(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			_, body, err := schemaParts()
			if err != nil {
				t.Fatal(err)
			}
			if failDDL {
				body += "\nINVALID SQL;"
			}
			err = replaceSchemaTx(context.Background(), tx, body, testRebuildSourceKey)
			if (err != nil) != failDDL {
				t.Fatalf("DDL error = %v", err)
			}
			// Simulate interruption at the boundary immediately before COMMIT.
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			assertDBCount(t, database, TableCanonicalTokenUsage, 1)
			if got := inspectLifecycle(t, path); got != (Compatibility{Exists: true, ResetRequired: true}) {
				t.Fatalf("rolled-back compatibility: %+v", got)
			}
			var updated int64
			if err := database.QueryRow("SELECT updated_at_ms FROM database_lifecycle").Scan(&updated); err != nil || updated != 123 {
				t.Fatalf("old lifecycle lost: %d, %v", updated, err)
			}
		})
	}
}

func TestResetAllPreservesLiveReaderSnapshotAndInode(t *testing.T) {
	database, path := newTestDB(t)
	defer func() { _ = database.Close() }()
	insertCanonicalToken(t, database, 1000, "codex", "old", "openai", "gpt", 10, 2, 0, 0, 0, 12)
	markLifecyclePending(t, database)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := database.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM canonical_token_usage").Scan(&count); err != nil || count != 1 {
		t.Fatalf("old reader: %d, %v", count, err)
	}
	if err := ResetAll(path); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, after) {
		t.Fatal("reset replaced the live database inode")
	}
	if err := tx.QueryRow("SELECT COUNT(*) FROM canonical_token_usage").Scan(&count); err != nil || count != 1 {
		t.Fatalf("reader snapshot lost: %d, %v", count, err)
	}
	if got := inspectLifecycle(t, path); got != (Compatibility{Exists: true}) {
		t.Fatalf("explicit reset lifecycle: %+v", got)
	}
	assertDBCount(t, database, TableCanonicalTokenUsage, 0)
	assertDBCount(t, database, TableDatabaseLifecycle, 1)
}

func TestResetCanonicalPreservesLifecycleAndRejectsPending(t *testing.T) {
	database, path := newTestDB(t)
	defer func() { _ = database.Close() }()
	insertCanonicalToken(t, database, 1000, "codex", "old", "openai", "gpt", 10, 2, 0, 0, 0, 12)
	execLifecycleSQL(t, database, "UPDATE database_lifecycle SET updated_at_ms = 123")
	if err := ResetCanonical(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	var updated int64
	if err := database.QueryRow("SELECT updated_at_ms FROM database_lifecycle").Scan(&updated); err != nil || updated != 123 {
		t.Fatalf("canonical reset changed lifecycle: %d, %v", updated, err)
	}
	markLifecyclePending(t, database)
	if err := ResetCanonical(context.Background(), database); !errors.Is(err, ErrRebuildPending) {
		t.Fatalf("pending canonical reset = %v", err)
	}
	if !inspectLifecycle(t, path).RebuildPending {
		t.Fatal("pending flag lost")
	}
	execLifecycleSQL(t, database, "UPDATE database_lifecycle SET data_generation = 0")
	if err := ResetCanonical(context.Background(), database); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("old generation canonical reset = %v", err)
	}
	if err := CompleteRecovery(context.Background(), database); !errors.Is(err, ErrRecoveryRequired) {
		t.Fatalf("old generation completion = %v", err)
	}
}

func TestApplySchemaDoesNotBlessMissingLifecycle(t *testing.T) {
	database, path := newTestDB(t)
	defer func() { _ = database.Close() }()
	execLifecycleSQL(t, database, "DELETE FROM database_lifecycle")
	if err := ApplySchema(context.Background(), database); err == nil {
		t.Fatal("ApplySchema blessed a missing lifecycle row")
	}
	assertDBCount(t, database, TableDatabaseLifecycle, 0)
	if _, err := OpenWritable(path); err == nil {
		t.Fatal("open blessed a missing lifecycle row")
	}
}

func TestCurrentLifecycleAllowsAdditionalTriggers(t *testing.T) {
	database, path := newTestDB(t)
	defer func() { _ = database.Close() }()
	execLifecycleSQL(t, database, `
		CREATE TRIGGER reject_raw_insert BEFORE INSERT ON raw_token_usage
		BEGIN SELECT RAISE(ABORT, 'injected ingest failure'); END
	`)
	for _, pending := range []bool{false, true} {
		want := Compatibility{Exists: true, RebuildPending: pending}
		if pending {
			markLifecyclePending(t, database)
			want.RebuildSourceKey = testRebuildSourceKey
		}
		if got := inspectLifecycle(t, path); got != want {
			t.Fatalf("trigger changed compatibility: %+v", got)
		}
		writer, err := OpenWritable(path)
		if err != nil {
			t.Fatal(err)
		}
		assertDBCount(t, writer, TableRawTokenUsage, 0)
		_ = writer.Close()
		reader, err := Open(path)
		if pending {
			if !errors.Is(err, ErrRebuildPending) {
				t.Fatalf("pending trigger database open = %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		} else {
			_ = reader.Close()
		}
	}
	_, err := database.Exec(`
		INSERT INTO raw_token_usage (
			raw_fact_key, harness, source_id, source_kind, collector, parser,
			observed_at_ms, usage_scope, quality
		) VALUES ('test', 'codex', 'test', 'test', 'test', 'test', 0, 'message', 'exact')
	`)
	if err == nil || !strings.Contains(err.Error(), "injected ingest failure") {
		t.Fatalf("trigger not preserved: %v", err)
	}
}

func TestLifecycleSourceKeyConstraints(t *testing.T) {
	for _, values := range []string{
		"rebuild_pending = 0, rebuild_source_key = 'unexpected-key'",
		"rebuild_pending = 0, rebuild_source_key = ''",
		"rebuild_pending = 1, rebuild_source_key = NULL",
		"rebuild_pending = 1, rebuild_source_key = ''",
	} {
		t.Run(values, func(t *testing.T) {
			database, path := newTestDB(t)
			defer func() { _ = database.Close() }()
			statement := "UPDATE database_lifecycle SET " + values
			if _, err := database.Exec(statement); err == nil {
				t.Fatal("invalid source state accepted by schema")
			}
			if got := inspectLifecycle(t, path); got != (Compatibility{Exists: true}) {
				t.Fatalf("failed write changed lifecycle: %+v", got)
			}
			// Older or external writers may bypass checks. Inspection must still
			// reject inconsistent markers before any recovery or analytics work.
			database.SetMaxOpenConns(1)
			execLifecycleSQL(t, database, "PRAGMA ignore_check_constraints = ON")
			execLifecycleSQL(t, database, statement)
			if _, err := InspectCompatibility(context.Background(), path); err == nil {
				t.Fatal("invalid source state accepted by inspection")
			}
		})
	}
}

func TestRecoverySourceScopeMustMatch(t *testing.T) {
	database, path := newTestDB(t)
	defer func() { _ = database.Close() }()
	release, err := AcquireWriterLock(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := ResetForRecovery(context.Background(), path, ""); err == nil {
		t.Fatal("empty recovery source key accepted")
	}
	// A valid key alone is not a reason to reset a compatible database.
	insertCanonicalToken(t, database, 1000, "codex", "ready", "openai", "gpt", 10, 2, 0, 0, 0, 12)
	if err := ResetForRecovery(context.Background(), path, testRebuildSourceKey); err != nil {
		t.Fatal(err)
	}
	assertDBCount(t, database, TableCanonicalTokenUsage, 1)
	if got := inspectLifecycle(t, path); got != (Compatibility{Exists: true}) {
		t.Fatalf("compatible database entered recovery: %+v", got)
	}
	execLifecycleSQL(t, database, "UPDATE database_lifecycle SET data_generation = 0")
	if err := ResetForRecovery(context.Background(), path, testRebuildSourceKey); err != nil {
		t.Fatal(err)
	}
	insertCanonicalToken(t, database, 2000, "codex", "partial", "openai", "gpt", 7, 1, 0, 0, 0, 8)
	execLifecycleSQL(t, database, "UPDATE database_lifecycle SET updated_at_ms = 123")
	for _, generation := range []int{CurrentDataGeneration, 0} {
		execLifecycleSQL(t, database, fmt.Sprintf("UPDATE database_lifecycle SET data_generation = %d", generation))
		if err := ResetForRecovery(context.Background(), path, "different-scope-fingerprint"); !errors.Is(err, ErrRebuildPending) {
			t.Fatalf("mismatched scope error = %v", err)
		}
		assertDBCount(t, database, TableCanonicalTokenUsage, 1)
		if got := inspectLifecycle(t, path); got != (Compatibility{
			Exists: true, ResetRequired: generation < CurrentDataGeneration,
			RebuildPending: true, RebuildSourceKey: testRebuildSourceKey,
		}) {
			t.Fatalf("mismatched retry changed state: %+v", got)
		}
	}
	execLifecycleSQL(t, database, fmt.Sprintf("UPDATE database_lifecycle SET data_generation = %d", CurrentDataGeneration))
	if err := ResetForRecovery(context.Background(), path, testRebuildSourceKey); err != nil {
		t.Fatal(err)
	}
	assertDBCount(t, database, TableCanonicalTokenUsage, 1)
	var updated int64
	if err := database.QueryRow("SELECT updated_at_ms FROM database_lifecycle").Scan(&updated); err != nil || updated != 123 {
		t.Fatalf("retry changed lifecycle timestamp: %d, %v", updated, err)
	}
	if err := CompleteRecovery(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	var sourceKey sql.NullString
	if err := database.QueryRow("SELECT rebuild_source_key FROM database_lifecycle").Scan(&sourceKey); err != nil || sourceKey.Valid {
		t.Fatalf("completed recovery retained key: %+v, %v", sourceKey, err)
	}
}

func TestBeginAnalyticsReadRejectsRecoveryAfterOpen(t *testing.T) {
	database, path := newTestDB(t)
	defer func() { _ = database.Close() }()
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	reader.SetMaxOpenConns(1)
	execLifecycleSQL(t, database, "UPDATE database_lifecycle SET data_generation = 0")
	if tx, err := BeginAnalyticsRead(context.Background(), reader); !errors.Is(err, ErrRecoveryRequired) {
		if tx != nil {
			_ = tx.Rollback()
		}
		t.Fatalf("incompatible analytics transaction = %v", err)
	}
	recoverLifecycle(t, path)
	if tx, err := BeginAnalyticsRead(context.Background(), reader); !errors.Is(err, ErrRebuildPending) {
		if tx != nil {
			_ = tx.Rollback()
		}
		t.Fatalf("pending analytics transaction = %v", err)
	}
	if got := reader.Stats().InUse; got != 0 {
		t.Fatalf("rejected transaction retained %d connections", got)
	}
	if err := CompleteRecovery(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	tx, err := BeginAnalyticsRead(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
}

func TestBeginAnalyticsReadPinsSnapshotAcrossRecovery(t *testing.T) {
	database, path := newTestDB(t)
	defer func() { _ = database.Close() }()
	insertCanonicalToken(t, database, 1000, "codex", "ready", "openai", "gpt", 10, 2, 0, 0, 0, 12)
	reader, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	tx, err := BeginAnalyticsRead(context.Background(), reader)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	// No analytics query yet: lifecycle validation itself must pin the snapshot.
	execLifecycleSQL(t, database, "UPDATE database_lifecycle SET data_generation = 0")
	recoverLifecycle(t, path)
	var total int
	if err := tx.QueryRow("SELECT SUM(total_tokens) FROM canonical_token_usage").Scan(&total); err != nil || total != 12 {
		t.Fatalf("analytics snapshot changed: %d, %v", total, err)
	}
	state, err := inspectCompatibility(context.Background(), tx)
	if err != nil || state != (Compatibility{Exists: true}) {
		t.Fatalf("lifecycle snapshot changed: %+v, %v", state, err)
	}
	if got := inspectLifecycle(t, path); !got.RebuildPending || got.RebuildSourceKey != testRebuildSourceKey {
		t.Fatalf("recovery not committed: %+v", got)
	}
	assertDBCount(t, database, TableCanonicalTokenUsage, 0)
}

func legacySchema(t *testing.T, version int) string {
	t.Helper()
	if version == 2 {
		return historicalEventSchema
	}
	schema, err := schemaFS.ReadFile(embeddedSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var statements []string
	for _, statement := range strings.Split(string(schema), ";") {
		if strings.Contains(statement, "database_lifecycle") ||
			(version < 6 && strings.Contains(statement, "normalization_work_queue")) ||
			(version < 7 && strings.Contains(statement, "source_refresh_state")) ||
			strings.Contains(statement, "user_version") {
			continue
		}
		if version == 3 {
			statement = strings.ReplaceAll(statement, ", 'claude-code'", "")
		}
		if version < 5 {
			var lines []string
			for _, line := range strings.Split(statement, "\n") {
				if !strings.Contains(line, "provider_source") {
					lines = append(lines, line)
				}
			}
			statement = strings.Join(lines, "\n")
		}
		statements = append(statements, statement)
	}
	return strings.Join(statements, ";") + fmt.Sprintf("; PRAGMA user_version = %d;", version)
}

// V2's shipped event-table family (6c5140d), omitting only indexes and checks.
// It never had ingest_runs/raw_token_usage or a generic token_events table.
const historicalEventSchema = `
CREATE TABLE oc_token_events (
 id INTEGER PRIMARY KEY, recorded_at TEXT, recorded_at_ms INTEGER, session_id TEXT,
 message_id TEXT, part_id TEXT, source TEXT, provider TEXT, model TEXT,
 input_tokens INTEGER, output_tokens INTEGER, reasoning_tokens INTEGER,
 cache_read_tokens INTEGER, cache_write_tokens INTEGER, total_tokens INTEGER
);
CREATE TABLE pi_token_events (
 id INTEGER PRIMARY KEY, recorded_at TEXT, recorded_at_ms INTEGER, session_id TEXT,
 message_id TEXT, provider TEXT, model TEXT, input_tokens INTEGER, output_tokens INTEGER,
 reasoning_tokens INTEGER, cache_read_tokens INTEGER, cache_write_tokens INTEGER, total_tokens INTEGER
);
CREATE TABLE oc_tps_samples (
 id INTEGER PRIMARY KEY, recorded_at TEXT, recorded_at_ms INTEGER, session_id TEXT,
 message_id TEXT, provider TEXT, model TEXT, output_tokens INTEGER, reasoning_tokens INTEGER,
 total_tokens INTEGER, duration_ms INTEGER, ttft_ms INTEGER, tokens_per_second REAL
);
CREATE TABLE pi_tps_samples (
 id INTEGER PRIMARY KEY, recorded_at TEXT, recorded_at_ms INTEGER, session_id TEXT,
 message_id TEXT, provider TEXT, model TEXT, output_tokens INTEGER, reasoning_tokens INTEGER,
 total_tokens INTEGER, duration_ms INTEGER, ttft_ms INTEGER, tokens_per_second REAL
);
CREATE TABLE oc_llm_requests (
 id INTEGER PRIMARY KEY, recorded_at TEXT, recorded_at_ms INTEGER, session_id TEXT,
 message_id TEXT, provider TEXT, model TEXT, attempt_index INTEGER, thinking_level TEXT
);
CREATE TABLE pi_llm_requests (
 id INTEGER PRIMARY KEY, recorded_at TEXT, recorded_at_ms INTEGER, session_id TEXT,
 message_id TEXT, provider TEXT, model TEXT, attempt_index INTEGER, thinking_level TEXT
);
CREATE TABLE oc_tool_calls (
 id INTEGER PRIMARY KEY, recorded_at TEXT, recorded_at_ms INTEGER, session_id TEXT,
 message_id TEXT, tool_call_id TEXT, tool_name TEXT, provider TEXT, model TEXT, status TEXT
);
CREATE TABLE pi_tool_calls (
 id INTEGER PRIMARY KEY, recorded_at TEXT, recorded_at_ms INTEGER, session_id TEXT,
 message_id TEXT, tool_call_id TEXT, tool_name TEXT, provider TEXT, model TEXT, status TEXT
);
INSERT INTO oc_token_events (id, session_id, total_tokens) VALUES (1, 'legacy', 42);
PRAGMA user_version = 2;
`
