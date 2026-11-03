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
	"github.com/flexdinesh/tokeninsights/packages/core/browser"
	"github.com/flexdinesh/tokeninsights/packages/core/config"
	"github.com/flexdinesh/tokeninsights/packages/core/localclient"
	"github.com/flexdinesh/tokeninsights/packages/core/pipeline"
	"github.com/muesli/termenv"
)

const localVisibilityTimeout = 30 * time.Second

var webCommand = commandSpec{name: "web", run: runWeb}
var openDashboard = browser.Open
var remoteSession = browser.RemoteSession
var connectLocalBackend = localclient.ConnectPrepared

func runWeb(invocation commandInvocation, args []string) error {
	defaults := invocation.defaults()
	flags := flag.NewFlagSet("tokeninsights web", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var syncBefore, fullRefresh bool
	var sourceDir string
	flags.Var(&browser.ModeFlag{}, "open", "open dashboard: mygo, electron, browser, auto or false")
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
	return runLocalWeb(invocation, settings, syncBefore, pipeline.SyncOptions{Harnesses: pipeline.SupportedHarnesses, Now: invocation.now, FullRefresh: fullRefresh, SourceDir: sourceDir})
}

func runLocalWeb(invocation commandInvocation, settings config.LocalSettings, syncBefore bool, capture pipeline.SyncOptions) error {
	ctx, cancel := context.WithCancel(invocation.context)
	defer cancel()
	session, err := connectLocalBackend(ctx, localclient.Options{CollectorDBPath: settings.CollectorDBPath, ServerDBPath: settings.ServerDBPath, AppDBPath: settings.ApplicationPath(), Host: settings.Host, Port: settings.Port, Capture: syncBefore, FullRefresh: capture.FullRefresh, SourceDir: capture.SourceDir}, func() error { return invocation.prepareLocalStorage(settings) })
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()
	remote := settings.Open != browser.ModeOff && remoteSession()
	printWebReady(invocation.stdout, session.URL, remote)
	reportOpen(ctx, invocation, settings.Open, session.URL)
	select {
	case <-ctx.Done():
		return nil
	case <-session.Done():
		return errors.New("local backend disconnected")
	}
}

// reportOpen opens the dashboard; failures leave the printed URL as recovery.
func reportOpen(ctx context.Context, invocation commandInvocation, mode browser.Mode, url string) {
	if mode == browser.ModeOff {
		return
	}
	result, err := openDashboard(ctx, mode, url)
	switch {
	case errors.Is(err, browser.ErrAppUnavailable), errors.Is(err, browser.ErrElectronUnavailable):
		_, _ = fmt.Fprintln(invocation.stderr, err.Error()+"; open dashboard URL above.")
	case err != nil:
		_, _ = fmt.Fprintln(invocation.stderr, "Could not open browser; open dashboard URL above.")
	case result.Via == browser.ViaApp:
		_, _ = fmt.Fprintln(invocation.stderr, "Opened in TokenInsights MyGo. Use --open=browser for the browser.")
	case result.Via == browser.ViaElectron:
		_, _ = fmt.Fprintln(invocation.stderr, "Opened in TokenInsights Electron. Use --open=browser for the browser.")
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
