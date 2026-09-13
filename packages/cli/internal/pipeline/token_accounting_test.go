package pipeline

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestOpenCodeSQLiteV1ToV2MigrationKeepsOneCanonicalMessage(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "opencode", "opencode.db")
	createOpenCodeSQLiteMessages(t, sourcePath, openCodeSQLiteMessage{
		ID: "row_message", SessionID: "session", TimeCreated: 1, TimeUpdated: 2,
		Data: `{"id":"embedded_message","role":"assistant","providerID":"anthropic","modelID":"model","tokens":{"input":80,"output":20},"time":{"created":1,"completed":2}}`,
	})
	options := SyncOptions{
		DBPath: dbPath, Harnesses: []Harness{HarnessOpenCode}, Normalize: true, SourceDir: sourceDir,
		Now: time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC),
	}
	if _, err := Sync(ctx, options); err != nil {
		t.Fatal(err)
	}

	database := openTestDB(t, dbPath)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE total_tokens = 100", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_messages WHERE harness_message_id = 'row_message'", 1)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	addOpenCodeSQLiteV2Messages(t, sourcePath, openCodeSQLiteV2Message{
		ID: "row_message", SessionID: "session", Type: "assistant", TimeCreated: 1, TimeUpdated: 3,
		Data: `{"model":{"providerID":"anthropic","id":"model"},"tokens":{"input":90,"output":30},"time":{"created":1,"completed":3}}`,
	})
	options.Now = options.Now.Add(time.Hour)
	if _, err := Sync(ctx, options); err != nil {
		t.Fatal(err)
	}

	database = openTestDB(t, dbPath)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE total_tokens = 120", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_messages WHERE harness_message_id = 'row_message'", 1)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	if err := db.ResetAll(dbPath); err != nil {
		t.Fatal(err)
	}
	options.Now = options.Now.Add(time.Hour)
	if _, err := Sync(ctx, options); err != nil {
		t.Fatal(err)
	}
	database = openTestDB(t, dbPath)
	defer func() { _ = database.Close() }()
	assertCount(t, database, "raw_token_usage", 1)
	assertCount(t, database, "canonical_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE total_tokens = 120", 1)
}

func TestPiJSONLNormalizesAdditiveTokenComponents(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	writeJSONL(t, filepath.Join(sourceDir, "pi", "project", "2026-01-01T00-00-00_pi_tokens.jsonl"),
		`{"type":"session","id":"pi_tokens"}`,
		`{"type":"message","id":"current","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","timestamp":1767225601000,"usage":{"input":70,"output":50,"reasoning":20,"cacheRead":25,"cacheWrite":5,"totalTokens":150}}}`,
		`{"type":"message","id":"legacy","timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","timestamp":1767225602000,"usage":{"input":100,"output":50,"reasoning":10,"cacheRead":25,"cacheWrite":5,"totalTokens":150}}}`,
		`{"type":"message","id":"bad-total","timestamp":"2026-01-01T00:00:03Z","message":{"role":"assistant","timestamp":1767225603000,"usage":{"input":10,"output":5,"cacheRead":0,"cacheWrite":0,"totalTokens":999}}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath: dbPath, Harnesses: []Harness{HarnessPi}, Normalize: true, SourceDir: sourceDir,
		Now: time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{RequestedHarnesses: 1, Synced: 1, RawFacts: 3, Observations: 3, Canonical: 3, Diagnostics: 2})

	database := openTestDB(t, dbPath)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE message_id = 'current' AND input_tokens = 70 AND output_tokens = 30 AND reasoning_tokens = 20 AND total_tokens = 150", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE message_id = 'legacy' AND input_tokens = 70 AND output_tokens = 40 AND reasoning_tokens = 10 AND total_tokens = 150", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE total_tokens = 150", 2)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE total_tokens = 15", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'pi_jsonl_inclusive_input_tokens'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'pi_jsonl_inconsistent_total'", 1)
}

func TestClaudeCodeJSONLMergesBeforeSplittingReasoning(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	writeJSONL(t, filepath.Join(sourceDir, "claude-code", "project", "claude_tokens.jsonl"),
		`{"type":"assistant","requestId":"request","timestamp":"2026-01-01T00:00:01Z","message":{"id":"message","role":"assistant","usage":{"input_tokens":100,"output_tokens":10,"cache_read_input_tokens":20,"cache_creation_input_tokens":5,"total_tokens":135}}}`,
		`{"type":"assistant","requestId":"request","timestamp":"2026-01-01T00:00:02Z","message":{"id":"message","role":"assistant","usage":{"input_tokens":100,"output_tokens":50,"output_tokens_details":{"thinking_tokens":20},"cache_read_input_tokens":20,"cache_creation_input_tokens":5,"total_tokens":175}}}`,
	)

	summary, err := Sync(ctx, SyncOptions{
		DBPath: dbPath, Harnesses: []Harness{HarnessClaudeCode}, Normalize: true, SourceDir: sourceDir,
		Now: time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertSummary(t, summary, Summary{RequestedHarnesses: 1, Synced: 1, RawFacts: 1, Observations: 1, Canonical: 1, Diagnostics: 1})

	database := openTestDB(t, dbPath)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE input_tokens = 100 AND output_tokens = 30 AND reasoning_tokens = 20 AND cache_read_tokens = 20 AND cache_write_tokens = 5 AND total_tokens = 175", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE total_tokens = 175", 1)
}

func TestJSONLAdaptersPreserveLargeIntegersAndRejectFractions(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)

	piPath := filepath.Join(t.TempDir(), "2026-01-01T00-00-00_pi_exact.jsonl")
	writeJSONL(t, piPath,
		`{"type":"session","id":"pi_exact"}`,
		`{"type":"message","id":"exact","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","timestamp":1767225601000,"usage":{"input":9007199254740993,"output":0,"totalTokens":9007199254740993}}}`,
		`{"type":"message","id":"fraction","timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","timestamp":1767225602000,"usage":{"input":1.5,"output":1,"totalTokens":2.5}}}`,
	)
	piFacts, piDiagnostics, err := (piJSONLAdapter{}).Parse(ctx, Source{Kind: piSessionJSONLSourceKind, Path: piPath}, SyncOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	assertSingleExactLargeFact(t, piFacts)
	assertDiagnosticCode(t, piDiagnostics, "pi_jsonl_invalid_tokens")

	claudePath := filepath.Join(t.TempDir(), "claude_exact.jsonl")
	writeJSONL(t, claudePath,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:01Z","message":{"id":"exact","role":"assistant","usage":{"input_tokens":9007199254740993,"output_tokens":0,"total_tokens":9007199254740993}}}`,
		`{"type":"assistant","timestamp":"2026-01-01T00:00:02Z","message":{"id":"fraction","role":"assistant","usage":{"input_tokens":1.5,"output_tokens":1,"total_tokens":2.5}}}`,
	)
	claudeFacts, claudeDiagnostics, err := (claudeCodeJSONLAdapter{}).Parse(ctx, Source{Kind: claudeCodeJSONLSourceKind, Path: claudePath}, SyncOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	assertSingleExactLargeFact(t, claudeFacts)
	assertDiagnosticCode(t, claudeDiagnostics, "claude_code_jsonl_invalid_tokens")
}

func TestOpenCodeSQLiteRejectsOverflowingComponentTotal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.db")
	createOpenCodeSQLiteMessages(t, path, openCodeSQLiteMessage{
		ID: "message", SessionID: "session", TimeCreated: 1, TimeUpdated: 1,
		Data: `{"role":"assistant","tokens":{"input":9223372036854775807,"output":1},"time":{"created":1}}`,
	})
	facts, diagnostics, err := (opencodeSQLiteAdapter{}).Parse(context.Background(), Source{Kind: opencodeSQLiteSourceKind, Path: path}, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Fatalf("got %d facts, want 0", len(facts))
	}
	assertDiagnosticCode(t, diagnostics, "opencode_sqlite_invalid_tokens")
}

func TestCodexTokenComponentsExcludeReasoningFromOutput(t *testing.T) {
	var usage map[string]interface{}
	if err := decodeJSONRecord(`{"input_tokens":100,"cached_input_tokens":20,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":150}`, &usage); err != nil {
		t.Fatal(err)
	}
	counts, diagnostics, ok := codexTokensFromUsage(usage)
	if !ok || len(diagnostics) != 0 {
		t.Fatalf("unexpected parse result: ok=%v diagnostics=%+v", ok, diagnostics)
	}
	assertTokenValue(t, counts.input, 80)
	assertTokenValue(t, counts.output, 40)
	assertTokenValue(t, counts.reasoning, 10)
	assertTokenValue(t, counts.cacheRead, 20)
	total, sumOK := tokenComponentSum(counts.input, counts.output, counts.reasoning, counts.cacheRead)
	if !sumOK || total != 150 {
		t.Fatalf("got additive total %d, want 150", total)
	}
}

func assertSingleExactLargeFact(t *testing.T, facts []RawTokenFact) {
	t.Helper()
	if len(facts) != 1 {
		t.Fatalf("got %d facts, want 1", len(facts))
	}
	assertTokenValue(t, facts[0].InputTokens, 9007199254740993)
	assertTokenValue(t, facts[0].TotalTokens, 9007199254740993)
}

func assertTokenValue(t *testing.T, value *int64, want int64) {
	t.Helper()
	if value == nil || *value != want {
		t.Fatalf("got token value %v, want %d", value, want)
	}
}

func assertDiagnosticCode(t *testing.T, diagnostics []Diagnostic, want string) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == want {
			return
		}
	}
	t.Fatalf("missing diagnostic %q in %+v", want, diagnostics)
}
