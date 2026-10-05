package cli

import (
	"context"
	"errors"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

var tuiCommand = commandSpec{name: "tui", run: runView}

var ensureViewServer = service.Ensure
var runViewSync = runSync

func runView(invocation commandInvocation, args []string) error {
	options, err := parseTableOptions(args, invocation.stderr, false, periodMonth)
	if err != nil {
		return err
	}
	remoteURL := options.serverURL

	if options.serverURL == "" {
		state, err := ensureViewServer(invocation.context, service.Options{DBPath: options.dbPath})
		if err != nil {
			return err
		}
		if state.Record == nil {
			return errors.New("local query server unavailable")
		}
		options.serverURL = state.Record.URL
	}
	if _, err := tableClient(options); err != nil {
		return err
	}
	if options.syncBeforeView {
		syncArgs := []string{"--collector-db-path", options.collectorDBPath, "--server-db-path", options.dbPath}
		if remoteURL != "" {
			syncArgs = append(syncArgs, "--server-url", remoteURL)
		}
		if options.token != "" {
			syncArgs = append(syncArgs, "--token", options.token)
		}
		if err := runViewSync(invocation, syncArgs); err != nil {
			return err
		}
	}
	model := newInteractiveModel(invocation.context, options, invocation.now, "unknown")
	defer model.cancelSync()
	finalModel, err := runInteractiveProgram(model, invocation.stdout)
	if err != nil {
		return err
	}
	if errors.Is(finalModel.err, context.Canceled) && finalModel.ctx.Err() != nil {
		return nil
	}
	return finalModel.err
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
