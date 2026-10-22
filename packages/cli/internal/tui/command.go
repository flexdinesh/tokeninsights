package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

// Command is one `tokeninsights tui` invocation. Resolve applies explicitly
// set configuration flags over environment, config file and defaults.
type Command struct {
	Context context.Context
	Args    []string
	Resolve func(config.Overrides) (config.LocalSettings, error)
	Now     time.Time
	Stdout  io.Writer
	Stderr  io.Writer
}

// UsageError reports invalid terminal dashboard options. The caller adds its
// own usage line; flag.ErrHelp remains reachable through Unwrap.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

var runViewCollector = collector.Run

// Run owns the terminal dashboard composition: it opens command-owned local
// storage, starts optional background capture and runs the interactive program.
func Run(command Command) error {
	options, err := parseViewerOptions(command.Args, command.Stderr, false, periodMonth, nil)
	if err != nil {
		return err
	}
	settings, err := command.Resolve(options.overrides)
	if err != nil {
		return err
	}
	options.appDBPath, options.dbPath, options.collectorDBPath = settings.AppDBPath, settings.ServerDBPath, settings.CollectorDBPath
	runtime, err := localruntime.OpenWithAppOptions(command.Context, settings.CollectorDBPath, settings.ServerDBPath, settings.ApplicationPath(), localruntime.Options{CaptureDetails: true})
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	for _, capability := range []serverfeatures.Capability{serverfeatures.TerminalDashboard, serverfeatures.Usage, serverfeatures.Facets} {
		if !runtime.Policy.Capabilities.Has(capability) {
			return fmt.Errorf("tui requires %s capability", capability)
		}
	}
	options.local, options.query = runtime, runtime.Query
	model := newInteractiveModel(command.Context, options, command.Now, runtime.Hostname)
	if options.syncOnStart {
		model = model.startCollection()
	}
	defer model.cancelSync()
	finalModel, err := runInteractiveProgram(model, command.Stdout)
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
