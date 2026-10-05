package serverstore

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnboundedTimestampSchemaRemainsUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-server.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// A previous server can contain dates acknowledged under the old safe-integer
	// rule. The new server must never upgrade, clear, or silently repair this file.
	previous := strings.Replace(Schema, "PRAGMA user_version = 2;", "PRAGMA user_version = 1;", 1)
	for _, column := range []string{"first_seen_at_ms", "last_seen_at_ms", "occurred_at_ms", "recorded_at_ms", "revision_value"} {
		previous = strings.Replace(previous, fmt.Sprintf("CHECK (typeof(%s) = 'integer' AND %s BETWEEN 0 AND 253402214399999)", column, column), fmt.Sprintf("CHECK (%s BETWEEN 0 AND 9007199254740991)", column), 1)
	}
	if _, err := database.Exec(previous); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		INSERT INTO server_metadata(id,database_id,owner_id,identity_version,semantics_version,created_at_ms) VALUES(1,'old-server','default',1,1,0);
		INSERT INTO canonical_sessions(semantic_key,harness,session_id,first_seen_at_ms,last_seen_at_ms) VALUES('old-session','pi','old-session',1767225600000000,1767225600000000);
		INSERT INTO canonical_token_usage(semantic_key,recorded_at_ms,harness,session_id,usage_scope,quality,is_countable,payload_hash) VALUES('old-fact',1767225600000000,'pi',1,'message','exact',1,'saved-payload');`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if store, err := CreateIfMissing(path); err == nil {
		_ = store.Close()
		t.Fatal("old timestamp contract accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("old server changed: error=%v", err)
	}
}
