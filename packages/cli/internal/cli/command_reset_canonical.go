package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
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
		return fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	if !confirm {
		_, err := fmt.Fprintf(invocation.stdout, "Would delete canonical sessions, messages, token usage, and normalization diagnostics from %s. Re-run with --confirm to apply.\n", strings.TrimSpace(dbPath))
		return err
	}
	path := strings.TrimSpace(dbPath)
	release, err := db.AcquireWriterLock(invocation.context, path)
	if err != nil {
		return err
	}
	defer release()
	// Read-only validation rejects incompatible or unfinished recovery data.
	readOnly, err := db.Open(path)
	if err != nil {
		return err
	}
	if err := readOnly.Close(); err != nil {
		return err
	}
	database, err := db.OpenWritable(path)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()
	if err := db.ResetCanonical(invocation.context, database); err != nil {
		return err
	}
	_, err = fmt.Fprintln(invocation.stdout, "reset-canonical complete")
	return err
}
