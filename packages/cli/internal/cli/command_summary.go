package cli

import (
	"fmt"
	"io"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

func printSummary(stdout io.Writer, command string, summary pipeline.Summary, dryRun bool) {
	prefix := command
	if dryRun {
		prefix += " dry-run"
		switch summary.Recovery {
		case pipeline.RecoveryReset:
			_, _ = fmt.Fprintln(stdout, "Recovery preview: would reset the database and rebuild all configured harnesses.")
		case pipeline.RecoveryResume:
			_, _ = fmt.Fprintln(stdout, "Recovery preview: would resume rebuilding all configured harnesses.")
		}
	}
	_, _ = fmt.Fprintf(stdout, "%s: requested=%d synced=%d skipped=%d failed=%d raw_facts=%d observations=%d canonical=%d diagnostics=%d\n",
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

func recoveryNotice(stderr io.Writer) func(pipeline.SyncProgressEvent) {
	return func(event pipeline.SyncProgressEvent) {
		switch event.Status {
		case pipeline.SyncProgressResetting:
			_, _ = fmt.Fprintln(stderr, "Resetting local usage data for compatibility…")
		case pipeline.SyncProgressRebuilding:
			_, _ = fmt.Fprintln(stderr, "Rebuilding usage from all configured local harnesses…")
		}
	}
}
