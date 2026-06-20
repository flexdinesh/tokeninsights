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

func insertSourceRefreshState(t *testing.T, database *sql.DB) {
	t.Helper()
	_, err := database.Exec(`
		INSERT INTO source_refresh_state (
			harness, source_kind, source_state_key, collector, parser,
			last_successful_refresh_at_ms, source_mtime_ms, source_size_bytes, updated_at_ms
		) VALUES ('opencode', 'opencode-sqlite', 'state-key', 'collector', 'parser', 1777042800000, 1776783600000, 1024, 1777042800000)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func assertDBCount(t *testing.T, database *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
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
	insertSourceRefreshState(t, database)

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
	var workCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM normalization_work_queue WHERE domain = ?", DomainTokenUsage).Scan(&workCount); err != nil {
		t.Fatal(err)
	}
	if workCount != 1 {
		t.Fatalf("got normalization work count %d, want 1", workCount)
	}
	var sourceStateCount int
	if err := database.QueryRow("SELECT COUNT(*) FROM source_refresh_state").Scan(&sourceStateCount); err != nil {
		t.Fatal(err)
	}
	if sourceStateCount != 1 {
		t.Fatalf("got source refresh state count %d, want 1", sourceStateCount)
	}
}

func TestResetAllClearsSourceRefreshStateAndNormalizationWork(t *testing.T) {
	database, dbPath := newTestDB(t)
	recordedAtMs := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, recordedAtMs, "opencode", "ses_1", "openai", "gpt", 100, 10, 5, 20, 1, 136)
	insertSourceRefreshState(t, database)
	if _, err := database.Exec(`
		INSERT OR IGNORE INTO normalization_work_queue (raw_fact_id, domain, enqueued_at_ms)
		SELECT id, ?, ? FROM raw_token_usage
	`, DomainTokenUsage, recordedAtMs); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if err := ResetAll(dbPath); err != nil {
		t.Fatal(err)
	}
	resetDB, err := OpenWritable(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resetDB.Close()
	assertDBCount(t, resetDB, "source_refresh_state", 0)
	assertDBCount(t, resetDB, "normalization_work_queue", 0)
	assertDBCount(t, resetDB, "raw_token_usage", 0)
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
	if rows[0].Harness != "opencode" || rows[0].InputTokens != 300 || rows[0].ContextUsedTokens != 232 || rows[0].TotalTokens != 394 {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
	if rows[1].Harness != "pi" || rows[1].ContextUsedTokens != 15 || rows[1].TotalTokens != 21 {
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
	if rows[0].Bucket != "2026-04-27" || rows[0].SessionCount != 1 || rows[0].ContextUsedTokens != 343 || rows[0].TotalTokens != 380 {
		t.Fatalf("unexpected first bucket: %+v", rows[0])
	}
	if rows[1].Bucket != "2026-04-20" || rows[1].SessionCount != 2 || rows[1].ContextUsedTokens != 232 || rows[1].TotalTokens != 394 {
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
	if rows[0].Model != "gpt-5" || rows[0].Providers != "azure, openai" || rows[0].Harnesses != "opencode, pi" || rows[0].SessionCount != 2 || rows[0].ContextUsedTokens != 232 || rows[0].TotalTokens != 394 {
		t.Fatalf("unexpected first model row: %+v", rows[0])
	}
	if rows[1].Model != "claude" || rows[1].Providers != "unknown" || rows[1].Harnesses != "codex" || rows[1].SessionCount != 1 {
		t.Fatalf("unexpected second model row: %+v", rows[1])
	}
}

func TestViewerContextAggregatesSessionPeakContextLoadByHarnessProviderModel(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, recordedAt, "codex", "ses_1", "openai", "gpt-5", 100, 10, 5, 20, 1, 136)
	insertCanonicalToken(t, database, recordedAt+1000, "codex", "ses_1", "openai", "gpt-5", 200, 20, 6, 30, 2, 258)
	insertCanonicalToken(t, database, recordedAt+2000, "codex", "ses_2", "openai", "gpt-5", 50, 5, 1, 5, 0, 61)
	insertCanonicalToken(t, database, recordedAt+3000, "codex", "ses_3", "openai", "gpt-5", 20, 2, 1, 0, 0, 23)
	insertCanonicalToken(t, database, recordedAt+4000, "codex", "ses_4", "openai", "gpt-5", 80, 8, 1, 0, 0, 89)
	insertCanonicalToken(t, database, recordedAt+5000, "opencode", "ses_1", "openai", "gpt-5", 500, 50, 10, 0, 0, 560)

	rows, err := ViewerContext(context.Background(), database, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	if rows[0].Harness != "opencode" || rows[0].Provider != "openai" || rows[0].Model != "gpt-5" || rows[0].SessionCount != 1 || rows[0].AverageContextUsedTokens != 500 || rows[0].MedianContextUsedTokens != 500 || rows[0].MaxContextUsedTokens != 500 {
		t.Fatalf("unexpected first context row: %+v", rows[0])
	}
	if rows[1].Harness != "codex" || rows[1].Provider != "openai" || rows[1].Model != "gpt-5" || rows[1].SessionCount != 4 || rows[1].AverageContextUsedTokens != 96 || rows[1].MedianContextUsedTokens != 67 || rows[1].MaxContextUsedTokens != 232 {
		t.Fatalf("unexpected second context row: %+v", rows[1])
	}
}

func TestViewerContextAppliesFiltersBeforeSessionPeakContextLoad(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	inRange := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local)
	outOfRange := time.Date(2026, 4, 23, 12, 0, 0, 0, time.Local)
	insertCanonicalToken(t, database, outOfRange.UnixMilli(), "codex", "ses_1", "openai", "gpt-5", 1000, 10, 5, 0, 0, 1015)
	insertCanonicalToken(t, database, inRange.UnixMilli(), "codex", "ses_1", "openai", "gpt-5", 100, 10, 5, 20, 1, 136)
	insertCanonicalToken(t, database, inRange.Add(time.Second).UnixMilli(), "opencode", "ses_2", "openai", "gpt-5", 500, 50, 10, 0, 0, 560)
	insertCanonicalToken(t, database, inRange.Add(2*time.Second).UnixMilli(), "codex", "ses_3", "anthropic", "claude", 300, 30, 7, 40, 3, 380)

	rows, err := ViewerContext(context.Background(), database, Filter{
		Start:     time.Date(2026, 4, 24, 0, 0, 0, 0, time.Local),
		End:       time.Date(2026, 4, 25, 0, 0, 0, 0, time.Local),
		Harnesses: []string{"codex"},
		Providers: []string{"openai"},
		Models:    []string{"gpt-5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].Harness != "codex" || rows[0].Provider != "openai" || rows[0].Model != "gpt-5" || rows[0].SessionCount != 1 || rows[0].MaxContextUsedTokens != 121 {
		t.Fatalf("unexpected filtered context row: %+v", rows[0])
	}
}

func TestViewerDimensionsExposeAllSummaryValues(t *testing.T) {
	database, _ := newTestDB(t)
	defer database.Close()
	recordedAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	insertCanonicalToken(t, database, recordedAt, "opencode", "ses_1", "openai", "gpt-5", 100, 10, 5, 20, 1, 136)
	insertCanonicalToken(t, database, recordedAt+1000, "pi", "ses_2", "azure", "gpt-5", 200, 20, 6, 30, 2, 258)
	insertCanonicalToken(t, database, recordedAt+2000, "codex", "ses_3", "anthropic", "gpt-5", 300, 30, 7, 40, 3, 380)

	rows, err := ViewerModels(context.Background(), database, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1: %+v", len(rows), rows)
	}
	if rows[0].Providers != "anthropic, azure, openai" || rows[0].Harnesses != "codex, opencode, pi" {
		t.Fatalf("unexpected summary values: %+v", rows[0])
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
	if rows[0].SessionID != "ses_1" || rows[0].Harness != "opencode" || rows[0].Providers != "anthropic, openai" || rows[0].Models != "claude, gpt-5" || rows[0].ContextUsedTokens != 232 || rows[0].TotalTokens != 394 {
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
