package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

var resetAllCommand = commandSpec{name: "reset-all", run: runResetAll}

func runResetAll(invocation commandInvocation, args []string) error {
	flags := flag.NewFlagSet("tokeninsights reset-all", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var dbPath string
	var confirm bool
	flags.StringVar(&dbPath, "db-path", defaultDBPath(), "path to tokeninsights sqlite db")
	flags.BoolVar(&confirm, "confirm", false, "confirm deletion")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	if !confirm {
		_, err := fmt.Fprintf(invocation.stdout, "Would transactionally reset application tables in %s. Re-run with --confirm to apply.\n", strings.TrimSpace(dbPath))
		return err
	}
	if err := db.ResetAll(strings.TrimSpace(dbPath)); err != nil {
		return err
	}
	_, err := fmt.Fprintln(invocation.stdout, "reset-all complete")
	return err
}
