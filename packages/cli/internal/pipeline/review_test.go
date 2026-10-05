package pipeline

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeIdentitySeparatorsDoNotCollideInLocalCanonicalBridge(t *testing.T) {
	for _, separator := range []string{"|", ":"} {
		t.Run(separator, func(t *testing.T) {
			source := t.TempDir()
			firstSession := "rebuild-a" + separator + "rebuild-b"
			secondMessage := "rebuild-b" + separator + "rebuild-c"
			writePiAssistantSession(t, filepath.Join(source, "pi", "first.jsonl"), firstSession, "rebuild-c", 10, 5)
			writePiAssistantSession(t, filepath.Join(source, "pi", "second.jsonl"), "rebuild-a", secondMessage, 20, 7)
			path := filepath.Join(t.TempDir(), "collector.sqlite")
			collectorRebuildSync(t, path, source, collectorRebuildClock())
			database := openTestDB(t, path)
			defer func() { _ = database.Close() }()
			assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 2)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 2)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_messages", 2)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 2)
			assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 42)
		})
	}
}

func TestFilenameDerivedSessionsRemainLocalAfterArtifactCopies(t *testing.T) {
	for _, harness := range []string{"pi", "claude-code"} {
		t.Run(harness, func(t *testing.T) {
			source := t.TempDir()
			artifact := filepath.Join(source, harness, "project", "date_rebuild-first.jsonl")
			line := `{"type":"message","id":"rebuild-message","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","provider":"fixture-provider","model":"fixture-model","usage":{"input":10,"output":5,"totalTokens":15}}}`
			if harness == "claude-code" {
				line = `{"type":"assistant","requestId":"rebuild-request","timestamp":"2026-01-01T00:00:01Z","message":{"id":"rebuild-message","role":"assistant","model":"fixture-model","usage":{"input_tokens":10,"output_tokens":5}}}`
			}
			writeJSONL(t, artifact, line)
			path := filepath.Join(t.TempDir(), "collector.sqlite")
			collectorRebuildSync(t, path, source, collectorRebuildClock())
			writeJSONL(t, filepath.Join(source, harness, "archive", "date_rebuild-copy.jsonl"), line)
			collectorRebuildSync(t, path, source, collectorRebuildClock())
			database := openTestDB(t, path)
			defer func() { _ = database.Close() }()
			assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 2)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 2)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 0)
			assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_diagnostics WHERE code = 'publication_ambiguous_session_identity'", 2)
		})
	}
}

func TestNativeSessionEvidenceSurvivesFilenameDerivedCopy(t *testing.T) {
	for _, harness := range []string{"pi", "claude-code"} {
		for _, nativeFirst := range []bool{false, true} {
			t.Run(harness+fmt.Sprint(nativeFirst), func(t *testing.T) {
				source := t.TempDir()
				first, second := "a", "z"
				if !nativeFirst {
					first, second = second, first
				}
				nativeFile := filepath.Join(source, harness, first, "date_rebuild-main.jsonl")
				weakFile := filepath.Join(source, harness, second, "date_rebuild-main.jsonl")
				if harness == "pi" {
					line := `{"type":"message","id":"rebuild-message","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","provider":"fixture-provider","model":"fixture-model","usage":{"input":10,"output":5,"totalTokens":15}}}`
					writeJSONL(t, nativeFile, `{"type":"session","id":"rebuild-main"}`, line)
					writeJSONL(t, weakFile, line)
				} else {
					native := `{"type":"assistant","sessionId":"date_rebuild-main","requestId":"rebuild-request","timestamp":"2026-01-01T00:00:01Z","message":{"id":"rebuild-message","role":"assistant","model":"fixture-model","usage":{"input_tokens":10,"output_tokens":5}}}`
					weak := strings.Replace(native, `"sessionId":"date_rebuild-main",`, "", 1)
					writeJSONL(t, nativeFile, native)
					writeJSONL(t, weakFile, weak)
				}
				path := filepath.Join(t.TempDir(), "collector.sqlite")
				collectorRebuildSync(t, path, source, collectorRebuildClock())
				database := openTestDB(t, path)
				defer func() { _ = database.Close() }()
				assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 1)
				assertSQLCount(t, database, "SELECT COUNT(*) FROM publication_journal", 1)
				assertSQLCount(t, database, "SELECT SUM(json_extract(payload_json, '$.totalTokens')) FROM publication_journal", 15)
			})
		}
	}
}
