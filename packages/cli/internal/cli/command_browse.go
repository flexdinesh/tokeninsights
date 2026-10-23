package cli

import (
	"flag"
	"fmt"
	"strings"
)

var browseCommand = commandSpec{name: "browse", run: runBrowse}

func runBrowse(invocation commandInvocation, args []string) error {
	flags := flag.NewFlagSet("tokeninsights browse", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	flags.String("server-url", "", "hosted dashboard URL")
	open := flags.Bool("open", true, "open browser automatically")
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
	if *open && openDashboard(url) != nil {
		_, _ = fmt.Fprintln(invocation.stderr, "Could not open browser; open dashboard URL above.")
	}
	return nil
}
