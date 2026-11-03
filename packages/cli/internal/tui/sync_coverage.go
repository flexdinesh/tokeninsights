package tui

import (
	"context"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/core/analytics"
)

const sharedSyncInterval = time.Second

func (m interactiveModel) cancelSync() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m interactiveModel) sharedSyncCmd() tea.Cmd {
	interval := 5 * time.Second
	if m.sharedSync.Running || m.refreshBusy() {
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
	if session := m.options.local; session != nil {
		// Read collection before processing so acceptance cannot observe an older
		// empty queue. Progress failures must not claim a completed refresh.
		details, err := session.Progress(m.ctx)
		if err != nil {
			message.err = err
			return message
		}
		message.collection, message.capture = &details.Collection, details.Captures
		state, err := session.ProcessingStatus(m.ctx)
		message.err = err
		if err == nil {
			message.refreshStatus = &state
			pending := state.Pending > 0 || state.Metadata.Generation != state.Metadata.TargetGeneration
			message.status = analytics.SyncStatus{Revision: state.Metadata.Revision, Running: pending}
			message.pending = pending
			// Query identity belongs to the same application client as the table reads.
			instance, err := session.Queries().Instance(m.ctx)
			message.err = err
			message.instanceID, message.dataEpoch, message.readiness = instance.InstanceID, state.Metadata.DatabaseID, instance.Readiness
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
	message.status = analytics.SyncStatus{Revision: state.Metadata.Revision, Running: state.Pending > 0 || state.Metadata.Generation != state.Metadata.TargetGeneration}
	message.pending = message.status.Running
	message.instanceID, message.dataEpoch, message.readiness = state.InstanceID, state.Metadata.DatabaseID, state.Readiness
	return message
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
