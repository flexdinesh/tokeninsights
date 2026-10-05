package cli

import (
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
	return tea.Tick(interval, func(time.Time) tea.Msg {
		client, err := tableClient(m.options)
		if err != nil {
			return sharedSyncMsg{err: err}
		}
		state, err := client.Status(m.ctx)
		if err != nil {
			return sharedSyncMsg{err: err}
		}
		readiness := ""
		if state.DataReadiness != nil {
			readiness = string(*state.DataReadiness)
		}
		return sharedSyncMsg{status: db.SyncStatus{Revision: state.Revision, Phase: string(state.Phase)}, instanceID: apiText(state.InstanceId), dataEpoch: apiText(state.DataEpoch), readiness: readiness}
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
