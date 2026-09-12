package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

var viewCommand = commandSpec{name: "view", run: runView}

func runView(invocation commandInvocation, args []string) error {
	options, err := parseTableOptions(args, invocation.stderr, false, periodMonth)
	if err != nil {
		return err
	}

	if options.noSync {
		database, err := db.Open(options.dbPath)
		if err != nil {
			return err
		}
		_ = database.Close()
	}

	hostname, hostnameErr := os.Hostname()
	model := newInteractiveModel(invocation.context, options, invocation.now, normalizeHostname(hostname, hostnameErr))
	finalModel, err := runInteractiveProgram(model, invocation.stdout)
	if err != nil {
		return err
	}
	if finalModel.syncErr != nil {
		printSummary(invocation.stdout, "sync", finalModel.syncSummary, false)
		if errors.Is(finalModel.syncErr, db.ErrRebuildPending) || errors.Is(finalModel.syncErr, db.ErrRecoveryRequired) {
			return fmt.Errorf("%w\n\nusage recovery is incomplete; retry `tokeninsights sync --all` with the original --db-path, --source-dir (if used), and source environment settings", finalModel.syncErr)
		}
		return fmt.Errorf("%w\n\nimplicit view sync failed; to refresh unaffected harnesses manually, run `tokeninsights sync --harness <harness>`, then open the existing canonical data with `tokeninsights view --no-sync`", finalModel.syncErr)
	}
	return nil
}

var runInteractiveProgram = func(model interactiveModel, stdout io.Writer) (interactiveModel, error) {
	finalModel, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithInput(os.Stdin), tea.WithOutput(stdout)).Run()
	if err != nil {
		return model, err
	}
	if interactive, ok := finalModel.(interactiveModel); ok {
		return interactive, nil
	}
	return model, nil
}
