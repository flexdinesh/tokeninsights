package pipeline

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
)

func TestRawJSONLIncrementalTailRewriteAndCaptureRollback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	store, err := rawcollectorstore.Open(t.Context(), filepath.Join(root, "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	header := "{\"type\":\"session\",\"id\":\"session\"}\n"
	usage := func(id string, input int) string {
		return fmt.Sprintf("{\"type\":\"message\",\"id\":%q,\"message\":{\"role\":\"assistant\",\"timestamp\":1700000000000,\"usage\":{\"input\":%d,\"output\":20},\"content\":[{\"type\":\"text\",\"text\":\"private sentinel\"}]}}\n", id, input)
	}
	source := Source{ID: "source", Harness: HarnessPi, Path: path}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	capture := func(want int) {
		t.Helper()
		got, err := extractJSONL(t.Context(), source, SyncOptions{}, store)
		if err != nil || got != want {
			t.Fatalf("capture got %d want %d: %v", got, want, err)
		}
	}
	first := header + usage("one", 100)
	write(first + "{\"type\":")
	capture(2) // session plus first usage; incomplete tail remains unread.
	capture(0)
	write(first + usage("two", 200))
	capture(1)
	capture(0)
	var offset, count int64
	var oldLineage string
	if err := store.DB.QueryRow("SELECT byte_offset,lineage FROM evidence_sources").Scan(&offset, &oldLineage); err != nil || offset != int64(len(first+usage("two", 200))) {
		t.Fatal(offset, err)
	}
	if _, err := store.DB.Exec("CREATE TRIGGER fail_capture BEFORE INSERT ON evidence_outbox BEGIN SELECT RAISE(ABORT,'synthetic write failure'); END"); err != nil {
		t.Fatal(err)
	}
	write(first + usage("two", 200) + usage("three", 300))
	if _, err := extractJSONL(t.Context(), source, SyncOptions{}, store); err == nil {
		t.Fatal("failed capture acknowledged")
	}
	var failedOffset int64
	if err := store.DB.QueryRow("SELECT byte_offset FROM evidence_sources").Scan(&failedOffset); err != nil || failedOffset != offset {
		t.Fatal("checkpoint advanced after rollback", failedOffset, err)
	}
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&count); err != nil || count != 3 {
		t.Fatal("partial outbox committed", count, err)
	}
	if _, err := store.DB.Exec("DROP TRIGGER fail_capture"); err != nil {
		t.Fatal(err)
	}
	capture(1)
	// A complete final value without a newline is captured once and rechecked.
	tail := usage("four", 400)
	write(first + usage("two", 200) + usage("three", 300) + tail[:len(tail)-1])
	capture(1)
	capture(0)
	write(first + usage("two", 200) + usage("three", 300) + tail)
	capture(0)
	// Rewrite must preserve prior evidence, rotate continuity and retain native IDs.
	write(header + usage("one", 150))
	capture(2)
	var lineage string
	if err := store.DB.QueryRow("SELECT lineage FROM evidence_sources").Scan(&lineage); err != nil || lineage == oldLineage {
		t.Fatal("rewrite reused continuity", lineage, err)
	}
	var body string
	if err := store.DB.QueryRow("SELECT record_json FROM evidence_outbox ORDER BY sequence DESC LIMIT 1").Scan(&body); err != nil {
		t.Fatal(err)
	}
	var record evidence.Record
	if err := json.Unmarshal([]byte(body), &record); err != nil {
		t.Fatal(err)
	}
	if evidence.String(record.Data, "id") != "one" || len(record.Context) != 1 || evidence.String(record.Context[0].Data, "id") != "session" {
		t.Fatal("native context lost", body)
	}
	if strings.Contains(body, "private sentinel") || strings.Contains(body, "content") {
		t.Fatal("private content retained")
	}
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&count); err != nil || count != 7 {
		t.Fatal("old evidence lost", count, err)
	}
}

// Revised old rows must not be skipped by a max-ID or max-timestamp cursor.
func TestRawSQLiteCaptureFindsRevisionToOlderMessage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "opencode.db")
	createOpenCodeSQLiteMessages(t, path,
		openCodeSQLiteMessage{ID: "old", SessionID: "session", TimeCreated: 1700000000000, TimeUpdated: 1700000000000, Data: "{\"role\":\"assistant\",\"tokens\":{\"input\":100,\"output\":20}}"},
		openCodeSQLiteMessage{ID: "new", SessionID: "session", TimeCreated: 1700000001000, TimeUpdated: 1700000001000, Data: "{\"role\":\"assistant\",\"tokens\":{\"input\":200,\"output\":20}}"})
	store, err := rawcollectorstore.Open(t.Context(), filepath.Join(root, "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	source := Source{ID: "source", Harness: HarnessOpenCode, Path: path}
	capture := func(want int) {
		t.Helper()
		got, err := extractSQLite(t.Context(), source, SyncOptions{}, store)
		if err != nil || got != want {
			t.Fatalf("capture got %d want %d: %v", got, want, err)
		}
	}
	capture(2)
	capture(0)
	sourceDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sourceDB.Close() }()
	if _, err := sourceDB.Exec("UPDATE message SET data=? WHERE id='old'", "{\"role\":\"assistant\",\"tokens\":{\"input\":150,\"output\":20},\"content\":\"private revised text\"}"); err != nil {
		t.Fatal(err)
	}
	capture(1)
	capture(0)
	var body string
	if err := store.DB.QueryRow("SELECT record_json FROM evidence_outbox ORDER BY sequence DESC LIMIT 1").Scan(&body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "private revised text") {
		t.Fatal("private revised content retained")
	}
	var record evidence.Record
	if err := json.Unmarshal([]byte(body), &record); err != nil || evidence.String(record.Data, "id") != "old" {
		t.Fatal("native old-row identity lost", body, err)
	}
	object, err := evidence.DecodeData(record.Data)
	if err != nil {
		t.Fatal(err)
	}
	native, ok := object["data"].(map[string]interface{})
	if !ok {
		t.Fatal(object)
	}
	tokens, ok := native["tokens"].(map[string]interface{})
	if !ok || tokens["input"] != json.Number("150") {
		t.Fatal("revised native counters lost", native)
	}
	var count int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM evidence_outbox").Scan(&count); err != nil || count != 3 {
		t.Fatal("original observation lost", count, err)
	}
}
