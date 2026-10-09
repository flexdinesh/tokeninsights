package cli

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/flexdinesh/tokeninsights/packages/cli/internal/collector"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/pipeline"
)

const terminalDeliveryInterval = time.Second

// terminalSyncProgress writes bounded stage transitions and coalesces receipt
// updates. It reports observed work without paths, remote text or percentages.
type terminalSyncProgress struct {
	mu           sync.Mutex
	out          io.Writer
	now          func() time.Time
	seen         map[string]bool
	lastDelivery collector.DeliveryProgress
	hasDelivery  bool
	lastReport   time.Time
}

func newTerminalSyncProgress(out io.Writer) *terminalSyncProgress {
	return &terminalSyncProgress{out: out, now: time.Now, seen: make(map[string]bool)}
}

func (p *terminalSyncProgress) Collection(event pipeline.SyncProgressEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if event.Harness == "" {
		switch event.Status {
		case pipeline.SyncProgressWaiting:
			p.once("waiting", "Waiting for another sync...")

		}
		return
	}
	switch event.Harness {
	case pipeline.HarnessCodex, pipeline.HarnessOpenCode, pipeline.HarnessClaudeCode, pipeline.HarnessPi:
	default:
		return
	}
	var label string
	switch event.Status {
	case pipeline.SyncProgressDiscovering:
		label = "finding sessions"
	case pipeline.SyncProgressSyncing:
		label = "reading sessions"
	case pipeline.SyncProgressSynced:
		label = "captured"
	case pipeline.SyncProgressSkipped:
		label = "no new usage"
	case pipeline.SyncProgressFailed:
		label = "collection failed"
		if event.Quarantined > 0 {
			label = fmt.Sprintf("incomplete; %d files quarantined", event.Quarantined)
		}
	case pipeline.SyncProgressWaiting:
		label = "waiting for another sync"
	default:
		return
	}
	p.once(string(event.Harness)+"/"+label, string(event.Harness)+": "+label)
}

func (p *terminalSyncProgress) once(key, line string) {
	if p.seen[key] {
		return
	}
	p.seen[key] = true
	_, _ = fmt.Fprintln(p.out, line)
}

func (p *terminalSyncProgress) Delivery(progress collector.DeliveryProgress) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.once("delivery", "Submitting retained evidence...")
	p.reportDelivery(progress, false)
}

func (p *terminalSyncProgress) reportDelivery(progress collector.DeliveryProgress, force bool) {
	if progress.Batches < 0 || progress.Accepted < 0 || progress.Pending < 0 || p.hasDelivery && progress == p.lastDelivery {
		return
	}
	if progress.Batches == 0 && progress.Accepted == 0 {
		if progress.PendingKnown && progress.Pending == 0 {
			p.once("empty-delivery", "No new evidence to submit.")
		}
		return
	}
	now := p.now()
	finished := progress.PendingKnown && progress.Pending == 0
	if !force && !finished && !p.lastReport.IsZero() && now.Sub(p.lastReport) < terminalDeliveryInterval {
		return
	}
	p.lastReport, p.lastDelivery, p.hasDelivery = now, progress, true
	if progress.PendingKnown {
		_, _ = fmt.Fprintf(p.out, "Accepted %d entries in %d batches; %d pending.\n", progress.Accepted, progress.Batches, progress.Pending)
	} else {
		_, _ = fmt.Fprintf(p.out, "Accepted %d entries in %d batches.\n", progress.Accepted, progress.Batches)
	}
}

func (p *terminalSyncProgress) Finish(result collector.Result) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reportDelivery(collector.DeliveryProgress{Accepted: result.Accepted, Batches: result.Batches, PendingKnown: result.PendingKnown, Pending: result.Pending}, true)
}
