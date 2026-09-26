package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestCodexLargeRecordPreservesUsageBeforeAndAfter(t *testing.T) {
	root := t.TempDir()
	large := strings.Repeat("x", 17*1024*1024)
	codexReplaySource(t, root, "large", codexReplayHeader("large", ""), codexReplayTurn("turn"), codexReplayUsage(1, 10, 10), `{"type":"response_item","payload":{"type":"custom_tool_call_output","output":"`+large+`"}}`, codexReplayUsage(2, 20, 30))
	// A usage-bearing record may also exceed the old limit. Its counters count.
	usage := strings.TrimSuffix(codexReplayUsage(3, 30, 60), "}") + `,"padding":"` + large + `"}`
	path := filepath.Join(root, "rollout-2026-01-01T00-00-00-large.jsonl")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(usage + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	opts := SyncOptions{DBPath: filepath.Join(t.TempDir(), "usage.sqlite"), Harnesses: []Harness{HarnessCodex}, SourceDir: root, Normalize: true}
	summary, err := Sync(context.Background(), opts)
	if err != nil || summary.Canonical != 3 {
		t.Fatalf("large records: %+v %v", summary, err)
	}
	database := openTestDB(t, opts.DBPath)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 60)
	if _, err := Sync(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 3)
}

func TestSyncContinuesAfterFailedSourceAndPreservesSharedStatus(t *testing.T) {
	root := t.TempDir()
	codexReplaySource(t, root, "a", codexReplayHeader("a", ""), codexReplayTurn("a-turn"), codexReplayUsage(1, 10, 10))
	broken := codexReplaySource(t, root, "b", codexReplayHeader("b", ""))
	codexReplaySource(t, root, "c", codexReplayHeader("c", ""), codexReplayTurn("c-turn"), codexReplayUsage(2, 20, 20))
	opts := SyncOptions{DBPath: filepath.Join(t.TempDir(), "usage.sqlite"), Harnesses: []Harness{HarnessCodex}, SourceDir: root, Normalize: true}
	opts.Progress = func(event SyncProgressEvent) {
		if event.Status == SyncProgressSyncing {
			if err := os.Remove(broken); err != nil {
				t.Fatal(err)
			}
		}
	}
	summary, err := Sync(context.Background(), opts)
	if err == nil || summary.Canonical != 2 || summary.Failed != 1 {
		t.Fatalf("partial sync: %+v %v", summary, err)
	}
	status, err := db.ReadSyncStatus(context.Background(), opts.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if status.Running || status.Phase != "failed" || status.TotalSources != 3 || status.CheckedSources != 3 || status.ReadySources != 2 || status.FailedSources != 1 || status.LastSuccessfulAtMs != 0 {
		t.Fatalf("shared status: %+v", status)
	}
	database := openTestDB(t, opts.DBPath)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 30)
	coverage, err := db.ViewerDayCoverage(context.Background(), database, db.Filter{Harnesses: []string{"codex"}, DayFrom: "2026-01-01", DayTo: "2026-01-02"}, time.Date(2026, 1, 2, 12, 0, 0, 0, time.Local))
	if err != nil || len(coverage) != 2 || coverage[0].Status != "partial" || coverage[1].Status != "partial" || coverage[1].Total != nil {
		t.Fatalf("coverage must not imply zero: %+v %v", coverage, err)
	}
}

func TestJSONLReadSnapshotAndDeferredTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.jsonl")
	if err := os.WriteFile(path, []byte("{\"n\":1}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	reader := newJSONLReader(context.Background(), file)
	writer, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString("{\"n\":2}\n{\"n\":"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	count := 0
	for reader.Scan() {
		count++
	}
	if count != 1 || reader.Err() != nil {
		t.Fatalf("reader chased append: %d %v", count, reader.Err())
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	reader = newJSONLReader(context.Background(), file)
	count = 0
	for reader.Scan() {
		count++
	}
	if count != 2 || !reader.deferred || reader.Err() != nil {
		t.Fatalf("unfinished tail: %d %+v", count, reader)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	reader = newJSONLReader(ctx, file)
	if reader.Scan() || !errors.Is(reader.Err(), context.Canceled) {
		t.Fatal("reader ignored cancellation")
	}
}

func TestIncompleteUsageTailRetriesWithoutLosingOrDuplicatingFacts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	path := codexReplaySource(t, root, "tail", codexReplayHeader("tail", ""), codexReplayTurn("turn"), codexReplayUsage(1, 10, 10))
	usage := codexReplayUsage(2, 20, 30)
	appendBytes := func(text string) {
		t.Helper()
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString(text); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	appendBytes(usage[:len(usage)/2])
	opts := SyncOptions{DBPath: filepath.Join(t.TempDir(), "usage.sqlite"), SourceDir: root, Harnesses: []Harness{HarnessCodex}, Normalize: true}
	summary, err := Sync(ctx, opts)
	if err != nil || summary.Canonical != 1 {
		t.Fatalf("partial tail: %+v %v", summary, err)
	}
	database := openTestDB(t, opts.DBPath)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT COUNT(*) FROM sync_sources WHERE status = 'deferred' AND min_occurred_at_ms IS NULL", 1)
	appendBytes(usage[len(usage)/2:] + "\n")
	summary, err = Sync(ctx, opts)
	if err != nil || summary.Canonical != 1 {
		t.Fatalf("completed tail: %+v %v", summary, err)
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 2)
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 30)
	if _, err := Sync(ctx, opts); err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 2)
}

func TestSyncPublishesHarnessBeforeNextHarnessIngest(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	piRoot := filepath.Join(root, "pi")
	if err := os.MkdirAll(piRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	writePiAssistantSession(t, filepath.Join(piRoot, "date_pi.jsonl"), "pi", "message", 10, 2)
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	observed := false
	_, err := Sync(ctx, SyncOptions{DBPath: path, SourceDir: root, Harnesses: SupportedHarnesses, Normalize: true, Progress: func(event SyncProgressEvent) {
		if event.Harness == HarnessPi && event.Published {
			observed = true
			database := openTestDB(t, path)
			defer func() { _ = database.Close() }()
			assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 1)
			status, err := db.ReadSyncStatus(ctx, path)
			if err != nil || !status.Running || status.Revision == 0 {
				t.Fatalf("committed publication: %+v %v", status, err)
			}
		}
	}})
	if err != nil || !observed {
		t.Fatalf("missing progressive publication: %v", err)
	}
	status, err := db.ReadSyncStatus(ctx, path)
	if err != nil || status.LastSuccessfulAtMs == 0 || status.Running {
		t.Fatalf("completed snapshot: %+v %v", status, err)
	}
}
