package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/version"
)

func Run(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	if len(args) == 0 {
		return RunInteractive(ctx, nil, stdout, stderr, now)
	}

	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(stdout, usageText())
		return nil
	case "--version", "version":
		fmt.Fprintln(stdout, version.String())
		return nil
	case "view":
		return RunInteractive(ctx, args[1:], stdout, stderr, now)
	case "sync":
		return runSync(ctx, args[1:], stdout, stderr, now)
	case "normalize":
		return runNormalize(ctx, args[1:], stdout, stderr, now)
	case "reset-canonical":
		return runResetCanonical(ctx, args[1:], stdout, stderr)
	case "reset-all":
		return runResetAll(args[1:], stdout, stderr)
	default:
		if strings.HasPrefix(args[0], "-") {
			return RunInteractive(ctx, args, stdout, stderr, now)
		}
		return fmt.Errorf("unknown command %q\n%w", args[0], ErrUsage)
	}
}

func runSync(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	flags := flag.NewFlagSet("tokeninsights sync", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var dbPath string
	var all bool
	var dryRun bool
	var noNormalize bool
	var sourceDir string
	var harnesses stringList
	flags.StringVar(&dbPath, "db-path", defaultDBPath(), "path to tokeninsights sqlite db")
	flags.Var(&harnesses, "harness", "harness to sync: opencode, pi, codex, or claude-code")
	flags.BoolVar(&all, "all", false, "sync all supported harnesses")
	flags.BoolVar(&dryRun, "dry-run", false, "discover and parse without writing")
	flags.BoolVar(&noNormalize, "no-normalize", false, "skip canonical normalization after raw ingest")
	flags.StringVar(&sourceDir, "source-dir", "", "override harness source directory")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	selectedHarnesses, err := syncHarnesses(all, harnesses)
	if err != nil {
		return err
	}
	summary, err := pipeline.Sync(ctx, pipeline.SyncOptions{
		DBPath:    strings.TrimSpace(dbPath),
		Harnesses: selectedHarnesses,
		DryRun:    dryRun,
		Normalize: !noNormalize,
		SourceDir: strings.TrimSpace(sourceDir),
		Now:       now,
	})
	printSummary(stdout, "sync", summary, dryRun)
	if err != nil {
		return err
	}
	return nil
}

func runNormalize(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer, now time.Time) error {
	flags := flag.NewFlagSet("tokeninsights normalize", flag.ContinueOnError)
	flags.SetOutput(stderr)
	var dbPath string
	var dryRun bool
	var harnesses stringList
	flags.StringVar(&dbPath, "db-path", defaultDBPath(), "path to tokeninsights sqlite db")
	flags.BoolVar(&dryRun, "dry-run", false, "compute without writing")
	flags.Var(&harnesses, "harness", "optional harness filter: opencode, pi, codex, or claude-code")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%v\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	if err := validateHarnesses(harnesses); err != nil {
		return err
	}
	summary, err := pipeline.Normalize(ctx, pipeline.NormalizeOptions{
		DBPath:    strings.TrimSpace(dbPath),
		DryRun:    dryRun,
		Harnesses: harnessList(harnesses),
		Now:       now,
	})
	printSummary(stdout, "normalize", summary, dryRun)
	return err
}

func runResetCanonical(ctx context.Context, args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("tokeninsights reset-canonical", flag.ContinueOnError)
	flags.SetOutput(stderr)
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
		fmt.Fprintf(stdout, "Would delete canonical sessions, messages, token usage, and normalization diagnostics from %s. Re-run with --confirm to apply.\n", strings.TrimSpace(dbPath))
		return nil
	}
	database, err := db.OpenWritable(strings.TrimSpace(dbPath))
	if err != nil {
		return err
	}
	defer database.Close()
	if err := db.ResetCanonical(ctx, database); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "reset-canonical complete")
	return nil
}

func runResetAll(args []string, stdout io.Writer, stderr io.Writer) error {
	flags := flag.NewFlagSet("tokeninsights reset-all", flag.ContinueOnError)
	flags.SetOutput(stderr)
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
		fmt.Fprintf(stdout, "Would delete and recreate %s plus SQLite sidecars. Re-run with --confirm to apply.\n", strings.TrimSpace(dbPath))
		return nil
	}
	if err := db.ResetAll(strings.TrimSpace(dbPath)); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "reset-all complete")
	return nil
}

func syncHarnesses(all bool, values stringList) ([]pipeline.Harness, error) {
	if all && len(values) > 0 {
		return nil, fmt.Errorf("choose either --all or --harness, not both\n%w", ErrUsage)
	}
	if !all && len(values) == 0 {
		return nil, fmt.Errorf("choose --all or --harness <harness>\n%w", ErrUsage)
	}
	if all {
		return pipeline.SupportedHarnesses, nil
	}
	if err := validateHarnesses(values); err != nil {
		return nil, err
	}
	return harnessList(values), nil
}

func harnessList(values stringList) []pipeline.Harness {
	result := make([]pipeline.Harness, 0, len(values))
	for _, value := range values {
		result = append(result, pipeline.Harness(value))
	}
	return result
}

func printSummary(stdout io.Writer, command string, summary pipeline.Summary, dryRun bool) {
	prefix := command
	if dryRun {
		prefix += " dry-run"
	}
	fmt.Fprintf(stdout, "%s: requested=%d synced=%d skipped=%d failed=%d raw_facts=%d observations=%d canonical=%d diagnostics=%d\n",
		prefix,
		summary.RequestedHarnesses,
		summary.Synced,
		summary.Skipped,
		summary.Failed,
		summary.RawFacts,
		summary.Observations,
		summary.Canonical,
		summary.Diagnostics,
	)
}

func usageText() string {
	return `usage: tokeninsights <command> [options]

commands:
  sync              ingest local harness data
  normalize         rebuild canonical facts from raw facts
  reset-canonical   delete canonical facts and diagnostics
  reset-all         recreate the local database
  view              open the interactive TUI`
}
