package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/flexdinesh/tokeninsights/packages/cli/internal/db"
)

type tableSummaryModel struct {
	rowCount      int
	totalValue    int64
	showTotal     bool
	loading       bool
	sessionCounts db.SessionCounts
}

const tableSummarySeparator = " · "

func newTableSummaryModel(rows []renderRow, activeTab tabMode, loading bool) tableSummaryModel {
	return tableSummaryModel{
		rowCount:   len(rows),
		totalValue: totalTokens(rows),
		showTotal:  tableSummaryShowsTotal(activeTab),
		loading:    loading,
	}
}

func tableSummaryShowsTotal(activeTab tabMode) bool {
	switch activeTab {
	case tabTokens, tabModels, tabProviders, tabHarnesses, tabSessions:
		return true
	default:
		return false
	}
}

func (summary tableSummaryModel) View(width int) string {
	if width <= 0 {
		return ""
	}
	if summary.loading {
		return tableSummarySurfaceStyle.Render(strings.Repeat(" ", width))
	}

	parts := []string{
		tableSummaryLabelStyle.Render(fmt.Sprintf("sessions %d shown / %d synced", summary.sessionCounts.Shown, summary.sessionCounts.Synced)),
		tableSummaryLabelStyle.Render(fmt.Sprintf("rows %d", summary.rowCount)),
	}
	if summary.showTotal {
		parts = append(parts, tableSummaryTotalStyle.Render(fmt.Sprintf("total %s", formatTableSummaryTokens(summary.totalValue))))
	}
	line := strings.Join(parts, tableSummarySeparatorStyle.Render(tableSummarySeparator))
	line = ansi.Cut(line, 0, width)
	return padStyledLine(line, width, tableSummarySurfaceStyle)
}

func formatTableSummaryTokens(value int64) string {
	if value == 0 {
		return "0"
	}
	return formatTokens(value)
}

func totalTokens(rows []renderRow) int64 {
	var total int64
	for _, row := range rows {
		total += row.totalValue
	}
	return total
}

var (
	tableSummarySurfaceStyle = lipgloss.NewStyle().
					Background(lipgloss.Color(rowStripeColor))
	tableSummaryLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245")).
				Background(lipgloss.Color(rowStripeColor))
	tableSummaryTotalStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("157")).
				Background(lipgloss.Color(rowStripeColor))
	tableSummarySeparatorStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("241")).
					Background(lipgloss.Color(rowStripeColor))
)
