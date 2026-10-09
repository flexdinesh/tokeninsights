package cli

import (
	"context"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/querymodel"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/serverfeatures"
)

const sharedSyncInterval = time.Second

func (m interactiveModel) cancelSync() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m interactiveModel) sharedSyncCmd() tea.Cmd {
	interval := 5 * time.Second
	if m.syncInFlight || m.sharedSync.Running || m.refreshBusy() {
		interval = sharedSyncInterval
	}
	return m.sharedSyncCmdAfter(interval)
}

func (m interactiveModel) sharedSyncCmdAfter(interval time.Duration) tea.Cmd {
	m.ctx, m.requestID = m.queryContext(m.statusQueries)
	return tea.Tick(interval, func(time.Time) tea.Msg { return m.loadSharedSync() })
}

func (m interactiveModel) loadSharedSync() sharedSyncMsg {
	message := sharedSyncMsg{requestID: m.requestID, generation: m.publicationGeneration}
	if !m.statusQueries.current(m.requestID) {
		message.err = context.Canceled
		return message
	}
	if runtime := m.options.local; runtime != nil {
		// Read collection before processing: an accepted attempt then observes a
		// status snapshot taken after acceptance, never an earlier empty queue.
		if runtime.Policy.Capabilities.Has(serverfeatures.CollectorProgress) {
			details := runtime.Progress.Details()
			message.collection, message.capture = &details.Collection, details.Captures
		}
		state, err := runtime.ProcessingStatus(m.ctx)
		message.err = err
		if err == nil {
			message.refreshStatus = &state
			pending := state.Pending > 0 || state.Metadata.Generation != state.Metadata.TargetGeneration
			message.status = querymodel.SyncStatus{Revision: state.Metadata.Revision, Running: pending}
			message.pending = pending
			message.instanceID, message.dataEpoch, message.readiness = runtime.InstanceID, state.Metadata.DatabaseID, "ready"
		}
		return message
	}
	client, err := tableClient(m.options)
	if err != nil {
		message.err = err
		return message
	}
	state, err := client.Status(m.ctx)
	if err != nil {
		message.err = err
		return message
	}
	message.status = querymodel.SyncStatus{Revision: state.Revision, Running: state.Pending > 0 || state.Generation != state.TargetGeneration}
	message.pending = message.status.Running
	message.instanceID, message.dataEpoch, message.readiness = state.InstanceId, state.DataEpoch, string(state.DataReadiness)
	return message
}

func (m interactiveModel) syncWorkLabel() string {
	if m.sharedSync.Running {
		return "Processing accepted usage · r Reload"
	}
	return "Saved server data · r Reload"
}

func (m interactiveModel) currentDayRows(rows []renderRow) []renderRow {
	display := slices.Clone(rows)
	for i := range display {
		row := &display[i]
		row.coverageStatus = ""
	}
	return display
}

func dayCoverageMarker(status string) string {
	switch status {
	case "checked":
		return "✓"
	case "empty":
		return "○"
	case "pending":
		return "…"
	case "updating":
		return "↻"
	case "incomplete":
		return "!"
	case "unverified":
		return "?"
	default:
		return ""
	}
}
