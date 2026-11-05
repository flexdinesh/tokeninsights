package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/serverfeatures"
)

// Command is one `tokeninsights tui` invocation. Resolve applies explicitly
// set configuration flags over environment, config file and defaults.
type Command struct {
	Context context.Context
	Args    []string
	Resolve func(config.Overrides) (config.LocalSettings, error)
	Prepare func(config.LocalSettings) error
	Connect func(context.Context, config.LocalSettings, func() error) (Session, error)
	Now     time.Time
	Stdout  io.Writer
	Stderr  io.Writer
}

// UsageError reports invalid terminal dashboard options. The caller adds its
// own usage line; flag.ErrHelp remains reachable through Unwrap.
type UsageError struct{ Err error }

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// Run attaches one terminal viewer to the shared backend. The backend owns
// storage and capture; closing this viewer cancels only its requests and lease.
func Run(command Command) error {
	options, err := parseViewerOptions(command.Args, command.Stderr, false, periodMonth, nil)
	if err != nil {
		return err
	}
	settings, err := command.Resolve(options.overrides)
	if err != nil {
		return err
	}
	options.dbPath, options.collectorDBPath = settings.ServerDBPath, settings.CollectorDBPath
	if !settings.Endpoint.IsLocal() && options.sourceDir != "" && !filepath.IsAbs(options.sourceDir) {
		return localclient.ErrRelativeRemoteSource
	}
	if settings.Endpoint.TCP != nil && (options.fullRefresh || options.sourceDir != "") {
		return usageError("TCP connections allow default capture only; use local or SSH for source overrides")
	}
	connect := command.Connect
	if connect == nil {
		connect = connectSession
	}
	session, err := connect(command.Context, settings, func() error {
		if command.Prepare != nil {
			return command.Prepare(settings)
		}
		return nil
	})
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	instance, err := session.Queries().Instance(command.Context)
	if err != nil {
		return err
	}
	for _, capability := range []serverfeatures.Capability{serverfeatures.TerminalDashboard, serverfeatures.Usage, serverfeatures.Facets} {
		if !instance.Capabilities.Has(capability) {
			return fmt.Errorf("tui requires %s capability", capability)
		}
	}
	options.local, options.query = session, session.Queries()
	options.connectionLabel = "Connected " + settings.Endpoint.String() + " · " + instance.Hostname + " [" + instance.InstanceID + "]"
	model := newInteractiveModel(command.Context, options, command.Now, instance.Hostname)
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
