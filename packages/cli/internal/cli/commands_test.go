package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"

	_ "modernc.org/sqlite"
)

func TestViewLaunchesProgressTUIBeforeImplicitSyncCompletes(t *testing.T) {
	sourceRoot := t.TempDir()
	t.Setenv("HOME", filepath.Join(sourceRoot, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(sourceRoot, "xdg"))
	t.Setenv("CODEX_HOME", filepath.Join(sourceRoot, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(sourceRoot, "claude"))

	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.Local)
	var launched bool

	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) (interactiveModel, error) {
		launched = true
		if model.options.dbPath != dbPath {
			t.Fatalf("interactive dbPath = %q, want %q", model.options.dbPath, dbPath)
		}
		if !model.syncing {
			t.Fatal("expected initial model to show sync progress")
		}
		if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected db not to exist before TUI launches, stat error = %v", err)
		}
		return model, nil
	})
	defer restore()

	var stdout bytes.Buffer
	err := Run(context.Background(), []string{"view", "--db-path", dbPath}, &stdout, io.Discard, now)
	if err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "" {
		t.Fatalf("implicit sync success printed %q, want no pre-launch output", stdout.String())
	}
	if !launched {
		t.Fatal("expected TUI to launch")
	}
}

func TestNoCommandLaunchesProgressTUIBeforeImplicitSyncCompletes(t *testing.T) {
	sourceRoot := t.TempDir()
	t.Setenv("HOME", filepath.Join(sourceRoot, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(sourceRoot, "xdg"))
	t.Setenv("CODEX_HOME", filepath.Join(sourceRoot, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(sourceRoot, "claude"))

	dbPath := filepath.Join(sourceRoot, "xdg", "tokeninsights", "tokeninsights.sqlite")
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) (interactiveModel, error) {
		launched = true
		if model.options.dbPath != dbPath {
			t.Fatalf("interactive dbPath = %q, want %q", model.options.dbPath, dbPath)
		}
		if !model.syncing {
			t.Fatal("expected initial model to show sync progress")
		}
		if _, err := os.Stat(dbPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected default db not to exist before TUI launches, stat error = %v", err)
		}
		return model, nil
	})
	defer restore()

	err := Run(context.Background(), []string{}, io.Discard, io.Discard, time.Date(2026, 6, 19, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if !launched {
		t.Fatal("expected TUI to launch")
	}
}

func TestViewNoSyncPreservesReadOnlyMissingDBBehavior(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.sqlite")
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) (interactiveModel, error) {
		launched = true
		return model, nil
	})
	defer restore()

	var stdout bytes.Buffer
	err := Run(context.Background(), []string{"view", "--db-path", dbPath, "--no-sync"}, &stdout, io.Discard, time.Date(2026, 6, 19, 10, 0, 0, 0, time.Local))
	if err == nil {
		t.Fatal("expected missing db error")
	}
	if !strings.Contains(err.Error(), "db not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if launched {
		t.Fatal("expected TUI not to launch")
	}
	if _, statErr := os.Stat(dbPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected no db to be created, stat error = %v", statErr)
	}
	if stdout.String() != "" {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestViewNoSyncLaunchesTableLoadingWithoutSyncProgress(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	if err := db.ResetAll(dbPath); err != nil {
		t.Fatal(err)
	}
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) (interactiveModel, error) {
		launched = true
		if model.syncing {
			t.Fatal("expected --no-sync not to show sync progress")
		}
		if !model.loading {
			t.Fatal("expected --no-sync to use table loading state")
		}
		return model, nil
	})
	defer restore()

	err := Run(context.Background(), []string{"view", "--db-path", dbPath, "--no-sync"}, io.Discard, io.Discard, time.Date(2026, 6, 19, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if !launched {
		t.Fatal("expected TUI to launch")
	}
}

func TestViewNoSyncDoesNotProcessPendingNormalizationWork(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "opencode", "opencode.db")
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	createOpenCodeSQLiteMessagesForCLI(t, sourcePath)

	var stdout bytes.Buffer
	err := Run(ctx, []string{"sync", "--db-path", dbPath, "--harness", "opencode", "--source-dir", sourceDir, "--no-normalize"}, &stdout, io.Discard, now)
	if err != nil {
		t.Fatal(err)
	}

	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) (interactiveModel, error) {
		launched = true
		if model.syncing {
			t.Fatal("expected --no-sync not to start implicit sync")
		}
		return model, nil
	})
	defer restore()

	err = Run(ctx, []string{"view", "--db-path", dbPath, "--no-sync"}, io.Discard, io.Discard, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !launched {
		t.Fatal("expected TUI to launch")
	}
	database, err := db.OpenWritable(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	assertCLIQueryCount(t, database, "SELECT COUNT(*) FROM normalization_work_queue WHERE domain = 'token_usage'", 1)
	assertCLIQueryCount(t, database, "SELECT COUNT(*) FROM canonical_token_usage", 0)
}

func TestNoCommandNoSyncUsesViewNoSync(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.sqlite")
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) (interactiveModel, error) {
		launched = true
		return model, nil
	})
	defer restore()

	err := Run(context.Background(), []string{"--db-path", dbPath, "--no-sync"}, io.Discard, io.Discard, time.Date(2026, 6, 19, 10, 0, 0, 0, time.Local))
	if err == nil {
		t.Fatal("expected missing db error")
	}
	if !strings.Contains(err.Error(), "db not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if launched {
		t.Fatal("expected TUI not to launch")
	}
}

func TestViewImplicitSyncFailureExitsTUIAndPrintsRecoveryGuidance(t *testing.T) {
	dbPath := t.TempDir()
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) (interactiveModel, error) {
		launched = true
		model.syncSummary = pipeline.Summary{RequestedHarnesses: 4, Failed: 1}
		model.syncErr = errors.New("db path is a directory")
		return model, nil
	})
	defer restore()

	var stdout bytes.Buffer
	err := Run(context.Background(), []string{"view", "--db-path", dbPath}, &stdout, io.Discard, time.Date(2026, 6, 19, 10, 0, 0, 0, time.Local))
	if err == nil {
		t.Fatal("expected implicit sync error")
	}
	if !launched {
		t.Fatal("expected TUI to launch before sync failure")
	}
	if !strings.Contains(stdout.String(), "sync: requested=") {
		t.Fatalf("stdout missing sync summary: %q", stdout.String())
	}
	if !strings.Contains(err.Error(), "db path is a directory") {
		t.Fatalf("error missing sync failure: %v", err)
	}
	if !strings.Contains(err.Error(), "tokeninsights sync --harness <harness>") {
		t.Fatalf("error missing targeted harness guidance: %v", err)
	}
	if !strings.Contains(err.Error(), "tokeninsights view --no-sync") {
		t.Fatalf("error missing no-sync guidance: %v", err)
	}
}

func TestSyncFullRefreshFlagForcesSourceRefresh(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "opencode", "opencode.db")
	now := time.Date(2026, 4, 24, 15, 0, 0, 0, time.UTC)
	createOpenCodeSQLiteMessagesForCLI(t, sourcePath)
	setFileModTimeForCLI(t, sourcePath, now.Add(-72*time.Hour))

	var stdout bytes.Buffer
	err := Run(ctx, []string{"sync", "--db-path", dbPath, "--harness", "opencode", "--source-dir", sourceDir}, &stdout, io.Discard, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=1 skipped=0 failed=0 raw_facts=1 observations=1 canonical=1 diagnostics=0") {
		t.Fatalf("unexpected first sync output: %q", stdout.String())
	}

	stdout.Reset()
	err = Run(ctx, []string{"sync", "--db-path", dbPath, "--harness", "opencode", "--source-dir", sourceDir}, &stdout, io.Discard, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=0 skipped=1 failed=0 raw_facts=0 observations=0 canonical=0 diagnostics=0") {
		t.Fatalf("unexpected skipped sync output: %q", stdout.String())
	}

	stdout.Reset()
	err = Run(ctx, []string{"sync", "--db-path", dbPath, "--harness", "opencode", "--source-dir", sourceDir, "--full-refresh"}, &stdout, io.Discard, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "sync: requested=1 synced=1 skipped=0 failed=0 raw_facts=0 observations=1 canonical=0 diagnostics=0") {
		t.Fatalf("unexpected full-refresh output: %q", stdout.String())
	}
}

func replaceInteractiveProgramRunnerForTest(t *testing.T, runner func(interactiveModel, io.Writer) (interactiveModel, error)) func() {
	t.Helper()
	previous := runInteractiveProgram
	runInteractiveProgram = runner
	return func() {
		runInteractiveProgram = previous
	}
}

func createOpenCodeSQLiteMessagesForCLI(t *testing.T, dbPath string) {
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
	if _, err := database.Exec(`
		INSERT INTO message (id, session_id, time_created, time_updated, data)
		VALUES (?, ?, ?, ?, ?)
	`, "m1", "oc_s1", 1770000000000, 1770000000000, `{"role":"assistant","providerID":"openai","modelID":"gpt-5","tokens":{"input":100,"output":50},"time":{"created":1770000000000}}`); err != nil {
		t.Fatal(err)
	}
}

func setFileModTimeForCLI(t *testing.T, path string, modTime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatal(err)
	}
}

func assertCLIQueryCount(t *testing.T, database *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(query).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s: got %d, want %d", query, got, want)
	}
}
