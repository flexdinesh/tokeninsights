package tui

import (
	"context"
	"database/sql"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"io"
	"testing"
	"time"
)

func replaceInteractiveProgramRunnerForTest(t *testing.T, runner func(interactiveModel, io.Writer) (interactiveModel, error)) func() {
	t.Helper()
	previous := runInteractiveProgram
	runInteractiveProgram = runner
	return func() {
		runInteractiveProgram = previous
	}
}

// runCommandForTest invokes the tui command as the CLI does after global
// configuration resolution from the environment.
func runCommandForTest(ctx context.Context, stdout io.Writer, args ...string) error {
	return Run(Command{Context: ctx, Args: args, Settings: config.EnvironmentDefaults(), Now: time.Now(), Stdout: stdout, Stderr: io.Discard})
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
