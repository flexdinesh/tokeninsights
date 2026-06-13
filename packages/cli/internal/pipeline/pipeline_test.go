package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"tokeninsights-cli/internal/db"

	_ "modernc.org/sqlite"
)

func TestSyncAndNormalizeHarnessFixtures(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)

	writeJSONL(t, filepath.Join(sourceDir, "opencode", "usage.jsonl"),
		`{"recorded_at_ms":1770000000000,"session_id":"oc_s1","message_id":"m1","provider":"openai","model":"gpt-5","usage":{"input_tokens":100,"output_tokens":20,"reasoning_tokens":5,"cache_read_tokens":7,"cache_write_tokens":3,"total_tokens":135}}`,
	)
	writeJSONL(t, filepath.Join(sourceDir, "pi", "usage.jsonl"),
		`{"recordedAtMs":1770000001000,"sessionId":"pi_s1","messageId":"turn1","usage":{"inputTokens":50,"outputTokens":10,"totalTokens":60}}`,
	)
	writeJSONL(t, filepath.Join(sourceDir, "codex", "usage.jsonl"),
		`{"timestamp_ms":1770000002000,"session":"cx_s1","messageID":"m1","provider":"openai","model":"gpt-5-mini","input":7,"output":8}`,
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
