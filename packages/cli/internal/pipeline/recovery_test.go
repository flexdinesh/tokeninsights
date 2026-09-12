package pipeline

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestRecoveryTargetedSyncRebuildsAllDefaultsOnce(t *testing.T) {
	ctx := context.Background()
	root := recoveryDefaultRoots(t)
	writePiAssistantSession(t, filepath.Join(root, "home", ".pi", "agent", "sessions", "project", "date_pi.jsonl"), "pi", "pi-message", 10, 2)
	writeCodexTokenSession(t, filepath.Join(root, "codex", "sessions", "rollout-2026-01-01T00-00-00-codex.jsonl"), "codex", "turn", "gpt", 20, 3)
	path := recoveryOldDatabase(t, false)
	var events []SyncProgressStatus
	options := SyncOptions{DBPath: path, Harnesses: []Harness{HarnessCodex}, Normalize: false, Progress: func(event SyncProgressEvent) {
		events = append(events, event.Status)
	}}
	summary, err := Sync(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Recovery != RecoveryReset || summary.RequestedHarnesses != len(SupportedHarnesses) {
		t.Fatalf("expected all-harness reset, got %+v", summary)
	}
	if len(events) < 2 || events[0] != SyncProgressResetting || events[1] != SyncProgressRebuilding {
		t.Fatalf("recovery progress = %v", events)
	}
	database := openTestDB(t, path)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 2)
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 35)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage WHERE session_id = 'obsolete'", 0)
	assertSQLCount(t, database, "SELECT rebuild_pending FROM database_lifecycle", 0)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	events = nil
	repeated, err := Sync(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if repeated.Recovery != RecoveryNone || repeated.RequestedHarnesses != 1 {
		t.Fatalf("compatible repeat should honor requested scope: %+v", repeated)
	}
	for _, event := range events {
		if event == SyncProgressResetting || event == SyncProgressRebuilding {
			t.Fatalf("compatible repeat restarted recovery: %v", events)
		}
	}
}

func TestRecoveryLegacySchemaRebuildsWithinAllHarnessOverride(t *testing.T) {
	root := recoveryDefaultRoots(t)
	writePiAssistantSession(t, filepath.Join(root, "home", ".pi", "agent", "sessions", "date_excluded.jsonl"), "excluded", "excluded", 999, 0)
	sourceRoot := t.TempDir()
	writePiAssistantSession(t, filepath.Join(sourceRoot, "pi", "date_included.jsonl"), "included", "included", 7, 0)
	path := recoveryOldDatabase(t, true)
	summary, err := Sync(context.Background(), SyncOptions{DBPath: path, Harnesses: SupportedHarnesses, SourceDir: sourceRoot})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Recovery != RecoveryReset {
		t.Fatalf("expected schema recovery, got %+v", summary)
	}
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 7)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM canonical_sessions WHERE session_id = 'excluded'", 0)
}

func TestRecoveryCustomSingleHarnessDefersWithoutChanges(t *testing.T) {
	path := recoveryOldDatabase(t, false)
	before := recoverySnapshot(t, path)
	for _, dryRun := range []bool{false, true} {
		_, err := Sync(context.Background(), SyncOptions{DBPath: path, Harnesses: []Harness{HarnessPi}, SourceDir: t.TempDir(), DryRun: dryRun})
		if !errors.Is(err, db.ErrRecoveryRequired) {
			t.Fatalf("dryRun=%v error = %v, want required recovery", dryRun, err)
		}
		if after := recoverySnapshot(t, path); !reflect.DeepEqual(before, after) {
			t.Fatalf("deferred recovery changed DB: before=%v after=%v", before, after)
		}
	}
}

func TestRecoveryDryRunsPreviewWithoutResetting(t *testing.T) {
	root := recoveryDefaultRoots(t)
	writePiAssistantSession(t, filepath.Join(root, "home", ".pi", "agent", "sessions", "date_pi.jsonl"), "pi", "pi", 5, 0)
	path := recoveryOldDatabase(t, true)
	before := recoverySnapshot(t, path)
	summary, err := Sync(context.Background(), SyncOptions{DBPath: path, Harnesses: []Harness{HarnessCodex}, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Recovery != RecoveryReset || summary.RequestedHarnesses != 4 || summary.RawFacts != 1 {
		t.Fatalf("sync preview = %+v", summary)
	}
	normalized, err := Normalize(context.Background(), NormalizeOptions{DBPath: path, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Recovery != RecoveryReset || normalized.RawFacts != 1 {
		t.Fatalf("normalize preview = %+v", normalized)
	}
	if after := recoverySnapshot(t, path); !reflect.DeepEqual(before, after) {
		t.Fatalf("dry run changed DB: before=%v after=%v", before, after)
	}
}

func TestRecoveryRetriesFailedHarnessWithoutErasingProgress(t *testing.T) {
	ctx := context.Background()
	root := recoveryDefaultRoots(t)
	piPath := filepath.Join(root, "home", ".pi", "agent", "sessions", "date_pi.jsonl")
	writePiAssistantSession(t, piPath, "pi", "pi", 12, 0)
	brokenSource := filepath.Join(root, "xdg", "opencode", "opencode.db")
	writeJSONL(t, brokenSource, "not a SQLite database")
	path := recoveryOldDatabase(t, false)
	options := SyncOptions{DBPath: path, Harnesses: SupportedHarnesses, Normalize: true}
	summary, err := Sync(ctx, options)
	if !errors.Is(err, db.ErrRebuildPending) || summary.Recovery != RecoveryReset {
		t.Fatalf("expected failed pending rebuild: summary=%+v err=%v", summary, err)
	}
	if database, err := db.Open(path); !errors.Is(err, db.ErrRebuildPending) {
		if database != nil {
			_ = database.Close()
		}
		t.Fatalf("pending recovery analytics error = %v", err)
	}
	database, err := db.OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 12)
	var originalRawID int
	if err := database.QueryRow("SELECT id FROM raw_token_usage WHERE session_id = 'pi'").Scan(&originalRawID); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	// Remove both files: the successful import must survive retry even if its
	// durable source is no longer available. A second reset would lose it.
	if err := os.Remove(brokenSource); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(piPath); err != nil {
		t.Fatal(err)
	}
	summary, err = Sync(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Recovery != RecoveryResume {
		t.Fatalf("expected resume, got %+v", summary)
	}
	database = openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 12)
	assertSQLCount(t, database, "SELECT id FROM raw_token_usage WHERE session_id = 'pi'", originalRawID)
	assertSQLCount(t, database, "SELECT rebuild_pending FROM database_lifecycle", 0)
}

func TestRecoveryNormalizeRebuildsInsteadOfReusingOldFacts(t *testing.T) {
	root := recoveryDefaultRoots(t)
	writePiAssistantSession(t, filepath.Join(root, "home", ".pi", "agent", "sessions", "date_pi.jsonl"), "pi", "pi", 9, 0)
	path := recoveryOldDatabase(t, false)
	summary, err := Normalize(context.Background(), NormalizeOptions{DBPath: path, Harnesses: []Harness{HarnessCodex}})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Recovery != RecoveryReset {
		t.Fatalf("normalize recovery = %+v", summary)
	}
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 9)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue", 0)
}

func TestRecoveryNormalizationFailureResumesPendingWork(t *testing.T) {
	ctx := context.Background()
	root := recoveryDefaultRoots(t)
	writePiAssistantSession(t, filepath.Join(root, "home", ".pi", "agent", "sessions", "date_pi.jsonl"), "pi", "pi", 11, 0)
	path := recoveryOldDatabase(t, false)
	options := SyncOptions{DBPath: path, Harnesses: SupportedHarnesses, Normalize: true}
	options.Progress = func(event SyncProgressEvent) {
		if event.Status != SyncProgressNormalizing {
			return
		}
		database, err := db.OpenWritable(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = database.Close() }()
		if _, err := database.Exec("CREATE TRIGGER fail_normalization BEFORE INSERT ON canonical_token_usage BEGIN SELECT RAISE(FAIL, 'test normalization failure'); END"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Sync(ctx, options); !errors.Is(err, db.ErrRebuildPending) {
		t.Fatalf("expected pending normalization failure, got %v", err)
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	assertSQLCount(t, database, "SELECT COUNT(*) FROM raw_token_usage", 1)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue", 1)
	if _, err := database.Exec("DROP TRIGGER fail_normalization"); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	options.Progress = nil
	summary, err := Sync(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Recovery != RecoveryResume {
		t.Fatalf("expected normalization resume, got %+v", summary)
	}
	database = openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 11)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue", 0)
}

func TestRecoveryConcurrentSyncsResetOnlyOnce(t *testing.T) {
	root := recoveryDefaultRoots(t)
	writePiAssistantSession(t, filepath.Join(root, "home", ".pi", "agent", "sessions", "date_pi.jsonl"), "pi", "pi", 8, 0)
	path := recoveryOldDatabase(t, false)
	start := make(chan struct{})
	results := make(chan Summary, 2)
	errorsFound := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			summary, err := Sync(context.Background(), SyncOptions{DBPath: path, Harnesses: SupportedHarnesses, Normalize: true})
			results <- summary
			errorsFound <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	resets := 0
	for summary := range results {
		if summary.Recovery == RecoveryReset {
			resets++
		}
	}
	if resets != 1 {
		t.Fatalf("concurrent syncs performed %d resets, want 1", resets)
	}
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 8)
}

func TestRecoveryRejectsNewerGenerationWithoutResetting(t *testing.T) {
	path := recoveryOldDatabase(t, false)
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("UPDATE database_lifecycle SET data_generation = ?", db.CurrentDataGeneration+1); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	before := recoverySnapshot(t, path)
	for _, dryRun := range []bool{false, true} {
		if _, err := Sync(context.Background(), SyncOptions{DBPath: path, Harnesses: SupportedHarnesses, DryRun: dryRun}); err == nil {
			t.Fatalf("dryRun=%v accepted newer data", dryRun)
		}
	}
	if after := recoverySnapshot(t, path); !reflect.DeepEqual(before, after) {
		t.Fatalf("newer DB changed: before=%v after=%v", before, after)
	}
}

func TestRecoveryRetriesRequireOriginalCustomRoot(t *testing.T) {
	recoveryDefaultRoots(t)
	root := t.TempDir()
	writePiAssistantSession(t, filepath.Join(root, "pi", "date_pi.jsonl"), "pi", "pi", 13, 0)
	broken := filepath.Join(root, "opencode", "opencode.db")
	writeJSONL(t, broken, "not SQLite")
	path := recoveryOldDatabase(t, false)
	options := SyncOptions{DBPath: path, Harnesses: SupportedHarnesses, SourceDir: root, Normalize: true}
	if _, err := Sync(context.Background(), options); !errors.Is(err, db.ErrRebuildPending) {
		t.Fatalf("expected interrupted custom-root rebuild, got %v", err)
	}
	before := recoverySnapshot(t, path)
	for _, sourceDir := range []string{"", t.TempDir()} {
		for _, dryRun := range []bool{false, true} {
			if _, err := Sync(context.Background(), SyncOptions{DBPath: path, Harnesses: SupportedHarnesses, SourceDir: sourceDir, DryRun: dryRun}); !errors.Is(err, db.ErrRebuildPending) {
				t.Fatalf("mismatched retry dryRun=%v error = %v", dryRun, err)
			}
		}
	}
	if _, err := Normalize(context.Background(), NormalizeOptions{DBPath: path}); !errors.Is(err, db.ErrRebuildPending) {
		t.Fatalf("normalize must not switch a custom rebuild to defaults: %v", err)
	}
	if after := recoverySnapshot(t, path); !reflect.DeepEqual(before, after) {
		t.Fatalf("mismatched recovery changed data: before=%v after=%v", before, after)
	}
	if err := os.Remove(broken); err != nil {
		t.Fatal(err)
	}
	options.SourceDir = root + "/."
	summary, err := Sync(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Recovery != RecoveryResume {
		t.Fatalf("original normalized scope did not resume: %+v", summary)
	}
	database := openTestDB(t, path)
	defer func() { _ = database.Close() }()
	assertSQLCount(t, database, "SELECT SUM(total_tokens) FROM canonical_token_usage", 13)
	assertSQLCount(t, database, "SELECT COUNT(*) FROM database_lifecycle WHERE rebuild_source_key IS NULL AND rebuild_pending = 0", 1)
}

func TestRecoveryRetriesRequireOriginalDefaultRootConfiguration(t *testing.T) {
	root := recoveryDefaultRoots(t)
	broken := filepath.Join(root, "xdg", "opencode", "opencode.db")
	writeJSONL(t, broken, "not SQLite")
	path := recoveryOldDatabase(t, false)
	options := SyncOptions{DBPath: path, Harnesses: SupportedHarnesses, Normalize: true}
	if _, err := Sync(context.Background(), options); !errors.Is(err, db.ErrRebuildPending) {
		t.Fatalf("expected interrupted default rebuild, got %v", err)
	}
	before := recoverySnapshot(t, path)
	t.Setenv("CODEX_HOME", filepath.Join(root, "other-codex"))
	if _, err := Sync(context.Background(), options); !errors.Is(err, db.ErrRebuildPending) {
		t.Fatalf("changed default configuration should not complete recovery: %v", err)
	}
	if after := recoverySnapshot(t, path); !reflect.DeepEqual(before, after) {
		t.Fatal("changed source configuration mutated pending data")
	}
	if before.SourceKey == "" || strings.Contains(before.SourceKey, root) {
		t.Fatalf("expected metadata-safe scope fingerprint, got %q", before.SourceKey)
	}
}

func TestRecoveryLockFailuresPreserveCompatibilityClassification(t *testing.T) {
	for _, pending := range []bool{false, true} {
		for _, normalize := range []bool{false, true} {
			t.Run(fmt.Sprintf("pending=%v/normalize=%v", pending, normalize), func(t *testing.T) {
				recoveryDefaultRoots(t)
				path := recoveryOldDatabase(t, false)
				options := SyncOptions{DBPath: path, Harnesses: SupportedHarnesses}
				expected := db.ErrRecoveryRequired
				if pending {
					key, err := recoverySourceKey(options)
					if err != nil {
						t.Fatal(err)
					}
					database, err := sql.Open("sqlite", path)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := database.Exec("UPDATE database_lifecycle SET data_generation = ?, rebuild_pending = 1, rebuild_source_key = ?", db.CurrentDataGeneration, key); err != nil {
						t.Fatal(err)
					}
					_ = database.Close()
					expected = db.ErrRebuildPending
				}
				release, err := db.AcquireWriterLock(context.Background(), path)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if normalize {
					_, err = Normalize(ctx, NormalizeOptions{DBPath: path})
				} else {
					_, err = Sync(ctx, options)
				}
				if !errors.Is(err, expected) || !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("lock failure lost recovery classification: %v", err)
				}
			})
		}
	}
}

func recoveryDefaultRoots(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "xdg"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(root, "claude"))
	return root
}

func recoveryOldDatabase(t *testing.T, legacySchema bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "usage.sqlite")
	root := t.TempDir()
	writePiAssistantSession(t, filepath.Join(root, "date_obsolete.jsonl"), "obsolete", "obsolete", 999, 0)
	if _, err := Sync(context.Background(), SyncOptions{DBPath: path, SourceDir: root, Harnesses: []Harness{HarnessPi}, Normalize: true, Now: time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	database, err := db.OpenWritable(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	statement := "UPDATE database_lifecycle SET data_generation = 0"
	if legacySchema {
		statement = "DROP TABLE database_lifecycle; PRAGMA user_version = 7"
	}
	if _, err := database.Exec(statement); err != nil {
		t.Fatal(err)
	}
	return path
}

type recoveryDatabaseSnapshot struct {
	Counts    []int64
	SourceKey string
}

func recoverySnapshot(t *testing.T, path string) recoveryDatabaseSnapshot {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	var snapshot recoveryDatabaseSnapshot
	for _, query := range []string{
		"PRAGMA user_version",
		"SELECT COUNT(*) FROM raw_token_usage",
		"SELECT COUNT(*) FROM raw_observations",
		"SELECT COUNT(*) FROM ingest_runs",
		"SELECT COALESCE(SUM(total_tokens), 0) FROM canonical_token_usage",
	} {
		var value int64
		if err := database.QueryRow(query).Scan(&value); err != nil {
			t.Fatal(err)
		}
		snapshot.Counts = append(snapshot.Counts, value)
	}
	var lifecycleExists int
	if err := database.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'database_lifecycle'").Scan(&lifecycleExists); err != nil {
		t.Fatal(err)
	}
	if lifecycleExists != 0 {
		var generation, pending, updated int64
		var sourceKey sql.NullString
		if err := database.QueryRow("SELECT data_generation, rebuild_pending, updated_at_ms, rebuild_source_key FROM database_lifecycle WHERE id = 1").Scan(&generation, &pending, &updated, &sourceKey); err != nil {
			t.Fatal(err)
		}
		snapshot.Counts = append(snapshot.Counts, generation, pending, updated)
		snapshot.SourceKey = sourceKey.String
	}
	return snapshot
}
