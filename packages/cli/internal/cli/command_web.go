package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/desktoplaunch"
)

const localVisibilityTimeout = 30 * time.Second

var webCommand = commandSpec{name: "web", run: runWeb}
var openDashboard = browser.Open
var openViewerDashboard = desktoplaunch.Open
var connectBackendSession = desktoplaunch.Connect

func viewerFlags(invocation commandInvocation, name string) (*flag.FlagSet, *bool, *bool, *string) {
	defaults := invocation.defaults()
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	capture := flags.Bool("capture", true, "capture usage on launch; false reads saved data")
	full := flags.Bool("full-refresh", false, "reread sources and retry quarantine")
	source := flags.String("source-dir", "", "capture source directory")
	flags.String("collector-db-path", defaults.CollectorDBPath, "collector database")
	flags.String("server-db-path", defaults.ServerDBPath, "token database")
	flags.String("connection", "local", "explicit local, SSH or TCP connection")
	return flags, capture, full, source
}

func runWeb(invocation commandInvocation, args []string) error {
	flags, capture, full, source := viewerFlags(invocation, "tokeninsights web")
	flags.String("host", "127.0.0.1", "loopback dashboard address")
	flags.Int("port", 0, "dashboard port; 0 chooses an available port")
	open := flags.Bool("open", true, "open browser; false prints URL only")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return ErrUsage
	}
	if !*capture && *full {
		return fmt.Errorf("--full-refresh requires --capture\n%w", ErrUsage)
	}
	overrides, err := config.FlagOverrides(flags)
	if err != nil {
		return err
	}
	settings, err := invocation.resolveLocalOverrides(overrides)
	if err != nil {
		return err
	}
	dashboard, err := openViewerDashboard(invocation.context, desktoplaunch.Specification(settings, *capture, *full, *source), settings.Host, settings.Port,
		func() error { return invocation.prepareLocalStorage(settings) })
	if err != nil {
		return err
	}
	defer func() { _ = dashboard.Close() }()
	printWebReady(invocation.stdout, dashboard.URL)
	if *open {
		if _, err := openDashboard(invocation.context, browser.ModeBrowser, dashboard.URL); err != nil {
			return err
		}
	}
	select {
	case <-invocation.context.Done():
		return nil
	case <-dashboard.Done():
		if invocation.context.Err() != nil {
			return nil
		}
		return errors.New("backend disconnected; rerun tokeninsights web")
	}
}

func printWebReady(output io.Writer, dashboardURL string) {
	_, _ = fmt.Fprintf(output, "TokenInsights ready\nDashboard: %s\nPress Ctrl+C to stop.\n", dashboardURL)
}
