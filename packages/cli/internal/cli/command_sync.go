package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/config"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/syncjob"
)

var syncCommand = commandSpec{name: "sync", run: runSync}

func runSync(invocation commandInvocation, args []string) error {
	if len(args) > 0 && args[0] == "status" {
		return runSyncStatus(invocation, args[1:])
	}
	flags := flag.NewFlagSet("tokeninsights sync", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	var dbPath string
	var all bool
	var dryRun bool
	var fullRefresh bool
	var sourceDir string
	var harnesses stringList
	var serverURL string
	var publishOnly, wait, debug, printURL bool
	flags.BoolVar(&wait, "wait", false, "wait for remote acceptance")
	flags.BoolVar(&debug, "debug", false, "show collection, acceptance and receipt processing")
	flags.BoolVar(&printURL, "print", false, "submit and print only the remote URL")
	settings := invocation.syncDefaults()
	flags.StringVar(&dbPath, "collector-db-path", settings.CollectorDBPath, "collector SQLite database")
	flags.StringVar(&serverURL, "server-url", settings.ServerURL, "authenticated hosted ingestion server")
	flags.BoolVar(&publishOnly, "publish-only", false, "submit retained raw evidence without collecting")
	flags.Var(&harnesses, "harness", "harness to sync: opencode, pi, codex, or claude-code")
	flags.BoolVar(&all, "all", false, "sync all supported harnesses")
	flags.BoolVar(&dryRun, "dry-run", false, "discover and parse without writing")
	flags.BoolVar(&fullRefresh, "full-refresh", false, "ignore source refresh state and retry quarantined sources")
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
	settings.ServerURL, settings.CollectorDBPath = strings.TrimSpace(serverURL), strings.TrimSpace(dbPath)
	if dryRun && (wait || debug || printURL) {
		return fmt.Errorf("--dry-run cannot combine with --wait/--debug/--print\n%w", ErrUsage)
	}
	if wait && debug {
		return fmt.Errorf("choose --wait or --debug\n%w", ErrUsage)
	}
	if !dryRun {
		return runDistributedSync(invocation, settings, selectedHarnesses, sourceDir, publishOnly, fullRefresh, wait, debug, printURL)
	}
	result, err := collector.Run(invocation.context, collector.Options{
		CollectorDBPath: settings.CollectorDBPath,
		SyncOptions:     pipeline.SyncOptions{Harnesses: selectedHarnesses, DryRun: true, FullRefresh: fullRefresh, SourceDir: strings.TrimSpace(sourceDir), Now: invocation.now},
	})
	printSummary(invocation.stdout, "sync", result.Collection, true)
	return err
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

var spawnSyncWorker = syncjob.Spawn

func runDistributedSync(invocation commandInvocation, settings config.SyncSettings, harnesses []pipeline.Harness, source string, publishOnly, fullRefresh, wait, debug, printURL bool) error {
	if os.Getenv("TOKENINSIGHTS_SERVER_TOKEN") != "" {
		return fmt.Errorf("TOKENINSIGHTS_SERVER_TOKEN removed; use TOKENINSIGHTS_ACCESS_TOKEN or config set distributed.server-token\n%w", ErrUsage)
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	canonical, err := collector.CanonicalEndpoint(settings.ServerURL)
	if err != nil {
		return err
	}
	settings.ServerURL = canonical
	spec, err := syncjob.NewSpec(settings, harnesses, source, publishOnly, fullRefresh)
	if err != nil {
		return err
	}
	store, err := syncjob.Open(invocation.context, spec.CollectorPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	spec.Debug = debug
	job, err := store.Enqueue(invocation.context, spec)
	if err != nil {
		return err
	}
	if !wait && !debug {
		if err := spawnSyncWorker(invocation.context, job, settings.ServerToken); err != nil {
			_ = store.Finish(invocation.context, job.ID, "failed", "worker_start_failed", 0)
			return err
		}
		if printURL {
			_, _ = fmt.Fprintln(invocation.stdout, canonical)
		}
		_, _ = fmt.Fprintln(invocation.stderr, "Sync started: "+job.ID+"; tokeninsights sync status")
		return nil
	}
	if printURL {
		_, _ = fmt.Fprintln(invocation.stdout, canonical)
	}
	run, cancel := context.WithTimeout(invocation.context, syncjob.Timeout)
	defer cancel()
	terminal := newTerminalSyncProgress(invocation.stderr)
	progress := syncjob.Progress{Collection: terminal.Collection, Delivery: terminal.Delivery}
	if debug {
		return runDebugSync(run, invocation, store, job, settings.ServerToken)
	}
	result, err := syncjob.RunRemote(run, store, job, settings.ServerToken, false, progress)
	terminal.Finish(result)
	if !printURL {
		printSummary(invocation.stdout, "sync", result.Collection, false)
		printDeliverySummary(invocation.stdout, result)
	}
	return err
}
func runSyncStatus(invocation commandInvocation, args []string) error {
	flags := flag.NewFlagSet("tokeninsights sync status", flag.ContinueOnError)
	flags.SetOutput(invocation.stderr)
	path := flags.String("collector-db-path", invocation.syncDefaults().CollectorDBPath, "collector database")
	asJSON := flags.Bool("json", false, "machine-readable job status")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return ErrUsage
	}
	store, err := syncjob.Open(invocation.context, *path)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	if err := store.RefreshAbandoned(invocation.context); err != nil {
		return err
	}
	job, err := store.Latest(invocation.context)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = io.WriteString(invocation.stdout, "No sync jobs.\n")
		return err
	}
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(invocation.stdout).Encode(job)
	}
	_, err = fmt.Fprintf(invocation.stdout, "%s: %s accepted=%d %s\n", job.ID, job.State, job.Accepted, job.Error)
	return err
}
