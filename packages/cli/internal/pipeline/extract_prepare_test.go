package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/evidence"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/rawcollectorstore"
)

func rawTestStore(t *testing.T) *rawcollectorstore.Store {
	t.Helper()
	store, err := rawcollectorstore.Open(t.Context(), filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
func rawTestWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
func rawTestRecords(t *testing.T, store *rawcollectorstore.Store) []evidence.Record {
	t.Helper()
	rows, err := store.DB.Query("SELECT record_json FROM evidence_outbox ORDER BY sequence")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var records []evidence.Record
	for rows.Next() {
		var body []byte
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		var record evidence.Record
		if err := json.Unmarshal(body, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}
func codexTestHeader() string {
	return "{\"type\":\"session_meta\",\"payload\":{\"id\":\"session\"}}\n"
}
func codexTestUsage(input int) string {
	return fmt.Sprintf("{\"type\":\"event_msg\",\"timestamp\":\"2026-10-07T12:00:00Z\",\"payload\":{\"type\":\"token_count\",\"info\":{\"last_token_usage\":{\"input_tokens\":%d,\"output_tokens\":2,\"total_tokens\":%d}}}}\n", input, input+2)
}

func TestRawOversizedIrrelevantKeepsUsageOrdinalsAndCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	store := rawTestStore(t)
	source := Source{ID: evidence.Tuple("source"), Harness: HarnessCodex, Path: path}
	body := codexTestHeader() + codexTestUsage(10) + `{"payload":{"output":"` + strings.Repeat("private sentinel", maxEvidenceLineBytes/len("private sentinel")+1) + `"},"type":"response_item"}` + "\n" + codexTestUsage(20)
	rawTestWrite(t, path, body)
	count, err := extractJSONL(t.Context(), source, SyncOptions{}, store)
	if err != nil || count != 3 {
		t.Fatal(count, err)
	}
	records := rawTestRecords(t, store)
	if len(records) != 3 || records[2].Ordinal != 4 || evidence.String(records[2].Data, "payload.type") != "token_count" || len(records[2].Context) != 1 {
		t.Fatal("usage/context/order missing", records)
	}
	for _, record := range records {
		encoded, _ := json.Marshal(record)
		if strings.Contains(string(encoded), "private sentinel") {
			t.Fatal("private content persisted")
		}
	}
	checkpoint, found, err := store.Checkpoint(t.Context(), source.ID, "codex-jsonl")
	if err != nil || !found || checkpoint.Offset != int64(len(body)) || checkpoint.Ordinal != 4 {
		t.Fatal(checkpoint, found, err)
	}
	count, err = extractJSONL(t.Context(), source, SyncOptions{}, store)
	if err != nil || count != 0 {
		t.Fatal("unchanged reread", count, err)
	}
	rawTestWrite(t, path, body+codexTestUsage(30))
	count, err = extractJSONL(t.Context(), source, SyncOptions{}, store)
	if err != nil || count != 1 {
		t.Fatal("append", count, err)
	}
}

func TestRawQuarantineSurvivesRestartAndRetriesChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	dbPath := filepath.Join(root, "collector.sqlite")
	store, err := rawcollectorstore.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	source := Source{ID: evidence.Tuple("source"), Harness: HarnessCodex, Path: path}
	initial := codexTestHeader() + codexTestUsage(10)
	rawTestWrite(t, path, initial)
	if count, err := extractJSONL(t.Context(), source, SyncOptions{}, store); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	baseline, _, _ := store.Checkpoint(t.Context(), source.ID, "codex-jsonl")
	bad := initial + codexTestUsage(20) + `{"type":"event_msg","payload":{"type":"token_count","private":"` + strings.Repeat("x", maxEvidenceLineBytes) + `"}}` + "\n"
	rawTestWrite(t, path, bad)
	if count, err := extractJSONL(t.Context(), source, SyncOptions{}, store); count != 0 || !errors.Is(err, errSourceRecordLimit) {
		t.Fatal(count, err)
	}
	if len(rawTestRecords(t, store)) != 2 {
		t.Fatal("failed source partially committed")
	}
	checkpoint, _, _ := store.Checkpoint(t.Context(), source.ID, "codex-jsonl")
	if !reflect.DeepEqual(checkpoint, baseline) {
		t.Fatal("quarantine advanced checkpoint")
	}
	marker, found, err := store.GetQuarantine(t.Context(), source.ID)
	if err != nil || !found || marker.Offset != int64(len(initial+codexTestUsage(20))) {
		t.Fatal(marker, found, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = rawcollectorstore.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extractJSONL(t.Context(), source, SyncOptions{}, store); !errors.Is(err, errSourceQuarantined) {
		t.Fatal("restart did not skip quarantined source", err)
	}
	// A parser policy upgrade must retry unchanged files, independent of schema.
	marker.ParserVersion = rawParserPolicyVersion + 1
	marker.RecordedAtMs = 1
	if err := store.SaveQuarantine(t.Context(), marker); err != nil {
		t.Fatal(err)
	}
	if _, err := extractJSONL(t.Context(), source, SyncOptions{}, store); !errors.Is(err, errSourceRecordLimit) {
		t.Fatal("parser policy did not retry", err)
	}
	marker, _, _ = store.GetQuarantine(t.Context(), source.ID)
	if marker.ParserVersion != rawParserPolicyVersion || marker.RecordedAtMs <= 1 {
		t.Fatal("parser failure marker not refreshed", marker)
	}
	// Explicit full refresh retries the file, while retaining previous evidence.
	marker.RecordedAtMs = 1
	if err := store.SaveQuarantine(t.Context(), marker); err != nil {
		t.Fatal(err)
	}
	if _, err := extractJSONL(t.Context(), source, SyncOptions{FullRefresh: true}, store); !errors.Is(err, errSourceRecordLimit) {
		t.Fatal("refresh did not retry", err)
	}
	marker, _, _ = store.GetQuarantine(t.Context(), source.ID)
	if marker.RecordedAtMs <= 1 {
		t.Fatal("refresh kept old failure", marker)
	}
	// Same size/mtime replacement must not preserve the quarantine signature.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "replacement.jsonl")
	rawTestWrite(t, replacement, bad)
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, err := extractJSONL(t.Context(), source, SyncOptions{}, store); !errors.Is(err, errSourceRecordLimit) {
		t.Fatal("replacement skipped", err)
	}
	replaced, _, _ := store.GetQuarantine(t.Context(), source.ID)
	if replaced.Signature == marker.Signature {
		t.Fatal("inode not in signature")
	}
	// A corrected source retries automatically and removes the marker.
	rawTestWrite(t, path, initial+codexTestUsage(20))
	if count, err := extractJSONL(t.Context(), source, SyncOptions{}, store); err != nil || count != 1 {
		t.Fatal("corrected source", count, err)
	}
	if _, found, err := store.GetQuarantine(t.Context(), source.ID); err != nil || found {
		t.Fatal("successful capture retained quarantine", found, err)
	}
	if len(rawTestRecords(t, store)) != 3 {
		t.Fatal("previous evidence lost")
	}
}

func TestRawOversizedPartialTailWaitsWithoutQuarantine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	store := rawTestStore(t)
	source := Source{ID: evidence.Tuple("source"), Harness: HarnessCodex, Path: path}
	initial := codexTestHeader() + codexTestUsage(10)
	unfinished := `{"type":"response_item","payload":{"output":"` + strings.Repeat("x", maxEvidenceLineBytes)
	rawTestWrite(t, path, initial+unfinished)
	if count, err := extractJSONL(t.Context(), source, SyncOptions{}, store); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	checkpoint, _, _ := store.Checkpoint(t.Context(), source.ID, "codex-jsonl")
	if checkpoint.Offset != int64(len(initial)) {
		t.Fatal("partial tail acknowledged", checkpoint)
	}
	if _, found, err := store.GetQuarantine(t.Context(), source.ID); err != nil || found {
		t.Fatal("growing tail quarantined", err)
	}
	rawTestWrite(t, path, initial+unfinished+`"}}`+"\n"+codexTestUsage(20))
	if count, err := extractJSONL(t.Context(), source, SyncOptions{}, store); err != nil || count != 1 {
		t.Fatal("completed tail", count, err)
	}
}

func TestPreparedCaptureMutationCancellationAndPrivateSpool(t *testing.T) {
	for _, mode := range []string{"rewrite", "replace", "checkpoint", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "session.jsonl")
			store := rawTestStore(t)
			source := Source{ID: evidence.Tuple("source"), Harness: HarnessCodex, Path: path}
			initial := codexTestHeader() + codexTestUsage(10)
			rawTestWrite(t, path, initial)
			p, err := prepareRawSource(t.Context(), source, SyncOptions{}, store, root)
			if err != nil {
				t.Fatal(err)
			}
			defer p.close()
			info, err := p.file.Stat()
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatal("insecure spool", info, err)
			}
			body, err := os.ReadFile(p.file.Name())
			if err != nil || strings.Contains(string(body), path) {
				t.Fatal("source path spooled", err)
			}
			if len(rawTestRecords(t, store)) != 0 {
				t.Fatal("preparation wrote collector evidence")
			}
			ctx := t.Context()
			switch mode {
			case "rewrite":
				rawTestWrite(t, path, codexTestHeader()+codexTestUsage(11))
			case "replace":
				replacement := filepath.Join(root, "replacement.jsonl")
				rawTestWrite(t, replacement, initial)
				if err := os.Rename(replacement, path); err != nil {
					t.Fatal(err)
				}
			case "checkpoint":
				tx, err := store.BeginCapture(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if err := tx.SaveCheckpoint(t.Context(), p.checkpoint); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if count, err := commitPreparedRaw(ctx, source, p, store); err == nil || count != 0 {
				t.Fatal("stale/canceled source committed", count, err)
			}
			if len(rawTestRecords(t, store)) != 0 {
				t.Fatal("rollback leaked partial records")
			}
			name := p.file.Name()
			p.close()
			if _, err := os.Stat(name); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("spool survived close", err)
			}
		})
	}
}

func TestRawParallelMatchesSerialAndWorkerAdmission(t *testing.T) {
	root := t.TempDir()
	for i := range 12 {
		rawTestWrite(t, filepath.Join(root, fmt.Sprintf("session-%02d.jsonl", i)), fmt.Sprintf("{\"type\":\"session\",\"id\":\"session-%d\"}\n{\"type\":\"message\",\"id\":\"message-%d\",\"message\":{\"role\":\"assistant\",\"usage\":{\"input\":%d,\"output\":2}}}\n", i, i, i+1))
	}
	var serial []evidence.Record
	for _, workers := range []int{1, 4, 100} {
		store := rawTestStore(t)
		adapter, _ := AdapterFor(HarnessPi)
		sources, err := adapter.Discover(t.Context(), DiscoverOptions{SourceDir: root})
		if err != nil {
			t.Fatal(err)
		}
		tx, err := store.BeginCapture(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		for i, source := range sources {
			key := evidence.Tuple("source-file", source.Harness, source.ID, source.Path)
			state := rawcollectorstore.Checkpoint{SourceKey: key, SourceID: key, Format: "pi-jsonl", Lineage: fmt.Sprintf("lineage-%d", i), Context: json.RawMessage(`{}`)}
			if err := tx.SaveCheckpoint(t.Context(), state); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		summary, err := Extract(t.Context(), SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessPi}, workers: workers}, store)
		if err != nil || summary.RawFacts != 24 || summary.Synced != 1 {
			t.Fatal(summary, err)
		}
		records := rawTestRecords(t, store)
		if serial == nil {
			serial = records
		} else if !reflect.DeepEqual(serial, records) {
			t.Fatal("parallel changed sanitized evidence/order")
		}
		summary, err = Extract(t.Context(), SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessPi}, workers: workers}, store)
		if err != nil || summary.RawFacts != 0 || summary.Skipped != 1 {
			t.Fatal("parallel replay requeued unchanged observations", summary, err)
		}
	}
	if rawWorkerCount(SyncOptions{}, 100) != rawPreparationWorkers || rawWorkerCount(SyncOptions{workers: 100}, 100)*rawPreparationWorkerBytes > rawPreparationMemoryBytes || rawWorkerCount(SyncOptions{workers: 4}, 2) != 2 {
		t.Fatal("worker/count/memory admission violated")
	}
}

func TestRawQuarantineReportsIncompleteHarness(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout-2026-10-07T12-00-00-session.jsonl")
	store := rawTestStore(t)
	rawTestWrite(t, path, codexTestHeader()+`{"type":"session_meta","payload":{"private":"`+strings.Repeat("x", maxEvidenceLineBytes)+`"}}`+"\n")
	var events []SyncProgressEvent
	options := SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessCodex}, Progress: func(event SyncProgressEvent) { events = append(events, event) }}
	for range 2 {
		summary, err := Extract(t.Context(), options, store)
		if err == nil || summary.Quarantined != 1 || summary.Failed != 1 || summary.Synced != 0 || summary.RawFacts != 0 {
			t.Fatal("incomplete source reported success", summary, err)
		}
		last := events[len(events)-1]
		if last.Status != SyncProgressFailed || last.Quarantined != 1 {
			t.Fatal("quarantine hidden", last)
		}
	}
}

func TestRawOversizedIrrelevantAcrossJSONLHarnesses(t *testing.T) {
	padding := strings.Repeat("private sentinel", maxEvidenceLineBytes/len("private sentinel")+1)
	for _, test := range []struct {
		harness                   Harness
		header, irrelevant, usage string
		want                      int
	}{
		{HarnessCodex, codexTestHeader(), `{"payload":{"output":"` + padding + `"},"type":"response_item"}` + "\n", codexTestUsage(10), 2},
		{HarnessPi, "{\"type\":\"session\",\"id\":\"pi-session\"}\n", `{"message":{"content":"` + padding + `","role":"user"},"type":"message"}` + "\n", "{\"type\":\"message\",\"id\":\"pi-message\",\"message\":{\"role\":\"assistant\",\"usage\":{\"input\":10,\"output\":2}}}\n", 2},
		{HarnessClaudeCode, "", `{"message":{"content":"` + padding + `"},"type":"user"}` + "\n", "{\"type\":\"assistant\",\"sessionId\":\"claude-session\",\"uuid\":\"message\",\"message\":{\"role\":\"assistant\",\"usage\":{\"input_tokens\":10,\"output_tokens\":2}}}\n", 1},
	} {
		t.Run(string(test.harness), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "session.jsonl")
			store := rawTestStore(t)
			source := Source{ID: evidence.Tuple("source", test.harness), Harness: test.harness, Path: path}
			body := test.header + test.irrelevant + test.usage
			rawTestWrite(t, path, body)
			if count, err := extractJSONL(t.Context(), source, SyncOptions{}, store); err != nil || count != test.want {
				t.Fatal(count, err)
			}
			records := rawTestRecords(t, store)
			last := records[len(records)-1]
			if !evidence.UsageRecord(string(test.harness), string(test.harness)+"-jsonl", last.Data) || last.Ordinal != int64(test.want+1) {
				t.Fatal("usage lost/order changed", last)
			}
			if test.header != "" && len(last.Context) != 1 {
				t.Fatal("native context missing")
			}
			for _, record := range records {
				body, _ := json.Marshal(record)
				if strings.Contains(string(body), "private sentinel") {
					t.Fatal("private oversized record persisted")
				}
			}
			checkpoint, _, err := store.Checkpoint(t.Context(), source.ID, string(test.harness)+"-jsonl")
			if err != nil || checkpoint.Offset != int64(len(body)) {
				t.Fatal("byte boundary wrong", checkpoint, err)
			}
			if count, err := extractJSONL(t.Context(), source, SyncOptions{}, store); err != nil || count != 0 {
				t.Fatal("replay changed evidence", count, err)
			}
		})
	}
}

func TestRawCodexDiscoveryDoesNotReadQuarantinedTranscript(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rollout-2026-10-07T12-00-00-session.jsonl")
	store := rawTestStore(t)
	// An oversized first session_meta would previously be decoded during legacy
	// discovery before durable quarantine could be consulted.
	rawTestWrite(t, path, `{"type":"session_meta","payload":{"id":"session","private":"`+strings.Repeat("x", maxEvidenceLineBytes)+`"}}`+"\n")
	firstStats := &syncStats{}
	summary, err := Extract(withSyncStats(t.Context(), firstStats), SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessCodex}}, store)
	if err == nil || summary.Quarantined != 1 || firstStats.sourceParses.Load() != 1 || firstStats.bytesRead.Load() < maxEvidenceLineBytes {
		t.Fatal("first source not quarantined", summary, err, firstStats)
	}
	retryStats := &syncStats{}
	summary, err = Extract(withSyncStats(t.Context(), retryStats), SyncOptions{SourceDir: root, Harnesses: []Harness{HarnessCodex}}, store)
	if err == nil || summary.Quarantined != 1 || retryStats.sourceParses.Load() != 0 || retryStats.bytesRead.Load() != 0 {
		t.Fatal("quarantine reread transcript during discovery", summary, err, retryStats)
	}
}
