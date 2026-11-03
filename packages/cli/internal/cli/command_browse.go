package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/core/browser"
)

var browseCommand = commandSpec{name: "browse", run: runBrowse}

func runBrowse(invocation commandInvocation, args []string) error {
	flags := flag.NewFlagSet("tokeninsights browse", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	flags.String("server-url", "", "hosted dashboard URL")
	flags.Var(&browser.ModeFlag{}, "open", "open hosted dashboard in browser; false prints its URL only")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument\n%w", ErrUsage)
	}
	settings, err := invocation.resolveBrowse(flags)
	if err != nil {
		return err
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	url := strings.TrimRight(settings.ServerURL, "/")
	_, _ = fmt.Fprintln(invocation.stdout, "Dashboard: "+url)
	if settings.Open != browser.ModeOff && remoteSession() {
		_, _ = fmt.Fprintln(invocation.stdout, "App: "+browser.AppLink(url))
	}
	mode := browser.ModeBrowser
	if settings.Open == browser.ModeOff {
		mode = browser.ModeOff
	}
	reportOpen(invocation.context, invocation, mode, url)
	return nil
}
