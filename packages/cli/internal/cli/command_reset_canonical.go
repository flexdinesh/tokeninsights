package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/app"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

var resetCanonicalCommand = commandSpec{name: "reset-canonical", run: runResetCanonical}

func runResetCanonical(invocation commandInvocation, args []string) error {
	flags := flag.NewFlagSet("tokeninsights reset-canonical", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var dbPath string
	var confirm bool
	flags.StringVar(&dbPath, "db-path", defaultDBPath(), "path to tokeninsights sqlite db")
	flags.BoolVar(&confirm, "confirm", false, "confirm deletion")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	if !confirm {
		_, err := fmt.Fprintf(invocation.stdout, "Would delete canonical sessions, messages, token usage, and normalization diagnostics from %s. Re-run with --confirm to apply.\n", strings.TrimSpace(dbPath))
		return err
	}
	path := strings.TrimSpace(dbPath)
	_, err := service.Mutate(invocation.context, path, app.Action{Kind: "reset-canonical"}, nil)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(invocation.stdout, "reset-canonical complete")
	return err
}
