package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectorCanonicalTimestampConstraints(t *testing.T) {
	database, _ := newTestDB(t)
	defer func() { _ = database.Close() }()
	insertCanonicalToken(t, database, 1767225600000, "pi", "timestamp-session", "unknown", "unknown", 1, 0, 0, 0, 0, 1)
	for _, statement := range []string{
		`INSERT INTO canonical_messages(semantic_key,session_id,harness,harness_message_id,occurred_at_ms) VALUES('message',1,'pi','native-message',0)`,
		`INSERT INTO publication_journal(sequence,fact_id,payload_hash,payload_json,identity_version,semantics_version,source_revision_rule,source_revision_value,created_at_ms) VALUES(1,'fact',printf('%064d',0),'{}',1,1,'claude-source-timestamp-v1',0,0)`,
		`INSERT INTO publication_entities(fact_id,payload_hash,sequence,source_revision_rule,source_revision_value) VALUES('fact',printf('%064d',0),1,'claude-source-timestamp-v1',0)`,
	} {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	for name, statement := range map[string]string{
		"session":          `UPDATE canonical_sessions SET first_seen_at_ms=?,last_seen_at_ms=?`,
		"message":          `UPDATE canonical_messages SET occurred_at_ms=?`,
		"fact":             `UPDATE canonical_token_usage SET recorded_at_ms=?`,
		"journal-revision": `INSERT INTO publication_journal(fact_id,payload_hash,payload_json,identity_version,semantics_version,source_revision_rule,source_revision_value,created_at_ms) VALUES('fact',printf('%064d',0),'{}',1,1,'claude-source-timestamp-v1',?,0)`,
		"entity-revision":  `UPDATE publication_entities SET source_revision_value=?`,
	} {
		for _, value := range []any{int64(-1), int64(0), int64(253402214399999), int64(253402214400000), float64(0.5)} {
			t.Run(fmt.Sprintf("%s/%v", name, value), func(t *testing.T) {
				args := []any{value}
				if name == "session" {
					args = append(args, value)
				}
				_, err := database.Exec(statement, args...)
				integer, ok := value.(int64)
				wantValid := ok && integer >= 0 && integer <= 253402214399999
				if (err == nil) != wantValid {
					t.Fatalf("SQL error=%v wantValid=%v", err, wantValid)
				}
			})
		}
	}
	if _, err := database.Exec(`UPDATE canonical_messages SET occurred_at_ms=NULL`); err != nil {
		t.Fatalf("nullable collector message timestamp rejected: %v", err)
	}
	if _, err := database.Exec(`UPDATE raw_token_usage SET occurred_at_ms=1767225600000000`); err != nil {
		t.Fatalf("raw evidence cannot be retained: %v", err)
	}
	var retained int
	if err := database.QueryRow(`SELECT COUNT(*) FROM raw_token_usage WHERE occurred_at_ms=1767225600000000`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("invalid raw evidence count=%d error=%v", retained, err)
	}
}

func TestUnboundedCollectorSchemaRemainsUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-collector.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := schemaFS.ReadFile(embeddedSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	previous := strings.Replace(string(schema), "PRAGMA user_version = 16;", "PRAGMA user_version = 15;", 1)
	for _, column := range []string{"first_seen_at_ms", "last_seen_at_ms", "recorded_at_ms"} {
		previous = strings.Replace(previous, fmt.Sprintf(" CHECK (typeof(%s) = 'integer' AND %s BETWEEN 0 AND 253402214399999)", column, column), "", 1)
	}
	previous = strings.Replace(previous, " CHECK (occurred_at_ms IS NULL OR (typeof(occurred_at_ms) = 'integer' AND occurred_at_ms BETWEEN 0 AND 253402214399999))", "", 1)
	previous = strings.ReplaceAll(previous, "typeof(source_revision_value) = 'integer' AND source_revision_value BETWEEN 0 AND 253402214399999", "source_revision_value >= 0 AND source_revision_value <= 9007199254740991")
	if _, err := database.Exec(previous); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO database_lifecycle(id,data_generation,rebuild_pending,updated_at_ms) VALUES(1,6,0,0)`); err != nil {
		t.Fatal(err)
	}
	insertCanonicalToken(t, database, 1767225600000000, "pi", "old-session", "unknown", "unknown", 1, 0, 0, 0, 0, 1)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InspectCompatibility(context.Background(), path); err == nil {
		t.Fatal("old timestamp contract accepted for collection")
	}
	for _, open := range []func(string) (*sql.DB, error){Open, OpenWritable} {
		if database, err := open(path); err == nil {
			_ = database.Close()
			t.Fatal("old timestamp contract accepted")
		}
	}
	if database, _, err := CreateIfMissing(path); err == nil {
		_ = database.Close()
		t.Fatal("old timestamp contract claimed as fresh")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("old collector changed: error=%v", err)
	}
}
