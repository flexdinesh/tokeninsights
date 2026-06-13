package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	if err := ResetAll(dbPath); err != nil {
		t.Fatal(err)
	}
	database, err := OpenWritable(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return database, dbPath
}

func insertCanonicalToken(t *testing.T, database *sql.DB, recordedAtMs int64, harness, sessionID, provider, model string, input, output, reasoning, cacheRead, cacheWrite, total int64) {
	t.Helper()
	sessionKey := harness + ":" + sessionID
	_, err := database.Exec(`
		INSERT INTO canonical_sessions (
			semantic_key, harness, session_id, first_seen_at_ms, last_seen_at_ms
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(semantic_key) DO UPDATE SET last_seen_at_ms = excluded.last_seen_at_ms
	`, sessionKey, harness, sessionID, recordedAtMs, recordedAtMs)
	if err != nil {
		t.Fatal(err)
	}
	var canonicalSessionID int64
	if err := database.QueryRow("SELECT id FROM canonical_sessions WHERE semantic_key = ?", sessionKey).Scan(&canonicalSessionID); err != nil {
		t.Fatal(err)
	}
	rawKey := fmt.Sprintf("%s:raw:%s:%s:%s:%d:%d:%d", sessionKey, time.UnixMilli(recordedAtMs), provider, model, input, output, total)
	_, err = database.Exec(`
		INSERT OR IGNORE INTO raw_token_usage (
			raw_fact_key, harness, source_id, source_kind, collector, parser, observed_at_ms,
			session_id, provider, model, usage_scope, quality, input_tokens, output_tokens,
			reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens
		) VALUES (?, ?, ?, 'test', 'test', 'test', ?, ?, ?, ?, 'message', 'exact', ?, ?, ?, ?, ?, ?)
	`, rawKey, harness, sessionID, recordedAtMs, sessionID, provider, model, input, output, reasoning, cacheRead, cacheWrite, total)
	if err != nil {
		t.Fatal(err)
	}
	var rawID int64
	if err := database.QueryRow("SELECT id FROM raw_token_usage WHERE raw_fact_key = ?", rawKey).Scan(&rawID); err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
		INSERT INTO canonical_token_usage (
			semantic_key, recorded_at_ms, harness, session_id, provider, model, usage_scope, quality,
			is_countable, input_tokens, output_tokens, reasoning_tokens, cache_read_tokens,
			cache_write_tokens, total_tokens, primary_raw_fact_id
		) VALUES (?, ?, ?, ?, ?, ?, 'message', 'exact', 1, ?, ?, ?, ?, ?, ?, ?)
	`, rawKey+":canonical", recordedAtMs, harness, canonicalSessionID, provider, model, input, output, reasoning, cacheRead, cacheWrite, total, rawID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCreateIfMissingCreatesCompatibleDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	database, created, err := CreateIfMissing(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if !created {
		t.Fatal("expected created=true")
	}

	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != SupportedSchemaVersion {
		t.Fatalf("got version %d, want %d", version, SupportedSchemaVersion)
	}
}

func TestOpenRejectsIncompatibleDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("CREATE TABLE old_table (id INTEGER PRIMARY KEY); PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = Open(dbPath)
	if err == nil {
		t.Fatal("expected schema version error")
	}
	if !strings.Contains(err.Error(), "reset-all --confirm") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResetCanonicalKeepsRawFacts(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	recordedAtMs := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, recordedAtMs, "opencode", "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)

	if err := ResetCanonical(context.Background(), database); err != nil {
		t.Fatal(err)
	}

	var rawCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM raw_token_usage").Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if rawCount != 1 {
		t.Fatalf("got raw count %d, want 1", rawCount)
	}
	var canonicalCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM canonical_token_usage").Scan(&canonicalCount); err != nil {
		t.Fatal(err)
	}
	if canonicalCount != 0 {
		t.Fatalf("got canonical count %d, want 0", canonicalCount)
	}
}

func TestAggregateCanonicalDaily(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	older := time.Date(2026, 4, 23, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, older, "pi", "ses_1", "anthropic", "claude", 10, 5, 1, 2, 3, 21)
	insertCanonicalToken(t, database, day, "opencode", "ses_2", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertCanonicalToken(t, database, day, "opencode", "ses_2", "openai", "gpt", 200, 20, 6, 30, 2, 258)

	rows, err := Aggregate(context.Background(), database, Filter{
		Start: time.Date(2026, 4, 23, 0, 0, 0, 0, time.Local),
	}, GroupByDay)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Harness != "opencode" || rows[0].InputTokens != 300 || rows[0].TotalTokens != 394 {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].Harness != "pi" || rows[1].TotalTokens != 21 {
		t.Fatalf("unexpected second row: %+v", rows[1])
	}
}

func TestViewerTokenBucketsAggregateByWeekWithSessionCounts(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	monday := time.Date(2026, 4, 20, 9, 0, 0, 0, time.Local).UnixMilli()
	friday := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	nextMonday := time.Date(2026, 4, 27, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, monday, "opencode", "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertCanonicalToken(t, database, friday, "pi", "ses_2", "anthropic", "claude", 200, 20, 6, 30, 2, 258)
	insertCanonicalToken(t, database, nextMonday, "codex", "ses_3", "unknown", "unknown", 300, 30, 7, 40, 3, 380)

	rows, err := ViewerTokenBuckets(context.Background(), database, Filter{}, BucketWeek)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[0].Bucket != "2026-04-27" || rows[0].SessionCount != 1 || rows[0].TotalTokens != 380 {
		t.Fatalf("unexpected first bucket: %+v", rows[0])
	}
	if rows[1].Bucket != "2026-04-20" || rows[1].SessionCount != 2 || rows[1].TotalTokens != 394 {
		t.Fatalf("unexpected second bucket: %+v", rows[1])
	}
}

func TestViewerModelsAggregateByModelOnly(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, recordedAt, "opencode", "ses_1", "openai", "gpt-5", 100, 10, 5, 20, 1, 136)
	insertCanonicalToken(t, database, recordedAt+1000, "pi", "ses_2", "azure", "gpt-5", 200, 20, 6, 30, 2, 258)
	insertCanonicalToken(t, database, recordedAt+2000, "codex", "ses_3", "unknown", "claude", 300, 30, 7, 40, 3, 380)

	rows, err := ViewerModels(context.Background(), database, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[0].Model != "gpt-5" || rows[0].Providers != "azure, openai" || rows[0].Harnesses != "opencode, pi" || rows[0].SessionCount != 2 || rows[0].TotalTokens != 394 {
		t.Fatalf("unexpected first model row: %+v", rows[0])
	}
	if rows[1].Model != "claude" || rows[1].Providers != "unknown" || rows[1].Harnesses != "codex" || rows[1].SessionCount != 1 {
		t.Fatalf("unexpected second model row: %+v", rows[1])
	}
}

func TestViewerSessionsAggregateByCanonicalSessionOnly(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, recordedAt, "opencode", "ses_1", "openai", "gpt-5", 100, 10, 5, 20, 1, 136)
	insertCanonicalToken(t, database, recordedAt+1000, "opencode", "ses_1", "anthropic", "claude", 200, 20, 6, 30, 2, 258)

	rows, err := ViewerSessions(context.Background(), database, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].SessionID != "ses_1" || rows[0].Harness != "opencode" || rows[0].Providers != "anthropic, openai" || rows[0].Models != "claude, gpt-5" || rows[0].TotalTokens != 394 {
		t.Fatalf("unexpected session row: %+v", rows[0])
	}
}

func TestAggregateNonTokenDomainsEmptyWithCanonicalTokens(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, day, "opencode", "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)

	aggregates := []struct {
		name string
		load func(context.Context, *sql.DB, Filter, GroupBy) ([]Row, error)
	}{
		{name: "tps", load: AggregateTPS},
		{name: "requests", load: AggregateRequests},
		{name: "tool calls", load: AggregateToolCalls},
		{name: "tool breakdown", load: AggregateToolBreakdown},
	}

	for _, aggregate := range aggregates {
		t.Run(aggregate.name, func(t *testing.T) {
			rows, err := aggregate.load(context.Background(), database, Filter{}, GroupByDay)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 0 {
				t.Fatalf("got %d rows, want 0: %+v", len(rows), rows)
			}
		})
	}
}

func TestAggregateCanonicalSessionAndFilters(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, day, "opencode", "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertCanonicalToken(t, database, day, "codex", "ses_2", "openai", "gpt", 200, 20, 10, 40, 2, 272)

	rows, err := Aggregate(context.Background(), database, Filter{Harnesses: []string{"codex"}}, GroupByDaySession)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Harness != "codex" || rows[0].SessionID != "ses_2" {
		t.Fatalf("unexpected rows: %+v", rows)
	}

	providers, err := AvailableProviders(context.Background(), database, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0] != "openai" {
		t.Fatalf("unexpected providers: %+v", providers)
	}

	harnesses, err := AvailableHarnesses(context.Background(), database, Filter{Providers: []string{"openai"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(harnesses, ",") != "codex,opencode" {
		t.Fatalf("unexpected harnesses: %+v", harnesses)
	}
}

func TestEventsFilterCanonicalRows(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	day := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, day, "opencode", "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertCanonicalToken(t, database, day, "pi", "ses_2", "anthropic", "claude", 200, 20, 6, 30, 2, 258)

	events, err := Events(context.Background(), database, Filter{Providers: []string{"openai"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Provider != "openai" {
		t.Fatalf("unexpected events: %+v", events)
	}
}
