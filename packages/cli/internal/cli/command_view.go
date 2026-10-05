package cli

import (
	"context"
	"errors"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

var tuiCommand = commandSpec{name: "tui", run: runView}

var ensureViewServer = service.Ensure
var runViewCollector = collector.Run

func runView(invocation commandInvocation, args []string) error {
	options, err := parseTableOptions(args, invocation.stderr, false, periodMonth)
	if err != nil {
		return err
	}
	if options.syncBeforeView {
		if err := collector.ValidatePaths(options.collectorDBPath, options.dbPath); err != nil {
			return err
		}
	}
	if !options.syncBeforeView && options.serverURL == "" {
		state, err := ensureViewServer(invocation.context, service.Options{DBPath: options.dbPath})
		if err != nil {
			return err
		}
		if state.Record == nil {
			return errors.New("local query server unavailable")
		}
		options.serverURL = state.Record.URL
	}
	if options.serverURL != "" {
		if _, err := tableClient(options); err != nil {
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
	var initial tea.Model = model
	if model.options.syncBeforeView {
		startup := newStartupModel(model)
		initial = startup
		defer func() {
			model.cancelSync()
			startup.workers.Wait()
		}()
	}
	finalModel, err := tea.NewProgram(initial, tea.WithAltScreen(), tea.WithInput(os.Stdin), tea.WithOutput(stdout)).Run()
	if err != nil {
		return model, err
	}
	if interactive, ok := finalModel.(interactiveModel); ok {
		return interactive, nil
	}
	if startup, ok := finalModel.(startupModel); ok {
		return startup.dashboard, nil
	}
	return model, nil
}
