package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

func TestViewPerformsImplicitSyncBeforeLaunchingTUI(t *testing.T) {
	sourceRoot := t.TempDir()
	t.Setenv("HOME", filepath.Join(sourceRoot, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(sourceRoot, "xdg"))
	t.Setenv("CODEX_HOME", filepath.Join(sourceRoot, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(sourceRoot, "claude"))

	dbPath := filepath.Join(t.TempDir(), "tokeninsights.sqlite")
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.Local)
	var launched bool

	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) error {
		launched = true
		if model.options.dbPath != dbPath {
			t.Fatalf("interactive dbPath = %q, want %q", model.options.dbPath, dbPath)
		}
		return nil
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
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected implicit sync to create db: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("expected created db to be readable: %v", err)
	}
	defer database.Close()
}

func TestNoCommandPerformsImplicitSyncBeforeLaunchingTUI(t *testing.T) {
	sourceRoot := t.TempDir()
	t.Setenv("HOME", filepath.Join(sourceRoot, "home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(sourceRoot, "xdg"))
	t.Setenv("CODEX_HOME", filepath.Join(sourceRoot, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(sourceRoot, "claude"))

	dbPath := filepath.Join(sourceRoot, "xdg", "tokeninsights", "tokeninsights.sqlite")
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) error {
		launched = true
		if model.options.dbPath != dbPath {
			t.Fatalf("interactive dbPath = %q, want %q", model.options.dbPath, dbPath)
		}
		return nil
	})
	defer restore()

	err := Run(context.Background(), []string{}, io.Discard, io.Discard, time.Date(2026, 6, 19, 10, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	if !launched {
		t.Fatal("expected TUI to launch")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected implicit sync to create default db: %v", err)
	}
}

func TestViewNoSyncPreservesReadOnlyMissingDBBehavior(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.sqlite")
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) error {
		launched = true
		return nil
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

func TestNoCommandNoSyncUsesViewNoSync(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.sqlite")
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) error {
		launched = true
		return nil
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

func TestViewImplicitSyncFailureDoesNotLaunchTUIAndPrintsRecoveryGuidance(t *testing.T) {
	dbPath := t.TempDir()
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) error {
		launched = true
		return nil
	})
	defer restore()

	var stdout bytes.Buffer
	err := Run(context.Background(), []string{"view", "--db-path", dbPath}, &stdout, io.Discard, time.Date(2026, 6, 19, 10, 0, 0, 0, time.Local))
	if err == nil {
		t.Fatal("expected implicit sync error")
	}
	if launched {
		t.Fatal("expected TUI not to launch")
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

func replaceInteractiveProgramRunnerForTest(t *testing.T, runner func(interactiveModel, io.Writer) error) func() {
	t.Helper()
	previous := runInteractiveProgram
	runInteractiveProgram = runner
	return func() {
		runInteractiveProgram = previous
	}
}
