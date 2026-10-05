package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/publication"
)

func TestCanonicalPublicationCommitsAtomicallyAndRebuildsStableFacts(t *testing.T) {
	source, path := collectorRebuildSetup(t)
	if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: SupportedHarnesses, Now: collectorRebuildClock()}); err != nil {
		t.Fatal(err)
	}
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 0)
	if _, err := database.Exec(`CREATE TRIGGER test_journal_fault BEFORE INSERT ON publication_journal WHEN (SELECT COUNT(*) FROM publication_journal) > 0 BEGIN SELECT RAISE(ABORT, 'injected journal fault'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path, Now: collectorRebuildClock()}); err == nil {
		t.Fatal("expected injected journal write failure")
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_entities", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue", 12)
	if _, err := database.Exec("DROP TRIGGER test_journal_fault"); err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path, Now: collectorRebuildClock()}); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 12)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 12)
	assertSQLCount(t, database, "SELECT SUM(json_extract(payload_json, '$.totalTokens')) FROM publication_journal", 1102)
	before := publicationFactHashes(t, database)
	if _, err := database.Exec("INSERT INTO normalization_work_queue (raw_fact_id,domain,enqueued_at_ms) SELECT id,'token_usage',0 FROM raw_token_usage ORDER BY id DESC"); err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path, Now: collectorRebuildClock().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 12)
	fresh := filepath.Join(t.TempDir(), "fresh.sqlite")
	collectorRebuildSync(t, fresh, source, collectorRebuildClock().Add(24*time.Hour))
	rebuilt := openTestDB(t, fresh)
	defer func() { _ = rebuilt.Close() }()
	if got := publicationFactHashes(t, rebuilt); !reflect.DeepEqual(got, before) {
		t.Fatalf("rebuild changed published IDs/payloads: got %v, want %v", got, before)
	}
}

func publicationFactHashes(t *testing.T, database *sql.DB) map[string]string {
	t.Helper()
	rows, err := database.Query("SELECT fact_id,payload_hash,payload_json FROM publication_journal ORDER BY sequence")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	facts := make(map[string]string)
	for rows.Next() {
		var id, hash, body string
		if err := rows.Scan(&id, &hash, &body); err != nil {
			t.Fatal(err)
		}
		var fact publication.Fact
		if err := json.Unmarshal([]byte(body), &fact); err != nil {
			t.Fatal(err)
		}
		if err := publication.ValidateFact(fact); err != nil {
			t.Fatalf("invalid journal fact: %v", err)
		}
		if fact.ID != id || publication.PayloadHash(fact) != hash {
			t.Fatal("journal identity/hash differs from immutable payload")
		}
		facts[id] = hash
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return facts
}

func TestPublicationQuarantinesAmbiguousNativeEvidence(t *testing.T) {
	source := t.TempDir()
	collectorRebuildCopyFile(t, filepath.Join(collectorRebuildFixtureDir(), "stages", "pi-missing-message-ids.jsonl"), filepath.Join(source, "pi", "project", "weak.jsonl"))
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 2)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'publication_ambiguous_native_identity'", 1)
}

func TestPublicationQuarantinesUnsupportedOpenCodeRevision(t *testing.T) {
	source := t.TempDir()
	for i := range 2 {
		created := int64(1767225601000 + i)
		data := `{"role":"assistant","providerID":"fixture-provider","modelID":"fixture-model","tokens":{"input":10,"output":5}}`
		createOpenCodeSQLiteMessages(t, filepath.Join(source, "opencode", []string{"opencode.db", "opencode-stable.db"}[i]), openCodeSQLiteMessage{ID: "rebuild-same", SessionID: "rebuild-main", TimeCreated: created, TimeUpdated: created, Data: data})
	}
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 2)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'publication_unsupported_native_revision'", 2)
}

func TestPublicationNeverUsesCollectionTimeForMissingOccurrence(t *testing.T) {
	source, path := collectorRebuildSetup(t)
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	// Simulate retained metadata with no occurrence evidence. Collection time is
	// usable local observation metadata but cannot be a published source fact.
	if _, err := database.Exec("UPDATE raw_token_usage SET occurred_at_ms = NULL WHERE id IN (SELECT primary_raw_fact_id FROM canonical_token_usage LIMIT 1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("DELETE FROM normalization_rule_state"); err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path, Now: collectorRebuildClock().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 12)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'publication_missing_occurrence'", 1)
}

func TestCanonicalAliasRefreshAndPublicationJournalCommitTogether(t *testing.T) {
	source := t.TempDir()
	writeJSONL(t, filepath.Join(source, "pi", "project", "alias.jsonl"),
		`{"type":"session","id":"rebuild-alias"}`,
		`{"type":"message","id":"rebuild-message","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","provider":"fixture-alias","model":"fixture-model","usage":{"input":10,"output":5,"totalTokens":15}}}`,
	)
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 1)
	providerAliases[HarnessPi]["fixture-alias"] = "fixture-canonical"
	defer delete(providerAliases[HarnessPi], "fixture-alias")
	if _, err := database.Exec(`CREATE TRIGGER test_alias_journal_fault BEFORE INSERT ON publication_journal BEGIN SELECT RAISE(ABORT, 'injected alias journal fault'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path, Harnesses: []Harness{HarnessPi}, Now: collectorRebuildClock().Add(time.Hour)}); err == nil {
		t.Fatal("expected alias journal write failure")
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE provider = 'fixture-alias'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 1)
	if _, err := database.Exec("DROP TRIGGER test_alias_journal_fault"); err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path, Harnesses: []Harness{HarnessPi}, Now: collectorRebuildClock().Add(2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE provider = 'fixture-canonical'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 2)
	assertSQLCount(t, database, "SELECT COUNT(DISTINCT fact_id) FROM publication_journal", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal WHERE sequence = 2 AND json_extract(payload_json, '$.provider') = 'fixture-canonical'", 1)
}

func TestClaudeMessageWithoutRequestCannotClaimSourceRevision(t *testing.T) {
	source := t.TempDir()
	artifact := filepath.Join(source, "claude-code", "project", "immutable.jsonl")
	older := `{"type":"assistant","timestamp":"2026-01-01T00:00:01Z","sessionId":"rebuild-main","message":{"id":"rebuild-message","role":"assistant","model":"fixture-model","usage":{"input_tokens":10,"output_tokens":5}}}`
	newer := `{"type":"assistant","timestamp":"2026-01-01T00:00:02Z","sessionId":"rebuild-main","message":{"id":"rebuild-message","role":"assistant","model":"fixture-model","usage":{"input_tokens":10,"output_tokens":6}}}`
	writeJSONL(t, artifact, older)
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	writeJSONL(t, artifact, older, newer)
	if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: []Harness{HarnessClaudeCode}, Normalize: true, Now: collectorRebuildClock().Add(time.Hour)}); err == nil {
		t.Fatal("message identity alone must not permit a revised source snapshot")
	}
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal WHERE source_revision_rule IS NULL AND json_extract(payload_json, '$.totalTokens') = 15", 1)
}
