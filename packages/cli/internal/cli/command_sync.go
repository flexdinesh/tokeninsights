package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

var syncCommand = commandSpec{name: "sync", run: runSync}

func runSync(invocation commandInvocation, args []string) error {
	flags := flag.NewFlagSet("tokeninsights sync", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var dbPath string
	var all bool
	var dryRun bool
	var fullRefresh bool
	var noNormalize bool
	var sourceDir string
	var harnesses stringList
	flags.StringVar(&dbPath, "db-path", defaultDBPath(), "path to tokeninsights sqlite db")
	flags.Var(&harnesses, "harness", "harness to sync: opencode, pi, codex, or claude-code")
	flags.BoolVar(&all, "all", false, "sync all supported harnesses")
	flags.BoolVar(&dryRun, "dry-run", false, "discover and parse without writing")
	flags.BoolVar(&fullRefresh, "full-refresh", false, "ignore source refresh state and parse discovered sources")
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
	summary, err := pipeline.Sync(invocation.context, pipeline.SyncOptions{
		DBPath:      strings.TrimSpace(dbPath),
		Harnesses:   selectedHarnesses,
		DryRun:      dryRun,
		FullRefresh: fullRefresh,
		Normalize:   !noNormalize,
		SourceDir:   strings.TrimSpace(sourceDir),
		Now:         invocation.now,
		Progress:    recoveryNotice(invocation.stderr),
	})
	printSummary(invocation.stdout, "sync", summary, dryRun)
	if err != nil {
		return err
	}
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
