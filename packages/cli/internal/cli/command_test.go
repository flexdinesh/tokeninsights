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

func TestRootRejectsViewerFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	err := Run(context.Background(), []string{"--db-path", path, "--no-sync"}, io.Discard, io.Discard, time.Now())
	if !errors.Is(err, ErrUsage) || !strings.Contains(err.Error(), "tokeninsights tui") {
		t.Fatalf("migration error: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("root viewer flags created database")
	}
}

func TestCommandHelpSucceedsWithoutDatabaseSideEffects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	t.Setenv("TOKENINSIGHTS_COLLECTOR_DB_PATH", path)
	t.Setenv("TOKENINSIGHTS_SERVER_DB_PATH", path)
	for _, command := range []string{"tui", "view", "sync", "collector", "normalize", "reset-all", "reset-canonical", "serve"} {
		if err := Run(context.Background(), []string{command, "--help"}, io.Discard, io.Discard, time.Now()); err != nil {
			t.Errorf("%s help: %v", command, err)
		}
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("help created database")
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
