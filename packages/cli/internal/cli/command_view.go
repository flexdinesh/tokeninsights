package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/clientworkflow"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

var tuiCommand = commandSpec{name: "tui", run: runView}

var ensureViewServer = service.Ensure
var runViewCollector = collector.Run

func runView(invocation commandInvocation, args []string) error {
	options, err := parseViewerOptionsWithDefaults(args, invocation.stderr, false, periodMonth, nil, invocation.defaults())
	if err != nil {
		return err
	}
	if options.serverKind == serverfeatures.Hosted {
		return fmt.Errorf("tui requires a personal server; use tokeninsights web\n%w", ErrUsage)
	}
	if options.syncBeforeView {
		if options.serverURL == "" {
			if err := collector.ValidatePaths(options.collectorDBPath, options.dbPath); err != nil {
				return err
			}
		}
	}
	if !options.syncBeforeView {
		settings := invocation.defaults()
		settings.ServerURL = options.serverURL
		session, err := clientworkflow.Resolve(invocation.context, settings, func(ctx context.Context) (service.State, error) {
			return ensureViewConfiguredLocal(ctx, options.dbPath, settings)
		})
		if err != nil {
			return err
		}
		if err := session.Require(serverfeatures.Read, serverfeatures.TerminalDashboard, serverfeatures.Usage, serverfeatures.Facets); err != nil {
			return err
		}
		options.serverURL, options.datasetID = session.URL, session.Descriptor.DatasetId
	}
	if options.serverURL != "" {
		if _, err := tableClient(options); err != nil {
			return err
		}
	}
	model := newInteractiveModel(invocation.context, options, invocation.now, "unknown")
	model.localSettings = invocation.defaults()
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
