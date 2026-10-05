package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func nativeGuardFixture(t *testing.T, name string) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "conformance", "collector-native-guards", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(body)), "\n")
}

func TestClaudeWeakIdentityCannotBlockNativeInAnyRecordOrder(t *testing.T) {
	records := nativeGuardFixture(t, "claude-weak-and-native.jsonl")
	var visit func([]string, []string)
	visit = func(prefix, remaining []string) {
		if len(remaining) == 0 {
			t.Run(fmt.Sprint(len(prefix), stableHash(strings.Join(prefix, "\n"))[:8]), func(t *testing.T) {
				source := t.TempDir()
				writeJSONL(t, filepath.Join(source, "claude-code", "fixture-session.jsonl"), prefix...)
				path := filepath.Join(t.TempDir(), "collector.sqlite")
				collectorRebuildSync(t, path, source, collectorRebuildClock())
				assertNativeGuardFacts(t, path, 4)
			})
			return
		}
		for i, record := range remaining {
			next := append([]string{}, remaining[:i]...)
			next = append(next, remaining[i+1:]...)
			visit(append(append([]string{}, prefix...), record), next)
		}
	}
	visit(nil, records)
}

func assertNativeGuardFacts(t *testing.T, path string, rawCount int) {
	t.Helper()
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", rawCount)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue", 0)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE first_seen_at_ms = 1767225601000 AND last_seen_at_ms = 1767225601000", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_messages WHERE occurred_at_ms = 1767225601000", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE input_tokens = 10 AND output_tokens = 8 AND reasoning_tokens = 2 AND total_tokens = 20", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal WHERE json_extract(payload_json,'$.totalTokens') = 20", 1)
	publicationFactHashes(t, database)
}

func TestClaudeWeakIdentityAcrossSyncsAndRetainedRaw(t *testing.T) {
	records := nativeGuardFixture(t, "claude-weak-and-native.jsonl")
	for _, nativeFirst := range []bool{false, true} {
		for _, rawOnly := range []bool{false, true} {
			t.Run(fmt.Sprint(nativeFirst, rawOnly), func(t *testing.T) {
				source := t.TempDir()
				artifact := filepath.Join(source, "claude-code", "fixture-session.jsonl")
				path := filepath.Join(t.TempDir(), "collector.sqlite")
				stages := [][]string{records[:3], records[3:]}
				if nativeFirst {
					stages[0], stages[1] = stages[1], stages[0]
				}
				for i, stage := range stages {
					writeJSONL(t, artifact, stage...)
					if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: []Harness{HarnessClaudeCode}, Normalize: !rawOnly, Now: collectorRebuildClock().Add(time.Duration(i) * time.Hour)}); err != nil {
						t.Fatal(err)
					}
				}
				if rawOnly {
					if err := os.RemoveAll(source); err != nil {
						t.Fatal(err)
					}
					if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path}); err != nil {
						t.Fatal(err)
					}
				}
				assertNativeGuardFacts(t, path, 4)
			})
		}
	}
}

func TestClaudeEquivalentOptionalCountersDoNotConflict(t *testing.T) {
	records := nativeGuardFixture(t, "claude-equivalent-counters.jsonl")
	for i, record := range records[1:] {
		for _, reverse := range []bool{false, true} {
			for _, separateSync := range []bool{false, true} {
				t.Run(fmt.Sprint(i, reverse, separateSync), func(t *testing.T) {
					source := t.TempDir()
					artifact := filepath.Join(source, "claude-code", "fixture.jsonl")
					path := filepath.Join(t.TempDir(), "collector.sqlite")
					ordered := []string{records[0], record}
					if reverse {
						ordered[0], ordered[1] = ordered[1], ordered[0]
					}
					if separateSync {
						writeJSONL(t, artifact, ordered[0])
						collectorRebuildSync(t, path, source, collectorRebuildClock())
						// Replacement forces normalization to compare independently
						// retained raw, rather than reusing the parser's merged row.
						writeJSONL(t, artifact, ordered[1])
					} else {
						writeJSONL(t, artifact, ordered...)
					}
					collectorRebuildSync(t, path, source, collectorRebuildClock().Add(time.Hour))
					database := openTestDB(t, path)
					defer func() { _ = database.Close() }()
					assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE input_tokens=10 AND output_tokens=5 AND reasoning_tokens=0 AND cache_read_tokens=0 AND cache_write_tokens=0 AND total_tokens=15", 1)
					assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 1)
					publicationFactHashes(t, database)
				})
			}
		}
	}
}

func TestPiWeakIdentityCannotWidenNativeEnvelopes(t *testing.T) {
	for _, nativeFirst := range []bool{false, true} {
		for _, separateSync := range []bool{false, true} {
			for _, rawOnly := range []bool{false, true} {
				t.Run(fmt.Sprint(nativeFirst, separateSync, rawOnly), func(t *testing.T) {
					source := t.TempDir()
					first, second := "a", "z"
					if nativeFirst {
						first, second = second, first
					}
					weakPath := filepath.Join(source, "pi", first, "date_fixture-session.jsonl")
					nativePath := filepath.Join(source, "pi", second, "date_fixture-session.jsonl")
					weak := []string{
						`{"type":"message","id":"fixture-message","timestamp":"2026-01-01T00:00:00Z","message":{"role":"assistant","usage":{"input":10,"output":6,"totalTokens":16}}}`,
						`{"type":"message","id":"fixture-message","timestamp":"2026-01-01T00:00:02Z","message":{"role":"assistant","usage":{"input":10,"output":7,"totalTokens":17}}}`,
						`{"type":"message","id":"fixture-message","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","usage":{"input":10,"output":8,"totalTokens":18}}}`,
					}
					path := filepath.Join(t.TempDir(), "collector.sqlite")
					writeStage := func(native bool) {
						if native {
							writeJSONL(t, nativePath, `{"type":"session","id":"fixture-session"}`, `{"type":"message","id":"fixture-message","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","usage":{"input":10,"output":5,"totalTokens":15}}}`)
						} else {
							writeJSONL(t, weakPath, weak...)
						}
					}
					sync := func() {
						if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: []Harness{HarnessPi}, Normalize: !rawOnly, Now: collectorRebuildClock()}); err != nil {
							t.Fatal(err)
						}
					}
					writeStage(nativeFirst)
					if separateSync {
						sync()
					}
					writeStage(!nativeFirst)
					sync()
					if rawOnly {
						if err := os.RemoveAll(source); err != nil {
							t.Fatal(err)
						}
						if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path}); err != nil {
							t.Fatal(err)
						}
					}
					database := openTestDB(t, path)
					defer func() { _ = database.Close() }()
					assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 4)
					assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE total_tokens=15", 1)
					assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE first_seen_at_ms=1767225601000 AND last_seen_at_ms=1767225601000", 1)
					assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_messages WHERE occurred_at_ms=1767225601000", 1)
					assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code='publication_ambiguous_session_identity'", 3)
					assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 1)
					assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue", 0)
					publicationFactHashes(t, database)
				})
			}
		}
	}
}

func TestConflictingWeakClaudeRowsDoNotBlockUnrelatedNative(t *testing.T) {
	source := t.TempDir()
	records := nativeGuardFixture(t, "claude-weak-and-native.jsonl")
	weakConflict := strings.Replace(records[1], `"output_tokens":7`, `"output_tokens":9`, 1)
	native := strings.Replace(records[3], `"sessionId":"fixture-session"`, `"sessionId":"unrelated-native-session"`, 1)
	writeJSONL(t, filepath.Join(source, "claude-code", "fixture-session.jsonl"), records[1], weakConflict, native)
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	assertNativeGuardFacts(t, path, 3)
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE session_id='unrelated-native-session'", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code='publication_ambiguous_session_identity'", 2)
}

func TestClaudeCanonicalComparisonPreservesInclusiveOutput(t *testing.T) {
	output, reasoning, input, total := int64(10), int64(2), int64(10), int64(20)
	fact := RawTokenFact{
		Harness: HarnessClaudeCode, SessionID: stringPointer("fixture-session"), MessageID: stringPointer("fixture-message"),
		OccurredAtMs: intPointer(1767225601000), InputTokens: &input, OutputTokens: &output, ReasoningTokens: &reasoning,
		Quality: "derived", UsageScope: "message", MetadataJSON: sourceIdentityJSON("native", stringPointer("fixture-request")),
	}
	withTotal := fact
	withTotal.TotalTokens = &total
	if !claudeCodeSameUsage(fact, withTotal) {
		t.Fatal("equivalent native snapshots must compare after canonical reasoning accounting")
	}
	changed := fact
	changed.ReasoningTokens = intPointer(1)
	if claudeCodeSameUsage(fact, changed) {
		t.Fatal("different component splits must conflict even when their totals match")
	}
	explicitProvider := fact
	explicitProvider.Provider = stringPointer("maybe-anthropic")
	if claudeCodeSameUsage(fact, explicitProvider) {
		t.Fatal("inferred and explicit provider evidence must remain distinct")
	}
	if *fact.OutputTokens != 10 || *withTotal.OutputTokens != 10 || fact.DedupeKey != "" || withTotal.DedupeKey != "" {
		t.Fatal("canonical comparison mutated parser snapshots")
	}
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			records := nativeGuardFixture(t, "claude-weak-and-native.jsonl")
			first := records[3]
			second := strings.Replace(first, `"output_tokens":10`, `"total_tokens":20,"output_tokens":10`, 1)
			ordered := []string{first, second}
			if reverse {
				ordered[0], ordered[1] = ordered[1], ordered[0]
			}
			source := t.TempDir()
			writeJSONL(t, filepath.Join(source, "claude-code", "fixture-session.jsonl"), ordered...)
			path := filepath.Join(t.TempDir(), "collector.sqlite")
			if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: []Harness{HarnessClaudeCode}, Now: collectorRebuildClock()}); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(source); err != nil {
				t.Fatal(err)
			}
			if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path}); err != nil {
				t.Fatal(err)
			}
			assertNativeGuardFacts(t, path, 1)
		})
	}
}

func TestClaudeCanonicalAttributionComparison(t *testing.T) {
	for _, same := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprint(same, reverse), func(t *testing.T) {
				first := strings.Replace(claudeIdentityRecord("fixture-session", "fixture-message", "fixture-request", "2026-01-01T00:00:01Z", 10, 5), `"model":"fixture-model"`, `"provider":"fireworks-ai","model":"accounts/fireworks/models/fixture-model"`, 1)
				second := strings.Replace(first, `"provider":"fireworks-ai","model":"accounts/fireworks/models/fixture-model"`, `"provider":"fireworks","model":"fixture-model"`, 1)
				if !same {
					second = strings.Replace(second, `"model":"fixture-model"`, `"model":"changed-model"`, 1)
				}
				ordered := []string{first, second}
				if reverse {
					ordered[0], ordered[1] = ordered[1], ordered[0]
				}
				artifact := filepath.Join(t.TempDir(), "fixture.jsonl")
				writeJSONL(t, artifact, ordered...)
				facts, _, err := (claudeCodeJSONLAdapter{}).Parse(context.Background(), Source{Harness: HarnessClaudeCode, Kind: claudeCodeJSONLSourceKind, Path: artifact}, SyncOptions{Now: collectorRebuildClock()})
				if same {
					if err != nil || len(facts) != 1 {
						t.Fatalf("equivalent attribution must merge: facts=%d error=%v", len(facts), err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "conflicting usage") {
					t.Fatalf("changed canonical attribution must conflict: %v", err)
				}
			})
		}
	}
}

func TestClaudeWeakRawReparseAndFreshRebuildPreservePublication(t *testing.T) {
	source := t.TempDir()
	records := nativeGuardFixture(t, "claude-weak-and-native.jsonl")
	writeJSONL(t, filepath.Join(source, "claude-code", "fixture-session.jsonl"), records...)
	path := filepath.Join(t.TempDir(), "collector.sqlite")
	collectorRebuildSync(t, path, source, collectorRebuildClock())
	database := openTestDB(t, path)
	want := publicationFactHashes(t, database)
	_ = database.Close()
	for iteration := range 100 {
		if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: []Harness{HarnessClaudeCode}, Normalize: true, FullRefresh: true, Now: collectorRebuildClock().Add(time.Duration(iteration+1) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	writeJSONL(t, filepath.Join(source, "claude-code", "copied", "fixture-session.jsonl"), records...)
	collectorRebuildSync(t, path, source, collectorRebuildClock().Add(200*time.Hour))
	fresh := filepath.Join(t.TempDir(), "replacement.sqlite")
	collectorRebuildSync(t, fresh, source, collectorRebuildClock().Add(300*time.Hour))
	for _, check := range []string{path, fresh} {
		assertNativeGuardFacts(t, check, 4)
		database := openTestDB(t, check)
		got := publicationFactHashes(t, database)
		_ = database.Close()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("copied/reparsed/rebuilt source changed publication: got %v, want %v", got, want)
		}
	}
}

func TestClaudeInvalidTimestampCannotReplaceValidRequest(t *testing.T) {
	for _, timestamp := range []string{"1969-12-31T23:59:59Z", "9999-12-31T00:00:00Z"} {
		for _, invalidFirst := range []bool{false, true} {
			t.Run(timestamp+fmt.Sprint(invalidFirst), func(t *testing.T) {
				valid := claudeIdentityRecord("fixture-session", "fixture-message", "fixture-request", "2026-01-01T00:00:01Z", 10, 5)
				invalid := claudeIdentityRecord("fixture-session", "fixture-message", "fixture-request", timestamp, 100, 500)
				records := []string{valid, invalid}
				if invalidFirst {
					records[0], records[1] = records[1], records[0]
				}
				source := t.TempDir()
				writeJSONL(t, filepath.Join(source, "claude-code", "fixture.jsonl"), records...)
				path := filepath.Join(t.TempDir(), "collector.sqlite")
				collectorRebuildSync(t, path, source, collectorRebuildClock())
				database := openTestDB(t, path)
				defer func() { _ = database.Close() }()
				assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE total_tokens=15", 1)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE first_seen_at_ms=1767225601000 AND last_seen_at_ms=1767225601000", 1)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code='claude_code_jsonl_invalid_time'", 1)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code='invalid_occurrence'", 1)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 2)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 1)
				publicationFactHashes(t, database)
			})
		}
	}
}

func TestNormalizeInvalidTimestampCannotPoisonNativeSession(t *testing.T) {
	for _, timestamp := range []int64{-1, 253402214400000} {
		for _, invalidFirst := range []bool{false, true} {
			t.Run(fmt.Sprint(timestamp, invalidFirst), func(t *testing.T) {
				source := t.TempDir()
				valid := `{"type":"message","id":"fixture-message","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","usage":{"input":10,"output":5,"totalTokens":15}}}`
				invalid := fmt.Sprintf(`{"type":"message","id":"fixture-message","message":{"role":"assistant","timestamp":%d,"usage":{"input":100,"output":500,"totalTokens":600}}}`, timestamp)
				records := []string{valid, invalid}
				if invalidFirst {
					records[0], records[1] = records[1], records[0]
				}
				writeJSONL(t, filepath.Join(source, "pi", "fixture.jsonl"), append([]string{`{"type":"session","id":"fixture-session"}`}, records...)...)
				path := filepath.Join(t.TempDir(), "collector.sqlite")
				if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: source, Harnesses: []Harness{HarnessPi}, Now: collectorRebuildClock()}); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(source); err != nil {
					t.Fatal(err)
				}
				if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path}); err != nil {
					t.Fatal(err)
				}
				database := openTestDB(t, path)
				defer func() { _ = database.Close() }()
				assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 2)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage WHERE total_tokens=15", 1)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE first_seen_at_ms=1767225601000 AND last_seen_at_ms=1767225601000", 1)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code='invalid_occurrence'", 1)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue", 0)
				publicationFactHashes(t, database)
			})
		}
	}
}
