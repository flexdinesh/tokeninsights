package tui

import (
	"context"
	"database/sql"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
)

func replaceInteractiveProgramRunnerForTest(t *testing.T, runner func(interactiveModel, io.Writer) (interactiveModel, error)) func() {
	t.Helper()
	previous := runInteractiveProgram
	runInteractiveProgram = runner
	return func() {
		runInteractiveProgram = previous
	}
}

// runCommandForTest invokes the tui command as the CLI does, resolving flags
// over the environment and an absent config file.
func runCommandForTest(t *testing.T, ctx context.Context, stdout io.Writer, args ...string) error {
	t.Helper()
	return Run(Command{Context: ctx, Args: args, Resolve: resolveForTest(t), Now: time.Now(), Stdout: stdout, Stderr: io.Discard})
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

func resolveForTest(t *testing.T) func(config.Overrides) (config.LocalSettings, error) {
	path := filepath.Join(t.TempDir(), "config.json")
	return func(overrides config.Overrides) (config.LocalSettings, error) {
		return config.ResolveLocal(path, true, overrides)
	}
}
