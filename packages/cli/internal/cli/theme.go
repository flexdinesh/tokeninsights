package cli

import (
	"github.com/charmbracelet/lipgloss"
)

// TUI visual tokens. Semantic roles only; no feature-specific colors.
// Ordinary surfaces inherit the terminal background. Only selection uses a fill.

const (
	tuiColGap          = 2
	tuiMinVisibleLines = 1
)

var (
	themeInverse = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1C1C24"}
	themeBrand   = lipgloss.AdaptiveColor{Light: "#292B32", Dark: "#E7E7EE"}
	themeAccent  = lipgloss.AdaptiveColor{Light: "#17669D", Dark: "#80CFFF"}
	themeMuted   = lipgloss.AdaptiveColor{Light: "#60636F", Dark: "#AAAAB7"}
	themeFaint   = lipgloss.AdaptiveColor{Light: "#A0A2AD", Dark: "#525460"}
	themeText    = lipgloss.AdaptiveColor{Light: "#292B32", Dark: "#E7E7EE"}
	themeTotal   = lipgloss.AdaptiveColor{Light: "#A2306C", Dark: "#F49AC2"}
	themeDanger  = lipgloss.AdaptiveColor{Light: "#9C3530", Dark: "#F0A39B"}

	themeTabActiveBg = themeAccent
	themeTabActiveFg = themeInverse

	themeSelectedBg = lipgloss.AdaptiveColor{Light: "#DFEAF3", Dark: "#273D4D"}
	themeSelectedFg = themeText
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(themeBrand)
	hintStyle   = lipgloss.NewStyle().Foreground(themeMuted)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(themeAccent)

	dimensionStyle   = lipgloss.NewStyle().Foreground(themeText)
	textCellStyle    = lipgloss.NewStyle().Foreground(themeText)
	mutedCellStyle   = lipgloss.NewStyle().Foreground(themeMuted)
	metricStyle      = lipgloss.NewStyle().Foreground(themeText)
	contextUsedStyle = lipgloss.NewStyle().Bold(true).Foreground(themeText)
	totalStyle       = lipgloss.NewStyle().Bold(true).Foreground(themeTotal)

	appSurfaceStyle  = lipgloss.NewStyle().Foreground(themeText)
	rowOddStyle      = lipgloss.NewStyle()
	selectedRowStyle = lipgloss.NewStyle().
				Foreground(themeSelectedFg).
				Background(themeSelectedBg)

	dividerStyle   = lipgloss.NewStyle().Foreground(themeFaint)
	footerStyle    = lipgloss.NewStyle().Foreground(themeMuted)
	footerDimStyle = lipgloss.NewStyle().Foreground(themeMuted)

	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(themeTabActiveFg).
			Background(themeTabActiveBg).
			Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(themeText).
				Padding(0, 1)
	popupTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(themeText)
	popupItemStyle = lipgloss.NewStyle().Foreground(themeText)

	statuslineSurfaceStyle   = lipgloss.NewStyle().Bold(false)
	statuslineBrandStyle     = lipgloss.NewStyle().Bold(true).Foreground(themeBrand)
	statuslineItemStyle      = lipgloss.NewStyle().Foreground(themeMuted)
	statuslineSeparatorStyle = lipgloss.NewStyle().Foreground(themeFaint)

	tableSummarySurfaceStyle   = lipgloss.NewStyle()
	tableSummaryLabelStyle     = lipgloss.NewStyle().Foreground(themeMuted)
	tableSummaryTotalStyle     = lipgloss.NewStyle().Bold(true).Foreground(themeTotal)
	tableSummarySeparatorStyle = lipgloss.NewStyle().Foreground(themeFaint)

	syncOKStyle   = lipgloss.NewStyle().Foreground(themeTotal)
	syncFailStyle = lipgloss.NewStyle().Foreground(themeDanger)
	syncBusyStyle = lipgloss.NewStyle().Foreground(themeAccent)
	syncSkipStyle = lipgloss.NewStyle().Foreground(themeMuted)
)
