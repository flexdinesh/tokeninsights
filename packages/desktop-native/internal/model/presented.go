package model

import (
	"slices"
	"strconv"
	"strings"

	"github.com/flexdinesh/tokeninsights/packages/core/dashboardview"
)

// presented is what a window shows that changes without its view being
// built again: what status and progress reads change, the reads a status
// read starts or settles, and the server's calendar day. It is small and
// comparable, so a result compares it with what the window last showed
// and redraws the window only when it differs: a settled window draws
// nothing while it polls.
type presented struct {
	// status is the header status; sync the synchronization views read.
	status dashboardview.StatusValue
	sync   dashboardview.SyncState
	// refreshPending disables Refresh.
	refreshPending bool
	// connectionUnavailable is a failed status read, the header's
	// connection state; connectionFailed and connectionErr the connection
	// banner and what it says.
	connectionUnavailable, connectionFailed bool
	connectionErr                           string
	processing                              processingShown
	// progress is the collector progress as its panel renders it.
	progress string
	// usage, excluded, facets and sessions are their reads, which a
	// status read starts, drops or fails.
	usage, excluded, facets, sessions readMark
	// today is the server's calendar date, which the dates show.
	today string
}

// processingShown is what the results' notes say of processing: the
// entries pending or a rebuild, and failed processing with its retry
// time while it waits.
type processingShown struct {
	pending            int64
	rebuilding, failed bool
	retryAtMs          int64
}

// readMark is where a usage or facets read stands: its number, which a
// read started or dropped changes, and whether it waits for status.
type readMark struct {
	seq            uint64
	awaitingStatus bool
}

func (u *usageState) mark() readMark  { return readMark{seq: u.seq, awaitingStatus: u.awaitingStatus} }
func (f *facetsState) mark() readMark { return readMark{seq: f.seq, awaitingStatus: f.awaitingStatus} }

// presented is what the window shows now of what reads change without
// building its view.
func (w *Window) presented() presented {
	sync := w.SyncState()
	p := presented{
		status: w.StatusValue(), sync: sync, refreshPending: w.RefreshPending(),
		connectionUnavailable: w.ConnectionUnavailable(), progress: w.progressShown(),
		usage: w.usage.mark(), excluded: w.excluded.mark(), facets: w.facets.mark(), sessions: w.sessions.read.mark(),
		today: w.ServerToday(),
	}
	if err := w.ConnectionError(); err != nil {
		p.connectionFailed, p.connectionErr = true, err.Error()
	}
	if status := w.sync.status; status != nil {
		p.processing = processingShown{
			pending: status.Pending, rebuilding: status.Metadata.Generation != status.Metadata.TargetGeneration,
			failed: status.Failed > 0,
		}
		if p.processing.failed && sync.ProcessingBackoff {
			p.processing.retryAtMs = status.FailedRetryAtMs
		}
	}
	return p
}

// present redraws the window when what it shows of reads differs from
// what it last showed. Results applied without building the view call
// it; a closed window draws nothing more.
func (w *Window) present() {
	if w.rt.Context.Err() != nil {
		return
	}
	p := w.presented()
	if p == w.sync.shown {
		return
	}
	w.sync.shown = p
	w.rt.Invalidate()
}

// Separators of progressShown's fields and attempts, which no field
// holds.
const (
	fieldSeparator   = "\x1f"
	attemptSeparator = "\x1e"
)

// progressShown is the collector progress as its panel renders it: each
// visible attempt's identity, whether it is active, its label, counts
// and harness states, leaving out what it does not show, as the time of
// the attempt's last update.
func (w *Window) progressShown() string {
	progress, ok := w.Progress()
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, attempt := range dashboardview.VisibleAttempts(progress.Attempts) {
		fields := []string{
			attempt.AttemptID, strconv.FormatBool(dashboardview.ActiveStage(attempt.Stage)),
			dashboardview.AttemptLabel(attempt), strconv.FormatInt(attempt.AcknowledgedEntries, decimalBase),
		}
		if attempt.PendingKnown && attempt.Pending > 0 {
			fields = append(fields, strconv.FormatInt(attempt.Pending, decimalBase))
		}
		names := make([]string, 0, len(attempt.Harnesses))
		for name := range attempt.Harnesses {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			fields = append(fields, name+"="+attempt.Harnesses[name])
		}
		b.WriteString(strings.Join(fields, fieldSeparator))
		b.WriteString(attemptSeparator)
	}
	return b.String()
}

// decimalBase formats progress counts.
const decimalBase = 10
