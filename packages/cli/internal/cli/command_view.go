package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
)

var tuiCommand = commandSpec{name: "tui", run: runView}

var runViewCollector = collector.Run

func runView(invocation commandInvocation, args []string) error {
	options, err := parseViewerOptionsWithDefaults(args, invocation.stderr, false, periodMonth, nil, invocation.defaults())
	if err != nil {
		return err
	}
	if options.mode == config.Distributed || options.serverURL != "" {
		return fmt.Errorf("tui requires single-process mode; use tokeninsights web\n%w", ErrUsage)
	}
	settings := invocation.defaults()
	settings.ServerDBPath, settings.AppDBPath = options.dbPath, options.appDBPath
	settings.Mode, settings.ServerURL = options.mode, options.serverURL
	if err := settings.ValidateDestination(); err != nil {
		return err
	}
	runtime, err := localruntime.OpenWithApp(invocation.context, options.collectorDBPath, options.dbPath, settings.ApplicationPath())
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	options.local = runtime
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
