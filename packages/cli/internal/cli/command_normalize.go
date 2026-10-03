package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/app"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
)

var normalizeCommand = commandSpec{name: "normalize", run: runNormalize}

func runNormalize(invocation commandInvocation, args []string) error {
	flags := flag.NewFlagSet("tokeninsights normalize", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var dbPath string
	var dryRun bool
	var harnesses stringList
	flags.StringVar(&dbPath, "db-path", defaultDBPath(), "path to tokeninsights sqlite db")
	flags.BoolVar(&dryRun, "dry-run", false, "compute without writing")
	flags.Var(&harnesses, "harness", "optional harness filter: opencode, pi, codex, or claude-code")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	if err := validateHarnesses(harnesses); err != nil {
		return err
	}
	sources, err := pipeline.ResolveSources("")
	if err != nil {
		return err
	}
	var summary pipeline.Summary
	if dryRun {
		summary, err = pipeline.Normalize(invocation.context, pipeline.NormalizeOptions{
			DBPath:    strings.TrimSpace(dbPath),
			DryRun:    dryRun,
			Harnesses: harnessList(harnesses),
			Now:       invocation.now,
			Progress:  recoveryNotice(invocation.stderr),
		})
	} else {
		action := app.Action{Kind: "normalize", Sources: sources, Now: invocation.now, Harnesses: harnessList(harnesses)}
		summary, err = service.Mutate(invocation.context, strings.TrimSpace(dbPath), action, recoveryNotice(invocation.stderr))
	}

	printSummary(invocation.stdout, "normalize", summary, dryRun)
	return err
}
