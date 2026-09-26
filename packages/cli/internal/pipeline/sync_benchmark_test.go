package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	benchmarkSourceCount = 12
	benchmarkFactCount   = 24
	benchmarkForkDepth   = 3
)

// Each iteration uses a fixed source population. Setup and mutation are excluded
// from timing; the measured operation includes discovery, persistence and normalize.
func BenchmarkSync(b *testing.B) {
	benchmarkSyncScenarios(b, 0)
}

func BenchmarkSyncWorkers(b *testing.B) {
	for _, workers := range []int{1, 0} {
		name := "Automatic"
		if workers == 1 {
			name = "Serial"
		}
		b.Run(name, func(b *testing.B) { benchmarkSyncScenarios(b, workers) })
	}
}

func benchmarkSyncScenarios(b *testing.B, workers int) {
	for _, scenario := range []string{"FirstIngest", "Unchanged", "PiAppend", "ChangedJSONL", "ForkTree", "FullRefresh"} {
		b.Run(scenario, func(b *testing.B) {
			b.ReportAllocs()
			var observations, rawFacts, canonical int
			var bytesRead, parses, commits int64
			for iteration := 0; iteration < b.N; iteration++ {
				b.StopTimer()
				root, err := os.MkdirTemp(b.TempDir(), "sources-")
				if err != nil {
					b.Fatal(err)
				}
				benchmarkSources(b, root, scenario == "ForkTree")
				options := SyncOptions{
					DBPath: filepath.Join(root, "usage.sqlite"), SourceDir: root,
					Harnesses: SupportedHarnesses, Normalize: true,
					Now:     time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
					workers: workers,
				}
				if scenario != "FirstIngest" {
					benchmarkSync(b, options)
				}
				if scenario == "PiAppend" {
					for source := 0; source < benchmarkSourceCount; source++ {
						path := benchmarkSourcePath(root, HarnessPi, source)
						benchmarkAppend(b, path, benchmarkPiMessage(source, benchmarkFactCount))
					}
				}
				if scenario == "ChangedJSONL" {
					for source := 0; source < benchmarkSourceCount; source++ {
						benchmarkAppend(b, benchmarkSourcePath(root, HarnessCodex, source), codexReplayTurn(fmt.Sprintf("extra-%d", source)), codexReplayUsage(25, 10, (benchmarkFactCount+1)*10))
						benchmarkAppend(b, benchmarkSourcePath(root, HarnessClaudeCode, source), benchmarkClaudeMessage(source, benchmarkFactCount))
					}
				}
				options.FullRefresh = scenario == "FullRefresh"
				stats := &syncStats{}
				options.stats = stats
				b.StartTimer()
				summary := benchmarkSync(b, options)
				b.StopTimer()
				observations += summary.Observations
				rawFacts += summary.RawFacts
				canonical += summary.Canonical
				bytesRead += stats.bytesRead.Load()
				parses += stats.sourceParses.Load()
				commits += stats.writerCommits.Load()
				if err := os.RemoveAll(root); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(observations)/float64(b.N), "observations/op")
			b.ReportMetric(float64(rawFacts)/float64(b.N), "raw-facts/op")
			b.ReportMetric(float64(canonical)/float64(b.N), "canonical/op")
			b.ReportMetric(float64(bytesRead)/float64(b.N), "jsonl-bytes/op")
			b.ReportMetric(float64(parses)/float64(b.N), "parses/op")
			b.ReportMetric(float64(commits)/float64(b.N), "commits/op")
		})
	}
}

func benchmarkSync(tb testing.TB, options SyncOptions) Summary {
	tb.Helper()
	summary, err := Sync(context.Background(), options)
	if err != nil {
		tb.Fatal(err)
	}
	return summary
}

func benchmarkSources(tb testing.TB, root string, forks bool) {
	tb.Helper()
	for _, harness := range SupportedHarnesses {
		if err := os.MkdirAll(filepath.Join(root, string(harness)), 0o700); err != nil {
			tb.Fatal(err)
		}
	}
	benchmarkOpenCode(tb, filepath.Join(root, string(HarnessOpenCode), "opencode.db"))
	for source := 0; source < benchmarkSourceCount; source++ {
		piLines := []string{fmt.Sprintf(`{"type":"session","version":1,"id":"pi-%d","timestamp":"2026-01-01T00:00:00Z"}`, source)}
		codexLines := []string{codexReplayHeader(fmt.Sprintf("codex-%d", source), "")}
		var claudeLines []string
		for fact := 0; fact < benchmarkFactCount; fact++ {
			piLines = append(piLines, benchmarkPiMessage(source, fact))
			codexLines = append(codexLines, codexReplayTurn(fmt.Sprintf("turn-%d-%d", source, fact)), codexReplayUsage(fact+1, 10, (fact+1)*10))
			claudeLines = append(claudeLines, benchmarkClaudeMessage(source, fact))
		}
		benchmarkWrite(tb, benchmarkSourcePath(root, HarnessPi, source), piLines...)
		benchmarkWrite(tb, benchmarkSourcePath(root, HarnessCodex, source), codexLines...)
		benchmarkWrite(tb, benchmarkSourcePath(root, HarnessClaudeCode, source), claudeLines...)
		if forks {
			parent := fmt.Sprintf("codex-%d", source)
			for depth := 1; depth < benchmarkForkDepth; depth++ {
				session := fmt.Sprintf("codex-%d-depth-%d", source, depth)
				codexLines[0] = codexReplayHeader(session, parent)
				codexLines = append(codexLines, codexReplayTurn(session), codexReplayUsage(benchmarkFactCount+depth, 10, (benchmarkFactCount+depth)*10))
				benchmarkWrite(tb, filepath.Join(root, string(HarnessCodex), "rollout-2026-01-01T00-00-00-"+session+".jsonl"), codexLines...)
				parent = session
			}
		}
	}
}

func benchmarkSourcePath(root string, harness Harness, source int) string {
	name := fmt.Sprintf("source-%d.jsonl", source)
	if harness == HarnessCodex {
		name = fmt.Sprintf("rollout-2026-01-01T00-00-00-codex-%d.jsonl", source)
	}
	return filepath.Join(root, string(harness), name)
}

func benchmarkPiMessage(source, fact int) string {
	return fmt.Sprintf(`{"type":"message","id":"pi-message-%d-%d","timestamp":"2026-01-01T00:00:%02dZ","message":{"role":"assistant","provider":"anthropic","model":"claude-sonnet-4","usage":{"input":10,"output":2,"cacheRead":0,"cacheWrite":0,"totalTokens":12}}}`, source, fact, fact+1)
}

func benchmarkClaudeMessage(source, fact int) string {
	return fmt.Sprintf(`{"type":"assistant","uuid":"claude-%d-%d","requestId":"request-%d-%d","timestamp":"2026-01-01T00:00:%02dZ","sessionId":"claude-session-%d","message":{"id":"claude-%d-%d","role":"assistant","model":"claude-sonnet-4","usage":{"input_tokens":10,"output_tokens":2}}}`, source, fact, source, fact, fact+1, source, source, fact)
}

func benchmarkWrite(tb testing.TB, path string, lines ...string) {
	tb.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		tb.Fatal(err)
	}
}

func benchmarkAppend(tb testing.TB, path string, lines ...string) {
	tb.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := file.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		_ = file.Close()
		tb.Fatal(err)
	}
	if err := file.Close(); err != nil {
		tb.Fatal(err)
	}
}

func benchmarkOpenCode(tb testing.TB, path string) {
	tb.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		tb.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if _, err := database.Exec(`CREATE TABLE message (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, time_created INTEGER NOT NULL, time_updated INTEGER NOT NULL, data TEXT NOT NULL)`); err != nil {
		tb.Fatal(err)
	}
	for source := 0; source < benchmarkSourceCount; source++ {
		for fact := 0; fact < benchmarkFactCount; fact++ {
			created := time.Date(2026, 1, 1, 0, 0, source*benchmarkFactCount+fact, 0, time.UTC).UnixMilli()
			message := fmt.Sprintf(`{"role":"assistant","modelID":"gpt-5","providerID":"openai","tokens":{"input":10,"output":2,"reasoning":0,"cache":{"read":0,"write":0}},"time":{"created":%d}}`, created)
			if _, err := database.Exec(`INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES (?, ?, ?, ?, ?)`, fmt.Sprintf("message-%d-%d", source, fact), fmt.Sprintf("session-%d", source), created, created, message); err != nil {
				tb.Fatal(err)
			}
		}
	}
}
