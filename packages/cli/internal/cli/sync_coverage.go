package cli

import (
	"context"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

const sharedSyncInterval = time.Second

func (m interactiveModel) cancelSync() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m interactiveModel) sharedSyncCmd() tea.Cmd {
	interval := 5 * time.Second
	if m.syncInFlight || m.sharedSync.Running {
		interval = sharedSyncInterval
	}
	m.ctx, m.requestID = m.queryContext(m.statusQueries)
	return tea.Tick(interval, func(time.Time) tea.Msg {
		message := sharedSyncMsg{requestID: m.requestID, generation: m.publicationGeneration}
		if !m.statusQueries.current(m.requestID) {
			message.err = context.Canceled
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
		message.status = db.SyncStatus{Revision: state.Revision, Phase: string(state.Phase)}
		message.instanceID, message.dataEpoch, message.readiness = state.InstanceId, state.DataEpoch, string(state.DataReadiness)
		return message
	})
}

func (m interactiveModel) syncWorkLabel() string {
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
