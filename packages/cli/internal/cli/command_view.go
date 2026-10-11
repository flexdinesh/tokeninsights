package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

var tuiCommand = commandSpec{name: "tui", run: runView}

var runViewCollector = collector.Run

func runView(invocation commandInvocation, args []string) error {
	options, err := parseViewerOptionsWithDefaults(args, invocation.stderr, false, periodMonth, nil, invocation.defaults())
	if err != nil {
		return err
	}
	settings := invocation.defaults()
	settings.ServerDBPath, settings.AppDBPath = options.dbPath, options.appDBPath
	runtime, err := localruntime.OpenWithAppOptions(invocation.context, options.collectorDBPath, options.dbPath, settings.ApplicationPath(), localruntime.Options{CaptureDetails: true})
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	for _, capability := range []serverfeatures.Capability{serverfeatures.TerminalDashboard, serverfeatures.Usage, serverfeatures.Facets} {
		if !runtime.Policy.Capabilities.Has(capability) {
			return fmt.Errorf("tui requires %s capability", capability)
		}
	}
	options.local = runtime
	model := newInteractiveModel(invocation.context, options, invocation.now, runtime.Hostname)
	if options.syncOnStart {
		model = model.startCollection()
	}
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
