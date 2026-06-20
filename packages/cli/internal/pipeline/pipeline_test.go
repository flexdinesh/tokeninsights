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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"

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
	ProviderSource   string `json:"provider_source"`
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
		RequestedHarnesses: 4,
		Synced:             4,
		RawFacts:           4,
		Observations:       4,
		Canonical:          4,
		Diagnostics:        1,
	})
}

func TestConformanceFixtureDefinesMissingSessionDiagnostics(t *testing.T) {
	assertConformanceFixture(t, filepath.Join("testdata", "conformance", "missing-session"), Summary{
		RequestedHarnesses: 4,
		Synced:             1,
		Skipped:            3,
		RawFacts:           1,
		Observations:       1,
		Diagnostics:        1,
	})
}

func TestSyncReportsHarnessProgress(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	var events []SyncProgressEvent

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: SupportedHarnesses,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		Progress: func(event SyncProgressEvent) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 4,
		Skipped:            4,
	})

	got := progressLabels(events)
	want := []string{
		"opencode:discovering",
		"opencode:skipped",
		"pi:discovering",
		"pi:skipped",
		"codex:discovering",
		"codex:skipped",
		"claude-code:discovering",
		"claude-code:skipped",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("progress events = %#v, want %#v", got, want)
	}
}

func TestSyncReportsSuccessfulHarnessAndNormalizationProgress(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := filepath.Join(t.TempDir(), "source")
	copyFixtureDir(t, filepath.Join(syncFirstBasicFixtureDir(), "source"), sourceDir)
	materializeOpenCodeSQLiteSource(t, sourceDir)
	var events []SyncProgressEvent

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: SupportedHarnesses,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC),
		Progress: func(event SyncProgressEvent) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 4,
		Synced:             4,
		RawFacts:           4,
		Observations:       4,
		Canonical:          4,
		Diagnostics:        1,
	})

	got := progressLabels(events)
	want := []string{
		"opencode:discovering",
		"opencode:syncing",
		"opencode:synced",
		"pi:discovering",
		"pi:syncing",
		"pi:synced",
		"codex:discovering",
		"codex:syncing",
		"codex:synced",
		"claude-code:discovering",
		"claude-code:syncing",
		"claude-code:synced",
		"normalizing",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("progress events = %#v, want %#v", got, want)
	}
}

func progressLabels(events []SyncProgressEvent) []string {
	labels := make([]string, 0, len(events))
	for _, event := range events {
		if event.Harness == "" {
			labels = append(labels, string(event.Status))
			continue
		}
		labels = append(labels, string(event.Harness)+":"+string(event.Status))
	}
	return labels
}

func TestOpenCodeSQLiteConformanceFixtureDefinesMessageTokenUsage(t *testing.T) {
	fixtureDir := filepath.Join("testdata", "conformance", "opencode-sqlite")
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := filepath.Join(t.TempDir(), "source")
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	createSQLiteFixtureDB(t,
		filepath.Join(sourceDir, "opencode", "opencode.db"),
		filepath.Join(fixtureDir, "source.sql"),
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
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           2,
		Observations:       2,
		Canonical:          2,
	})

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

func TestOpenCodeRecentSourceRefreshSkipsOldUnchangedSource(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "opencode", "opencode.db")
	createOpenCodeSQLiteMessages(t, sourcePath, openCodeSQLiteMessage{
		ID:          "m1",
		SessionID:   "oc_s1",
		TimeCreated: 1770000000000,
		TimeUpdated: 1770000000000,
		Data:        `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":100,"output":50},"time":{"created":1770000000000}}`,
	})
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

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
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state WHERE harness = 'opencode' AND source_kind = 'opencode-sqlite'", 1)

	repeatSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessOpenCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatSummary, Summary{
		RequestedHarnesses: 1,
		Skipped:            1,
	})
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 1)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 0", 1)
}

func TestOpenCodeRecentSourceRefreshParsesRecentTouchedAndMissingStateSources(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)

	t.Run("recent", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
		sourceDir := t.TempDir()
		sourcePath := writeOpenCodeSourceRefreshFixture(t, sourceDir)
		setFileModTime(t, sourcePath, now.Add(-24*time.Hour))

		syncOpenCodeSourceRefreshFixture(t, ctx, dbPath, sourceDir, now)
		repeatSummary, err := Sync(ctx, SyncOptions{
			DBPath:    dbPath,
			Harnesses: []Harness{HarnessOpenCode},
			Normalize: true,
			SourceDir: sourceDir,
			Now:       now.Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		assertSummary(t, repeatSummary, Summary{
			RequestedHarnesses: 1,
			Synced:             1,
			Observations:       1,
		})

		database := openTestDB(t, dbPath)
		defer database.Close()
		assertCount(t, database, "raw_token_usage", 1)
		assertCount(t, database, "raw_observations", 2)
		assertCount(t, database, "canonical_token_usage", 1)
		assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1", 1)
	})

	t.Run("touched", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
		sourceDir := t.TempDir()
		sourcePath := writeOpenCodeSourceRefreshFixture(t, sourceDir)
		setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

		syncOpenCodeSourceRefreshFixture(t, ctx, dbPath, sourceDir, now)
		setFileModTime(t, sourcePath, now.Add(30*time.Minute))
		repeatSummary, err := Sync(ctx, SyncOptions{
			DBPath:    dbPath,
			Harnesses: []Harness{HarnessOpenCode},
			Normalize: true,
			SourceDir: sourceDir,
			Now:       now.Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		assertSummary(t, repeatSummary, Summary{
			RequestedHarnesses: 1,
			Synced:             1,
			Observations:       1,
		})

		database := openTestDB(t, dbPath)
		defer database.Close()
		assertCount(t, database, "raw_token_usage", 1)
		assertCount(t, database, "raw_observations", 2)
		assertCount(t, database, "canonical_token_usage", 1)
		assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1", 1)
	})

	t.Run("missing-state", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
		sourceDir := t.TempDir()
		sourcePath := writeOpenCodeSourceRefreshFixture(t, sourceDir)
		setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

		syncOpenCodeSourceRefreshFixture(t, ctx, dbPath, sourceDir, now)
		database := openTestDB(t, dbPath)
		defer database.Close()
		if _, err := database.Exec("DELETE FROM source_refresh_state"); err != nil {
			t.Fatal(err)
		}

		repeatSummary, err := Sync(ctx, SyncOptions{
			DBPath:    dbPath,
			Harnesses: []Harness{HarnessOpenCode},
			Normalize: true,
			SourceDir: sourceDir,
			Now:       now.Add(time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		assertSummary(t, repeatSummary, Summary{
			RequestedHarnesses: 1,
			Synced:             1,
			Observations:       1,
		})
		assertCount(t, database, "raw_token_usage", 1)
		assertCount(t, database, "raw_observations", 2)
		assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state WHERE harness = 'opencode' AND source_kind = 'opencode-sqlite'", 1)
	})
}

func TestOpenCodeRecentSourceRefreshDryRunPreviewsSkipWithoutWriting(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := writeOpenCodeSourceRefreshFixture(t, sourceDir)
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	syncOpenCodeSourceRefreshFixture(t, ctx, dbPath, sourceDir, now)
	dryRunSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessOpenCode},
		DryRun:    true,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, dryRunSummary, Summary{
		RequestedHarnesses: 1,
		Skipped:            1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "ingest_runs", 1)
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state WHERE last_successful_refresh_at_ms = 1777042800000", 1)
}

func TestSyncFullRefreshIgnoresSourceRefreshStateWithoutRequeueingRawFacts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := writeOpenCodeSourceRefreshFixture(t, sourceDir)
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	syncOpenCodeSourceRefreshFixture(t, ctx, dbPath, sourceDir, now)
	skippedSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessOpenCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, skippedSummary, Summary{
		RequestedHarnesses: 1,
		Skipped:            1,
	})

	fullRefreshAt := now.Add(2 * time.Hour)
	fullRefreshSummary, err := Sync(ctx, SyncOptions{
		DBPath:      dbPath,
		Harnesses:   []Harness{HarnessOpenCode},
		FullRefresh: true,
		Normalize:   true,
		SourceDir:   sourceDir,
		Now:         fullRefreshAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, fullRefreshSummary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		Observations:       1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 2)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state WHERE last_successful_refresh_at_ms = "+strconv.FormatInt(fullRefreshAt.UnixMilli(), 10), 1)
}

func TestOpenCodeRecentSourceRefreshPreservesInvalidSchemaDiagnostics(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "opencode", "opencode.db")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("CREATE TABLE other_table (id text PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

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
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Skipped:            1,
		Diagnostics:        1,
	})

	tokenInsightsDB := openTestDB(t, dbPath)
	defer tokenInsightsDB.Close()
	assertSQLCount(t, tokenInsightsDB, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'opencode_sqlite_missing_message_table'", 1)
	assertSQLCount(t, tokenInsightsDB, "SELECT COUNT(*) FROM source_refresh_state WHERE harness = 'opencode' AND source_kind = 'opencode-sqlite'", 1)
}

func TestPiJSONLSyncsAssistantMessageTokenUsage(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl"),
		`{"type":"session","version":1,"id":"pi_s1","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/redacted/project"}`,
		`{"type":"model_change","id":"evt_model","parentId":null,"timestamp":"2026-01-01T00:00:01.000Z","provider":"anthropic","modelId":"claude-sonnet-4"}`,
		`{"type":"message","id":"msg_a","parentId":"evt_model","timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[],"provider":"anthropic","model":"claude-sonnet-4","usage":{"input":100,"output":50,"cacheRead":20,"cacheWrite":5,"totalTokens":175,"cost":{"total":0}},"stopReason":"stop","timestamp":1767225602000,"responseId":"redacted"}}`,
		`{"type":"message","id":"msg_user","parentId":"msg_a","timestamp":"2026-01-01T00:00:03.000Z","message":{"role":"user","content":[],"timestamp":1767225603000}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertEqualJSON(t, queryRawTokenUsage(t, database), []expectedRawTokenUsage{
		{
			Harness:          "pi",
			SourceKind:       "pi-session-jsonl",
			SessionID:        stringPointer("pi_s1"),
			MessageID:        stringPointer("msg_a"),
			Provider:         stringPointer("anthropic"),
			Model:            stringPointer("claude-sonnet-4"),
			UsageScope:       "message",
			Quality:          "exact",
			InputTokens:      intPointer(100),
			OutputTokens:     intPointer(50),
			ReasoningTokens:  nil,
			CacheReadTokens:  intPointer(20),
			CacheWriteTokens: intPointer(5),
			TotalTokens:      intPointer(175),
		},
	})
	assertEqualJSON(t, queryCanonicalTokenUsage(t, database), []expectedCanonicalTokenUsage{
		{
			Harness:          "pi",
			SessionID:        "pi_s1",
			MessageID:        "msg_a",
			Provider:         "anthropic",
			ProviderSource:   "explicit",
			Model:            "claude-sonnet-4",
			UsageScope:       "message",
			Quality:          "exact",
			IsCountable:      1,
			InputTokens:      100,
			OutputTokens:     50,
			ReasoningTokens:  0,
			CacheReadTokens:  20,
			CacheWriteTokens: 5,
			TotalTokens:      175,
		},
	})
}

func TestCodexJSONLSyncsTokenCountUsage(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "codex", "2026", "01", "01", "rollout-2026-01-01T00-00-00-codex_s1.jsonl"),
		`{"timestamp":"2026-01-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex_s1","timestamp":"2026-01-01T00:00:00.000Z","source":"cli","originator":"codex_cli_rs","model_provider":"openai","cwd":"/redacted/project"}}`,
		`{"timestamp":"2026-01-01T00:00:01.000Z","type":"turn_context","payload":{"turn_id":"turn_1","model":"gpt-5.5","cwd":"/redacted/project"}}`,
		`{"timestamp":"2026-01-01T00:00:02.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":150},"total_token_usage":{"input_tokens":100,"cached_input_tokens":20,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":150}}}}`,
		`{"timestamp":"2026-01-01T00:00:03.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":30,"cached_input_tokens":10,"output_tokens":5,"reasoning_output_tokens":0,"total_tokens":35},"total_token_usage":{"input_tokens":130,"cached_input_tokens":30,"output_tokens":55,"reasoning_output_tokens":10,"total_tokens":185}}}}`,
		`{"timestamp":"2026-01-01T00:00:04.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":30,"cached_input_tokens":10,"output_tokens":5,"reasoning_output_tokens":0,"total_tokens":35},"total_token_usage":{"input_tokens":130,"cached_input_tokens":30,"output_tokens":55,"reasoning_output_tokens":10,"total_tokens":185}}}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessCodex},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           2,
		Observations:       2,
		Canonical:          2,
		Diagnostics:        1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertEqualJSON(t, queryRawTokenUsage(t, database), []expectedRawTokenUsage{
		{
			Harness:          "codex",
			SourceKind:       "codex-session-jsonl",
			SessionID:        stringPointer("codex_s1"),
			MessageID:        stringPointer("turn_1:1767225602000"),
			Provider:         stringPointer("openai"),
			Model:            stringPointer("gpt-5.5"),
			UsageScope:       "message",
			Quality:          "exact",
			InputTokens:      intPointer(80),
			OutputTokens:     intPointer(50),
			ReasoningTokens:  intPointer(10),
			CacheReadTokens:  intPointer(20),
			CacheWriteTokens: nil,
			TotalTokens:      nil,
		},
		{
			Harness:          "codex",
			SourceKind:       "codex-session-jsonl",
			SessionID:        stringPointer("codex_s1"),
			MessageID:        stringPointer("turn_1:1767225603000"),
			Provider:         stringPointer("openai"),
			Model:            stringPointer("gpt-5.5"),
			UsageScope:       "message",
			Quality:          "exact",
			InputTokens:      intPointer(20),
			OutputTokens:     intPointer(5),
			ReasoningTokens:  intPointer(0),
			CacheReadTokens:  intPointer(10),
			CacheWriteTokens: nil,
			TotalTokens:      nil,
		},
	})
	assertEqualJSON(t, queryCanonicalTokenUsage(t, database), []expectedCanonicalTokenUsage{
		{
			Harness:          "codex",
			SessionID:        "codex_s1",
			MessageID:        "turn_1:1767225602000",
			Provider:         "openai",
			ProviderSource:   "explicit",
			Model:            "gpt-5.5",
			UsageScope:       "message",
			Quality:          "exact",
			IsCountable:      1,
			InputTokens:      80,
			OutputTokens:     50,
			ReasoningTokens:  10,
			CacheReadTokens:  20,
			CacheWriteTokens: 0,
			TotalTokens:      160,
		},
		{
			Harness:          "codex",
			SessionID:        "codex_s1",
			MessageID:        "turn_1:1767225603000",
			Provider:         "openai",
			ProviderSource:   "explicit",
			Model:            "gpt-5.5",
			UsageScope:       "message",
			Quality:          "exact",
			IsCountable:      1,
			InputTokens:      20,
			OutputTokens:     5,
			ReasoningTokens:  0,
			CacheReadTokens:  10,
			CacheWriteTokens: 0,
			TotalTokens:      35,
		},
	})
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'codex_jsonl_duplicate_token_snapshot'", 1)
}

func TestClaudeCodeJSONLDryRunDiscoversAndParsesMainSessionTokenUsage(t *testing.T) {
	ctx := context.Background()
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "claude-code", "project-a", "claude_main.jsonl"),
		`{"type":"assistant","uuid":"msg_a","requestId":"req_a","timestamp":"2026-01-01T00:00:02.000Z","message":{"id":"msg_api_a","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":20,"cache_creation_input_tokens":5}}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		Harnesses: []Harness{HarnessClaudeCode},
		DryRun:    true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Diagnostics:        1,
	})
}

func TestClaudeCodeJSONLSyncsMainSessionTokenUsage(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "claude-code", "project-a", "claude_main.jsonl"),
		`{"type":"assistant","uuid":"uuid_a","requestId":"req_a","timestamp":"2026-01-01T00:00:02.000Z","message":{"id":"msg_api_a","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":20,"cache_creation_input_tokens":5}}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessClaudeCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertEqualJSON(t, queryRawTokenUsage(t, database), []expectedRawTokenUsage{
		{
			Harness:          "claude-code",
			SourceKind:       "claude-code-session-jsonl",
			SessionID:        stringPointer("claude_main"),
			MessageID:        stringPointer("msg_api_a"),
			Provider:         nil,
			Model:            stringPointer("claude-sonnet-4-5"),
			UsageScope:       "message",
			Quality:          "derived",
			InputTokens:      intPointer(100),
			OutputTokens:     intPointer(50),
			ReasoningTokens:  nil,
			CacheReadTokens:  intPointer(20),
			CacheWriteTokens: intPointer(5),
			TotalTokens:      nil,
		},
	})
	assertEqualJSON(t, queryCanonicalTokenUsage(t, database), []expectedCanonicalTokenUsage{
		{
			Harness:          "claude-code",
			SessionID:        "claude_main",
			MessageID:        "msg_api_a",
			Provider:         "maybe-anthropic",
			ProviderSource:   "inferred",
			Model:            "claude-sonnet-4-5",
			UsageScope:       "message",
			Quality:          "derived",
			IsCountable:      1,
			InputTokens:      100,
			OutputTokens:     50,
			ReasoningTokens:  0,
			CacheReadTokens:  20,
			CacheWriteTokens: 5,
			TotalTokens:      175,
		},
	})
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'claude_code_jsonl_transcript_derived' AND severity = 'info'", 1)
}

func TestClaudeCodeJSONLMergesStreamingDuplicateAssistantUsage(t *testing.T) {
	ctx := context.Background()
	sourceDir := t.TempDir()
	path := filepath.Join(sourceDir, "project-a", "claude_main.jsonl")
	writeJSONL(t, path,
		`{"type":"assistant","uuid":"uuid_a","requestId":"req_a","timestamp":"2026-01-01T00:00:02.000Z","message":{"id":"msg_api_a","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":20,"cache_creation_input_tokens":5}}}`,
		`{"type":"assistant","uuid":"uuid_a","requestId":"req_a","timestamp":"2026-01-01T00:00:03.000Z","message":{"id":"msg_api_a","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":20,"cache_creation_input_tokens":5}}}`,
	)

	adapter := claudeCodeJSONLAdapter{}
	facts, _, err := adapter.Parse(ctx, Source{
		Harness: HarnessClaudeCode,
		ID:      "test-source",
		Kind:    claudeCodeJSONLSourceKind,
		Path:    path,
	}, SyncOptions{Now: time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("got %d facts, want 1", len(facts))
	}
	if facts[0].MessageID == nil || *facts[0].MessageID != "msg_api_a" {
		t.Fatalf("got message id %v, want msg_api_a", facts[0].MessageID)
	}
	if facts[0].OutputTokens == nil || *facts[0].OutputTokens != 50 {
		t.Fatalf("got output tokens %v, want 50", facts[0].OutputTokens)
	}
}

func TestClaudeCodeJSONLAttributesSidechainUsageToParentSession(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "claude-code", "project-a", "parent_session.jsonl"),
		`{"type":"assistant","uuid":"uuid_main","requestId":"req_main","timestamp":"2026-01-01T00:00:02.000Z","sessionId":"parent_session","message":{"id":"msg_main","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":10,"output_tokens":5}}}`,
	)
	writeJSONL(t, filepath.Join(sourceDir, "claude-code", "project-a", "agent-sidechain.jsonl"),
		`{"type":"assistant","uuid":"uuid_agent","requestId":"req_agent","timestamp":"2026-01-01T00:00:03.000Z","sessionId":"parent_session","isSidechain":true,"message":{"id":"msg_agent","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":20,"output_tokens":10}}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessClaudeCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           2,
		Observations:       2,
		Canonical:          2,
		Diagnostics:        2,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE session_id = 'parent_session'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE session_id = 'agent-sidechain'", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage ctu INNER JOIN canonical_sessions cs ON cs.id = ctu.session_id WHERE cs.session_id = 'parent_session'", 2)
}

func TestClaudeCodeJSONLSuppressesCopiedTranscriptFacts(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	line := `{"type":"assistant","uuid":"uuid_a","requestId":"req_a","timestamp":"2026-01-01T00:00:02.000Z","sessionId":"claude_copy","message":{"id":"msg_api_a","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":50}}}`
	writeJSONL(t, filepath.Join(sourceDir, "claude-code", "project-a", "claude_copy.jsonl"), line)
	writeJSONL(t, filepath.Join(sourceDir, "claude-code", "project-b", "claude_copy.jsonl"), line)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessClaudeCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        3,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'claude_code_jsonl_duplicate_suppressed'", 1)
}

func TestCodexJSONLBackfillsModelForPendingTokenCount(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "codex", "2026", "01", "01", "rollout-2026-01-01T00-00-00-codex_pending.jsonl"),
		`{"timestamp":"2026-01-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex_pending","timestamp":"2026-01-01T00:00:00.000Z","source":"cli","originator":"codex_cli_rs","model_provider":"openai"}}`,
		`{"timestamp":"2026-01-01T00:00:02.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":20,"cached_input_tokens":5,"output_tokens":7,"reasoning_output_tokens":1},"total_token_usage":{"input_tokens":20,"cached_input_tokens":5,"output_tokens":7,"reasoning_output_tokens":1}}}}`,
		`{"timestamp":"2026-01-01T00:00:03.000Z","type":"turn_context","payload":{"turn_id":"turn_pending","model":"gpt-5.5"}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessCodex},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE model = 'gpt-5.5' AND message_id = 'turn_pending:1767225602000'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'codex_jsonl_missing_model'", 0)
}

func TestCodexJSONLSkipsRegressedCumulativeSnapshot(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "codex", "2026", "01", "01", "rollout-2026-01-01T00-00-00-codex_regression.jsonl"),
		`{"timestamp":"2026-01-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex_regression","model_provider":"openai"}}`,
		`{"timestamp":"2026-01-01T00:00:01.000Z","type":"turn_context","payload":{"turn_id":"turn_1","model":"gpt-5.5"}}`,
		`{"timestamp":"2026-01-01T00:00:02.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"cached_input_tokens":10,"output_tokens":20,"reasoning_output_tokens":0},"total_token_usage":{"input_tokens":100,"cached_input_tokens":10,"output_tokens":20,"reasoning_output_tokens":0}}}}`,
		`{"timestamp":"2026-01-01T00:00:03.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":5,"cached_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":0},"total_token_usage":{"input_tokens":90,"cached_input_tokens":10,"output_tokens":18,"reasoning_output_tokens":0}}}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessCodex},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'codex_jsonl_stale_token_snapshot'", 1)
}

func TestPiJSONLUsesFilenameSessionFallbackWhenHeaderIsMissing(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_fallback.jsonl"),
		`{"type":"message","id":"msg_a","parentId":null,"timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[],"usage":{"input":10,"output":5,"cacheRead":0,"cacheWrite":0,"totalTokens":15},"timestamp":1767225602000}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE session_id = 'pi_fallback'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE provider = 'unknown' AND model = 'unknown'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'pi_jsonl_missing_session_header'", 1)
}

func TestPiJSONLPrefersHeaderSessionWhenFilenameDiffers(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writePiAssistantSession(t, filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_filename_session.jsonl"), "header_session", "msg_a", 10, 5)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE session_id = 'header_session'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE session_id = 'filename_session'", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'pi_jsonl_session_id_mismatch'", 1)
}

func TestPiJSONLDiscoveryIgnoresNestedSessionFilesBelowWorkspace(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writePiAssistantSession(t, filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl"), "pi_s1", "msg_a", 10, 5)
	writePiAssistantSession(t, filepath.Join(sourceDir, "pi", "project", "nested", "2026-01-01T00-00-00_pi_s2.jsonl"), "pi_s2", "msg_b", 20, 10)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE session_id = 'pi_s1'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE session_id = 'pi_s2'", 0)
}

func TestPiJSONLCopiedSessionFilesDedupeByLogicalSessionSource(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writePiAssistantSession(t, filepath.Join(sourceDir, "pi", "project-a", "2026-01-01T00-00-00_pi_s1.jsonl"), "pi_s1", "msg_a", 10, 5)
	writePiAssistantSession(t, filepath.Join(sourceDir, "pi", "project-b", "2026-01-01T00-00-00_pi_s1.jsonl"), "pi_s1", "msg_a", 10, 5)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       2,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 2)
	assertCount(t, database, "canonical_token_usage", 1)
}

func TestPiJSONLEmitsDiagnosticsForInvalidRowsAndIngestsUsableWarnings(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writeJSONL(t, filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl"),
		`{"type":"session","version":1,"id":"pi_s1","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/redacted/project"}`,
		`{malformed}`,
		`{"type":"message","parentId":null,"timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"assistant","content":[],"provider":"anthropic","model":"claude-sonnet-4","usage":{"input":-10,"output":5,"cacheRead":0,"cacheWrite":0,"totalTokens":-5}}}`,
		`{"type":"message","id":"msg_invalid_tokens","parentId":null,"timestamp":"2026-01-01T00:00:02.000Z","message":{"role":"assistant","content":[],"usage":{"input":"bad","output":5},"timestamp":1770000002000}}`,
		`{"type":"message","id":"msg_missing_time","parentId":null,"timestamp":"not-a-time","message":{"role":"assistant","content":[],"usage":{"input":10,"output":5,"totalTokens":15}}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        5,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE input_tokens = 0 AND output_tokens = 5 AND total_tokens = 0", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'pi_jsonl_parse_error'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'pi_jsonl_missing_message_id'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'pi_jsonl_negative_tokens'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'pi_jsonl_invalid_tokens'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'pi_jsonl_missing_time'", 1)
}

func TestOpenCodeSQLiteSuppressesDuplicateChannelRows(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	messageData := `{"id":"assistant_a","role":"assistant","providerID":"anthropic","modelID":"claude-sonnet-4","tokens":{"input":100,"output":50,"reasoning":10,"cache":{"read":20,"write":5}},"time":{"created":1700000000000,"completed":1700000000450}}`
	createOpenCodeSQLiteMessages(t, filepath.Join(sourceDir, "opencode", "opencode.db"), openCodeSQLiteMessage{
		ID:          "msg_a",
		SessionID:   "ses_a",
		TimeCreated: 1700000000000,
		TimeUpdated: 1700000000450,
		Data:        messageData,
	})
	createOpenCodeSQLiteMessages(t, filepath.Join(sourceDir, "opencode", "opencode-stable.db"), openCodeSQLiteMessage{
		ID:          "msg_a_copy",
		SessionID:   "ses_fork",
		TimeCreated: 1700000000000,
		TimeUpdated: 1700000000450,
		Data:        messageData,
	})

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
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'opencode_sqlite_duplicate_suppressed'", 1)
}

func TestOpenCodeSQLiteClampsNegativeTokenComponents(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	createOpenCodeSQLiteMessages(t, filepath.Join(sourceDir, "opencode", "opencode.db"), openCodeSQLiteMessage{
		ID:          "msg_negative",
		SessionID:   "ses_a",
		TimeCreated: 1700000000000,
		TimeUpdated: 1700000000450,
		Data:        `{"role":"assistant","providerID":"anthropic","modelID":"claude-sonnet-4","tokens":{"input":-100,"output":50,"reasoning":-10,"cache":{"read":-20,"write":5}},"time":{"created":1700000000000,"completed":1700000000450}}`,
	})

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
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE input_tokens = 0 AND output_tokens = 50 AND reasoning_tokens = 0 AND cache_read_tokens = 0 AND cache_write_tokens = 5", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE input_tokens = 0 AND output_tokens = 50 AND reasoning_tokens = 0 AND cache_read_tokens = 0 AND cache_write_tokens = 5 AND total_tokens = 55", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'opencode_sqlite_negative_tokens'", 1)
}

func TestSyncWithWallClockNowCompletesIngestRun(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	createOpenCodeSQLiteMessages(t, filepath.Join(sourceDir, "opencode", "opencode.db"), openCodeSQLiteMessage{
		ID:          "m1",
		SessionID:   "oc_s1",
		TimeCreated: 1770000000000,
		TimeUpdated: 1770000000000,
		Data:        `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":10,"output":5},"time":{"created":1770000000000}}`,
	})

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessOpenCode},
		Normalize: true,
		SourceDir: sourceDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND completed_at_ms >= started_at_ms", 1)
}

func TestOpenCodeSQLiteSyncAcceptsRelativeSourceDir(t *testing.T) {
	ctx := context.Background()
	workingDir := t.TempDir()
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.Chdir(workingDir); err != nil {
		t.Fatal(err)
	}
	createOpenCodeSQLiteMessages(t, filepath.Join("source", "opencode", "opencode.db"), openCodeSQLiteMessage{
		ID:          "m1",
		SessionID:   "oc_s1",
		TimeCreated: 1770000000000,
		TimeUpdated: 1770000000000,
		Data:        `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":10,"output":5},"time":{"created":1770000000000}}`,
	})

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    "tokeninsights.sqlite",
		Harnesses: []Harness{HarnessOpenCode},
		Normalize: true,
		SourceDir: "source",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})
}

func assertConformanceFixture(t *testing.T, fixtureDir string, wantSummary Summary) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := filepath.Join(t.TempDir(), "source")
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	copyFixtureDir(t, filepath.Join(fixtureDir, "source"), sourceDir)
	materializeOpenCodeSQLiteSource(t, sourceDir)

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
	materializeOpenCodeSQLiteSource(t, sourceDir)

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
	if summary.RequestedHarnesses != 4 || summary.Synced != 4 || summary.RawFacts != 4 || summary.Observations != 4 || summary.Canonical != 4 || summary.Diagnostics != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 4)
	assertCount(t, database, "raw_observations", 4)
	assertCount(t, database, "canonical_sessions", 4)
	assertCount(t, database, "canonical_token_usage", 4)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 1 AND diagnostic_count = 0", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 1 AND diagnostic_count = 1", 1)
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
	if secondSummary.RawFacts != 0 || secondSummary.Observations != 4 || secondSummary.Canonical != 0 || secondSummary.Diagnostics != 1 {
		t.Fatalf("unexpected repeat summary: %+v", secondSummary)
	}
	assertCount(t, database, "raw_token_usage", 4)
	assertCount(t, database, "raw_observations", 8)
	assertCount(t, database, "canonical_token_usage", 4)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 1 AND diagnostic_count = 0", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 1 AND diagnostic_count = 1", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1 AND canonical_count = 0 AND diagnostic_count = 0", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1 AND canonical_count = 0 AND diagnostic_count = 1", 1)

	normalizeSummary, err := Normalize(ctx, NormalizeOptions{DBPath: dbPath, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if normalizeSummary.Canonical != 0 || normalizeSummary.Diagnostics != 0 {
		t.Fatalf("unexpected normalize diagnostics: %+v", normalizeSummary)
	}
	assertCount(t, database, "canonical_token_usage", 4)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 1 AND diagnostic_count = 0", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 1 AND observation_count = 1 AND canonical_count = 1 AND diagnostic_count = 1", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1 AND canonical_count = 0 AND diagnostic_count = 0", 3)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1 AND canonical_count = 0 AND diagnostic_count = 1", 1)
}

func TestSyncNoNormalizeLeavesPendingTokenUsageWorkForNormalize(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writePiAssistantSession(t, filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl"), "pi_s1", "msg_a", 100, 50)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: false,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "canonical_token_usage", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 1)

	repeatSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: false,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatSummary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		Observations:       1,
	})
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 1)

	dryRunSummary, err := Normalize(ctx, NormalizeOptions{DBPath: dbPath, DryRun: true, Now: now.Add(2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, dryRunSummary, Summary{Canonical: 1})
	assertCount(t, database, "canonical_token_usage", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 1)

	normalizeSummary, err := Normalize(ctx, NormalizeOptions{DBPath: dbPath, Now: now.Add(3 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, normalizeSummary, Summary{Canonical: 1})
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 0)

	repeatNormalizeSummary, err := Normalize(ctx, NormalizeOptions{DBPath: dbPath, Now: now.Add(4 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatNormalizeSummary, Summary{})
}

func TestPiRecentSourceRefreshSkipsOldUnchangedSource(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl")
	writePiAssistantSession(t, sourcePath, "pi_s1", "msg_a", 100, 50)
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state WHERE harness = 'pi' AND source_kind = 'pi-session-jsonl'", 1)

	repeatSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatSummary, Summary{
		RequestedHarnesses: 1,
		Skipped:            1,
	})
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 1)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 0", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 0)
}

func TestPiRecentSourceRefreshParsesRecentUnchangedSource(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl")
	writePiAssistantSession(t, sourcePath, "pi_s1", "msg_a", 100, 50)
	setFileModTime(t, sourcePath, now.Add(-24*time.Hour))

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	repeatSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatSummary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		Observations:       1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 2)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 0)
}

func TestPiRecentSourceRefreshParsesTouchedOldSource(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl")
	writePiAssistantSession(t, sourcePath, "pi_s1", "msg_a", 100, 50)
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})
	setFileModTime(t, sourcePath, now.Add(30*time.Minute))

	repeatSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatSummary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		Observations:       1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 2)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1", 1)
}

func TestPiRecentSourceRefreshParsesOldSourceWhenStateIsMissing(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl")
	writePiAssistantSession(t, sourcePath, "pi_s1", "msg_a", 100, 50)
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	if _, err := database.Exec("DELETE FROM source_refresh_state"); err != nil {
		t.Fatal(err)
	}

	repeatSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatSummary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		Observations:       1,
	})
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 2)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state", 1)
}

func TestPiRecentSourceRefreshDryRunPreviewsSkipWithoutWriting(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl")
	writePiAssistantSession(t, sourcePath, "pi_s1", "msg_a", 100, 50)
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	dryRunSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		DryRun:    true,
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, dryRunSummary, Summary{
		RequestedHarnesses: 1,
		Skipped:            1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "ingest_runs", 1)
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state WHERE last_successful_refresh_at_ms = 1777042800000", 1)
}

func TestPiRecentSourceRefreshNormalizesPendingWorkWhenSourceIsSkipped(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl")
	writePiAssistantSession(t, sourcePath, "pi_s1", "msg_a", 100, 50)
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: false,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
	})

	repeatSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatSummary, Summary{
		RequestedHarnesses: 1,
		Skipped:            1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 0", 1)
}

func TestCodexRecentSourceRefreshSkipsOldUnchangedSource(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "codex", "2026", "01", "01", "rollout-2026-01-01T00-00-00-codex_s1.jsonl")
	writeCodexTokenSession(t, sourcePath, "codex_s1", "turn_1", "gpt-5.5", 100, 50)
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessCodex},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state WHERE harness = 'codex' AND source_kind = 'codex-session-jsonl'", 1)

	repeatSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessCodex},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatSummary, Summary{
		RequestedHarnesses: 1,
		Skipped:            1,
	})
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 1)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 0", 1)
}

func TestClaudeCodeRecentSourceRefreshSkipsOldUnchangedSource(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	sourcePath := filepath.Join(sourceDir, "claude-code", "project-a", "claude_main.jsonl")
	writeClaudeCodeAssistantSession(t, sourcePath, "claude_main", "msg_a", 100, 50)
	setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessClaudeCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state WHERE harness = 'claude-code' AND source_kind = 'claude-code-session-jsonl'", 1)

	repeatSummary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessClaudeCode},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, repeatSummary, Summary{
		RequestedHarnesses: 1,
		Skipped:            1,
	})
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "raw_observations", 1)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 0", 1)
}

func TestJSONLRecentSourceRefreshParsesRecentTouchedAndMissingStateSources(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	cases := []jsonlRecentSourceRefreshCase{
		codexRecentSourceRefreshCase(),
		claudeCodeRecentSourceRefreshCase(),
	}

	for _, testCase := range cases {
		t.Run(testCase.name+"/recent", func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
			sourceDir := t.TempDir()
			sourcePath := testCase.sourcePath(sourceDir)
			testCase.writeSource(t, sourcePath)
			setFileModTime(t, sourcePath, now.Add(-24*time.Hour))

			syncJSONLRecentSourceRefreshFixture(t, ctx, dbPath, sourceDir, testCase, now)
			repeatSummary, err := Sync(ctx, SyncOptions{
				DBPath:    dbPath,
				Harnesses: []Harness{testCase.harness},
				Normalize: true,
				SourceDir: sourceDir,
				Now:       now.Add(time.Hour),
			})
			if err != nil {
				t.Fatal(err)
			}
			assertSummary(t, repeatSummary, Summary{
				RequestedHarnesses: 1,
				Synced:             1,
				Observations:       1,
				Diagnostics:        testCase.parseDiagnostics,
			})

			database := openTestDB(t, dbPath)
			defer database.Close()
			assertCount(t, database, "raw_token_usage", 1)
			assertCount(t, database, "raw_observations", 2)
			assertCount(t, database, "canonical_token_usage", 1)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1", 1)
		})

		t.Run(testCase.name+"/touched", func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
			sourceDir := t.TempDir()
			sourcePath := testCase.sourcePath(sourceDir)
			testCase.writeSource(t, sourcePath)
			setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

			syncJSONLRecentSourceRefreshFixture(t, ctx, dbPath, sourceDir, testCase, now)
			setFileModTime(t, sourcePath, now.Add(30*time.Minute))
			repeatSummary, err := Sync(ctx, SyncOptions{
				DBPath:    dbPath,
				Harnesses: []Harness{testCase.harness},
				Normalize: true,
				SourceDir: sourceDir,
				Now:       now.Add(time.Hour),
			})
			if err != nil {
				t.Fatal(err)
			}
			assertSummary(t, repeatSummary, Summary{
				RequestedHarnesses: 1,
				Synced:             1,
				Observations:       1,
				Diagnostics:        testCase.parseDiagnostics,
			})

			database := openTestDB(t, dbPath)
			defer database.Close()
			assertCount(t, database, "raw_token_usage", 1)
			assertCount(t, database, "raw_observations", 2)
			assertCount(t, database, "canonical_token_usage", 1)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'completed' AND raw_fact_count = 0 AND observation_count = 1", 1)
		})

		t.Run(testCase.name+"/missing-state", func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
			sourceDir := t.TempDir()
			sourcePath := testCase.sourcePath(sourceDir)
			testCase.writeSource(t, sourcePath)
			setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

			syncJSONLRecentSourceRefreshFixture(t, ctx, dbPath, sourceDir, testCase, now)
			database := openTestDB(t, dbPath)
			defer database.Close()
			if _, err := database.Exec("DELETE FROM source_refresh_state"); err != nil {
				t.Fatal(err)
			}

			repeatSummary, err := Sync(ctx, SyncOptions{
				DBPath:    dbPath,
				Harnesses: []Harness{testCase.harness},
				Normalize: true,
				SourceDir: sourceDir,
				Now:       now.Add(time.Hour),
			})
			if err != nil {
				t.Fatal(err)
			}
			assertSummary(t, repeatSummary, Summary{
				RequestedHarnesses: 1,
				Synced:             1,
				Observations:       1,
				Diagnostics:        testCase.parseDiagnostics,
			})
			assertCount(t, database, "raw_token_usage", 1)
			assertCount(t, database, "raw_observations", 2)
			assertSQLCount(t, database, testCase.stateCountSQL, 1)
		})
	}
}

func TestJSONLRecentSourceRefreshDryRunPreviewsSkipWithoutWriting(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	cases := []jsonlRecentSourceRefreshCase{
		codexRecentSourceRefreshCase(),
		claudeCodeRecentSourceRefreshCase(),
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
			sourceDir := t.TempDir()
			sourcePath := testCase.sourcePath(sourceDir)
			testCase.writeSource(t, sourcePath)
			setFileModTime(t, sourcePath, now.Add(-72*time.Hour))

			syncJSONLRecentSourceRefreshFixture(t, ctx, dbPath, sourceDir, testCase, now)
			dryRunSummary, err := Sync(ctx, SyncOptions{
				DBPath:    dbPath,
				Harnesses: []Harness{testCase.harness},
				DryRun:    true,
				Normalize: true,
				SourceDir: sourceDir,
				Now:       now.Add(time.Hour),
			})
			if err != nil {
				t.Fatal(err)
			}
			assertSummary(t, dryRunSummary, Summary{
				RequestedHarnesses: 1,
				Skipped:            1,
			})

			database := openTestDB(t, dbPath)
			defer database.Close()
			assertCount(t, database, "ingest_runs", 1)
			assertCount(t, database, "raw_token_usage", 1)
			assertCount(t, database, "raw_observations", 1)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM source_refresh_state WHERE last_successful_refresh_at_ms = 1777042800000", 1)
		})
	}
}

func TestResetCanonicalRequeuesRawTokenUsageForNormalize(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	writePiAssistantSession(t, filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_s1.jsonl"), "pi_s1", "msg_a", 100, 50)

	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{HarnessPi},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 0)

	if err := db.ResetCanonical(ctx, database); err != nil {
		t.Fatal(err)
	}
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "canonical_token_usage", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 1)

	normalizeSummary, err := Normalize(ctx, NormalizeOptions{DBPath: dbPath, Now: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, normalizeSummary, Summary{Canonical: 1})
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 0)
}

func TestSyncDryRunWritesNothing(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	createOpenCodeSQLiteMessages(t, filepath.Join(sourceDir, "opencode", "opencode.db"), openCodeSQLiteMessage{
		ID:          "m1",
		SessionID:   "oc_s1",
		TimeCreated: 1770000000000,
		TimeUpdated: 1770000000000,
		Data:        `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":10,"output":5},"time":{"created":1770000000000}}`,
	})

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
	createOpenCodeSQLiteMessages(t, filepath.Join(sourceDir, "opencode", "opencode.db"), openCodeSQLiteMessage{
		ID:          "m1",
		SessionID:   "oc_s1",
		TimeCreated: 1770000000000,
		TimeUpdated: 1770000000000,
		Data:        `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":10,"output":5},"time":{"created":1770000000000}}`,
	})

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
	if summary.RequestedHarnesses != 4 || summary.Synced != 1 || summary.Skipped != 3 || summary.RawFacts != 1 || summary.Observations != 1 || summary.Canonical != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}

	database := openTestDB(t, dbPath)
	defer database.Close()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE harness = 'opencode'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE harness IN ('pi', 'codex', 'claude-code')", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE harness = 'opencode'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE harness IN ('pi', 'codex', 'claude-code')", 0)
}

func TestSyncSingleHarnessSourceDirScansDirectoryDirectly(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	createOpenCodeSQLiteMessages(t, filepath.Join(sourceDir, "opencode.db"), openCodeSQLiteMessage{
		ID:          "m1",
		SessionID:   "oc_s1",
		TimeCreated: 1770000000000,
		TimeUpdated: 1770000000000,
		Data:        `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":10,"output":5},"time":{"created":1770000000000}}`,
	})

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
	createOpenCodeSQLiteMessages(t, filepath.Join(sourceDir, "opencode", "opencode.db"), openCodeSQLiteMessage{
		ID:          "m1",
		SessionID:   "",
		TimeCreated: 1770000000000,
		TimeUpdated: 1770000000000,
		Data:        `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":10,"output":5},"time":{"created":1770000000000}}`,
	})

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
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 0)
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
	createOpenCodeSQLiteMessages(t, filepath.Join(sourceDir, "opencode", "opencode.db"), openCodeSQLiteMessage{
		ID:          "m1",
		SessionID:   "oc_s1",
		TimeCreated: 1770000000000,
		TimeUpdated: 1770000000000,
		Data:        `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":10,"output":5},"time":{"created":1770000000000}}`,
	})
	if err := os.MkdirAll(filepath.Join(sourceDir, "pi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(sourceDir, "pi", "missing-target.jsonl"), filepath.Join(sourceDir, "pi", "broken.jsonl")); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(sourceDir, "codex", "2026", "01", "01", "rollout-2026-01-01T00-00-00-cx_s1.jsonl"),
		`{"timestamp":"2026-01-01T00:00:00.000Z","type":"session_meta","payload":{"id":"cx_s1","model_provider":"openai"}}`,
		`{"timestamp":"2026-01-01T00:00:01.000Z","type":"turn_context","payload":{"turn_id":"turn1","model":"gpt-5-mini"}}`,
		`{"timestamp":"2026-01-01T00:00:02.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":7,"cached_input_tokens":0,"output_tokens":8,"reasoning_output_tokens":0},"total_token_usage":{"input_tokens":7,"cached_input_tokens":0,"output_tokens":8,"reasoning_output_tokens":0}}}}`,
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
	if summary.Failed != 1 || summary.Synced != 2 || summary.Skipped != 1 || summary.RawFacts != 2 || summary.Observations != 2 || summary.Canonical != 2 {
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
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	database, _, err := db.CreateIfMissing(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	occurredAt := int64(1770000000000)
	sessionID := "rollback_s1"
	provider := "openai"
	model := "gpt-5"
	validMessageID := "m1"
	invalidMessageID := "m2"
	validInput := int64(10)
	invalidInput := int64(-1)
	output := int64(5)
	source := Source{
		Harness: HarnessCodex,
		ID:      "rollback-source",
		Kind:    "rollback-source-kind",
		Path:    "rollback-source-path",
	}
	adapter := failingWriteAdapter{facts: []RawTokenFact{
		{
			Harness:      HarnessCodex,
			SourceID:     "rollback-source",
			SourceKind:   "rollback-source-kind",
			Collector:    defaultCollector,
			Parser:       defaultParser,
			ObservedAtMs: now.UnixMilli(),
			OccurredAtMs: &occurredAt,
			SessionID:    &sessionID,
			MessageID:    &validMessageID,
			Provider:     &provider,
			Model:        &model,
			UsageScope:   "message",
			Quality:      "exact",
			InputTokens:  &validInput,
			OutputTokens: &output,
		},
		{
			Harness:      HarnessCodex,
			SourceID:     "rollback-source",
			SourceKind:   "rollback-source-kind",
			Collector:    defaultCollector,
			Parser:       defaultParser,
			ObservedAtMs: now.UnixMilli(),
			OccurredAtMs: &occurredAt,
			SessionID:    &sessionID,
			MessageID:    &invalidMessageID,
			Provider:     &provider,
			Model:        &model,
			UsageScope:   "message",
			Quality:      "exact",
			InputTokens:  &invalidInput,
			OutputTokens: &output,
		},
	}}

	summary, err := ingestSource(ctx, database, adapter, SyncOptions{
		Collector: defaultCollector,
		Parser:    defaultParser,
		Now:       now,
	}, HarnessCodex, source, map[string]bool{})
	if err == nil {
		t.Fatal("expected source write failure")
	}
	if summary.RawFacts != 0 || summary.Observations != 0 || summary.Canonical != 0 || summary.Diagnostics != 0 {
		t.Fatalf("unexpected failed ingest summary: %+v", summary)
	}
	assertCount(t, database, "raw_token_usage", 0)
	assertCount(t, database, "raw_observations", 0)
	assertCount(t, database, "canonical_token_usage", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM ingest_runs WHERE status = 'failed' AND raw_fact_count = 0 AND observation_count = 0 AND diagnostic_count = 0 AND error_message IS NOT NULL", 1)
}

type openCodeSQLiteMessage struct {
	ID          string
	SessionID   string
	TimeCreated int64
	TimeUpdated int64
	Data        string
}

type failingWriteAdapter struct {
	facts []RawTokenFact
}

type jsonlRecentSourceRefreshCase struct {
	name             string
	harness          Harness
	sourcePath       func(string) string
	writeSource      func(*testing.T, string)
	parseDiagnostics int
	stateCountSQL    string
}

func (adapter failingWriteAdapter) Harness() Harness {
	return HarnessCodex
}

func (adapter failingWriteAdapter) Discover(context.Context, DiscoverOptions) ([]Source, error) {
	return nil, nil
}

func (adapter failingWriteAdapter) Parse(context.Context, Source, SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	return adapter.facts, nil, nil
}

func codexRecentSourceRefreshCase() jsonlRecentSourceRefreshCase {
	return jsonlRecentSourceRefreshCase{
		name:    "codex",
		harness: HarnessCodex,
		sourcePath: func(root string) string {
			return filepath.Join(root, "codex", "2026", "01", "01", "rollout-2026-01-01T00-00-00-codex_s1.jsonl")
		},
		writeSource: func(t *testing.T, path string) {
			t.Helper()
			writeCodexTokenSession(t, path, "codex_s1", "turn_1", "gpt-5.5", 100, 50)
		},
		stateCountSQL: "SELECT COUNT(*) FROM source_refresh_state WHERE harness = 'codex' AND source_kind = 'codex-session-jsonl'",
	}
}

func claudeCodeRecentSourceRefreshCase() jsonlRecentSourceRefreshCase {
	return jsonlRecentSourceRefreshCase{
		name:    "claude-code",
		harness: HarnessClaudeCode,
		sourcePath: func(root string) string {
			return filepath.Join(root, "claude-code", "project-a", "claude_main.jsonl")
		},
		writeSource: func(t *testing.T, path string) {
			t.Helper()
			writeClaudeCodeAssistantSession(t, path, "claude_main", "msg_a", 100, 50)
		},
		parseDiagnostics: 1,
		stateCountSQL:    "SELECT COUNT(*) FROM source_refresh_state WHERE harness = 'claude-code' AND source_kind = 'claude-code-session-jsonl'",
	}
}

func syncJSONLRecentSourceRefreshFixture(t *testing.T, ctx context.Context, dbPath string, sourceDir string, testCase jsonlRecentSourceRefreshCase, now time.Time) {
	t.Helper()
	summary, err := Sync(ctx, SyncOptions{
		DBPath:    dbPath,
		Harnesses: []Harness{testCase.harness},
		Normalize: true,
		SourceDir: sourceDir,
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
		Diagnostics:        testCase.parseDiagnostics,
	})
}

func writeOpenCodeSourceRefreshFixture(t *testing.T, sourceDir string) string {
	t.Helper()
	sourcePath := filepath.Join(sourceDir, "opencode", "opencode.db")
	createOpenCodeSQLiteMessages(t, sourcePath, openCodeSQLiteMessage{
		ID:          "m1",
		SessionID:   "oc_s1",
		TimeCreated: 1770000000000,
		TimeUpdated: 1770000000000,
		Data:        `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":100,"output":50},"time":{"created":1770000000000}}`,
	})
	return sourcePath
}

func syncOpenCodeSourceRefreshFixture(t *testing.T, ctx context.Context, dbPath string, sourceDir string, now time.Time) {
	t.Helper()
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
	assertSummary(t, summary, Summary{
		RequestedHarnesses: 1,
		Synced:             1,
		RawFacts:           1,
		Observations:       1,
		Canonical:          1,
	})
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

func writePiAssistantSession(t *testing.T, path string, sessionID string, messageID string, inputTokens int64, outputTokens int64) {
	t.Helper()
	writeJSONL(t, path,
		`{"type":"session","version":1,"id":"`+sessionID+`","timestamp":"2026-01-01T00:00:00.000Z","cwd":"/redacted/project"}`,
		`{"type":"message","id":"`+messageID+`","parentId":null,"timestamp":"2026-01-01T00:00:01.000Z","message":{"role":"assistant","content":[],"provider":"anthropic","model":"claude-sonnet-4","usage":{"input":`+intString(inputTokens)+`,"output":`+intString(outputTokens)+`,"cacheRead":0,"cacheWrite":0,"totalTokens":`+intString(inputTokens+outputTokens)+`},"timestamp":1770000001000}}`,
	)
}

func writeCodexTokenSession(t *testing.T, path string, sessionID string, turnID string, model string, inputTokens int64, outputTokens int64) {
	t.Helper()
	writeJSONL(t, path,
		`{"timestamp":"2026-01-01T00:00:00.000Z","type":"session_meta","payload":{"id":"`+sessionID+`","model_provider":"openai"}}`,
		`{"timestamp":"2026-01-01T00:00:01.000Z","type":"turn_context","payload":{"turn_id":"`+turnID+`","model":"`+model+`"}}`,
		`{"timestamp":"2026-01-01T00:00:02.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":`+intString(inputTokens)+`,"cached_input_tokens":0,"output_tokens":`+intString(outputTokens)+`,"reasoning_output_tokens":0},"total_token_usage":{"input_tokens":`+intString(inputTokens)+`,"cached_input_tokens":0,"output_tokens":`+intString(outputTokens)+`,"reasoning_output_tokens":0}}}}`,
	)
}

func writeClaudeCodeAssistantSession(t *testing.T, path string, sessionID string, messageID string, inputTokens int64, outputTokens int64) {
	t.Helper()
	writeJSONL(t, path,
		`{"type":"assistant","uuid":"`+messageID+`","requestId":"req_`+messageID+`","timestamp":"2026-01-01T00:00:02.000Z","sessionId":"`+sessionID+`","message":{"id":"`+messageID+`","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":`+intString(inputTokens)+`,"output_tokens":`+intString(outputTokens)+`}}}`,
	)
}

func setFileModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func createSQLiteFixtureDB(t *testing.T, dbPath string, sqlPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	statements, err := os.ReadFile(sqlPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(string(statements)); err != nil {
		t.Fatal(err)
	}
}

func materializeOpenCodeSQLiteSource(t *testing.T, sourceDir string) {
	t.Helper()
	sqlPath := filepath.Join(sourceDir, "opencode", "source.sql")
	if _, err := os.Stat(sqlPath); errors.Is(err, os.ErrNotExist) {
		return
	} else if err != nil {
		t.Fatal(err)
	}
	createSQLiteFixtureDB(t, filepath.Join(sourceDir, "opencode", "opencode.db"), sqlPath)
}

func createOpenCodeSQLiteMessages(t *testing.T, dbPath string, messages ...openCodeSQLiteMessage) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec(`
		CREATE TABLE message (
			id text PRIMARY KEY,
			session_id text NOT NULL,
			time_created integer NOT NULL,
			time_updated integer NOT NULL,
			data text NOT NULL
		)
	`); err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if _, err := database.Exec(`
			INSERT INTO message (id, session_id, time_created, time_updated, data)
			VALUES (?, ?, ?, ?, ?)
		`, message.ID, message.SessionID, message.TimeCreated, message.TimeUpdated, message.Data); err != nil {
			t.Fatal(err)
		}
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
		SELECT ctu.harness, cs.session_id, COALESCE(cm.harness_message_id, ''), ctu.provider, ctu.provider_source, ctu.model,
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
			&row.ProviderSource,
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

func stringPointer(value string) *string {
	return &value
}

func intPointer(value int64) *int64 {
	return &value
}

func intString(value int64) string {
	return strconv.FormatInt(value, 10)
}
