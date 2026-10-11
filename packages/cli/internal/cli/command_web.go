package cli

import (
	"context"
	"flag"
	"fmt"
	"net"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/adapters/sqlanalytics"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/browser"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverruntime"
)

const localVisibilityTimeout = 30 * time.Second

var webCommand = commandSpec{name: "web", run: runWeb}
var openDashboard = browser.Open
var runWebCollector = collector.Run
var serveLocalWeb = serverruntime.Serve

func runWeb(invocation commandInvocation, args []string) error {
	settings := invocation.defaults()
	flags := flag.NewFlagSet("tokeninsights web", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var syncBefore, fullRefresh bool
	var sourceDir string
	openBrowser := flags.Bool("open", true, "open browser automatically")
	flags.StringVar(&settings.AppDBPath, "app-db-path", settings.AppDBPath, "application SQLite database")
	flags.StringVar(&settings.ServerDBPath, "server-db-path", settings.ServerDBPath, "local server database")
	flags.StringVar(&settings.CollectorDBPath, "collector-db-path", settings.CollectorDBPath, "collector database")
	flags.StringVar(&settings.Host, "host", settings.Host, "local web/API bind IPv4 address")
	flags.IntVar(&settings.Port, "port", settings.Port, "local web/API port (0 chooses available)")
	flags.BoolVar(&syncBefore, "sync", true, "refresh usage in the background; --sync=false reads saved data")
	flags.BoolVar(&fullRefresh, "full-refresh", false, "reread sources and retry quarantine at startup")
	flags.StringVar(&sourceDir, "source-dir", "", "override startup capture source directory")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument\n%w", ErrUsage)
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	if !syncBefore && fullRefresh {
		return fmt.Errorf("--full-refresh requires startup collection\n%w", ErrUsage)
	}
	return runLocalWeb(invocation, settings, syncBefore, *openBrowser, pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, Now: invocation.now, FullRefresh: fullRefresh, SourceDir: sourceDir})
}

func runLocalWeb(invocation commandInvocation, settings config.LocalSettings, syncBefore, openBrowser bool, capture pipeline.SyncOptions) error {
	ctx, cancel := context.WithCancel(invocation.context)
	defer cancel()
	listener, err := net.Listen("tcp4", net.JoinHostPort(settings.Host, fmt.Sprint(settings.Port)))
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	runtime, err := localruntime.OpenWithApp(ctx, settings.CollectorDBPath, settings.ServerDBPath, settings.ApplicationPath())
	if err != nil {
		return err
	}
	defer func() { _ = runtime.Close() }()
	if syncBefore {
		runtime.StartCollection(ctx, capture, runWebCollector)
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return err
	}
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, port)
	handler := server.NewDataHandlerWithOptions(ctx, sqlanalytics.Source{Store: runtime.Store}, invocation.stderr, server.DataHandlerOptions{
		Host: settings.Host, InstanceID: runtime.InstanceID, Hostname: runtime.Hostname,
		Policy: runtime.Policy, Progress: runtime.Progress,
	})
	return serveLocalWeb(ctx, runtime.Store, []serverruntime.Binding{{Listener: listener, Handler: handler}}, func() error {
		_, _ = fmt.Fprintln(invocation.stdout, "Dashboard: "+url)
		if openBrowser && openDashboard(url) != nil {
			_, _ = fmt.Fprintln(invocation.stderr, "Could not open browser; open dashboard URL above.")
		}
		return nil
	})
}
