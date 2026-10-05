package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func claudeIdentityRecord(session, message, request, timestamp string, input, output int64) string {
	return fmt.Sprintf(`{"type":"assistant","sessionId":%q,"requestId":%q,"timestamp":%q,"message":{"id":%q,"role":"assistant","model":"fixture-model","usage":{"input_tokens":%d,"output_tokens":%d}}}`, session, request, timestamp, message, input, output)
}

func TestClaudeNativeIdentityScopesMessageRequestAndSession(t *testing.T) {
	source := t.TempDir()
	artifact := filepath.Join(source, "claude-code", "project", "scope.jsonl")
	writeJSONL(t, artifact,
		claudeIdentityRecord("rebuild-main", "message", "request-a", "2026-01-01T00:00:01Z", 10, 5),
		claudeIdentityRecord("rebuild-main", "message", "request-b", "2026-01-01T00:00:01Z", 10, 5),
		claudeIdentityRecord("rebuild-other", "message", "request-a", "2026-01-01T00:00:01Z", 10, 5),
		claudeIdentityRecord("rebuild-main", "message|request", "a", "2026-01-01T00:00:01Z", 10, 5),
		claudeIdentityRecord("rebuild-main", "message", "request|a", "2026-01-01T00:00:01Z", 10, 5),
	)
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 5)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 5)
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 75)
}

func TestClaudeLatestSnapshotDoesNotSynthesizeComponentMaxima(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			source := t.TempDir()
			artifact := filepath.Join(source, "claude-code", "project", "snapshot.jsonl")
			older := claudeIdentityRecord("rebuild-main", "message", "request", "2026-01-01T00:00:01Z", 100, 5)
			newer := claudeIdentityRecord("rebuild-main", "message", "request", "2026-01-01T00:00:02Z", 90, 10)
			records := []string{older, newer}
			if reverse {
				records = []string{newer, older}
			}
			writeJSONL(t, artifact, records...)
			path := filepath.Join(t.TempDir(), "collector.sqlite")
			collectorRebuildSync(t, path, source, collectorRebuildClock())
			// A copied older transcript must not roll back the canonical revision,
			// including when normalization later replays retained raw in reverse order.
			writeJSONL(t, filepath.Join(source, "claude-code", "archive", "older.jsonl"), older)
			collectorRebuildSync(t, path, source, collectorRebuildClock().Add(time.Hour))
			database := openTestDB(t, path)
			defer func() { _ = database.Close() }()
			assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 1)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE input_tokens = 90 AND output_tokens = 10 AND total_tokens = 100 AND recorded_at_ms = 1767225602000", 1)
			if _, err := database.Exec("INSERT INTO normalization_work_queue (raw_fact_id, domain, enqueued_at_ms) SELECT id, 'token_usage', 0 FROM raw_token_usage ORDER BY occurred_at_ms DESC"); err != nil {
				t.Fatal(err)
			}
			if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path}); err != nil {
				t.Fatal(err)
			}
			assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE input_tokens = 90 AND output_tokens = 10 AND total_tokens = 100", 1)
		})
	}
}

func TestClaudeEqualSourceRevisionConflictPreservesSavedUsage(t *testing.T) {
	for _, sameSource := range []bool{false, true} {
		t.Run(fmt.Sprint(sameSource), func(t *testing.T) {
			source := t.TempDir()
			artifact := filepath.Join(source, "claude-code", "project", "snapshot.jsonl")
			first := claudeIdentityRecord("rebuild-main", "message", "request", "2026-01-01T00:00:01Z", 10, 5)
			changed := claudeIdentityRecord("rebuild-main", "message", "request", "2026-01-01T00:00:01Z", 10, 6)
			writeJSONL(t, artifact, first)
			path := filepath.Join(t.TempDir(), "collector.sqlite")
			collectorRebuildSync(t, path, source, collectorRebuildClock())
			if sameSource {
				writeJSONL(t, artifact, first, changed)
			} else {
				writeJSONL(t, filepath.Join(source, "claude-code", "archive", "conflict.jsonl"), changed)
			}
			_, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: []Harness{HarnessClaudeCode}, Normalize: true, Now: collectorRebuildClock().Add(time.Hour)})
			if err == nil || !strings.Contains(err.Error(), "conflicting usage at the same source timestamp") {
				t.Fatalf("got %v, want explicit native revision conflict", err)
			}
			database := openTestDB(t, path)
			defer func() { _ = database.Close() }()
			assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 1)
			assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 15)
			if sameSource {
				assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 1)
			} else {
				// Cross-source conflicting normalized observations remain in the
				// metadata-only raw store for investigation/retry.
				assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 2)
			}
		})
	}
}

func TestOpenCodeChannelCopiesRequireMatchingNativeIdentities(t *testing.T) {
	source := t.TempDir()
	data := `{"id":"same_json_payload_id","role":"assistant","providerID":"fixture-provider","modelID":"fixture-model","tokens":{"input":10,"output":5,"reasoning":0},"time":{"created":1767225601000}}`
	for i, nativeID := range []string{"rebuild-first", "rebuild-second"} {
		createOpenCodeSQLiteMessages(t, filepath.Join(source, "opencode", fmt.Sprintf("opencode-%d.db", i)), openCodeSQLiteMessage{ID: nativeID, SessionID: "rebuild-main", TimeCreated: 1767225601000, TimeUpdated: 1767225601000, Data: data})
	}
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 2)
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 30)
}
