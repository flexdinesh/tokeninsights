package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/browser"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/clientworkflow"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/networkprefs"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

var webCommand = commandSpec{name: "web", run: runWeb}
var openDashboard = browser.Open
var runWebCollector = collector.Run
var ensureWebServer = service.Ensure

func runWeb(invocation commandInvocation, args []string) error {
	settings := invocation.defaults()
	flags := flag.NewFlagSet("tokeninsights web", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var syncBefore bool
	flags.StringVar(&settings.ServerURL, "server-url", settings.ServerURL, "dashboard server; empty selects local")
	flags.StringVar(&settings.ServerDBPath, "server-db-path", settings.ServerDBPath, "local server database")
	flags.StringVar(&settings.CollectorDBPath, "collector-db-path", settings.CollectorDBPath, "collector database")
	flags.StringVar(&settings.Host, "host", settings.Host, "local web/API bind IPv4 address")
	flags.IntVar(&settings.Port, "port", settings.Port, "local web/API port (0 chooses available)")
	flags.BoolVar(&syncBefore, "sync", true, "collect and submit before viewing; --sync=false reads saved data")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument\n%w", ErrUsage)
	}
	settings.ServerURL = strings.TrimSpace(settings.ServerURL)
	options := localOptions(settings.ServerDBPath, settings)
	bindRequested := false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "host":
			options.Host = &settings.Host
			bindRequested = true
		case "port":
			options.Port = &settings.Port
			bindRequested = true
		}
	})
	if bindRequested && settings.ServerURL != "" {
		return fmt.Errorf("--host/--port require a managed local server\n%w", ErrUsage)
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
	session, err := clientworkflow.Resolve(invocation.context, settings, func(ctx context.Context) (service.State, error) { return ensureWebServer(ctx, options) })
	if err != nil {
		return err
	}
	if err := session.Require(serverfeatures.Read, serverfeatures.WebDashboard, serverfeatures.Usage, serverfeatures.Facets); err != nil {
		return err
	}
	open := func() {
		_, _ = fmt.Fprintln(invocation.stdout, "Dashboard: "+session.URL)
		if openDashboard(session.URL) != nil {
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
	observer := session.Observe(invocation.context)
	if settings.ServerKind == serverfeatures.Personal {
		open()
	}
	_, _ = fmt.Fprintln(invocation.stderr, "Syncing usage...")
	terminal := newTerminalSyncProgress(invocation.stderr)
	result, syncErr := runWebCollector(invocation.context, collector.Options{
		CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, ServerURL: settings.ServerURL, Token: settings.ServerToken,
		Destination:      session.Destination,
		SyncOptions:      pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, Normalize: true, Now: invocation.now, Progress: func(event pipeline.SyncProgressEvent) { terminal.Collection(event); observer.Collection(event) }},
		DeliveryProgress: func(progress collector.DeliveryProgress) { terminal.Delivery(progress); observer.Delivery(progress) },
	})
	observer.Finish(result, syncErr)
	terminal.Finish(result)
	printSummary(invocation.stdout, "sync", result.Collection, false)
	printDeliverySummary(invocation.stdout, result)
	if settings.ServerKind == serverfeatures.Hosted {
		open()
	}
	return syncErr
}
