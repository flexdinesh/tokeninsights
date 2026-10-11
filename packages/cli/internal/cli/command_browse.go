package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
)

var browseCommand = commandSpec{name: "browse", run: runBrowse}

func runBrowse(invocation commandInvocation, args []string) error {
	settings := config.BrowseSettings{}
	if invocation.browse != nil {
		settings = *invocation.browse
	}
	flags := flag.NewFlagSet("tokeninsights browse", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	flags.StringVar(&settings.ServerURL, "server-url", settings.ServerURL, "hosted dashboard URL")
	open := flags.Bool("open", true, "open browser automatically")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected argument\n%w", ErrUsage)
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	url := strings.TrimRight(settings.ServerURL, "/")
	_, _ = fmt.Fprintln(invocation.stdout, "Dashboard: "+url)
	if *open && openDashboard(url) != nil {
		_, _ = fmt.Fprintln(invocation.stderr, "Could not open browser; open dashboard URL above.")
	}
	return nil
}
