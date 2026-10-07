package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/clientworkflow"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/service"
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
	var serverDBPath, serverURL, token string
	var publishOnly bool
	settings := invocation.defaults()
	flags.StringVar(&dbPath, "collector-db-path", settings.CollectorDBPath, "collector SQLite database")
	flags.StringVar(&serverDBPath, "server-db-path", settings.ServerDBPath, "local server DuckDB database")
	flags.StringVar(&serverURL, "server-url", settings.ServerURL, "ingestion server; empty selects local")
	flags.BoolVar(&publishOnly, "publish-only", false, "submit retained raw evidence without collecting")
	flags.Var(&harnesses, "harness", "harness to sync: opencode, pi, codex, or claude-code")
	flags.BoolVar(&all, "all", false, "sync all supported harnesses")
	flags.BoolVar(&dryRun, "dry-run", false, "discover and parse without writing")
	flags.BoolVar(&fullRefresh, "full-refresh", false, "ignore source refresh state and parse discovered sources")
	flags.BoolVar(&noNormalize, "no-normalize", false, "deprecated compatibility option; processing is always on server")
	flags.StringVar(&sourceDir, "source-dir", "", "override harness source directory")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w\n%w", err, ErrUsage)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q\n%w", flags.Arg(0), ErrUsage)
	}
	selectedHarnesses, err := syncHarnesses(all, harnesses)
	if err != nil {
		return err
	}
	settings.ServerURL, settings.ServerDBPath, settings.CollectorDBPath = strings.TrimSpace(serverURL), strings.TrimSpace(serverDBPath), strings.TrimSpace(dbPath)
	token = settings.ServerToken
	var session clientworkflow.Session
	observer := &clientworkflow.Observer{}
	if !dryRun {
		if settings.ServerURL == "" {
			if err := collector.ValidatePaths(settings.CollectorDBPath, settings.ServerDBPath); err != nil {
				return err
			}
		}
		session, err = clientworkflow.Resolve(invocation.context, settings, func(ctx context.Context) (service.State, error) {
			return ensureConfiguredLocal(ctx, settings.ServerDBPath, settings)
		})
		if err != nil {
			return err
		}
		if err := session.VerifyIngestion(invocation.context); err != nil {
			return err
		}
		observer = session.Observe(invocation.context)
	}
	terminal := newTerminalSyncProgress(invocation.stderr)
	result, err := collector.Run(invocation.context, collector.Options{
		CollectorDBPath: strings.TrimSpace(dbPath), ServerDBPath: strings.TrimSpace(serverDBPath), ServerURL: serverURL, Token: token, PublishOnly: publishOnly,
		Destination:      session.Destination,
		DeliveryProgress: func(progress collector.DeliveryProgress) { terminal.Delivery(progress); observer.Delivery(progress) },
		SyncOptions: pipeline.SyncOptions{
			Harnesses:   selectedHarnesses,
			DryRun:      dryRun,
			FullRefresh: fullRefresh,
			Normalize:   !noNormalize,
			SourceDir:   strings.TrimSpace(sourceDir),
			Now:         invocation.now,
			Progress:    func(event pipeline.SyncProgressEvent) { terminal.Collection(event); observer.Collection(event) },
		},
		EnsureLocal: func(ctx context.Context) (string, error) {
			state, err := ensureConfiguredLocal(ctx, strings.TrimSpace(serverDBPath), settings)
			if err != nil {
				return "", err
			}
			return state.Record.URL, nil
		},
	})
	observer.Finish(result, err)
	if !dryRun {
		terminal.Finish(result)
	}
	printSummary(invocation.stdout, "sync", result.Collection, dryRun)
	if !dryRun {
		printDeliverySummary(invocation.stdout, result)
	}
	if err != nil {
		return err
	}
	return nil
}

func syncHarnesses(all bool, values stringList) ([]pipeline.Harness, error) {
	if all && len(values) > 0 {
		return nil, fmt.Errorf("choose either --all or --harness, not both\n%w", ErrUsage)
	}
	if all || len(values) == 0 {
		return pipeline.SupportedHarnesses, nil
	}
	if err := validateHarnesses(values); err != nil {
		return nil, err
	}
	return harnessList(values), nil
}
