package cli

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

const sharedSyncInterval = time.Second
const percentageScale = 100

func (m interactiveModel) cancelSync() {
	if m.cancel != nil {
		m.cancel()
	}
}

func (m interactiveModel) sharedSyncCmd() tea.Cmd {
	return tea.Tick(sharedSyncInterval, func(time.Time) tea.Msg {
		status, err := db.ReadSyncStatus(m.ctx, m.options.dbPath)
		return sharedSyncMsg{status: status, err: err}
	})
}

func (m interactiveModel) syncWorkLabel() string {
	s := m.sharedSync
	if s.JobID == 0 {
		if m.syncInFlight || m.sharedSync.Running {
			return "Checking local sources"
		}
		return "Local sources not checked"
	}
	work := s.Phase
	if s.DiscoveryComplete {
		percent := int64(percentageScale)
		if s.TotalSources > 0 {
			percent = s.CheckedSources * percentageScale / s.TotalSources
		}
		work = fmt.Sprintf("%s · %d/%d sources · %d%% checked · %d ready · %d failed", s.Phase, s.CheckedSources, s.TotalSources, percent, s.ReadySources, s.FailedSources)
	}
	if !m.options.noSync && !s.Running && !m.syncInFlight {
		work += " · u retry sync"
	}
	return work
}

func (m interactiveModel) coverageSummary() string {
	checked := 0
	for _, d := range m.coverage {
		if d.Status == "checked" || d.Status == "empty" {
			checked++
		}
	}
	return fmt.Sprintf("Source coverage %d/%d days checked", checked, len(m.coverage))
}

func dayCoverageLabel(d db.DayCoverage) string {
	switch d.Status {
	case "empty":
		return "empty"
	case "checked":
		return "checked"
	case "unverified":
		return "unverified"
	case "partial":
		if d.FailedSources > 0 {
			return "incomplete"
		}
		return "updating"
	default:
		return "pending"
	}
}

func withDayCoverage(rows []renderRow, days []db.DayCoverage, selected sortMode) []renderRow {
	byDay := make(map[string]db.DayCoverage, len(days))
	seen := make(map[string]bool, len(rows))
	for _, d := range days {
		byDay[d.Day] = d
	}
	for i := range rows {
		seen[rows[i].bucket] = true
		if d, ok := byDay[rows[i].bucket]; ok {
			rows[i].coverageStatus = dayCoverageLabel(d)
		}
	}
	// Long ranges retain a compact recent calendar; every day remains in coverage.
	visible := days
	if len(visible) > 31 {
		visible = visible[len(visible)-7:]
	}
	for _, d := range visible {
		if seen[d.Day] || d.HasUsage {
			continue
		}
		value := "—"
		if d.Status == "empty" {
			value = "0"
		}
		rows = append(rows, renderRow{bucket: d.Day, coverageStatus: dayCoverageLabel(d), placeholder: true, sessions: value, inputTokens: value, outputTokens: value, reasoningTokens: value, cacheReadTokens: value, cacheWriteTokens: value, totalTokens: value})
	}
	sortRenderRows(rows, tabTokens, selected)
	return rows
}
