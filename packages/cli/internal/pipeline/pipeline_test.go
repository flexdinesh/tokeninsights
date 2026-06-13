package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"tokeninsights-cli/internal/db"

	_ "modernc.org/sqlite"
)

type expectedRawTokenUsage struct {
	Harness          string  `json:"harness"`
	SourceKind       string  `json:"source_kind"`
	SessionID        *string `json:"session_id"`
	MessageID        *string `json:"message_id"`
	Provider         *string `json:"provider"`
	Model            *string `json:"model"`
	UsageScope       string  `json:"usage_scope"`
	Quality          string  `json:"quality"`
	InputTokens      *int64  `json:"input_tokens"`
	OutputTokens     *int64  `json:"output_tokens"`
	ReasoningTokens  *int64  `json:"reasoning_tokens"`
	CacheReadTokens  *int64  `json:"cache_read_tokens"`
	CacheWriteTokens *int64  `json:"cache_write_tokens"`
	TotalTokens      *int64  `json:"total_tokens"`
}

type expectedCanonicalTokenUsage struct {
	Harness          string `json:"harness"`
	SessionID        string `json:"session_id"`
	MessageID        string `json:"message_id"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	UsageScope       string `json:"usage_scope"`
	Quality          string `json:"quality"`
	IsCountable      int64  `json:"is_countable"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	ReasoningTokens  int64  `json:"reasoning_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
}

type expectedRawObservation struct {
	Harness      string  `json:"harness"`
	SessionID    *string `json:"session_id"`
	MessageID    *string `json:"message_id"`
	ObservedAtMs int64   `json:"observed_at_ms"`
}

type expectedDiagnostic struct {
	Harness  string `json:"harness"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

func TestConformanceFixtureDefinesRawCanonicalAndDiagnostics(t *testing.T) {
	assertConformanceFixture(t, syncFirstBasicFixtureDir(), Summary{
		RequestedHarnesses: 3,
		Synced:             3,
		RawFacts:           3,
		Observations:       3,
		Canonical:          3,
	})
}

func TestConformanceFixtureDefinesMissingSessionDiagnostics(t *testing.T) {
	assertConformanceFixture(t, filepath.Join("testdata", "conformance", "missing-session"), Summary{
		RequestedHarnesses: 3,
		Synced:             1,
		Skipped:            2,
		RawFacts:           1,
		Observations:       1,
		Diagnostics:        1,
	})
}

func assertConformanceFixture(t *testing.T, fixtureDir string, wantSummary Summary) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := filepath.Join(t.TempDir(), "source")
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	copyFixtureDir(t, filepath.Join(fixtureDir, "source"), sourceDir)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: SupportedHarnesses,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, wantSummary)

	database := openTestDB(t, dbPath)
	defer database.Close()

	assertEqualJSON(t,
		queryRawTokenUsage(t, database),
		readExpectedJSON[[]expectedRawTokenUsage](t, filepath.Join(fixtureDir, "expected", "raw_token_usage.json")),
	)
	assertEqualJSON(t,
		queryRawObservations(t, database),
		readExpectedJSON[[]expectedRawObservation](t, filepath.Join(fixtureDir, "expected", "raw_observations.json")),
	)
	assertEqualJSON(t,
		queryCanonicalTokenUsage(t, database),
		readExpectedJSON[[]expectedCanonicalTokenUsage](t, filepath.Join(fixtureDir, "expected", "canonical_token_usage.json")),
	)
	assertEqualJSON(t,
		queryDiagnostics(t, database),
		readExpectedJSON[[]expectedDiagnostic](t, filepath.Join(fixtureDir, "expected", "normalization_diagnostics.json")),
	)
}

func assertSummary(t *testing.T, got Summary, want Summary) {
	t.Helper()
	if got.RequestedHarnesses != want.RequestedHarnesses ||
		got.Synced != want.Synced ||
		got.Skipped != want.Skipped ||
		got.Failed != want.Failed ||
		got.RawFacts != want.RawFacts ||
		got.Observations != want.Observations ||
		got.Canonical != want.Canonical ||
		got.Diagnostics != want.Diagnostics {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func syncFirstBasicFixtureDir() string {
	return filepath.Join("testdata", "conformance", "sync-first-basic")
}

func TestSyncAndNormalizeHarnessFixtures(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	copyFixtureDir(t, filepath.Join(syncFirstBasicFixtureDir(), "source"), sourceDir)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: SupportedHarnesses,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.RequestedHarnesses != 3 || summary.Synced != 3 || summary.RawFacts != 3 || summary.Observations != 3 || summary.Canonical != 3 || summary.Diagnostics != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 3)
	assertCount(t, database, "raw_observations", 3)
	assertCount(t, database, "canonical_sessions", 3)
	assertCount(t, database, "canonical_token_usage", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 1 AND diagnostic_count = 0", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE harness = 'pi' AND provider IS NULL AND model IS NULL", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE harness = 'pi' AND provider = 'unknown' AND model = 'unknown'", 1)

	secondSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: SupportedHarnesses,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if secondSummary.RawFacts != 0 || secondSummary.Observations != 3 || secondSummary.Canonical != 0 || secondSummary.Diagnostics != 0 {
		t.Fatalf("unexpected repeat summary: %+v", secondSummary)
	}
	assertCount(t, database, "raw_token_usage", 3)
	assertCount(t, database, "raw_observations", 6)
	assertCount(t, database, "canonical_token_usage", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 1 AND diagnostic_count = 0", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1 AND canonical_count = 0 AND diagnostic_count = 0", 3)

	normalizeSummary, err := Normalize(ctx, NormalizeOptions{DBPath: dbPath, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if normalizeSummary.Canonical != 0 || normalizeSummary.Diagnostics != 0 {
		t.Fatalf("unexpected normalize diagnostics: %+v", normalizeSummary)
	}
	assertCount(t, database, "canonical_token_usage", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 1 AND diagnostic_count = 0", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1 AND canonical_count = 0 AND diagnostic_count = 0", 3)
}

func TestSyncDryRunWritesNothing(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	writeJSONL(t, filepath.Join(sourceDir, "opencode", "usage.jsonl"),
		`{"recorded_at_ms":1770000000000,"session_id":"oc_s1","message_id":"m1","provider":"openai","model":"gpt-5","input_tokens":10,"output_tokens":5}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessOpenCode},
		DryRun:    true,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Synced != 1 || summary.RawFacts != 1 || summary.Observations != 0 || summary.Canonical != 0 {
		t.Fatalf("unexpected dry-run summary: %+v", summary)
	}
	if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run created db or returned unexpected stat error: %v", err)
	}
}

func TestSyncAllSourceDirUsesHarnessSubdirectoriesOnly(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "opencode", "usage.jsonl"),
		`{"recorded_at_ms":1770000000000,"session_id":"oc_s1","message_id":"m1","provider":"openai","model":"gpt-5","input_tokens":10,"output_tokens":5}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: SupportedHarnesses,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.RequestedHarnesses != 3 || summary.Synced != 1 || summary.Skipped != 2 || summary.RawFacts != 1 || summary.Observations != 1 || summary.Canonical != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE harness = 'opencode'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE harness IN ('pi', 'codex')", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE harness = 'opencode'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE harness IN ('pi', 'codex')", 0)
}

func TestSyncSingleHarnessSourceDirScansDirectoryDirectly(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "usage.jsonl"),
		`{"recorded_at_ms":1770000000000,"session_id":"oc_s1","message_id":"m1","provider":"openai","model":"gpt-5","input_tokens":10,"output_tokens":5}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessOpenCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.RequestedHarnesses != 1 || summary.Synced != 1 || summary.Skipped != 0 || summary.RawFacts != 1 || summary.Observations != 1 || summary.Canonical != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE harness = 'opencode'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE harness = 'opencode'", 1)
}

func TestMissingSessionWritesDiagnosticOnly(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "opencode", "usage.jsonl"),
		`{"recorded_at_ms":1770000000000,"message_id":"m1","provider":"openai","model":"gpt-5","input_tokens":10,"output_tokens":5}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessOpenCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.RawFacts != 1 || summary.Canonical != 0 || summary.Diagnostics != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "canonical_token_usage", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'missing_session'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 0 AND diagnostic_count = 1", 1)

	normalizeSummary, err := Normalize(ctx, NormalizeOptions{DBPath: dbPath, Now: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if normalizeSummary.Canonical != 0 || normalizeSummary.Diagnostics != 0 {
		t.Fatalf("unexpected repeat normalize summary: %+v", normalizeSummary)
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'missing_session'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 0 AND diagnostic_count = 1", 1)
}

func TestSyncAllPartialSuccessNormalizesSuccessfulHarnesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("broken symlink fixture requires Unix-style symlink behavior")
	}

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "opencode", "usage.jsonl"),
		`{"recorded_at_ms":1770000000000,"session_id":"oc_s1","message_id":"m1","provider":"openai","model":"gpt-5","input_tokens":10,"output_tokens":5}`,
	)
	if err := os.MkdirAll(filepath.Join(sourceDir, "pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(sourceDir, "pi", "missing-target.jsonl"), filepath.Join(sourceDir, "pi", "broken.jsonl")); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(sourceDir, "codex", "usage.jsonl"),
		`{"recorded_at_ms":1770000002000,"session_id":"cx_s1","message_id":"m1","provider":"openai","model":"gpt-5-mini","input_tokens":7,"output_tokens":8}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: SupportedHarnesses,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err == nil {
		t.Fatal("expected partial failure")
	}
	if summary.Failed != 1 || summary.Synced != 2 || summary.RawFacts != 2 || summary.Observations != 2 || summary.Canonical != 2 {
		t.Fatalf("unexpected partial summary: %+v", summary)
	}

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'failed'", 1)
	assertCount(t, database, "raw_token_usage", 2)
	assertCount(t, database, "canonical_token_usage", 2)
}

func TestSourceIngestWriteFailureRollsBackSource(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "opencode", "usage.jsonl"),
		`{"recorded_at_ms":1770000000000,"session_id":"oc_s1","message_id":"m1","provider":"openai","model":"gpt-5","input_tokens":10,"output_tokens":5}`,
		`{"recorded_at_ms":1770000001000,"session_id":"oc_s1","message_id":"m2","provider":"openai","model":"gpt-5","input_tokens":-1,"output_tokens":5}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessOpenCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err == nil {
		t.Fatal("expected source write failure")
	}
	if summary.Failed != 1 || summary.Synced != 0 || summary.RawFacts != 0 || summary.Observations != 0 || summary.Canonical != 0 || summary.Diagnostics != 0 {
		t.Fatalf("unexpected failed ingest summary: %+v", summary)
	}

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 0)
	assertCount(t, database, "raw_observations", 0)
	assertCount(t, database, "canonical_token_usage", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'failed' AND raw_fact_count = 0 AND observation_count = 0 AND diagnostic_count = 0 AND error_message IS NOT NULL", 1)
}

func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openTestDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()
	database, err := db.OpenWritable(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	return database
}

func assertCount(t *testing.T, database *sql.DB, table string, want int) {
	t.Helper()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM "+table, want)
}

func assertSQLCount(t *testing.T, database *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s: got %d, want %d", query, got, want)
	}
}

func copyFixtureDir(t *testing.T, source string, destination string) {
	t.Helper()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(source, entry.Name())
		destinationPath := filepath.Join(destination, entry.Name())
		if entry.IsDir() {
			copyFixtureDir(t, sourcePath, destinationPath)
			continue
		}
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(destinationPath, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readExpectedJSON[T comparableFixture](t *testing.T, path string) T {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result T
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

type comparableFixture interface {
	[]expectedRawTokenUsage | []expectedRawObservation | []expectedCanonicalTokenUsage | []expectedDiagnostic
}

func assertEqualJSON[T comparableFixture](t *testing.T, got T, want T) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	gotJSON, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("fixture mismatch\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
}

func queryRawTokenUsage(t *testing.T, database *sql.DB) []expectedRawTokenUsage {
	t.Helper()
	rows, err := database.Query(`
		SELECT harness, source_kind, session_id, message_id, provider, model, usage_scope, quality,
			input_tokens, output_tokens, reasoning_tokens, cache_read_tokens, cache_write_tokens, total_tokens
		FROM raw_token_usage
		ORDER BY harness, session_id, message_id
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	result := []expectedRawTokenUsage{}
	for rows.Next() {
		var row expectedRawTokenUsage
		var sessionID sql.NullString
		var messageID sql.NullString
		var provider sql.NullString
		var model sql.NullString
		var inputTokens sql.NullInt64
		var outputTokens sql.NullInt64
		var reasoningTokens sql.NullInt64
		var cacheReadTokens sql.NullInt64
		var cacheWriteTokens sql.NullInt64
		var totalTokens sql.NullInt64
		if err := rows.Scan(
			&row.Harness,
			&row.SourceKind,
			&sessionID,
			&messageID,
			&provider,
			&model,
			&row.UsageScope,
			&row.Quality,
			&inputTokens,
			&outputTokens,
			&reasoningTokens,
			&cacheReadTokens,
			&cacheWriteTokens,
			&totalTokens,
		); err != nil {
			t.Fatal(err)
		}
		row.SessionID = nullStringPointer(sessionID)
		row.MessageID = nullStringPointer(messageID)
		row.Provider = nullStringPointer(provider)
		row.Model = nullStringPointer(model)
		row.InputTokens = nullIntPointer(inputTokens)
		row.OutputTokens = nullIntPointer(outputTokens)
		row.ReasoningTokens = nullIntPointer(reasoningTokens)
		row.CacheReadTokens = nullIntPointer(cacheReadTokens)
		row.CacheWriteTokens = nullIntPointer(cacheWriteTokens)
		row.TotalTokens = nullIntPointer(totalTokens)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func queryCanonicalTokenUsage(t *testing.T, database *sql.DB) []expectedCanonicalTokenUsage {
	t.Helper()
	rows, err := database.Query(`
		SELECT ctu.harness, cs.session_id, COALESCE(cm.harness_message_id, ''), ctu.provider, ctu.model,
			ctu.usage_scope, ctu.quality, ctu.is_countable, ctu.input_tokens, ctu.output_tokens,
			ctu.reasoning_tokens, ctu.cache_read_tokens, ctu.cache_write_tokens, ctu.total_tokens
		FROM canonical_token_usage ctu
		INNER JOIN canonical_sessions cs ON cs.id = ctu.session_id
		LEFT JOIN canonical_messages cm ON cm.id = ctu.message_id
		ORDER BY ctu.harness, cs.session_id, cm.harness_message_id
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	result := []expectedCanonicalTokenUsage{}
	for rows.Next() {
		var row expectedCanonicalTokenUsage
		if err := rows.Scan(
			&row.Harness,
			&row.SessionID,
			&row.MessageID,
			&row.Provider,
			&row.Model,
			&row.UsageScope,
			&row.Quality,
			&row.IsCountable,
			&row.InputTokens,
			&row.OutputTokens,
			&row.ReasoningTokens,
			&row.CacheReadTokens,
			&row.CacheWriteTokens,
			&row.TotalTokens,
		); err != nil {
			t.Fatal(err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func queryRawObservations(t *testing.T, database *sql.DB) []expectedRawObservation {
	t.Helper()
	rows, err := database.Query(`
		SELECT r.harness, r.session_id, r.message_id, ro.observed_at_ms
		FROM raw_observations ro
		INNER JOIN raw_token_usage r ON r.id = ro.raw_fact_id
		ORDER BY r.harness, r.session_id, r.message_id, ro.observed_at_ms
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	result := []expectedRawObservation{}
	for rows.Next() {
		var row expectedRawObservation
		var sessionID sql.NullString
		var messageID sql.NullString
		if err := rows.Scan(&row.Harness, &sessionID, &messageID, &row.ObservedAtMs); err != nil {
			t.Fatal(err)
		}
		row.SessionID = nullStringPointer(sessionID)
		row.MessageID = nullStringPointer(messageID)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func queryDiagnostics(t *testing.T, database *sql.DB) []expectedDiagnostic {
	t.Helper()
	rows, err := database.Query(`
		SELECT harness, severity, code, message
		FROM normalization_diagnostics
		ORDER BY harness, code, message
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	result := []expectedDiagnostic{}
	for rows.Next() {
		var row expectedDiagnostic
		if err := rows.Scan(&row.Harness, &row.Severity, &row.Code, &row.Message); err != nil {
			t.Fatal(err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullIntPointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
