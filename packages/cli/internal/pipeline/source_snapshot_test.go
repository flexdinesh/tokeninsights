package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func preparedSourceState(t *testing.T, result preparedSource, options SyncOptions) sourceState {
	t.Helper()
	if result.parseErr != nil || result.cursor == nil || result.status != "ingested" {
		t.Fatalf("source did not establish continuity: %+v", result)
	}
	return sourceState{hasRefresh: true, hasCursor: true, cursor: *result.cursor,
		refresh: sourceRefreshState{collector: options.Collector, parser: options.Parser,
			sourceMtimeMs: result.metadata.mtimeMs, sourceSizeBytes: result.metadata.sizeBytes,
			lastSuccessfulRefreshAtMs: options.Now.UnixMilli()}}
}

func TestPreparedPiVerifiesOldSameSizeRewrite(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "2026-01-01_session.jsonl")
	writePiAssistantSession(t, path, "session", "msg_a", 100, 50)
	mtime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	setFileModTime(t, path, mtime)
	source := (piJSONLAdapter{}).source(path, filepath.Dir(path))
	options := SyncOptions{Collector: "test", Parser: "test", Now: mtime.Add(30 * 24 * time.Hour)}
	first := preparePiSource(ctx, source, options, sourceState{})
	state := preparedSourceState(t, first, options)
	if result := preparePiSource(ctx, source, options, state); !result.unchanged || len(result.facts) != 0 {
		t.Fatalf("unchanged source: %+v", result)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rewritten := strings.Replace(string(content), `"id":"msg_a"`, `"id":"msg_b"`, 1)
	if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
		t.Fatal(err)
	}
	setFileModTime(t, path, mtime)
	result := preparePiSource(ctx, source, options, state)
	if result.unchanged || result.parseErr != nil || len(result.facts) != 1 || result.facts[0].MessageID == nil || *result.facts[0].MessageID != "msg_b" {
		t.Fatalf("old rewritten source skipped: %+v", result)
	}
}

func TestPreparedPiAppendCarriesVerifiedPrefix(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "2026-01-01_session.jsonl")
	writePiAssistantSession(t, path, "session", "msg_a", 100, 50)
	source := (piJSONLAdapter{}).source(path, filepath.Dir(path))
	options := SyncOptions{Collector: "test", Parser: "test", Now: time.Unix(1, 0)}
	first := preparePiSource(ctx, source, options, sourceState{})
	state := preparedSourceState(t, first, options)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`{"type":"message","id":"msg_b","message":{"role":"assistant","usage":{"input":5,"output":2},"timestamp":1770000002000}}` + "\n")
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("append: %v, close: %v", err, closeErr)
	}
	result := preparePiSource(ctx, source, options, state)
	if result.parseErr != nil || result.cursor == nil || len(result.facts) != 1 || result.facts[0].MessageID == nil || *result.facts[0].MessageID != "msg_b" {
		t.Fatalf("append did not parse only new facts: %+v", result)
	}
	prefix, boundary, terminated, err := sourceCursorHashes(path, result.metadata.sizeBytes)
	if err != nil || !terminated || prefix != result.cursor.prefixHash || boundary != result.cursor.boundaryHash {
		t.Fatalf("advanced cursor differs from full file: %+v, %v", result.cursor, err)
	}
}

func TestPreparedJSONLFingerprintMatchesParsedBytesAndLocations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeClaudeCodeAssistantSession(t, path, "session", "msg_a", 100, 50)
	source := Source{Harness: HarnessClaudeCode, ID: "test", Kind: claudeCodeJSONLSourceKind, Path: path}
	options := SyncOptions{Collector: "test", Parser: "test", Now: time.Unix(1, 0)}
	result := prepareJSONLSource(ctx, claudeCodeJSONLAdapter{}, source, options, sourceState{})
	state := preparedSourceState(t, result, options)
	fingerprint, valid := fingerprintSource(ctx, source, options)
	if !valid || fingerprint != result.fingerprint || len(result.facts) != 1 {
		t.Fatalf("parsed fingerprint disagrees with verification: %+v, %+v", result, fingerprint)
	}
	if repeat := prepareJSONLSource(ctx, claudeCodeJSONLAdapter{}, source, options, state); !repeat.unchanged || len(repeat.facts) != 0 {
		t.Fatalf("verified unchanged source parsed again: %+v", repeat)
	}
}

func TestPreparedJSONLEscapedLocationKeysVerifyAndInvalidate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	for _, harness := range []Harness{HarnessClaudeCode, HarnessCodex} {
		t.Run(string(harness), func(t *testing.T) {
			ctx := context.Background()
			repoPath := filepath.Join(t.TempDir(), "checkout")
			for _, args := range [][]string{{"init", "-q", repoPath}, {"-C", repoPath, "remote", "add", "origin", "https://example.com/team/first.git"}} {
				if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v: %s", args, err, output)
				}
			}
			path := filepath.Join(t.TempDir(), "session.jsonl")
			source := Source{Harness: harness, ID: "test", Path: path}
			newAdapter := func() Adapter { return claudeCodeJSONLAdapter{} }
			if harness == HarnessClaudeCode {
				source.Kind = claudeCodeJSONLSourceKind
				writeJSONL(t, path, fmt.Sprintf(`{"type":"assistant","uuid":"msg","timestamp":"2026-01-01T00:00:02.000Z","sessionId":"session","\u0063wd":%q,"message":{"id":"msg","role":"assistant","model":"claude-sonnet-4-5","usage":{"input_tokens":100,"output_tokens":50}}}`, repoPath))
			} else {
				source.Kind = codexSessionJSONLSourceKind
				newAdapter = func() Adapter { return &codexJSONLAdapter{} }
				writeJSONL(t, path,
					fmt.Sprintf(`{"type":"session_meta","payload":{"id":"session","model_provider":"openai","\u0063wd":%q,"git":{"\u0072epository_url":"https://example.com/team/first.git"}}}`, repoPath),
					`{"type":"turn_context","payload":{"turn_id":"turn","model":"gpt-5"}}`,
					`{"timestamp":"2026-01-01T00:00:02.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":100,"output_tokens":50},"total_token_usage":{"input_tokens":100,"output_tokens":50}}}}`,
				)
			}
			options := SyncOptions{Collector: "test", Parser: "test", Now: time.Unix(1, 0), locationResolver: &locationResolver{}}
			first := prepareJSONLSource(ctx, newAdapter(), source, options, sourceState{})
			state := preparedSourceState(t, first, options)
			fingerprint, valid := fingerprintSource(ctx, source, options)
			if !valid || fingerprint != first.fingerprint || len(first.facts) != 1 || first.facts[0].Location == nil || first.facts[0].Location.DirectoryKey == "" || first.facts[0].Location.RepositoryKey == "" {
				t.Fatalf("escaped location proof differs: %+v, %+v", first, fingerprint)
			}
			if repeat := prepareJSONLSource(ctx, newAdapter(), source, options, state); !repeat.unchanged {
				t.Fatalf("escaped unchanged source reparsed: %+v", repeat)
			}
			if output, err := exec.Command("git", "-C", repoPath, "remote", "set-url", "origin", "https://example.com/team/second.git").CombinedOutput(); err != nil {
				t.Fatalf("change remote: %v: %s", err, output)
			}
			options.locationResolver = &locationResolver{}
			changed := prepareJSONLSource(ctx, newAdapter(), source, options, state)
			if changed.parseErr != nil || changed.unchanged || len(changed.facts) != 1 || changed.cursor == nil || changed.fingerprint.content != first.fingerprint.content || changed.fingerprint.location == first.fingerprint.location {
				t.Fatalf("escaped location change did not invalidate unchanged marker: %+v", changed)
			}
		})
	}
}

func TestPreparedJSONLIncompleteTailDoesNotEstablishMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeClaudeCodeAssistantSession(t, path, "session", "msg_a", 100, 50)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`{"type":"assistant"`)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("append: %v, close: %v", err, closeErr)
	}
	source := Source{Harness: HarnessClaudeCode, ID: "test", Kind: claudeCodeJSONLSourceKind, Path: path}
	result := prepareJSONLSource(context.Background(), claudeCodeJSONLAdapter{}, source, SyncOptions{}, sourceState{})
	if result.parseErr != nil || result.status != "deferred" || result.cursor != nil || len(result.facts) != 1 {
		t.Fatalf("unfinished tail established continuity: %+v", result)
	}
}

type mutatingReadAdapter struct {
	Adapter
	mutate func() error
}

func (a mutatingReadAdapter) Parse(ctx context.Context, source Source, options SyncOptions) ([]RawTokenFact, []Diagnostic, error) {
	facts, diagnostics, err := a.Adapter.Parse(ctx, source, options)
	if err == nil {
		err = a.mutate()
	}
	return facts, diagnostics, err
}

func TestPreparedJSONLDoesNotTrustRewriteAfterParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	writeClaudeCodeAssistantSession(t, path, "session", "msg_a", 100, 50)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	adapter := mutatingReadAdapter{Adapter: claudeCodeJSONLAdapter{}, mutate: func() error {
		rewritten := strings.Replace(string(content), `"msg_a"`, `"msg_b"`, 1)
		if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
			return err
		}
		return os.Chtimes(path, info.ModTime(), info.ModTime())
	}}
	source := Source{Harness: HarnessClaudeCode, ID: "test", Kind: claudeCodeJSONLSourceKind, Path: path}
	result := prepareJSONLSource(context.Background(), adapter, source, SyncOptions{}, sourceState{})
	if result.parseErr != nil || result.status != "deferred" || result.cursor != nil || len(result.facts) != 1 {
		t.Fatalf("rewrite after parsing established continuity: %+v", result)
	}
}
