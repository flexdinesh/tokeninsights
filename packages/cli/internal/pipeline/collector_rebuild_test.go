package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

// These tests exercise the current real adapters and SQLite pipeline. They do
// not pretend a collector journal or server ingestion endpoint already exists.
func TestCollectorRebuildFixture(t *testing.T) {
	source, path := collectorRebuildSetup(t)
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	collectorRebuildAssertFacts(t, path)
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	for _, code := range []string{"codex_jsonl_duplicate_token_snapshot", "codex_jsonl_stale_token_snapshot", "codex_jsonl_replay_resolved"} {
		var count int
		if err := database.QueryRow("SELECT COUNT(*) FROM normalization_diagnostics WHERE code = ?", code).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Errorf("CFI-001: missing diagnostic %s", code)
		}
	}
}

func TestCollectorRebuildRepeatedSyncPreservesFactsAndIdentities(t *testing.T) {
	source, path := collectorRebuildSetup(t)
	now := collectorRebuildClock()
	collectorRebuildSync(t, path, source, now)
	want := collectorRebuildIdentities(t, path)
	const repeatCount = 100
	for iteration := range repeatCount {
		if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: SupportedHarnesses, Normalize: true, FullRefresh: true, Now: now.Add(time.Duration(iteration+1) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	collectorRebuildAssertFacts(t, path)
	if got := collectorRebuildIdentities(t, path); !reflect.DeepEqual(got, want) {
		t.Fatalf("CFI-002: repeated sync changed canonical identity/time: got %+v, want %+v", got, want)
	}
}

func TestCollectorRebuildFreshDatabaseAtDifferentClocks(t *testing.T) {
	source, path := collectorRebuildSetup(t)
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	want := collectorRebuildIdentities(t, path)
	// No operational or canonical state is copied into these replacement DBs.
	for _, clock := range []time.Time{collectorRebuildClock().Add(-48 * time.Hour), collectorRebuildClock().Add(365 * 24 * time.Hour)} {
		fresh := filepath.Join(t.TempDir(), "replacement.sqlite")
		collectorRebuildSync(t, fresh, source, clock)
		collectorRebuildAssertFacts(t, fresh)
		if got := collectorRebuildIdentities(t, fresh); !reflect.DeepEqual(got, want) {
			t.Errorf("CFI-003: new database at %s changed canonical identity/time: got %+v, want %+v", clock, got, want)
		}
	}
}

func TestCollectorRebuildNormalizeRetainedRawWithoutSources(t *testing.T) {
	source, path := collectorRebuildSetup(t)
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	want := collectorRebuildIdentities(t, path)
	database := openTestDB(t, path)
	if err := db.ResetCanonical(context.Background(), database); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(source); err != nil {
		t.Fatal(err)
	}
	if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path, Harnesses: SupportedHarnesses, Now: collectorRebuildClock().Add(24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	collectorRebuildAssertFacts(t, path)
	if got := collectorRebuildIdentities(t, path); !reflect.DeepEqual(got, want) {
		t.Fatalf("CFI-004: raw-only normalization changed canonical identity/time: got %+v, want %+v", got, want)
	}
}

func TestCollectorRebuildMovingArtifactsPreservesFacts(t *testing.T) {
	source, path := collectorRebuildSetup(t)
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	want := collectorRebuildIdentities(t, path)
	relocated := filepath.Join(t.TempDir(), "relocated")
	if err := os.Rename(source, relocated); err != nil {
		t.Fatal(err)
	}
	for _, move := range [][2]string{
		{"pi/project/main.jsonl", "pi/archive/renamed-pi.jsonl"},
		{"claude-code/project/main.jsonl", "claude-code/archive/renamed-claude.jsonl"},
		{"codex/rollout-parent.jsonl", "codex/archive/rollout-renamed-parent.jsonl"},
	} {
		target := filepath.Join(relocated, move[1])
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(filepath.Join(relocated, move[0]), target); err != nil {
			t.Fatal(err)
		}
	}
	collectorRebuildSync(t, path, relocated, collectorRebuildClock().Add(time.Hour))
	collectorRebuildAssertFacts(t, path)
	fresh := filepath.Join(t.TempDir(), "replacement.sqlite")
	collectorRebuildSync(t, fresh, relocated, collectorRebuildClock().Add(24*time.Hour))
	collectorRebuildAssertFacts(t, fresh)
	for _, check := range []string{path, fresh} {
		if got := collectorRebuildIdentities(t, check); !reflect.DeepEqual(got, want) {
			t.Errorf("CFI-005: moving artifacts changed canonical identities: got %+v, want %+v", got, want)
		}
	}
}

func TestCollectorRebuildCopiedArtifactsDoNotMultiplyUsage(t *testing.T) {
	source, path := collectorRebuildSetup(t)
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	want := collectorRebuildIdentities(t, path)
	// Codex copied parent files make ancestry ambiguous; that separate condition
	// must not be represented as verified replay. Its verified-fork case is CFI-001.
	for _, harness := range []string{"pi", "claude-code"} {
		copyFixtureDir(t, filepath.Join(source, harness, "project"), filepath.Join(source, harness, "archive"))
	}
	collectorRebuildSync(t, path, source, collectorRebuildClock().Add(time.Hour))
	collectorRebuildAssertFacts(t, path)
	fresh := filepath.Join(t.TempDir(), "replacement.sqlite")
	collectorRebuildSync(t, fresh, source, collectorRebuildClock().Add(24*time.Hour))
	collectorRebuildAssertFacts(t, fresh)
	if got := collectorRebuildIdentities(t, fresh); !reflect.DeepEqual(got, want) {
		t.Fatalf("CFI-006: copied artifacts changed identity: got %+v, want %+v", got, want)
	}
}

func TestCollectorRebuildOpenCodeEqualTimestampDistinctNativeRequests(t *testing.T) {
	source := t.TempDir()
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	createSQLiteFixtureDB(t, filepath.Join(source, "opencode", "opencode.db"), filepath.Join(collectorRebuildFixtureDir(), "stages", "opencode-equal-time.sql"))
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	// CFI-007: native IDs a and b identify independent requests even with equal counters,
	// timestamps, provider and model. V1/V2 copies of a still count only once.
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE harness = 'opencode'", 3)
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage WHERE harness = 'opencode'", 276)
}

func TestCollectorRebuildClaudeAppendMatchesFreshFinalParse(t *testing.T) {
	source := t.TempDir()
	artifact := filepath.Join(source, "claude-code", "project", "stream.jsonl")
	collectorRebuildCopyFile(t, filepath.Join(collectorRebuildFixtureDir(), "stages", "claude-partial.jsonl"), artifact)
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	collectorRebuildCopyFile(t, filepath.Join(collectorRebuildFixtureDir(), "stages", "claude-complete.jsonl"), artifact)
	collectorRebuildSync(t, path, source, collectorRebuildClock().Add(time.Hour))
	fresh := filepath.Join(t.TempDir(), "replacement.sqlite")
	collectorRebuildSync(t, fresh, source, collectorRebuildClock().Add(24*time.Hour))
	// Changed-input test: partial then complete must contribute exactly the same
	// one request as parsing the final source into an empty DB.
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 120)
	if got, want := collectorRebuildIdentities(t, path), collectorRebuildIdentities(t, fresh); !reflect.DeepEqual(got, want) {
		t.Fatalf("CFI-008: staged sync differs from fresh final parse: got %+v, want %+v", got, want)
	}
}

func TestCollectorRebuildPiMissingIDsRetainRawEvidence(t *testing.T) {
	source := t.TempDir()
	collectorRebuildCopyFile(t, filepath.Join(collectorRebuildFixtureDir(), "stages", "pi-missing-message-ids.jsonl"), filepath.Join(source, "pi", "project", "weak.jsonl"))
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	// CFI-009: retain both source observations and diagnose missing identity.
	// Different counters alone cannot prove independent requests versus revisions.
	// Canonical counting waits for an explicit weak-identity policy.
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 2)
	assertSQLCount(t, database, "SELECT EXISTS(SELECT 1 FROM normalization_diagnostics WHERE code = 'pi_jsonl_missing_message_id')", 1)
}

func collectorRebuildFixtureDir() string {
	return filepath.Join("..", "..", "testdata", "conformance", "collector-rebuild")
}
func collectorRebuildClock() time.Time { return time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC) }

func collectorRebuildSetup(t *testing.T) (string, string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source")
	copyFixtureDir(t, filepath.Join(collectorRebuildFixtureDir(), "source"), source)
	materializeOpenCodeSQLiteSource(t, source)
	return source, filepath.Join(t.TempDir(), "collector.sqlite")
}

func collectorRebuildSync(t *testing.T, path, source string, now time.Time) {
	t.Helper()
	if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: SupportedHarnesses, Normalize: true, Now: now}); err != nil {
		t.Fatal(err)
	}
}

func collectorRebuildCopyFile(t *testing.T, from, to string) {
	t.Helper()
	content, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func collectorRebuildAssertFacts(t *testing.T, path string) {
	t.Helper()
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	want := readExpectedJSON[[]expectedCanonicalTokenUsage](t, filepath.Join(collectorRebuildFixtureDir(), "expected", "canonical_facts.json"))
	got := queryCanonicalTokenUsage(t, database)
	if len(got) != len(want) {
		t.Fatalf("CFI-001: canonical count = %d, want %d; facts %+v", len(got), len(want), got)
	}
	for i, row := range got {
		if row.Harness == "codex" {
			// Codex reconstructs a snapshot hash suffix. The hand-authored oracle pins
			// source turn+time; complete observable IDs are compared across rebuilds.
			if !strings.HasPrefix(row.MessageID, want[i].MessageID) {
				t.Errorf("Codex native turn/time identity = %q, want prefix %q", row.MessageID, want[i].MessageID)
			}
			suffix := strings.TrimPrefix(row.MessageID, want[i].MessageID)
			if len(suffix) != 64 {
				t.Errorf("Codex snapshot identity suffix = %q, want SHA-256", suffix)
			}
			got[i].MessageID = want[i].MessageID
		}
	}
	assertEqualJSON(t, got, want)
	content, err := os.ReadFile(filepath.Join(collectorRebuildFixtureDir(), "expected", "totals.json"))
	if err != nil {
		t.Fatal(err)
	}
	var expected collectorRebuildTotals
	if err := json.Unmarshal(content, &expected); err != nil {
		t.Fatal(err)
	}
	var actual collectorRebuildTotals
	if err := database.QueryRow(`SELECT COUNT(*),SUM(input_tokens),SUM(output_tokens),SUM(reasoning_tokens),SUM(cache_read_tokens),SUM(cache_write_tokens),SUM(total_tokens) FROM canonical_token_usage WHERE is_countable = 1`).Scan(&actual.Facts, &actual.Input, &actual.Output, &actual.Reasoning, &actual.CacheRead, &actual.CacheWrite, &actual.Total); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("canonical totals = %+v, want %+v", actual, expected)
	}
}

type collectorRebuildTotals struct {
	Facts      int64 `json:"facts"`
	Input      int64 `json:"input_tokens"`
	Output     int64 `json:"output_tokens"`
	Reasoning  int64 `json:"reasoning_tokens"`
	CacheRead  int64 `json:"cache_read_tokens"`
	CacheWrite int64 `json:"cache_write_tokens"`
	Total      int64 `json:"total_tokens"`
}

type collectorRebuildIdentity struct {
	Harness, Session, Message, FactKey, SessionKey, MessageKey string
	RecordedAt                                                 int64
}

func collectorRebuildIdentities(t *testing.T, path string) []collectorRebuildIdentity {
	t.Helper()
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	return collectorRebuildQueryIdentities(t, database)
}

func collectorRebuildQueryIdentities(t *testing.T, database *sql.DB) []collectorRebuildIdentity {
	t.Helper()
	rows, err := database.Query(`SELECT u.harness,s.session_id,COALESCE(m.harness_message_id,''),u.semantic_key,s.semantic_key,COALESCE(m.semantic_key,''),u.recorded_at_ms FROM canonical_token_usage u JOIN canonical_sessions s ON s.id=u.session_id LEFT JOIN canonical_messages m ON m.id=u.message_id ORDER BY u.harness,s.session_id,m.harness_message_id,u.semantic_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	result := []collectorRebuildIdentity{}
	for rows.Next() {
		var identity collectorRebuildIdentity
		if err := rows.Scan(&identity.Harness, &identity.Session, &identity.Message, &identity.FactKey, &identity.SessionKey, &identity.MessageKey, &identity.RecordedAt); err != nil {
			t.Fatal(err)
		}
		if identity.FactKey == "" || identity.SessionKey == "" || identity.MessageKey == "" {
			t.Errorf("missing canonical identity: %+v", identity)
		}
		result = append(result, identity)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
