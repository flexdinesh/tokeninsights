package cli

import (
	"context"
	"flag"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/browser"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/clientworkflow"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/networkprefs"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
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
	var syncBefore bool
	openBrowser := flags.Bool("open", true, "open browser automatically")
	flags.StringVar(&settings.Mode, "mode", settings.Mode, "single-process or distributed")
	flags.StringVar(&settings.AppDBPath, "app-db-path", settings.AppDBPath, "application SQLite database")
	flags.StringVar(&settings.ServerURL, "server-url", settings.ServerURL, "dashboard server; empty selects local")
	flags.StringVar(&settings.ServerDBPath, "server-db-path", settings.ServerDBPath, "local server database")
	flags.StringVar(&settings.CollectorDBPath, "collector-db-path", settings.CollectorDBPath, "collector database")
	flags.StringVar(&settings.Host, "host", settings.Host, "local web/API bind IPv4 address")
	flags.IntVar(&settings.Port, "port", settings.Port, "local web/API port (0 chooses available)")
	flags.BoolVar(&syncBefore, "sync", true, "refresh usage in the background; --sync=false reads saved data")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument\n%w", ErrUsage)
	}
	settings.ServerURL = strings.TrimSpace(settings.ServerURL)
	bindRequested := false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "host":
			bindRequested = true
		case "port":
			bindRequested = true
		}
	})
	if bindRequested && settings.ServerURL != "" {
		return fmt.Errorf("--host/--port require single-process mode\n%w", ErrUsage)
	}
	if err := networkprefs.ValidateHost(settings.Host); err != nil {
		return err
	}
	if settings.Port < 0 || settings.Port > 65535 {
		return fmt.Errorf("invalid --port\n%w", ErrUsage)
	}
	if syncBefore && settings.ServerURL == "" {
		if err := collector.ValidatePaths(settings.CollectorDBPath, settings.ServerDBPath); err != nil {
			return err
		}
	}
	if err := settings.ValidateDestination(); err != nil {
		return err
	}
	if settings.EffectiveMode() == config.SingleProcess {
		return runLocalWeb(invocation, settings, syncBefore, *openBrowser)
	}
	session, err := clientworkflow.Resolve(invocation.context, settings)
	if err != nil {
		return err
	}
	if err := session.Require(serverfeatures.Read, serverfeatures.WebDashboard, serverfeatures.Usage, serverfeatures.Facets); err != nil {
		return err
	}
	open := func() {
		_, _ = fmt.Fprintln(invocation.stdout, "Dashboard: "+session.URL)
		if *openBrowser && openDashboard(session.URL) != nil {
			_, _ = fmt.Fprintln(invocation.stderr, "Could not open browser; open dashboard URL above.")
		}
	}
	if !syncBefore {
		open()
		return nil
	}
	if err := session.VerifyIngestion(invocation.context); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(invocation.stderr, "Syncing usage...")
	terminal := newTerminalSyncProgress(invocation.stderr)
	result, syncErr := runWebCollector(invocation.context, collector.Options{
		CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath,
		Destination:      session.Destination,
		SyncOptions:      pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, Now: invocation.now, Progress: terminal.Collection},
		DeliveryProgress: terminal.Delivery,
	})
	terminal.Finish(result)
	printSummary(invocation.stdout, "sync", result.Collection, false)
	printDeliverySummary(invocation.stdout, result)
	open()
	return syncErr
}

func runLocalWeb(invocation commandInvocation, settings config.Settings, syncBefore, openBrowser bool) error {
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
		runtime.StartCollection(ctx, pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, Now: invocation.now}, runWebCollector)
	}
	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return err
	}
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, port)
	handler := server.NewDataHandlerWithOptions(ctx, runtime.Store, invocation.stderr, server.DataHandlerOptions{
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
