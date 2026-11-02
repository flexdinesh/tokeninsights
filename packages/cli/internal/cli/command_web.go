package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/browser"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/localruntime"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/processusage"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/server"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverruntime"
	"github.com/muesli/termenv"
)

const localVisibilityTimeout = 30 * time.Second

var webCommand = commandSpec{name: "web", run: runWeb}
var openDashboard = browser.Open
var remoteSession = browser.RemoteSession
var runWebCollector = collector.Run
var serveLocalWeb = serverruntime.Serve

func runWeb(invocation commandInvocation, args []string) error {
	defaults := invocation.defaults()
	flags := flag.NewFlagSet("tokeninsights web", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var syncBefore, fullRefresh bool
	var sourceDir string
	flags.Var(&browser.ModeFlag{}, "open", "open dashboard: auto (desktop app, else browser), app, browser or false")
	flags.String("app-db-path", defaults.AppDBPath, "application SQLite database")
	flags.String("server-db-path", defaults.ServerDBPath, "local server database")
	flags.String("collector-db-path", defaults.CollectorDBPath, "collector database")
	flags.String("host", defaults.Host, "local web/API bind IPv4 address")
	flags.Int("port", defaults.Port, "local web/API port (0 chooses available)")
	flags.BoolVar(&syncBefore, "sync", true, "refresh usage in the background; --sync=false reads saved data")
	flags.BoolVar(&fullRefresh, "full-refresh", false, "reread sources and retry quarantine at startup")
	flags.StringVar(&sourceDir, "source-dir", "", "override startup capture source directory")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument\n%w", ErrUsage)
	}
	settings, err := invocation.resolveLocal(flags)
	if err != nil {
		return err
	}
	if !syncBefore && fullRefresh {
		return fmt.Errorf("--full-refresh requires startup collection\n%w", ErrUsage)
	}
	if err := invocation.prepareLocalStorage(settings); err != nil {
		return err
	}
	return runLocalWeb(invocation, settings, syncBefore, pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, Now: invocation.now, FullRefresh: fullRefresh, SourceDir: sourceDir})
}

func runLocalWeb(invocation commandInvocation, settings config.LocalSettings, syncBefore bool, capture pipeline.SyncOptions) error {
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
	handler := server.NewDataHandlerWithOptions(ctx, runtime.DataSource(), invocation.stderr, server.DataHandlerOptions{
		Host: settings.Host, InstanceID: runtime.InstanceID, Hostname: runtime.Hostname,
		Policy: runtime.Policy, Progress: runtime.Progress, ProcessUsage: processusage.New(),
	})
	return serveLocalWeb(ctx, runtime.Store, []serverruntime.Binding{{Listener: listener, Handler: handler}}, func() error {
		remote := settings.Open != browser.ModeOff && remoteSession()
		printWebReady(invocation.stdout, url, remote)
		reportOpen(ctx, invocation, settings.Open, url)
		return nil
	})
}

// reportOpen opens the dashboard; failures leave the printed URL as recovery.
func reportOpen(ctx context.Context, invocation commandInvocation, mode browser.Mode, url string) {
	if mode == browser.ModeOff {
		return
	}
	result, err := openDashboard(ctx, mode, url)
	switch {
	case errors.Is(err, browser.ErrAppUnavailable):
		_, _ = fmt.Fprintln(invocation.stderr, err.Error()+"; open dashboard URL above.")
	case err != nil:
		_, _ = fmt.Fprintln(invocation.stderr, "Could not open browser; open dashboard URL above.")
	case result.Via == browser.ViaApp:
		_, _ = fmt.Fprintln(invocation.stderr, "Opened in the TokenInsights app. Use --open=browser for the browser.")
	}
}

// printWebReady prints the dashboard URL. Remote (SSH) sessions also get the
// desktop app link and, for loopback dashboards, an SSH tunnel hint.
func printWebReady(output io.Writer, dashboardURL string, remote bool) {
	renderer := lipgloss.NewRenderer(output)
	title := renderer.NewStyle().Bold(true)
	ready := renderer.NewStyle().Foreground(lipgloss.Color("2"))
	link := renderer.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	hint := renderer.NewStyle().Faint(true)
	_, _ = fmt.Fprintf(output, "\n  %s  %s\n\n  Dashboard:  %s\n",
		title.Render("TokenInsights"), ready.Render("ready"), link.Render(dashboardURL))
	if remote {
		appLink := browser.AppLink(dashboardURL)
		_, _ = fmt.Fprintf(output, "  App:        %s\n", hyperlink(renderer, appLink, link.Render(appLink)))
		if tunnel := tunnelHint(dashboardURL); tunnel != "" {
			_, _ = fmt.Fprintf(output, "  Tunnel:     %s\n", tunnel)
		}
		_, _ = fmt.Fprintf(output, "\n  %s\n", hint.Render("Open these on your own machine; this SSH session cannot open them."))
	}
	_, _ = fmt.Fprintf(output, "\n  %s\n\n", hint.Render("Press Ctrl+C to stop."))
}

// hyperlink wraps text in an OSC 8 link on styled terminals, so custom schemes are clickable.
func hyperlink(renderer *lipgloss.Renderer, target, text string) string {
	if renderer.ColorProfile() == termenv.Ascii {
		return text
	}
	return ansi.SetHyperlink(target) + text + ansi.ResetHyperlink()
}

// tunnelHint returns an ssh -L command for loopback dashboards.
func tunnelHint(dashboardURL string) string {
	parsed, err := url.Parse(dashboardURL)
	if err != nil {
		return ""
	}
	address, err := netip.ParseAddr(parsed.Hostname())
	if err != nil || !address.IsLoopback() {
		return ""
	}
	port := parsed.Port()
	return "ssh -L " + port + ":" + net.JoinHostPort(parsed.Hostname(), port) + " <host>"
}
