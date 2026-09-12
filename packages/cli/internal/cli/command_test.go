package cli

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestUnknownCommand(t *testing.T) {
	err := Run(context.Background(), []string{"nope"}, io.Discard, io.Discard, time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown command") || !errors.Is(err, ErrUsage) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommandNamesAndAliasesAreUnique(t *testing.T) {
	owners := make(map[string]string)
	for _, command := range commands {
		for _, name := range append([]string{command.name}, command.aliases...) {
			if owner, exists := owners[name]; exists {
				t.Fatalf("command name %q shared by %q and %q", name, owner, command.name)
			}
			owners[name] = command.name
		}
	}
}

func TestCommandAliasesResolveToCanonicalCommand(t *testing.T) {
	for _, command := range commands {
		for _, name := range append([]string{command.name}, command.aliases...) {
			resolved, ok := commandByName(name)
			if !ok {
				t.Fatalf("command %q not found", name)
			}
			if resolved.name != command.name {
				t.Fatalf("command %q resolved to %q, want %q", name, resolved.name, command.name)
			}
		}
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

func TestLeadingFlagsRouteToView(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.sqlite")
	var launched bool
	restore := replaceInteractiveProgramRunnerForTest(t, func(model interactiveModel, stdout io.Writer) (interactiveModel, error) {
		launched = true
		return model, nil
	})
	defer restore()

	err := Run(context.Background(), []string{"--db-path", dbPath, "--no-sync"}, io.Discard, io.Discard, time.Now())
	if err == nil || !strings.Contains(err.Error(), "db not found") {
		t.Fatalf("expected missing db error, got %v", err)
	}
	if launched {
		t.Fatal("expected TUI not to launch")
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
	defer func() { _ = database.Close() }()
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
